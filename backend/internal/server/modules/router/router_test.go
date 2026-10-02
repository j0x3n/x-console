package router_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// The module is registered in app/modules.go, so testutil.New(t) includes it.

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func setup(t *testing.T) (*testutil.Env, *fakeUbus, *router.Module, *clock) {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*router.Module](env.App.Deps.Registry, router.ServiceKey)
	if !ok {
		t.Fatal("router module not registered")
	}
	c := &clock{t: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
	router.SetNow(m, c.now)
	f := newFakeUbus(t)
	env.Elevate()
	env.MustDo(http.MethodPut, "/router/config", map[string]any{"url": f.URL() + "/ubus", "username": "xconsole", "password": "secret", "mode": "direct"}, nil)
	return env, f, m, c
}

func TestConfig(t *testing.T) {
	env := testutil.New(t)
	if code, _ := env.Do(http.MethodGet, "/router/status", nil, nil); code != http.StatusPreconditionFailed {
		t.Fatalf("status before config: %d", code)
	}
	f := newFakeUbus(t)
	if code, _ := env.Do(http.MethodPut, "/router/config", map[string]any{"url": f.URL(), "username": "xconsole", "password": "secret"}, nil); code != http.StatusForbidden {
		t.Fatalf("save without elevation: %d", code)
	}
	env.Elevate()
	code, body := env.Do(http.MethodPut, "/router/config", map[string]any{"url": f.URL(), "username": "xconsole", "password": "wrong"}, nil)
	if code != http.StatusBadRequest || !strings.Contains(string(body), "登录路由器失败") {
		t.Fatalf("wrong password: %d %s", code, body)
	}
	var cfg struct {
		URL         string `json:"url"`
		Username    string `json:"username"`
		Mode        string `json:"mode"`
		HasPassword bool   `json:"hasPassword"`
	}
	env.MustDo(http.MethodGet, "/router/config", nil, &cfg)
	if cfg.URL != "" {
		t.Fatalf("saved after a failed login: %+v", cfg)
	}
	env.MustDo(http.MethodPut, "/router/config", map[string]any{"url": f.URL() + "/", "username": "xconsole", "password": "secret"}, &cfg)
	if cfg.URL != f.URL() || !cfg.HasPassword || cfg.Mode != "direct" {
		t.Fatalf("config: %+v", cfg)
	}
	// Saving again without a password keeps the stored one.
	env.MustDo(http.MethodPut, "/router/config", map[string]any{"url": f.URL(), "username": "xconsole"}, &cfg)
	code, body = env.Do(http.MethodGet, "/router/config", nil, nil)
	if code != http.StatusOK || strings.Contains(string(body), "secret") {
		t.Fatalf("config leaks the password: %s", body)
	}
	env.MustDo(http.MethodPut, "/router/config", map[string]any{"url": ""}, &cfg)
	if code, _ := env.Do(http.MethodGet, "/router/status", nil, nil); code != http.StatusPreconditionFailed {
		t.Fatalf("status after clearing: %d", code)
	}
}

type status struct {
	Hostname      string
	Model         string
	Firmware      string
	UptimeSeconds int64
	Load          []float64
	Wan           *struct {
		Name string
		Up   bool
		Ipv4 []string
	}
	Interfaces  []struct{ Name string }
	RxRate      *float64
	TxRate      *float64
	ClientCount int
}

func TestStatusClientsAndRelogin(t *testing.T) {
	env, f, _, c := setup(t)
	var st status
	env.MustDo(http.MethodGet, "/router/status", nil, &st)
	if st.Model != "Xiaomi AX3600" || st.Firmware != "OpenWrt 24.10.0 r28427" || st.UptimeSeconds != 3600 || len(st.Load) != 3 || st.Load[0] != 1 {
		t.Fatalf("status: %+v", st)
	}
	if st.Wan == nil || !st.Wan.Up || len(st.Wan.Ipv4) != 1 || st.Wan.Ipv4[0] != "100.64.1.2/32" || len(st.Interfaces) != 2 {
		t.Fatalf("interfaces: %+v %+v", st.Wan, st.Interfaces)
	}
	if st.RxRate != nil || st.ClientCount != 3 {
		t.Fatalf("first read: rate %v, clients %d", st.RxRate, st.ClientCount)
	}
	// Five seconds later the counters moved: that is the live rate.
	f.set(func(f *fakeUbus) { f.rx += 50000; f.tx += 5000 })
	c.t = c.t.Add(5 * time.Second)
	env.MustDo(http.MethodGet, "/router/status", nil, &st)
	if st.RxRate == nil || *st.RxRate != 10000 || *st.TxRate != 1000 {
		t.Fatalf("rate: %v %v", st.RxRate, st.TxRate)
	}

	var clients struct {
		Items []struct {
			Mac, Name, IP  string
			Ipv6           []string
			LeaseExpiresAt *time.Time
			OnlineSince    *time.Time
		}
	}
	env.MustDo(http.MethodGet, "/router/clients", nil, &clients)
	got := []string{}
	for _, it := range clients.Items {
		got = append(got, it.Name+" "+it.IP)
	}
	if strings.Join(got, ",") != "phone 192.168.1.3,nas 192.168.1.20,printer 192.168.1.100" {
		t.Fatalf("clients: %v", got)
	}
	nas := clients.Items[1]
	if nas.Mac != "AA:BB:CC:00:00:02" || len(nas.Ipv6) != 1 || nas.LeaseExpiresAt == nil || nas.OnlineSince == nil {
		t.Fatalf("nas: %+v", nas)
	}

	// rpcd restarted: the old session is gone and the module logs in again.
	f.expire()
	env.MustDo(http.MethodGet, "/router/status", nil, &st)
	f.mu.Lock()
	logins := f.logins
	f.mu.Unlock()
	if logins < 3 { // config check, first client, relogin
		t.Fatalf("logins: %d", logins)
	}

	// A method the ACL does not allow shows the reason.
	f.set(func(f *fakeUbus) { f.denied["system.board"] = true })
	code, body := env.Do(http.MethodGet, "/router/status", nil, nil)
	if code != http.StatusBadGateway || !strings.Contains(string(body), "ACL") {
		t.Fatalf("denied: %d %s", code, body)
	}
}

func TestTrafficAndWANAlerts(t *testing.T) {
	env, f, m, c := setup(t)
	ctx := t.Context()
	// One sample a minute for ten minutes, 60 KB down and 6 KB up each.
	for i := 0; i <= 10; i++ {
		m.Poll(ctx)
		f.set(func(f *fakeUbus) { f.rx += 60000; f.tx += 6000 })
		c.t = c.t.Add(time.Minute)
	}
	var tr struct {
		Range       string
		StepSeconds int
		RxBytes     int64
		TxBytes     int64
		Points      []struct {
			At     time.Time
			RxRate float64
			TxRate float64
		}
	}
	env.MustDo(http.MethodGet, "/router/traffic?range=24h", nil, &tr)
	if tr.StepSeconds != 300 || tr.RxBytes != 600000 || tr.TxBytes != 60000 || len(tr.Points) != 3 || tr.Points[0].RxRate != 1000 {
		t.Fatalf("traffic: %+v", tr)
	}
	env.MustDo(http.MethodGet, "/router/traffic?range=7d", nil, &tr)
	if tr.StepSeconds != 3600 || len(tr.Points) != 1 || tr.Points[0].RxRate != 1000 {
		t.Fatalf("traffic 7d: %+v", tr)
	}
	// A PPPoE redial resets the counters: the new value counts as new traffic.
	f.set(func(f *fakeUbus) { f.rx = 3000; f.tx = 300 })
	m.Poll(ctx)
	env.MustDo(http.MethodGet, "/router/traffic?range=24h", nil, &tr)
	if tr.RxBytes != 603000 {
		t.Fatalf("after reset: %d", tr.RxBytes)
	}

	type notes struct {
		Items []struct{ Kind, Title, Body, Link string }
	}
	var n notes
	// WAN down for one minute: nothing yet. Two minutes: one notification.
	f.set(func(f *fakeUbus) { f.wanUp = false })
	for i := 0; i < 4; i++ {
		c.t = c.t.Add(time.Minute)
		m.Poll(ctx)
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &n)
	if len(n.Items) != 1 || n.Items[0].Kind != "router.wan_down" || n.Items[0].Title != "WAN 口掉线了" || n.Items[0].Link != "/router" {
		t.Fatalf("down: %+v", n.Items)
	}
	f.set(func(f *fakeUbus) { f.wanUp = true })
	c.t = c.t.Add(time.Minute)
	m.Poll(ctx)
	env.MustDo(http.MethodGet, "/notifications", nil, &n)
	if len(n.Items) != 2 || n.Items[0].Kind != "router.wan_up" || !strings.Contains(n.Items[0].Body, "断了 4 分钟") || !strings.Contains(n.Items[0].Body, "100.64.1.2") {
		t.Fatalf("back: %+v", n.Items)
	}
	// A short blip sends nothing.
	f.set(func(f *fakeUbus) { f.wanUp = false })
	c.t = c.t.Add(time.Minute)
	m.Poll(ctx)
	f.set(func(f *fakeUbus) { f.wanUp = true })
	c.t = c.t.Add(time.Minute)
	m.Poll(ctx)
	// The router stops answering: that is reported too.
	f.srv.Close()
	for i := 0; i < 3; i++ {
		c.t = c.t.Add(time.Minute)
		m.Poll(ctx)
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &n)
	if len(n.Items) != 3 || n.Items[0].Title != "连不上家里的路由器" {
		t.Fatalf("unreachable: %+v", n.Items)
	}
}

func TestRestartAndReboot(t *testing.T) {
	env, f, _, _ := setup(t)
	if code, _ := env.Do(http.MethodPost, "/router/interfaces/wan;reboot/restart", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("bad name: %d", code)
	}
	env.MustDo(http.MethodPost, "/router/interfaces/wan/restart", nil, nil)
	if !f.called("network.interface.wan.down") || !f.called("network.interface.wan.up") {
		t.Fatalf("calls: %v", f.calls)
	}
	if code, _ := env.Do(http.MethodPost, "/router/reboot", map[string]any{"confirm": "yes"}, nil); code != http.StatusBadRequest {
		t.Fatalf("reboot without the word: %d", code)
	}
	if f.called("system.reboot") {
		t.Fatal("rebooted without confirmation")
	}
	if code, _ := env.Do(http.MethodPost, "/router/reboot", map[string]any{"confirm": "重启"}, nil); code != http.StatusAccepted {
		t.Fatalf("reboot: %d", code)
	}
	if !f.called("system.reboot") {
		t.Fatal("no reboot call")
	}
}

// A refusal from the router (ACL) is a setup problem, not an outage.
func TestDeniedIsNotAnOutage(t *testing.T) {
	env, f, m, c := setup(t)
	f.set(func(f *fakeUbus) { f.denied["network.interface.dump"] = true })
	for i := 0; i < 4; i++ {
		c.t = c.t.Add(time.Minute)
		m.Poll(t.Context())
	}
	var n struct{ Items []struct{ Kind string } }
	env.MustDo(http.MethodGet, "/notifications", nil, &n)
	if len(n.Items) != 0 {
		t.Fatalf("notifications: %+v", n.Items)
	}
}

// B93: routers without LuCI answer "Object not found" for luci-rpc.
func TestClientsWithoutLuci(t *testing.T) {
	env, f, _, c := setup(t)
	type item struct{ Mac, Name, IP string }
	list := func() []string {
		var out struct{ Items []item }
		env.MustDo(http.MethodGet, "/router/clients", nil, &out)
		got := []string{}
		for _, it := range out.Items {
			got = append(got, it.Mac+" "+it.Name+" "+it.IP)
		}
		return got
	}
	expiry := c.t.Add(time.Hour).Unix()
	f.set(func(f *fakeUbus) {
		f.noLuci = true
		f.files = map[string]string{
			"/tmp/dhcp.leases": fmt.Sprintf("%d aa:bb:cc:00:00:02 192.168.1.20 nas 01:aa\n0 aa:bb:cc:00:00:03 192.168.1.3 * *\n", expiry),
			"/proc/net/arp": "IP address       HW type     Flags       HW address            Mask     Device\n" +
				"192.168.1.3      0x1         0x2         aa:bb:cc:00:00:03     *        br-lan\n" +
				"192.168.1.50     0x1         0x2         aa:bb:cc:00:00:05     *        br-lan\n" +
				"192.168.1.60     0x1         0x0         00:00:00:00:00:00     *        br-lan\n",
		}
	})
	if got := strings.Join(list(), ","); got != "AA:BB:CC:00:00:03  192.168.1.3,AA:BB:CC:00:00:02 nas 192.168.1.20,AA:BB:CC:00:00:05  192.168.1.50" {
		t.Fatalf("lease file and arp: %s", got)
	}

	// odhcpd answers once the lease file is gone; MACs come without colons.
	c.t = c.t.Add(time.Minute)
	f.set(func(f *fakeUbus) {
		delete(f.files, "/tmp/dhcp.leases")
		f.odhcpd = []map[string]any{{"mac": "aabbcc000007", "hostname": "tv", "address": "192.168.1.7", "valid": 600}}
	})
	if got := strings.Join(list(), ","); !strings.Contains(got, "AA:BB:CC:00:00:07 tv 192.168.1.7") {
		t.Fatalf("odhcpd: %s", got)
	}

	// Nothing to read: a clear message instead of "Object not found".
	c.t = c.t.Add(time.Minute)
	f.set(func(f *fakeUbus) { f.odhcpd, f.files = nil, nil })
	code, body := env.Do(http.MethodGet, "/router/clients", nil, nil)
	if code != http.StatusBadGateway || !strings.Contains(string(body), "读不到在线设备") {
		t.Fatalf("no source: %d %s", code, body)
	}
}
