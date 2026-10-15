package router_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/router"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// B230: events and the router's system log.

type logItem struct {
	Id                                int64
	At                                time.Time
	Level, Facility, Program, Message string
}

type logPage struct {
	Items    []logItem
	HasMore  bool
	Programs []string
}

type eventItem struct {
	Id          int64
	At          time.Time
	Kind, Title string
	Detail      string
	Mac         *string
}

type eventPage struct {
	Items   []eventItem
	HasMore bool
}

func getLogs(t *testing.T, env *testutil.Env, query string) logPage {
	t.Helper()
	var p logPage
	env.MustDo(http.MethodGet, "/router/logs"+query, nil, &p)
	return p
}

func getEvents(t *testing.T, env *testutil.Env, query string) eventPage {
	t.Helper()
	var p eventPage
	env.MustDo(http.MethodGet, "/router/events"+query, nil, &p)
	return p
}

// withLog adds the log and time zone sections to a report.
func withLog(body, tz, logText string) string {
	return body + "#xc:tz\n" + tz + "\n#xc:log\n" + logText
}

func messages(items []logItem) string {
	var out []string
	for _, i := range items {
		out = append(out, i.Message)
	}
	return strings.Join(out, "|")
}

func TestParseLogs(t *testing.T) {
	loc := router.ZoneFromOffset("+0800")
	if loc == nil {
		t.Fatal("no zone")
	}
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	text := "Wed Oct 15 19:59:30 2026 daemon.info dnsmasq-dhcp[1234]: DHCPACK(br-lan) 192.168.1.11 aa:bb:cc:00:01:01 phone\n" +
		"Wed Oct 15 19:59:31 2026 kern.warn kernel: [  123.4] br-lan: port 3(wlan0) entered disabled state\n" +
		"Wed Oct  5 19:59:32 2026 daemon.err hostapd: wlan0: STA aa:bb:cc:00:01:01 IEEE 802.11: disassociated\n" +
		"Wed Oct 15 19:59:33 2026 authpriv.notice dropbear[88]:Exit before auth (user '', 0 fails)\n" +
		"this line is nothing like the others\n" +
		"\n"
	got := router.ParseLogs(text, loc, 0, now)
	if len(got) != 5 {
		t.Fatalf("lines: %d", len(got))
	}
	want := []struct{ level, facility, program, message string }{
		{"info", "daemon", "dnsmasq-dhcp", "DHCPACK(br-lan) 192.168.1.11 aa:bb:cc:00:01:01 phone"},
		{"warn", "kern", "kernel", "[  123.4] br-lan: port 3(wlan0) entered disabled state"},
		{"err", "daemon", "hostapd", "wlan0: STA aa:bb:cc:00:01:01 IEEE 802.11: disassociated"},
		{"notice", "authpriv", "dropbear", "Exit before auth (user '', 0 fails)"},
		{"", "", "", "this line is nothing like the others"},
	}
	for i, w := range want {
		l := got[i]
		if l.Level() != w.level || l.Facility() != w.facility || l.Program() != w.program || l.Message() != w.message {
			t.Errorf("line %d: %+v", i, l)
		}
	}
	// The router is in UTC+8: 19:59:30 there is 11:59:30 UTC.
	if !got[0].At().Equal(time.Date(2026, 10, 15, 11, 59, 30, 0, time.UTC)) || !got[2].At().Equal(time.Date(2026, 10, 5, 11, 59, 32, 0, time.UTC)) {
		t.Errorf("times: %v %v", got[0].At(), got[2].At())
	}
	// A line that cannot be read gets the time of the report.
	if !got[4].At().Equal(now) {
		t.Errorf("raw line time: %v", got[4].At())
	}
	// The panel's clock is ahead: the times move with it, but never past now.
	got = router.ParseLogs(text, loc, time.Hour, now)
	if !got[0].At().Equal(time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("skewed: %v", got[0].At())
	}
	// Only 500 lines are taken.
	many := strings.Repeat("Wed Oct 15 19:59:30 2026 daemon.info a[1]: x\n", 600)
	if n := len(router.ParseLogs(many, loc, 0, now)); n != 500 {
		t.Errorf("limit: %d", n)
	}
	for in, ok := range map[string]bool{"+0800": true, "-0530": true, "0800": false, "+08": false, "": false, "+9900": false} {
		if (router.ZoneFromOffset(in) != nil) != ok {
			t.Errorf("zone %q", in)
		}
	}
}

func TestLogsStoredAndListed(t *testing.T) {
	env, _, c, tok := setupPush(t)
	// The router's clock is an hour behind the panel's and it is in UTC+8.
	routerNow := c.t.Add(-time.Hour)
	body := reportBody(1000, 500, true, routerNow.Unix())
	text := "Thu Oct  9 18:59:50 2026 daemon.info dnsmasq-dhcp[1]: DHCPACK phone\n" +
		"Thu Oct  9 18:59:51 2026 kern.warn kernel: wlan0 dropped\n" +
		"Thu Oct  9 18:59:52 2026 daemon.err hostapd: wlan0 failed\n" +
		"Thu Oct  9 18:59:53 2026 daemon.crit procd: service died\n" +
		"Thu Oct  9 18:59:54 2026 daemon.notice netifd: link up\n" +
		"not a log line\n"
	if code := post(t, env, tok.Token, withLog(body, "+0800", text)); code != http.StatusOK {
		t.Fatalf("report: %d", code)
	}
	p := getLogs(t, env, "")
	// The newest is first, and the unreadable line is kept as it came.
	if messages(p.Items) != "not a log line|link up|service died|wlan0 failed|wlan0 dropped|DHCPACK phone" || p.HasMore {
		t.Fatalf("items: %s", messages(p.Items))
	}
	// 18:59:50 at UTC+8 is 10:59:50 on the router's clock, an hour behind: 11:59:50 here.
	last := p.Items[5]
	if !last.At.Equal(time.Date(2026, 10, 9, 11, 59, 50, 0, time.UTC)) || last.Level != "info" || last.Facility != "daemon" || last.Program != "dnsmasq-dhcp" {
		t.Fatalf("last: %+v", last)
	}
	if strings.Join(p.Programs, ",") != "dnsmasq-dhcp,hostapd,kernel,netifd,procd" {
		t.Fatalf("programs: %v", p.Programs)
	}

	// Levels mean this one and the worse ones.
	if got := messages(getLogs(t, env, "?level=warn").Items); got != "service died|wlan0 failed|wlan0 dropped" {
		t.Fatalf("warn: %s", got)
	}
	if got := messages(getLogs(t, env, "?level=err").Items); got != "service died|wlan0 failed" {
		t.Fatalf("err: %s", got)
	}
	if got := messages(getLogs(t, env, "?program=hostapd").Items); got != "wlan0 failed" {
		t.Fatalf("program: %s", got)
	}
	if got := messages(getLogs(t, env, "?q=wlan0").Items); got != "wlan0 failed|wlan0 dropped" {
		t.Fatalf("q: %s", got)
	}
	if got := messages(getLogs(t, env, "?q=netifd").Items); got != "link up" {
		t.Fatalf("q on the program: %s", got)
	}
	if got := messages(getLogs(t, env, "?q=%25").Items); got != "" {
		t.Fatalf("percent is not a wildcard: %s", got)
	}
	if got := messages(getLogs(t, env, "?q=wlan0&level=err").Items); got != "wlan0 failed" {
		t.Fatalf("q and level: %s", got)
	}
	if code, _ := env.Do(http.MethodGet, "/router/logs?level=info", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("bad level: %d", code)
	}

	// Pages.
	first := getLogs(t, env, "?limit=4")
	if len(first.Items) != 4 || !first.HasMore {
		t.Fatalf("first page: %d %v", len(first.Items), first.HasMore)
	}
	second := getLogs(t, env, fmt.Sprintf("?limit=4&before=%d", first.Items[3].Id))
	if messages(second.Items) != "wlan0 dropped|DHCPACK phone" || second.HasMore {
		t.Fatalf("second page: %s %v", messages(second.Items), second.HasMore)
	}
}

func TestLogsWithoutTimeZoneUseThePanelsZone(t *testing.T) {
	env, _, c, tok := setupPush(t)
	body := reportBody(1000, 500, true, c.t.Unix())
	// No #xc:tz: an old script. The panel's own zone (Asia/Shanghai, UTC+8) is used.
	if code := post(t, env, tok.Token, body+"#xc:log\nThu Oct  9 19:59:50 2026 daemon.info a[1]: hello\n"); code != http.StatusOK {
		t.Fatalf("report: %d", code)
	}
	p := getLogs(t, env, "")
	if len(p.Items) != 1 || !p.Items[0].At.Equal(time.Date(2026, 10, 9, 11, 59, 50, 0, time.UTC)) {
		t.Fatalf("items: %+v", p.Items)
	}
	// A report without the section adds nothing.
	post(t, env, tok.Token, body)
	if n := len(getLogs(t, env, "").Items); n != 1 {
		t.Fatalf("lines: %d", n)
	}
}

func TestEventsForDevicesAndCommands(t *testing.T) {
	env, _, c, tok := setupPush(t)
	home := devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr(neighLine("192.168.1.12", macPC, "REACHABLE"))}
	away := devOpts{leases: leasesPhone, wifi: hostapd("wlan0"), neigh: ptr(neighLine("192.168.1.12", macPC, "REACHABLE"))}
	report(t, env, tok, c, home)
	c.t = c.t.Add(10 * time.Second)
	report(t, env, tok, c, away)
	c.t = c.t.Add(10 * time.Second)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone, macTV), neigh: away.neigh})
	env.MustDo(http.MethodPost, "/router/interfaces/wan/restart", nil, nil)
	env.MustDo(http.MethodPost, "/router/reboot", map[string]any{"confirm": "重启"}, nil)

	all := getEvents(t, env, "")
	kinds := []string{}
	for _, e := range all.Items {
		kinds = append(kinds, e.Kind)
	}
	if strings.Join(kinds, ",") != "reboot,interface_restart,device_new,device_online,device_offline,devices_seeded" || all.HasMore {
		t.Fatalf("kinds: %v", kinds)
	}
	if e := all.Items[1]; e.Title != "重启接口 wan" || !strings.Contains(e.Detail, "操作者 "+testutil.Username) || !strings.Contains(e.Detail, "排队") {
		t.Fatalf("restart: %+v", e)
	}
	if e := all.Items[0]; e.Title != "重启路由器" || e.Mac != nil {
		t.Fatalf("reboot: %+v", e)
	}
	if e := all.Items[4]; e.Mac == nil || *e.Mac != macPhone || e.Title != "phone 离线了" || !strings.Contains(e.Detail, "IP 192.168.1.11") {
		t.Fatalf("offline: %+v", e)
	}

	if got := len(getEvents(t, env, "?group=device").Items); got != 4 {
		t.Fatalf("device group: %d", got)
	}
	if got := getEvents(t, env, "?group=command").Items; len(got) != 2 {
		t.Fatalf("command group: %+v", got)
	}
	if got := getEvents(t, env, "?group=wan").Items; len(got) != 0 {
		t.Fatalf("wan group: %+v", got)
	}
	if got := getEvents(t, env, "?mac="+strings.ToLower(macPhone)).Items; len(got) != 2 {
		t.Fatalf("one device: %d", len(got))
	}
	if got := getEvents(t, env, "?group=device&mac="+macTV).Items; len(got) != 1 || got[0].Kind != "device_new" {
		t.Fatalf("new device: %+v", got)
	}
	if code, _ := env.Do(http.MethodGet, "/router/events?group=nothing", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("bad group: %d", code)
	}
	if code, _ := env.Do(http.MethodGet, "/router/events?mac=nonsense", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("bad mac: %d", code)
	}

	// Pages: newest first, before is the id of the last row.
	first := getEvents(t, env, "?limit=2")
	if len(first.Items) != 2 || !first.HasMore {
		t.Fatalf("first: %+v", first)
	}
	second := getEvents(t, env, fmt.Sprintf("?limit=2&before=%d", first.Items[1].Id))
	if len(second.Items) != 2 || !second.HasMore || second.Items[0].Id >= first.Items[1].Id {
		t.Fatalf("second: %+v", second)
	}
	third := getEvents(t, env, fmt.Sprintf("?limit=2&before=%d", second.Items[1].Id))
	if len(third.Items) != 2 || third.HasMore {
		t.Fatalf("third: %+v", third)
	}
	if got := len(getEvents(t, env, "?limit=1000").Items); got != 6 {
		t.Fatalf("limit over the maximum: %d", got)
	}
}

func TestEventsForWANAndReports(t *testing.T) {
	env, m, c, tok := setupPush(t)
	// The router stops reporting.
	post(t, env, tok.Token, reportBody(1000, 500, true, c.t.Unix()))
	for i := 0; i < 6; i++ {
		c.t = c.t.Add(time.Minute)
		m.Poll(t.Context())
	}
	post(t, env, tok.Token, reportBody(2000, 900, true, c.t.Unix()))
	// The WAN goes down for three minutes and comes back.
	for i := 0; i < 4; i++ {
		c.t = c.t.Add(time.Minute)
		post(t, env, tok.Token, reportBody(2000, 900, false, c.t.Unix()))
	}
	post(t, env, tok.Token, reportBody(3000, 900, true, c.t.Unix()))

	kinds := []string{}
	for _, e := range getEvents(t, env, "").Items {
		if e.Kind != "devices_seeded" {
			kinds = append(kinds, e.Kind)
		}
	}
	if strings.Join(kinds, ",") != "wan_up,wan_down,report_back,report_lost" {
		t.Fatalf("kinds: %v", kinds)
	}
	if got := getEvents(t, env, "?group=report").Items; len(got) != 2 || got[1].Title != "家里的路由器不上报了" {
		t.Fatalf("report group: %+v", got)
	}
	if got := getEvents(t, env, "?group=wan").Items; len(got) != 2 || got[1].Title != "WAN 口掉线了" || got[0].Title != "家里网络恢复了" {
		t.Fatalf("wan group: %+v", got)
	}
}

func TestEventsForDirectRestart(t *testing.T) {
	env, _, _, _ := setup(t)
	env.MustDo(http.MethodPost, "/router/interfaces/wan/restart", nil, nil)
	env.MustDo(http.MethodPost, "/router/reboot", map[string]any{"confirm": "重启"}, nil)
	items := getEvents(t, env, "?group=command").Items
	if len(items) != 2 || items[0].Kind != "reboot" || items[1].Kind != "interface_restart" || strings.Contains(items[1].Detail, "排队") {
		t.Fatalf("events: %+v", items)
	}
}

func TestLogsPruneByDays(t *testing.T) {
	env, m, c, tok := setupPush(t)
	post(t, env, tok.Token, reportBody(1000, 500, true, c.t.Unix()))
	now := c.t.Unix()
	day := int64(24 * 3600)
	for _, age := range []int64{0, 1*day - 60, 2 * day, 5 * day, 9 * day} {
		execSQL(t, env, `INSERT INTO router_events (at, kind, title) VALUES (?, 'wan_down', 'x')`, now-age)
		execSQL(t, env, `INSERT INTO router_logs (at, message) VALUES (?, 'x')`, now-age)
	}
	// The default is 7 days.
	m.Poll(t.Context())
	if a, b := countRows(t, env, `SELECT COUNT(*) FROM router_events WHERE kind = 'wan_down'`), countRows(t, env, `SELECT COUNT(*) FROM router_logs`); a != 4 || b != 4 {
		t.Fatalf("7 days: %d events, %d lines", a, b)
	}
	// 1 day: the next hourly cleanup removes the rest.
	env.MustDo(http.MethodPut, "/router/options", map[string]any{"deviceNotify": false, "newDeviceNotify": true, "notifyCooldownMinutes": 15, "logDays": 1}, nil)
	m.Poll(t.Context())
	if a := countRows(t, env, `SELECT COUNT(*) FROM router_logs`); a != 4 {
		t.Fatalf("pruned again within the hour: %d", a)
	}
	c.t = c.t.Add(61 * time.Minute)
	post(t, env, tok.Token, reportBody(1000, 500, true, c.t.Unix()))
	m.Poll(t.Context())
	if a, b := countRows(t, env, `SELECT COUNT(*) FROM router_events WHERE kind = 'wan_down'`), countRows(t, env, `SELECT COUNT(*) FROM router_logs`); a != 1 || b != 1 {
		t.Fatalf("1 day: %d events, %d lines", a, b)
	}
}

func TestLogsRowLimit(t *testing.T) {
	env, m, c, tok := setupPush(t)
	post(t, env, tok.Token, reportBody(1000, 500, true, c.t.Unix()))
	execSQL(t, env, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 200100) INSERT INTO router_logs (at, message) SELECT ?, 'row ' || i FROM n`, c.t.Unix())
	m.Poll(t.Context())
	if n := countRows(t, env, `SELECT COUNT(*) FROM router_logs`); n != 200000 {
		t.Fatalf("rows: %d", n)
	}
	// The oldest 100 are gone.
	if n := countRows(t, env, `SELECT COUNT(*) FROM router_logs WHERE message = 'row 100' OR message = 'row 1'`); n != 0 {
		t.Fatalf("old rows kept: %d", n)
	}
	if n := countRows(t, env, `SELECT COUNT(*) FROM router_logs WHERE message = 'row 101' OR message = 'row 200100'`); n != 2 {
		t.Fatalf("new rows lost: %d", n)
	}
}

func execSQL(t *testing.T, env *testutil.Env, query string, args ...any) {
	t.Helper()
	if _, err := env.App.Deps.DB.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}
