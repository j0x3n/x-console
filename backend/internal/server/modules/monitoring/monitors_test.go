package monitoring_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
)

func TestHTTPMonitorDownAndUp(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	var healthy atomic.Bool
	healthy.Store(true)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("oops"))
			return
		}
		_, _ = w.Write([]byte("<h1>hello world</h1>"))
	}))
	defer site.Close()

	events, cancel := env.App.Deps.Bus.Subscribe("monitor.", 64)
	defer cancel()

	expectStatus(t, env, http.MethodPost, "/monitors", api.MonitorInput{Kind: "http", Name: "x", Target: "ftp://example.com"}, 400, "validation_failed")
	expectStatus(t, env, http.MethodPost, "/monitors", api.MonitorInput{Kind: "http", Name: "x", Target: site.URL, IntervalSeconds: ptr(5)}, 400, "validation_failed")
	expectStatus(t, env, http.MethodGet, "/monitors/999", nil, 404, "not_found")
	expectStatus(t, env, http.MethodPost, "/monitors/999/check", nil, 404, "not_found")

	var mon api.Monitor
	env.MustDo(http.MethodPost, "/monitors", api.MonitorInput{Kind: "http", Name: "Blog", Target: site.URL + "/", Keyword: ptr("hello")}, &mon)
	if mon.IntervalSeconds != 60 || mon.LastStatus != api.Unknown || !mon.Enabled || mon.TimeoutMs != 10000 {
		t.Fatalf("created: %+v", mon)
	}
	path := fmt.Sprintf("/monitors/%d", mon.Id)

	var res api.MonitorResult
	env.MustDo(http.MethodPost, path+"/check", nil, &res)
	if !res.Ok || res.StatusCode == nil || *res.StatusCode != 200 {
		t.Fatalf("healthy check: %+v", res)
	}

	// Down: the first failure is not an alert, the second is, the third is quiet.
	healthy.Store(false)
	start := time.Now()
	for i := range 3 {
		if _, err := m.CheckNow(ctx, mon.Id, start.Add(time.Duration(i+1)*time.Minute)); err != nil {
			t.Fatal(err)
		}
		env.MustDo(http.MethodGet, path, nil, &mon)
		want := api.Up
		if i >= 1 {
			want = api.Down
		}
		if mon.LastStatus != want || mon.ConsecutiveFailures != i+1 || !strings.Contains(mon.LastError, "500") {
			t.Fatalf("after failure %d: %+v", i+1, mon)
		}
	}
	// Up again: one recovery notice, then quiet.
	healthy.Store(true)
	for i := range 2 {
		if _, err := m.CheckNow(ctx, mon.Id, start.Add(time.Duration(i+10)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	env.MustDo(http.MethodGet, path, nil, &mon)
	if mon.LastStatus != api.Up || mon.ConsecutiveFailures != 0 || mon.LastError != "" {
		t.Fatalf("recovered: %+v", mon)
	}
	if down := notifications(t, env, "monitor.down"); len(down) != 1 || down[0] != "网站打不开：Blog" {
		t.Fatalf("down notifications: %v", down)
	}
	if up := notifications(t, env, "monitor.up"); len(up) != 1 {
		t.Fatalf("up notifications: %v", up)
	}
	counts := map[string]int{}
	for len(events) > 0 {
		counts[(<-events).Topic]++
	}
	if counts["monitor.down"] != 1 || counts["monitor.up"] != 1 || counts["monitor.checked"] != 6 || counts["monitor.created"] != 1 {
		t.Fatalf("events: %v", counts)
	}

	// Keyword missing counts as a failure.
	env.MustDo(http.MethodPatch, path, api.MonitorPatch{Keyword: ptr("goodbye")}, &mon)
	env.MustDo(http.MethodPost, path+"/check", nil, &res)
	if res.Ok || !strings.Contains(res.Error, "goodbye") {
		t.Fatalf("keyword: %+v", res)
	}

	// History: 7 results today, one merged point per result.
	var hist api.MonitorResults
	env.MustDo(http.MethodGet, path+"/results?range=24h", nil, &hist)
	if hist.Total != 7 || len(hist.Items) != 7 || hist.Uptime < 42 || hist.Uptime > 43 {
		t.Fatalf("history: total %d items %d uptime %v", hist.Total, len(hist.Items), hist.Uptime)
	}
	expectStatus(t, env, http.MethodGet, path+"/results?range=1y", nil, 400, "")

	// The scheduler checks due monitors only.
	far := time.Now().Add(time.Hour)
	if err := m.CheckDue(ctx, far); err != nil {
		t.Fatal(err)
	}
	if err := m.CheckDue(ctx, far.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	var total int
	_ = env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM monitor_results WHERE monitor_id = ?`, mon.Id).Scan(&total)
	if total != 8 {
		t.Fatalf("results after scheduler: %d", total)
	}

	// Disabled monitors are not checked.
	env.MustDo(http.MethodPatch, path, api.MonitorPatch{Enabled: ptr(false)}, &mon)
	_ = m.CheckDue(ctx, far.Add(time.Hour))
	_ = env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM monitor_results WHERE monitor_id = ?`, mon.Id).Scan(&total)
	if total != 8 {
		t.Fatalf("disabled monitor checked: %d", total)
	}

	// Retention: results older than 30 days go away.
	if err := m.Cleanup(ctx, time.Now().Add(31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	_ = env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM monitor_results`).Scan(&total)
	if total != 0 {
		t.Fatalf("cleanup left %d", total)
	}

	var list []api.Monitor
	runAction(t, env, "monitors.list", map[string]any{"kind": "http"}, &list)
	if len(list) != 1 || list[0].Name != "Blog" {
		t.Fatalf("monitors.list: %+v", list)
	}
	env.MustDo(http.MethodDelete, path, nil, nil)
	expectStatus(t, env, http.MethodDelete, path, nil, 404, "not_found")
}

func ptr[T any](v T) *T { return &v }

// selfSigned makes a certificate for 127.0.0.1 that expires at notAfter.
func selfSigned(t *testing.T, notAfter time.Time) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "test.local"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, DNSNames: []string{"test.local"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, leaf
}

func TestTLSMonitorExpiry(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	now := time.Now().UTC()
	cert, leaf := selfSigned(t, now.Add(10*24*time.Hour))
	var mu sync.Mutex
	current := cert
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	// GetConfigForClient runs on every handshake, also without SNI.
	srv.TLS = &tls.Config{GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
		mu.Lock()
		defer mu.Unlock()
		return &tls.Config{Certificates: []tls.Certificate{current}}, nil
	}}
	srv.StartTLS()
	defer srv.Close()
	addr := srv.Listener.Addr().String()

	var mon api.Monitor
	env.MustDo(http.MethodPost, "/monitors", api.MonitorInput{Kind: "tls", Name: "Cert", Target: "https://" + addr + "/path"}, &mon)
	if mon.Target != addr || mon.IntervalSeconds != 6*3600 {
		t.Fatalf("created: %+v", mon)
	}
	// Not trusted by the system roots: the check fails, the expiry is still read.
	res, err := m.CheckNow(ctx, mon.Id, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Ok == 1 || !strings.Contains(res.Error, "证书无效") || !strings.Contains(res.Detail, "test.local") {
		t.Fatalf("untrusted: %+v", res)
	}

	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	m.SetTLSRoots(pool)
	var sent []string
	for i, at := range []time.Time{now, now.Add(time.Hour), now.Add(4 * 24 * time.Hour), now.Add(8 * 24 * time.Hour), now.Add(8*24*time.Hour + time.Hour)} {
		res, err := m.CheckNow(ctx, mon.Id, at)
		if err != nil {
			t.Fatal(err)
		}
		if res.Ok != 1 {
			t.Fatalf("check %d: %+v", i, res)
		}
	}
	sent = notifications(t, env, "monitor.expiring")
	// 14 days (seen on the first check, also covering the untrusted check),
	// 7 days, 3 days.
	if len(sent) != 3 || sent[0] != "证书还有 9 天过期：Cert" || sent[1] != "证书还有 5 天过期：Cert" || sent[2] != "证书还有 1 天过期：Cert" {
		t.Fatalf("expiring: %v", sent)
	}
	env.MustDo(http.MethodGet, fmt.Sprintf("/monitors/%d", mon.Id), nil, &mon)
	if mon.ExpiresAt == nil || mon.ExpiresAt.Unix() != leaf.NotAfter.Unix() || mon.DaysLeft == nil || *mon.DaysLeft < 9 {
		t.Fatalf("monitor: %+v", mon)
	}

	// Expired: the check fails; no more reminders for this certificate.
	res, err = m.CheckNow(ctx, mon.Id, now.Add(11*24*time.Hour))
	if err != nil || res.Ok == 1 || !strings.Contains(res.Error, "过期") {
		t.Fatalf("expired: %+v %v", res, err)
	}
	if n := len(notifications(t, env, "monitor.expiring")); n != 3 {
		t.Fatalf("expiring after expiry: %d", n)
	}

	// A renewed certificate starts the reminders over.
	renewed, leaf2 := selfSigned(t, now.Add(20*24*time.Hour))
	pool.AddCert(leaf2)
	mu.Lock()
	current = renewed
	mu.Unlock()
	if _, err := m.CheckNow(ctx, mon.Id, now.Add(7*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if sent := notifications(t, env, "monitor.expiring"); len(sent) != 4 {
		t.Fatalf("after renewal: %v", sent)
	}
}

func TestDomainMonitorRDAP(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	now := time.Now().UTC()
	expiry := now.Add(20 * 24 * time.Hour).Truncate(time.Second)
	var paths []string
	var mu sync.Mutex
	rdap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path != "/domain/example.com" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/rdap+json")
		fmt.Fprintf(w, `{"ldhName":"EXAMPLE.COM","events":[{"eventAction":"registration","eventDate":"1995-08-14T04:00:00Z"},
			{"eventAction":"expiration","eventDate":%q}],
			"entities":[{"roles":["registrar"],"vcardArray":["vcard",[["version",{},"text","4.0"],["fn",{},"text","Example Registrar, Inc."]]]}]}`,
			expiry.Format(time.RFC3339))
	}))
	defer rdap.Close()
	if err := env.App.Deps.Settings.Set(ctx, monitoring.RDAPBaseKey, rdap.URL+"/"); err != nil {
		t.Fatal(err)
	}
	var whoisQueries []string
	var whoisMu sync.Mutex
	whoisLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer whoisLn.Close()
	go func() {
		for {
			c, err := whoisLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 256)
				n, _ := c.Read(buf)
				whoisMu.Lock()
				whoisQueries = append(whoisQueries, strings.TrimSpace(string(buf[:n])))
				whoisMu.Unlock()
				_, _ = io.WriteString(c, "no whois server\r\n")
			}(c)
		}
	}()
	if err := env.App.Deps.Settings.Set(ctx, monitoring.WhoisIANAKey, whoisLn.Addr().String()); err != nil {
		t.Fatal(err)
	}

	var mon api.Monitor
	env.MustDo(http.MethodPost, "/monitors", api.MonitorInput{Kind: "domain", Name: "Example", Target: "https://www.Example.com/"}, &mon)
	if mon.Target != "example.com" || mon.IntervalSeconds != 86400 {
		t.Fatalf("created: %+v", mon)
	}
	var res api.MonitorResult
	env.MustDo(http.MethodPost, fmt.Sprintf("/monitors/%d/check", mon.Id), nil, &res)
	if !res.Ok || res.Detail.ExpiresAt == nil || !res.Detail.ExpiresAt.Equal(expiry) || res.Detail.Registrar == nil ||
		*res.Detail.Registrar != "Example Registrar, Inc." {
		t.Fatalf("check: %+v", res)
	}
	for _, at := range []time.Time{now.Add(time.Hour), now.Add(14 * 24 * time.Hour), now.Add(15 * 24 * time.Hour)} {
		if _, err := m.CheckNow(ctx, mon.Id, at); err != nil {
			t.Fatal(err)
		}
	}
	sent := notifications(t, env, "monitor.expiring")
	if len(sent) != 2 || !strings.HasPrefix(sent[0], "域名还有 19 天过期") || !strings.HasPrefix(sent[1], "域名还有 5 天过期") {
		t.Fatalf("expiring: %v", sent)
	}

	var missing api.Monitor
	env.MustDo(http.MethodPost, "/monitors", api.MonitorInput{Kind: "domain", Name: "Nope", Target: "missing.org"}, &missing)
	env.MustDo(http.MethodPost, fmt.Sprintf("/monitors/%d/check", missing.Id), nil, &res)
	if res.Ok || !strings.Contains(res.Error, "查不到") {
		t.Fatalf("missing: %+v", res)
	}
	mu.Lock()
	defer mu.Unlock()
	if paths[0] != "/domain/example.com" || paths[len(paths)-1] != "/domain/missing.org" {
		t.Fatalf("rdap paths: %v", paths)
	}
	whoisMu.Lock()
	defer whoisMu.Unlock()
	if len(whoisQueries) != 1 || whoisQueries[0] != "org" {
		t.Fatalf("whois queries: %v", whoisQueries)
	}

	var list []api.Monitor
	env.MustDo(http.MethodGet, "/monitors?kind=domain", nil, &list)
	if len(list) != 2 {
		t.Fatalf("list: %+v", list)
	}
}
