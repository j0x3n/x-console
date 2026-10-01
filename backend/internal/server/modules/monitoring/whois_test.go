package monitoring_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
)

func serveWhois(t *testing.T, answer func(string) string) (string, func() []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var mu sync.Mutex
	var got []string
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))
				buf := make([]byte, 1024)
				n, _ := c.Read(buf)
				q := strings.TrimSpace(string(buf[:n]))
				mu.Lock()
				got = append(got, q)
				mu.Unlock()
				_, _ = io.WriteString(c, answer(q))
			}(c)
		}
	}()
	return ln.Addr().String(), func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := append([]string(nil), got...)
		return out
	}
}

func TestDomainWhoisAndManualExpiry(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	imAddr, _ := serveWhois(t, func(string) string {
		return "Registrar: Isle of Man\r\nExpiry Date: 02/01/2027\r\n"
	})
	regAddr, regQueries := serveWhois(t, func(string) string {
		return "Registrar: Second Registrar\r\nRegistry Expiry Date: 2027-03-04T00:00:00Z\r\n"
	})
	comAddr, comQueries := serveWhois(t, func(string) string {
		return "Registrar WHOIS Server: " + regAddr + "\r\nExpiry Date: 01/02/2027\r\n"
	})
	orgAddr, _ := serveWhois(t, func(string) string {
		return "Domain Name: NONE.ORG\r\n"
	})
	ianaAddr, ianaQueries := serveWhois(t, func(q string) string {
		switch q {
		case "im":
			return "whois: " + imAddr + "\r\n"
		case "com":
			return "whois: " + comAddr + "\r\n"
		case "org":
			return "whois: " + orgAddr + "\r\n"
		default:
			return "\r\n"
		}
	})
	rdap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/domain/ok.example" {
			w.Header().Set("Content-Type", "application/rdap+json")
			_, _ = io.WriteString(w, `{"events":[{"eventAction":"expiration","eventDate":"2028-01-01T00:00:00Z"}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer rdap.Close()
	if err := env.App.Deps.Settings.Set(ctx, monitoring.RDAPBaseKey, rdap.URL); err != nil {
		t.Fatal(err)
	}
	if err := env.App.Deps.Settings.Set(ctx, monitoring.WhoisIANAKey, ianaAddr); err != nil {
		t.Fatal(err)
	}

	check := func(name, target string) (api.Monitor, api.MonitorResult) {
		t.Helper()
		var mon api.Monitor
		env.MustDo(http.MethodPost, "/monitors", api.MonitorInput{Kind: "domain", Name: name, Target: target}, &mon)
		var res api.MonitorResult
		env.MustDo(http.MethodPost, fmt.Sprintf("/monitors/%d/check", mon.Id), nil, &res)
		env.MustDo(http.MethodGet, fmt.Sprintf("/monitors/%d", mon.Id), nil, &mon)
		return mon, res
	}

	okMon, okRes := check("ok", "ok.example")
	if !okRes.Ok || okRes.Detail.Source == nil || *okRes.Detail.Source != api.MonitorResultDetailSourceRdap ||
		okMon.ExpirySource == nil || *okMon.ExpirySource != api.MonitorExpirySourceRdap {
		t.Fatalf("rdap: %+v %+v", okMon, okRes)
	}
	if len(ianaQueries()) != 0 {
		t.Fatalf("rdap success queried whois: %v", ianaQueries())
	}

	imMon, imRes := check("im", "day.im")
	wantDay := time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)
	if !imRes.Ok || imRes.Detail.ExpiresAt == nil || !imRes.Detail.ExpiresAt.Equal(wantDay) ||
		imRes.Detail.Registrar == nil || *imRes.Detail.Registrar != "Isle of Man" ||
		imRes.Detail.Source == nil || *imRes.Detail.Source != api.MonitorResultDetailSourceWhois ||
		imMon.ExpirySource == nil || *imMon.ExpirySource != api.MonitorExpirySourceWhois {
		t.Fatalf("whois day-first: %+v %+v", imMon, imRes)
	}

	comMon, comRes := check("com", "jump.com")
	wantJump := time.Date(2027, 3, 4, 0, 0, 0, 0, time.UTC)
	if !comRes.Ok || comRes.Detail.ExpiresAt == nil || !comRes.Detail.ExpiresAt.Equal(wantJump) ||
		comRes.Detail.Registrar == nil || *comRes.Detail.Registrar != "Second Registrar" ||
		comMon.ExpirySource == nil || *comMon.ExpirySource != api.MonitorExpirySourceWhois {
		t.Fatalf("whois referral: %+v %+v", comMon, comRes)
	}
	if len(comQueries()) == 0 || len(regQueries()) == 0 {
		t.Fatalf("referral queries com=%v reg=%v", comQueries(), regQueries())
	}

	noneMon, noneRes := check("none", "none.org")
	if noneRes.Ok || noneRes.Error != "RDAP 和 WHOIS 都查不到这个域名，可以手动填到期日期" || noneMon.ExpiresAt != nil || noneMon.ExpirySource != nil {
		t.Fatalf("missing: %+v %+v", noneMon, noneRes)
	}

	// The check after a manual date change runs after the reply.
	waitMonitor := func(id int64, done func(api.Monitor) bool) api.Monitor {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			var mon api.Monitor
			env.MustDo(http.MethodGet, fmt.Sprintf("/monitors/%d", id), nil, &mon)
			if done(mon) {
				return mon
			}
			if time.Now().After(deadline) {
				t.Fatalf("monitor %d: %+v", id, mon)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	day := openapi_types.Date{Time: time.Date(2027, 6, 15, 0, 0, 0, 0, time.UTC)}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/monitors/%d", noneMon.Id), api.MonitorPatch{ManualExpiresAt: &day}, &noneMon)
	if noneMon.ManualExpiresAt == nil {
		t.Fatalf("manual date not saved: %+v", noneMon)
	}
	noneMon = waitMonitor(noneMon.Id, func(x api.Monitor) bool { return x.ExpirySource != nil })
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	wantManual := time.Date(2027, 6, 15, 23, 59, 59, 0, loc)
	if noneMon.ExpiresAt == nil || !noneMon.ExpiresAt.Equal(wantManual) ||
		noneMon.ExpirySource == nil || *noneMon.ExpirySource != api.MonitorExpirySourceManual ||
		noneMon.ManualExpiresAt == nil || noneMon.ManualExpiresAt.Format("2006-01-02") != "2027-06-15" {
		t.Fatalf("manual: %+v", noneMon)
	}

	clear := true
	var clearedMon api.Monitor
	env.MustDo(http.MethodPatch, fmt.Sprintf("/monitors/%d", noneMon.Id), api.MonitorPatch{ClearManualExpiry: &clear}, &clearedMon)
	clearedMon = waitMonitor(noneMon.Id, func(x api.Monitor) bool { return x.ExpirySource == nil })
	if clearedMon.ExpiresAt != nil || clearedMon.ExpirySource != nil || clearedMon.ManualExpiresAt != nil ||
		clearedMon.LastError != "RDAP 和 WHOIS 都查不到这个域名，可以手动填到期日期" {
		t.Fatalf("cleared: %+v", clearedMon)
	}

	var site api.Monitor
	env.MustDo(http.MethodPost, "/monitors", api.MonitorInput{Kind: "http", Name: "web", Target: rdap.URL}, &site)
	expectStatus(t, env, http.MethodPatch, fmt.Sprintf("/monitors/%d", site.Id), api.MonitorPatch{ManualExpiresAt: &day}, 400, "validation_failed")
}

// A failed lookup keeps the last known date: the list still shows it and
// the reminder already sent does not go out again.
func TestDomainFailureKeepsExpiry(t *testing.T) {
	env, m := setup(t)
	ctx := t.Context()
	now := time.Now().UTC()
	expiry := now.Add(20 * 24 * time.Hour).Truncate(time.Second)
	var mu sync.Mutex
	down := false
	rdap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		failing := down
		mu.Unlock()
		if failing {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/rdap+json")
		fmt.Fprintf(w, `{"events":[{"eventAction":"expiration","eventDate":%q}]}`, expiry.Format(time.RFC3339))
	}))
	defer rdap.Close()
	ianaAddr, _ := serveWhois(t, func(string) string { return "\r\n" })
	if err := env.App.Deps.Settings.Set(ctx, monitoring.RDAPBaseKey, rdap.URL); err != nil {
		t.Fatal(err)
	}
	if err := env.App.Deps.Settings.Set(ctx, monitoring.WhoisIANAKey, ianaAddr); err != nil {
		t.Fatal(err)
	}
	var mon api.Monitor
	env.MustDo(http.MethodPost, "/monitors", api.MonitorInput{Kind: "domain", Name: "E", Target: "example.com"}, &mon)
	at := now.Add(time.Hour)
	if _, err := m.CheckNow(ctx, mon.Id, at); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	down = true
	mu.Unlock()
	res, err := m.CheckNow(ctx, mon.Id, at.Add(time.Hour))
	if err != nil || res.Ok == 1 {
		t.Fatalf("failing check: %+v %v", res, err)
	}
	env.MustDo(http.MethodGet, fmt.Sprintf("/monitors/%d", mon.Id), nil, &mon)
	if mon.ExpiresAt == nil || !mon.ExpiresAt.Equal(expiry) || mon.ExpirySource == nil || *mon.ExpirySource != api.MonitorExpirySourceRdap {
		t.Fatalf("after failure: %+v", mon)
	}
	mu.Lock()
	down = false
	mu.Unlock()
	if _, err := m.CheckNow(ctx, mon.Id, at.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if sent := notifications(t, env, "monitor.expiring"); len(sent) != 1 {
		t.Fatalf("expiring: %v", sent)
	}
}

// A slow RDAP server leaves time for WHOIS, and a registrar answer without a
// date falls back to the registry's.
func TestDomainSlowRDAPAndReferralFallback(t *testing.T) {
	env, _ := setup(t)
	ctx := t.Context()
	release := make(chan struct{})
	defer close(release)
	rdap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer rdap.Close()
	defer rdap.CloseClientConnections()
	regAddr, regQueries := serveWhois(t, func(string) string { return "Registrar: Small Registrar\r\n" })
	comAddr, _ := serveWhois(t, func(string) string {
		return "Registrar WHOIS Server: " + regAddr + "\r\nRegistry Expiry Date: 2027-05-06T00:00:00Z\r\n"
	})
	ianaAddr, _ := serveWhois(t, func(string) string { return "whois: " + comAddr + "\r\n" })
	if err := env.App.Deps.Settings.Set(ctx, monitoring.RDAPBaseKey, rdap.URL); err != nil {
		t.Fatal(err)
	}
	if err := env.App.Deps.Settings.Set(ctx, monitoring.WhoisIANAKey, ianaAddr); err != nil {
		t.Fatal(err)
	}
	timeout := 1000
	var mon api.Monitor
	env.MustDo(http.MethodPost, "/monitors", api.MonitorInput{Kind: "domain", Name: "slow", Target: "slow.com", TimeoutMs: &timeout}, &mon)
	var res api.MonitorResult
	env.MustDo(http.MethodPost, fmt.Sprintf("/monitors/%d/check", mon.Id), nil, &res)
	want := time.Date(2027, 5, 6, 0, 0, 0, 0, time.UTC)
	if !res.Ok || res.Detail.ExpiresAt == nil || !res.Detail.ExpiresAt.Equal(want) ||
		res.Detail.Source == nil || *res.Detail.Source != api.MonitorResultDetailSourceWhois {
		t.Fatalf("slow rdap: %+v", res)
	}
	if len(regQueries()) == 0 {
		t.Fatal("registrar was not asked")
	}
}
