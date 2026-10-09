package router_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// B114: the router script posts its state.

type pushToken struct {
	Token, ReportUrl, Script string
}

func setupPush(t *testing.T) (*testutil.Env, *router.Module, *clock, pushToken) {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*router.Module](env.App.Deps.Registry, router.ServiceKey)
	if !ok {
		t.Fatal("router module not registered")
	}
	c := &clock{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	router.SetNow(m, c.now)
	env.Elevate()
	var tok pushToken
	env.MustDo(http.MethodPost, "/router/push/token", nil, &tok)
	return env, m, c, tok
}

// reportBody is what report.sh sends. rx and tx are the WAN byte counters.
func reportBody(rx, tx int, wanUp bool, routerNow int64) string {
	return fmt.Sprintf(`#xc:time
%d
#xc:board
{"hostname":"OpenWrt","model":"Xiaomi AX3600","release":{"description":"OpenWrt 24.10.0 r28427"}}
#xc:info
{"uptime":7200,"load":[65536,0,0],"memory":{"total":1000,"available":600}}
#xc:interfaces
{"interface":[
 {"interface":"loopback","up":true,"device":"lo"},
 {"interface":"lan","up":true,"uptime":7000,"proto":"static","device":"br-lan","l3_device":"br-lan","ipv4-address":[{"address":"192.168.1.1","mask":24}]},
 {"interface":"wan","up":%t,"uptime":3000,"proto":"dhcp","device":"eth1","l3_device":"eth1","ipv4-address":[{"address":"100.64.1.2","mask":32}]}
]}
#xc:devices
{"eth1":{"statistics":{"rx_bytes":%d,"tx_bytes":%d}},"br-lan":{"statistics":{"rx_bytes":5,"tx_bytes":5}}}
#xc:leases
%d aa:bb:cc:00:00:02 192.168.1.20 nas 01:aa
0 aa:bb:cc:00:00:03 192.168.1.3 * *
#xc:arp
IP address       HW type     Flags       HW address            Mask     Device
192.168.1.3      0x1         0x2         aa:bb:cc:00:00:03     *        br-lan
192.168.1.50     0x1         0x2         aa:bb:cc:00:00:05     *        br-lan
192.168.1.60     0x1         0x0         00:00:00:00:00:00     *        br-lan
`, routerNow, wanUp, rx, tx, routerNow+3600)
}

// post sends a report like the script does: plain text, bearer token, no session.
func post(t *testing.T, env *testutil.Env, token, body string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, env.URL("/router/report"), strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "text/plain")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{}).Do(req) // no cookies: the router has no session
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestPushToken(t *testing.T) {
	env, _, c, tok := setupPush(t)
	if len(tok.Token) != 64 || !strings.HasSuffix(tok.ReportUrl, "/api/v1/router/report") {
		t.Fatalf("token: %+v", tok)
	}
	for _, want := range []string{"URL='" + tok.ReportUrl + "'", "TOKEN='" + tok.Token + "'", "crontab", "ubus call network.interface dump"} {
		if !strings.Contains(tok.Script, want) {
			t.Fatalf("install script misses %q:\n%s", want, tok.Script)
		}
	}
	if strings.Contains(tok.Script, "@URL@") || strings.Contains(tok.Script, "@TOKEN@") {
		t.Fatal("placeholders left in the script")
	}

	var cfg struct {
		Mode         string
		URL          string
		HasPassword  bool
		ReportUrl    string
		LastReportAt *time.Time
	}
	env.MustDo(http.MethodGet, "/router/config", nil, &cfg)
	if cfg.Mode != "push" || cfg.ReportUrl != tok.ReportUrl || cfg.LastReportAt != nil {
		t.Fatalf("config: %+v", cfg)
	}
	// The token is not readable again.
	if _, raw := env.Do(http.MethodGet, "/router/config", nil, nil); strings.Contains(string(raw), tok.Token) {
		t.Fatalf("config leaks the token: %s", raw)
	}

	// Wrong or missing token, with or without a session.
	body := reportBody(1000, 500, true, c.t.Unix())
	if code := post(t, env, "", body); code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", code)
	}
	if code := post(t, env, strings.Repeat("0", 64), body); code != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", code)
	}
	if code := post(t, env, tok.Token, body); code != http.StatusNoContent {
		t.Fatalf("report: %d", code)
	}
	// A new token replaces the old one.
	var next pushToken
	env.MustDo(http.MethodPost, "/router/push/token", nil, &next)
	if code := post(t, env, tok.Token, body); code != http.StatusUnauthorized {
		t.Fatalf("old token still works: %d", code)
	}
	if code := post(t, env, next.Token, body); code != http.StatusNoContent {
		t.Fatalf("new token: %d", code)
	}
}

func TestPushNeedsElevation(t *testing.T) {
	env := testutil.New(t)
	if code, _ := env.Do(http.MethodPost, "/router/push/token", nil, nil); code != http.StatusForbidden {
		t.Fatalf("token without elevation: %d", code)
	}
	// Not in push mode: any token is refused.
	if code := post(t, env, strings.Repeat("a", 64), reportBody(1, 1, true, 1)); code != http.StatusUnauthorized {
		t.Fatalf("report without push mode: %d", code)
	}
}

func TestPushStatusClientsAndTraffic(t *testing.T) {
	env, _, c, tok := setupPush(t)

	// Before the first report the pages say so.
	code, raw := env.Do(http.MethodGet, "/router/status", nil, nil)
	if code != http.StatusBadGateway || !strings.Contains(string(raw), "还没有收到路由器的上报") {
		t.Fatalf("before the first report: %d %s", code, raw)
	}

	if code := post(t, env, tok.Token, reportBody(1000, 500, true, c.t.Unix())); code != http.StatusNoContent {
		t.Fatalf("first report: %d", code)
	}
	var st status
	var src struct{ Source string }
	env.MustDo(http.MethodGet, "/router/status", nil, &st)
	env.MustDo(http.MethodGet, "/router/status", nil, &src)
	if st.Model != "Xiaomi AX3600" || st.Firmware != "OpenWrt 24.10.0 r28427" || st.Wan == nil || !st.Wan.Up || st.Wan.Ipv4[0] != "100.64.1.2/32" {
		t.Fatalf("status: %+v", st)
	}
	if src.Source != "push" || st.RxRate != nil || st.ClientCount != 3 || len(st.Interfaces) != 2 {
		t.Fatalf("first report: source %s rate %v clients %d ifaces %d", src.Source, st.RxRate, st.ClientCount, len(st.Interfaces))
	}

	// A minute later: 600000 bytes down and 60000 up is 10000 and 1000 per second.
	c.t = c.t.Add(time.Minute)
	if code := post(t, env, tok.Token, reportBody(601000, 60500, true, c.t.Unix())); code != http.StatusNoContent {
		t.Fatalf("second report: %d", code)
	}
	env.MustDo(http.MethodGet, "/router/status", nil, &st)
	if st.RxRate == nil || *st.RxRate != 10000 || *st.TxRate != 1000 {
		t.Fatalf("rate: %v %v", st.RxRate, st.TxRate)
	}

	var clients struct {
		Items []struct {
			Mac, Name, IP  string
			LeaseExpiresAt *time.Time
		}
	}
	env.MustDo(http.MethodGet, "/router/clients", nil, &clients)
	got := []string{}
	for _, it := range clients.Items {
		got = append(got, it.Mac+" "+it.Name+" "+it.IP)
	}
	// Lease times use the router's clock: one hour left, not whatever the clocks differ by.
	if strings.Join(got, ",") != "AA:BB:CC:00:00:03  192.168.1.3,AA:BB:CC:00:00:02 nas 192.168.1.20,AA:BB:CC:00:00:05  192.168.1.50" {
		t.Fatalf("clients: %v", got)
	}
	if exp := clients.Items[1].LeaseExpiresAt; exp == nil || !exp.Equal(c.t.Add(time.Hour)) {
		t.Fatalf("lease expiry: %v", exp)
	}

	// The minute is in the traffic table.
	var tr struct {
		RxBytes, TxBytes int64
		Points           []struct{ RxRate, TxRate float64 }
	}
	env.MustDo(http.MethodGet, "/router/traffic?range=24h", nil, &tr)
	if tr.RxBytes != 600000 || tr.TxBytes != 60000 || len(tr.Points) != 1 {
		t.Fatalf("traffic: %+v", tr)
	}

	var cfg struct{ LastReportAt *time.Time }
	env.MustDo(http.MethodGet, "/router/config", nil, &cfg)
	if cfg.LastReportAt == nil || !cfg.LastReportAt.Equal(c.t) {
		t.Fatalf("last report: %v", cfg.LastReportAt)
	}
}

func TestPushBadReport(t *testing.T) {
	env, _, c, tok := setupPush(t)
	for name, body := range map[string]string{
		"empty":         "",
		"no interfaces": "#xc:board\n{}\n#xc:info\n{}\n",
		"not json":      "#xc:board\nhello\n#xc:info\n{}\n#xc:interfaces\n{}\n",
	} {
		if code := post(t, env, tok.Token, body); code != http.StatusBadRequest {
			t.Fatalf("%s: %d", name, code)
		}
	}
	// A report without counters still works, only the speed is missing.
	body := strings.Join(strings.Split(reportBody(1, 1, true, c.t.Unix()), "#xc:devices")[:1], "")
	body += "#xc:leases\n#xc:arp\n"
	if code := post(t, env, tok.Token, body); code != http.StatusNoContent {
		t.Fatalf("no counters: %d", code)
	}
}

func TestPushOfflineAndBack(t *testing.T) {
	env, m, c, tok := setupPush(t)
	ctx := t.Context()
	var n struct {
		Items []struct{ Kind, Title, Body string }
	}
	notes := func() int {
		env.MustDo(http.MethodGet, "/notifications", nil, &n)
		return len(n.Items)
	}

	m.Poll(ctx) // nothing reported yet: the panel gives the router a full period first
	post(t, env, tok.Token, reportBody(1000, 500, true, c.t.Unix()))
	for i := 0; i < 2; i++ {
		c.t = c.t.Add(time.Minute)
		m.Poll(ctx)
	}
	if notes() != 0 {
		t.Fatalf("two quiet minutes: %+v", n.Items)
	}
	// Three minutes without a report: offline, once.
	for i := 0; i < 4; i++ {
		c.t = c.t.Add(time.Minute)
		m.Poll(ctx)
	}
	if notes() != 1 || n.Items[0].Kind != "router.wan_down" || n.Items[0].Title != "家里的路由器不上报了" {
		t.Fatalf("offline: %+v", n.Items)
	}
	if code, raw := env.Do(http.MethodGet, "/router/status", nil, nil); code != http.StatusBadGateway || !strings.Contains(string(raw), "没收到路由器的上报") {
		t.Fatalf("status while offline: %d %s", code, raw)
	}
	// The next report brings it back, with the length of the outage.
	post(t, env, tok.Token, reportBody(2000, 900, true, c.t.Unix()))
	if notes() != 2 || n.Items[0].Kind != "router.wan_up" {
		t.Fatalf("back: %+v", n.Items)
	}
}

func TestPushWanDownAndNoRestart(t *testing.T) {
	env, m, c, tok := setupPush(t)
	var n struct {
		Items []struct{ Kind, Title string }
	}
	post(t, env, tok.Token, reportBody(1000, 500, false, c.t.Unix()))
	c.t = c.t.Add(time.Minute)
	post(t, env, tok.Token, reportBody(1000, 500, false, c.t.Unix()))
	c.t = c.t.Add(90 * time.Second)
	post(t, env, tok.Token, reportBody(1000, 500, false, c.t.Unix()))
	m.Poll(t.Context())
	env.MustDo(http.MethodGet, "/notifications", nil, &n)
	if len(n.Items) != 1 || n.Items[0].Title != "WAN 口掉线了" {
		t.Fatalf("wan down: %+v", n.Items)
	}

	// The panel cannot reach a router that reports by itself.
	if code, raw := env.Do(http.MethodPost, "/router/interfaces/wan/restart", nil, nil); code != http.StatusConflict {
		t.Fatalf("restart: %d %s", code, raw)
	}
	if code, _ := env.Do(http.MethodPost, "/router/reboot", map[string]any{"confirm": "重启"}, nil); code != http.StatusConflict {
		t.Fatalf("reboot: %d", code)
	}
}

// Saving an address switches back to reading the router over ubus; removing
// the settings ends push mode and its token.
func TestPushSwitchBack(t *testing.T) {
	env, _, c, tok := setupPush(t)
	f := newFakeUbus(t)
	env.MustDo(http.MethodPut, "/router/config", map[string]any{"url": f.URL(), "username": "xconsole", "password": "secret"}, nil)
	var cfg struct{ Mode string }
	env.MustDo(http.MethodGet, "/router/config", nil, &cfg)
	if cfg.Mode != "direct" {
		t.Fatalf("mode: %s", cfg.Mode)
	}
	if code := post(t, env, tok.Token, reportBody(1, 1, true, c.t.Unix())); code != http.StatusUnauthorized {
		t.Fatalf("token after switching back: %d", code)
	}
	if code, _ := env.Do(http.MethodPut, "/router/config", map[string]any{"url": f.URL(), "mode": "push"}, nil); code != http.StatusBadRequest {
		t.Fatalf("push through the config form: %d", code)
	}

	env.MustDo(http.MethodPost, "/router/push/token", nil, &tok)
	env.MustDo(http.MethodPut, "/router/config", map[string]any{"url": ""}, nil)
	env.MustDo(http.MethodGet, "/router/config", nil, &cfg)
	if cfg.Mode != "direct" {
		t.Fatalf("mode after removing: %s", cfg.Mode)
	}
	if code := post(t, env, tok.Token, reportBody(1, 1, true, c.t.Unix())); code != http.StatusUnauthorized {
		t.Fatalf("token after removing: %d", code)
	}
}
