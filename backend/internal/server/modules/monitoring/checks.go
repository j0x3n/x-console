package monitoring

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

const (
	// maxBodyScan is how much of a page the keyword check reads.
	maxBodyScan = 1 << 20
	// RDAPBaseKey is the setting that overrides the RDAP service.
	RDAPBaseKey = "monitoring.rdap_url"
	// defaultRDAPBase redirects to the registry's own RDAP server.
	defaultRDAPBase = "https://rdap.org"
)

// probeResult is the outcome of one check before it is stored.
type probeResult struct {
	OK         bool
	StatusCode *int64
	Latency    time.Duration
	Err        string
	Expires    *time.Time
	Subject    string
	Issuer     string
	Registrar  string
}

// monitorLock returns the lock of one monitor.
func (m *Module) monitorLock(id int64) *sync.Mutex {
	m.busyMu.Lock()
	defer m.busyMu.Unlock()
	l, ok := m.busy[id]
	if !ok {
		l = &sync.Mutex{}
		m.busy[id] = l
	}
	return l
}

// checkDue checks every enabled monitor whose interval has passed. A monitor
// still being checked (slow timeout, manual check) is skipped this round.
func (m *Module) checkDue(ctx context.Context, now time.Time) error {
	rows, err := m.q.ListEnabledMonitors(ctx)
	if err != nil {
		return err
	}
	sem := make(chan struct{}, checkWorkers)
	var wg sync.WaitGroup
	for _, x := range rows {
		if !isDue(x.LastCheckedAt, time.Duration(x.IntervalSeconds)*time.Second, now) {
			continue
		}
		lock := m.monitorLock(x.ID)
		if !lock.TryLock() {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer lock.Unlock()
			sem <- struct{}{}
			defer func() { <-sem }()
			if _, err := m.check(ctx, x.ID, now); err != nil && ctx.Err() == nil {
				m.d.Log.Warn("monitor check failed", "monitor", x.ID, "err", err)
			}
		}()
	}
	wg.Wait()
	return nil
}

// checkNow checks one monitor right away (the "check now" button).
func (m *Module) checkNow(ctx context.Context, id int64, now time.Time) (db.MonitorResult, error) {
	lock := m.monitorLock(id)
	lock.Lock()
	defer lock.Unlock()
	return m.check(ctx, id, now)
}

// check probes a monitor, stores the result, updates the alert state and
// sends notifications. The caller holds the monitor's lock.
func (m *Module) check(ctx context.Context, id int64, now time.Time) (db.MonitorResult, error) {
	x, err := m.getMonitor(ctx, id)
	if err != nil {
		return db.MonitorResult{}, err
	}
	p := m.probe(ctx, x, now)

	detail := resultDetail{ExpiresAt: p.Expires, Subject: p.Subject, Issuer: p.Issuer, Registrar: p.Registrar}
	if p.Expires != nil {
		detail.DaysLeft = ptr(daysLeft(*p.Expires, now))
	}
	res, err := m.q.InsertMonitorResult(ctx, db.InsertMonitorResultParams{MonitorID: x.ID, At: now, Ok: boolInt(p.OK),
		StatusCode: p.StatusCode, LatencyMs: p.Latency.Milliseconds(), Error: p.Err, Detail: mustJSON(detail)})
	if err != nil {
		return db.MonitorResult{}, err
	}

	state, change := nextState(monitorState{Status: x.LastStatus, Failures: int(x.ConsecutiveFailures)}, p.OK)

	// Expiry reminders: the thresholds already sent belong to one expiry
	// date; a renewed certificate or domain starts over.
	expires, notified := x.ExpiresAt, []int{}
	_ = json.Unmarshal([]byte(x.ExpiryNotified), &notified)
	var expiring *float64
	if p.Expires != nil {
		if !sameExpiry(x.ExpiresAt, *p.Expires) {
			notified = []int{}
		}
		expires = p.Expires
		left := daysLeft(*p.Expires, now)
		if _, due, marked := expiryDue(left, thresholdsFor(x.Kind), notified); due {
			expiring = &left
			notified = marked
		}
	}

	updated, err := m.q.SetMonitorState(ctx, db.SetMonitorStateParams{ID: x.ID, LastStatus: state.Status, LastCheckedAt: &now,
		LastError: p.Err, ConsecutiveFailures: int64(state.Failures), ExpiresAt: expires, ExpiryNotified: mustJSON(notified)})
	if err != nil {
		return db.MonitorResult{}, err
	}
	out := toAPIMonitor(updated, now)
	switch change {
	case wentDown:
		m.d.Bus.Publish("monitor.down", out)
		m.notify(ctx, "monitor.down", notify.PriorityHigh, downTitle(updated), p.Err, updated)
	case cameUp:
		m.d.Bus.Publish("monitor.up", out)
		m.notify(ctx, "monitor.up", notify.PriorityNormal, "已恢复："+updated.Name, updated.Target, updated)
	}
	if expiring != nil {
		m.d.Bus.Publish("monitor.expiring", out)
		prio := notify.PriorityNormal
		if *expiring <= 3 {
			prio = notify.PriorityHigh
		}
		m.notify(ctx, "monitor.expiring", prio, expiringTitle(updated, *expiring), expiringBody(updated, *p.Expires), updated)
	}
	m.d.Bus.Publish("monitor.checked", map[string]any{"id": x.ID, "ok": p.OK, "status": state.Status, "at": now})
	return res, nil
}

func kindLabel(kind string) string {
	switch kind {
	case kindTLS:
		return "证书"
	case kindDomain:
		return "域名"
	}
	return "网站"
}

func downTitle(x db.Monitor) string {
	switch x.Kind {
	case kindTLS:
		return "证书检查失败：" + x.Name
	case kindDomain:
		return "域名查询失败：" + x.Name
	}
	return "网站打不开：" + x.Name
}

func expiringTitle(x db.Monitor, left float64) string {
	if left <= 0 {
		return kindLabel(x.Kind) + "已经过期：" + x.Name
	}
	days := int(left)
	if days < 1 {
		return kindLabel(x.Kind) + "不到 1 天就过期：" + x.Name
	}
	return fmt.Sprintf("%s还有 %d 天过期：%s", kindLabel(x.Kind), days, x.Name)
}

func expiringBody(x db.Monitor, expires time.Time) string {
	return x.Target + " 在 " + expires.UTC().Format("2006-01-02") + " 过期"
}

// monitorLink is the page of a monitor.
func monitorLink(x db.Monitor) string {
	if x.Kind == kindHTTP {
		return "/monitoring?monitor=" + itoa(x.ID)
	}
	return "/monitoring/certs?monitor=" + itoa(x.ID)
}

func (m *Module) notify(ctx context.Context, kind, prio, title, body string, x db.Monitor) {
	_, err := m.d.Notify.Send(ctx, notify.Notification{Kind: kind, Title: title, Body: body, Link: monitorLink(x),
		Priority: prio, Source: "monitoring", Data: map[string]any{"monitorId": x.ID}})
	if err != nil {
		m.d.Log.Warn("monitor notification failed", "monitor", x.ID, "err", err)
	}
}

// probe runs the check of one monitor.
func (m *Module) probe(ctx context.Context, x db.Monitor, now time.Time) probeResult {
	timeout := time.Duration(x.TimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	switch x.Kind {
	case kindHTTP:
		return m.probeHTTP(ctx, x)
	case kindTLS:
		return m.probeTLS(ctx, x.Target, now)
	case kindDomain:
		return m.probeDomain(ctx, x.Target)
	}
	return probeResult{Err: "未知的监控类型"}
}

// transport is shared by the HTTP checks. Keep-alive is off so every check
// opens a fresh connection, like a visitor would.
func (m *Module) transport() *http.Transport {
	return &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{RootCAs: m.tlsRoots.Load()},
	}
}

// probeHTTP fetches the URL and checks the status code and keyword.
func (m *Module) probeHTTP(ctx context.Context, x db.Monitor) probeResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, x.Target, nil)
	if err != nil {
		return probeResult{Err: err.Error()}
	}
	req.Header.Set("User-Agent", "x-console-monitor/1")
	start := time.Now()
	resp, err := (&http.Client{Transport: m.transport()}).Do(req)
	if err != nil {
		return probeResult{Latency: time.Since(start), Err: netError(err)}
	}
	defer resp.Body.Close()
	var body []byte
	if x.Keyword != "" {
		body, err = io.ReadAll(io.LimitReader(resp.Body, maxBodyScan))
	} else {
		_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyScan))
	}
	res := probeResult{Latency: time.Since(start), StatusCode: ptr(int64(resp.StatusCode))}
	switch {
	case err != nil:
		res.Err = "读取页面失败：" + netError(err)
	case x.ExpectedStatus > 0 && int64(resp.StatusCode) != x.ExpectedStatus:
		res.Err = fmt.Sprintf("状态码是 %d，期望 %d", resp.StatusCode, x.ExpectedStatus)
	case x.ExpectedStatus == 0 && (resp.StatusCode < 200 || resp.StatusCode > 399):
		res.Err = fmt.Sprintf("状态码是 %d", resp.StatusCode)
	case x.Keyword != "" && !bytes.Contains(body, []byte(x.Keyword)):
		res.Err = "页面里没有找到“" + x.Keyword + "”"
	default:
		res.OK = true
	}
	return res
}

// netError shortens common network errors.
func netError(err error) string {
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	var netErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		return "超时"
	case errors.As(err, &netErr) && netErr.Timeout():
		return "超时"
	case errors.As(err, &dnsErr):
		return "域名解析失败：" + dnsErr.Name
	case errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.Errno(10061)): // Windows WSAECONNREFUSED
		return "连接被拒绝"
	case errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.Errno(10054)): // Windows WSAECONNRESET
		return "连接被重置"
	case errors.As(err, &certErr):
		return "证书无效：" + certErr.Err.Error()
	}
	// Drop the `Get "https://...":` prefix of *url.Error; the URL is shown anyway.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err.Error()
	}
	return err.Error()
}

// probeTLS connects and reads the certificate. The chain is verified
// against the system roots (or tlsRoots) at now; an invalid or expired
// certificate fails the check but its expiry is still recorded.
func (m *Module) probeTLS(ctx context.Context, target string, now time.Time) probeResult {
	host, port, err := tlsAddress(target)
	if err != nil {
		return probeResult{Err: errText(err)}
	}
	start := time.Now()
	d := tls.Dialer{Config: &tls.Config{ServerName: host, InsecureSkipVerify: true}} //nolint:gosec // verified below
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return probeResult{Latency: time.Since(start), Err: netError(err)}
	}
	defer c.Close()
	latency := time.Since(start)
	certs := c.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return probeResult{Latency: latency, Err: "对方没有提供证书"}
	}
	leaf := certs[0]
	res := probeResult{Latency: latency, Expires: ptr(leaf.NotAfter.UTC()), Subject: leaf.Subject.CommonName,
		Issuer: leaf.Issuer.CommonName}
	if res.Subject == "" && len(leaf.DNSNames) > 0 {
		res.Subject = leaf.DNSNames[0]
	}
	inter := x509.NewCertPool()
	for _, c := range certs[1:] {
		inter.AddCert(c)
	}
	_, verr := leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: m.tlsRoots.Load(), Intermediates: inter, CurrentTime: now})
	switch {
	case !now.Before(leaf.NotAfter):
		res.Err = "证书已经过期"
	case verr != nil:
		res.Err = "证书无效：" + verr.Error()
	default:
		res.OK = true
	}
	return res
}

// rdapBase returns the RDAP service URL, overridable in settings.
func (m *Module) rdapBase(ctx context.Context) string {
	var base string
	if err := m.d.Settings.Get(ctx, RDAPBaseKey, &base); err != nil || strings.TrimSpace(base) == "" {
		return defaultRDAPBase
	}
	return strings.TrimRight(strings.TrimSpace(base), "/")
}

// rdapDomain is the part of an RDAP domain answer we read.
type rdapDomain struct {
	Events []struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	} `json:"events"`
	Entities []struct {
		Roles []string          `json:"roles"`
		VCard []json.RawMessage `json:"vcardArray"`
	} `json:"entities"`
}

// probeDomain reads the expiry date of a domain from RDAP.
func (m *Module) probeDomain(ctx context.Context, domain string) probeResult {
	u := m.rdapBase(ctx) + "/domain/" + domain
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return probeResult{Err: err.Error()}
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	start := time.Now()
	resp, err := (&http.Client{Transport: m.transport()}).Do(req)
	if err != nil {
		return probeResult{Latency: time.Since(start), Err: "RDAP 查询失败：" + netError(err)}
	}
	defer resp.Body.Close()
	res := probeResult{Latency: time.Since(start), StatusCode: ptr(int64(resp.StatusCode))}
	if resp.StatusCode == http.StatusNotFound {
		res.Err = "RDAP 查不到这个域名"
		return res
	}
	if resp.StatusCode != http.StatusOK {
		res.Err = fmt.Sprintf("RDAP 返回 %d", resp.StatusCode)
		return res
	}
	var data rdapDomain
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyScan)).Decode(&data); err != nil {
		res.Err = "RDAP 返回的内容看不懂"
		return res
	}
	for _, e := range data.Events {
		if e.Action != "expiration" {
			continue
		}
		t, err := time.Parse(time.RFC3339, e.Date)
		if err != nil {
			continue
		}
		res.Expires = ptr(t.UTC())
	}
	for _, e := range data.Entities {
		if slices.Contains(e.Roles, "registrar") {
			res.Registrar = vcardName(e.VCard)
		}
	}
	if res.Expires == nil {
		res.Err = "RDAP 没有给出到期时间"
		return res
	}
	res.OK = true
	return res
}

// vcardName reads the fn property of a jCard: ["vcard", [["fn", {}, "text", "Name"], ...]].
func vcardName(card []json.RawMessage) string {
	if len(card) < 2 {
		return ""
	}
	var props [][]any
	if json.Unmarshal(card[1], &props) != nil {
		return ""
	}
	for _, p := range props {
		if len(p) >= 4 && p[0] == "fn" {
			if s, ok := p[3].(string); ok {
				return s
			}
		}
	}
	return ""
}
