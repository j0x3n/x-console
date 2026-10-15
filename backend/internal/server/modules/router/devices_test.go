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

// B228: devices, notes, online and offline notifications.

const (
	macPhone = "AA:BB:CC:00:01:01"
	macPC    = "AA:BB:CC:00:01:02"
	macTV    = "AA:BB:CC:00:01:03"
)

// devOpts says what one report contains. A nil wifi or neigh means the script
// does not send that section.
type devOpts struct {
	leases string
	arp    string
	wifi   *string
	neigh  *string
	nlbw   *string
}

func ptr(s string) *string { return &s }

// hostapd is the #xc:wifi section of a router with hostapd objects.
func hostapd(iface string, macs ...string) *string {
	clients := make([]string, 0, len(macs))
	for _, m := range macs {
		clients = append(clients, fmt.Sprintf(`"%s":{"auth":true,"assoc":true}`, strings.ToLower(m)))
	}
	return ptr(fmt.Sprintf("=hostapd hostapd.%s\n{\"freq\":2437,\"clients\":{%s}}\n", iface, strings.Join(clients, ",")))
}

func neighLine(ip, mac, state string) string {
	return fmt.Sprintf("%s dev br-lan lladdr %s %s\n", ip, strings.ToLower(mac), state)
}

// devReport is reportBody with the device sections chosen by the test.
func devReport(now time.Time, o devOpts) string {
	body := strings.SplitN(reportBody(1000, 500, true, now.Unix()), "#xc:leases", 2)[0]
	body += "#xc:leases\n" + o.leases + "#xc:arp\nIP address HW type Flags HW address Mask Device\n" + o.arp
	if o.wifi != nil {
		body += "#xc:wifi\n" + *o.wifi
	}
	if o.neigh != nil {
		body += "#xc:neigh\n" + *o.neigh
	}
	if o.nlbw != nil {
		body += "#xc:nlbw\n" + *o.nlbw
	}
	return body
}

const leasesPhone = "0 aa:bb:cc:00:01:01 192.168.1.11 phone 01:aa\n0 aa:bb:cc:00:01:02 192.168.1.12 pc 01:bb\n0 aa:bb:cc:00:01:03 192.168.1.13 tv 01:cc\n"

type apiDevice struct {
	Mac, DisplayName, Note string
	Name, Ip, Conn         *string
	WifiIface              *string
	Online, Notify         bool
	CooldownMinutes        *int
	OnlineSince            *time.Time
	LastSeen, FirstSeen    time.Time
}

type deviceList struct {
	Items            []apiDevice
	TrafficAvailable bool
}

func listDevices(t *testing.T, env *testutil.Env) deviceList {
	t.Helper()
	var l deviceList
	env.MustDo(http.MethodGet, "/router/devices", nil, &l)
	return l
}

func findDevice(t *testing.T, env *testutil.Env, mac string) apiDevice {
	t.Helper()
	for _, d := range listDevices(t, env).Items {
		if d.Mac == mac {
			return d
		}
	}
	t.Fatalf("device %s not listed", mac)
	return apiDevice{}
}

type note struct{ Kind, Title, Body string }

// deviceNotes returns the device notifications, newest first.
func deviceNotes(t *testing.T, env *testutil.Env) []note {
	t.Helper()
	var n struct{ Items []note }
	env.MustDo(http.MethodGet, "/notifications", nil, &n)
	var out []note
	for _, it := range n.Items {
		if strings.HasPrefix(it.Kind, "router.device") {
			out = append(out, it)
		}
	}
	return out
}

func patchDevice(t *testing.T, env *testutil.Env, mac, noteText string, notify bool, cooldown any) {
	t.Helper()
	env.MustDo(http.MethodPatch, "/router/devices/"+mac, map[string]any{"note": noteText, "notify": notify, "cooldownMinutes": cooldown}, nil)
}

func putOptions(t *testing.T, env *testutil.Env, deviceNotify bool) {
	t.Helper()
	env.MustDo(http.MethodPut, "/router/options", map[string]any{"deviceNotify": deviceNotify, "newDeviceNotify": true, "notifyCooldownMinutes": 15, "logDays": 7}, nil)
}

func report(t *testing.T, env *testutil.Env, tok pushToken, c *clock, o devOpts) {
	t.Helper()
	if code := post(t, env, tok.Token, devReport(c.t, o)); code != http.StatusOK {
		t.Fatalf("report: %d", code)
	}
}

func eventCount(t *testing.T, env *testutil.Env, kind string) int {
	t.Helper()
	return countRows(t, env, `SELECT COUNT(*) FROM router_events WHERE kind = ?`, kind)
}

func TestDeviceWifiOnlineOffline(t *testing.T) {
	env, _, c, tok := setupPush(t)
	phone := func(wifi *string, state string) devOpts {
		return devOpts{leases: leasesPhone, wifi: wifi, neigh: ptr(neighLine("192.168.1.11", macPhone, state))}
	}
	report(t, env, tok, c, phone(hostapd("wlan0", macPhone), "REACHABLE"))
	d := findDevice(t, env, macPhone)
	if !d.Online || d.Conn == nil || *d.Conn != "wifi" || d.WifiIface == nil || *d.WifiIface != "wlan0" || d.Ip == nil || *d.Ip != "192.168.1.11" || d.DisplayName != "phone" {
		t.Fatalf("device: %+v", d)
	}
	putOptions(t, env, true)
	patchDevice(t, env, macPhone, "", true, 0)

	// The report where the phone is gone from the Wi-Fi list is the offline,
	// even though the neighbor table still has a stale entry.
	c.t = c.t.Add(3 * time.Second)
	report(t, env, tok, c, phone(hostapd("wlan0"), "STALE"))
	if d := findDevice(t, env, macPhone); d.Online {
		t.Fatalf("still online: %+v", d)
	}
	notes := deviceNotes(t, env)
	if len(notes) != 1 || notes[0].Kind != "router.device_offline" || notes[0].Title != "phone 离线了" || !strings.Contains(notes[0].Body, "IP 192.168.1.11") || !strings.Contains(notes[0].Body, "Wi-Fi") {
		t.Fatalf("offline notification: %+v", notes)
	}

	c.t = c.t.Add(3 * time.Second)
	report(t, env, tok, c, phone(hostapd("wlan0", macPhone), "REACHABLE"))
	notes = deviceNotes(t, env)
	if len(notes) != 2 || notes[0].Kind != "router.device_online" || notes[0].Title != "phone 上线了" || !strings.Contains(notes[0].Body, "离开了") {
		t.Fatalf("online notification: %+v", notes)
	}
	if n := eventCount(t, env, "device_online") + eventCount(t, env, "device_offline"); n != 2 {
		t.Fatalf("events: %d", n)
	}
	var n struct {
		Items []struct{ Link, Source string }
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &n)
	if n.Items[0].Link != "/router?device="+macPhone || n.Items[0].Source != "router" {
		t.Fatalf("notification fields: %+v", n.Items[0])
	}
}

func TestDeviceNotifySwitches(t *testing.T) {
	env, _, c, tok := setupPush(t)
	on := devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr("")}
	off := devOpts{leases: leasesPhone, wifi: hostapd("wlan0"), neigh: ptr("")}
	flip := func(o devOpts) {
		c.t = c.t.Add(3 * time.Second)
		report(t, env, tok, c, o)
	}
	report(t, env, tok, c, on)
	// Both switches are off by default.
	flip(off)
	flip(on)
	if n := deviceNotes(t, env); len(n) != 0 {
		t.Fatalf("notified with everything off: %+v", n)
	}
	// The master switch alone is not enough.
	putOptions(t, env, true)
	flip(off)
	flip(on)
	if n := deviceNotes(t, env); len(n) != 0 {
		t.Fatalf("notified with the device switch off: %+v", n)
	}
	// The device switch alone is not enough either.
	putOptions(t, env, false)
	patchDevice(t, env, macPhone, "", true, 0)
	flip(off)
	flip(on)
	if n := deviceNotes(t, env); len(n) != 0 {
		t.Fatalf("notified with the master switch off: %+v", n)
	}
	putOptions(t, env, true)
	flip(off)
	if n := deviceNotes(t, env); len(n) != 1 {
		t.Fatalf("both on: %+v", n)
	}
	// Every flip is in the event list whether or not anyone was told.
	if n := eventCount(t, env, "device_offline"); n != 4 {
		t.Fatalf("offline events: %d", n)
	}
}

func TestDeviceCooldownAndCatchUp(t *testing.T) {
	env, m, c, tok := setupPush(t)
	on := devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr("")}
	off := devOpts{leases: leasesPhone, wifi: hostapd("wlan0"), neigh: ptr("")}
	report(t, env, tok, c, on)
	putOptions(t, env, true)
	patchDevice(t, env, macPhone, "", true, nil) // follows the default of 15 minutes
	if d := findDevice(t, env, macPhone); d.CooldownMinutes != nil {
		t.Fatalf("cooldown: %v", *d.CooldownMinutes)
	}

	start := c.t
	step := func(d time.Duration, o devOpts) {
		c.t = start.Add(d)
		report(t, env, tok, c, o)
	}
	step(10*time.Second, off) // told: offline
	step(20*time.Second, on)  // inside the window: silent
	step(30*time.Second, off)
	step(40*time.Second, on)
	if n := deviceNotes(t, env); len(n) != 1 || n[0].Title != "phone 离线了" {
		t.Fatalf("inside the window: %+v", n)
	}
	// The window is over. The phone is online, the last notification said
	// offline, so one more tells where it is and how often it changed.
	c.t = start.Add(16 * time.Minute)
	m.Poll(t.Context())
	n := deviceNotes(t, env)
	if len(n) != 2 || n[0].Kind != "router.device_online" || n[0].Title != "phone 现在上线了" || !strings.Contains(n[0].Body, "上下线 3 次") {
		t.Fatalf("catch up: %+v", n)
	}
	// Nothing more while it stays as told.
	c.t = start.Add(40 * time.Minute)
	m.Poll(t.Context())
	if got := deviceNotes(t, env); len(got) != 2 {
		t.Fatalf("repeated: %+v", got)
	}
}

func TestDeviceCooldownSameStateIsSilent(t *testing.T) {
	env, m, c, tok := setupPush(t)
	on := devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr("")}
	off := devOpts{leases: leasesPhone, wifi: hostapd("wlan0"), neigh: ptr("")}
	report(t, env, tok, c, on)
	putOptions(t, env, true)
	patchDevice(t, env, macPhone, "", true, 5)
	start := c.t
	for i, o := range []devOpts{off, on, off} { // told offline, then back and gone again
		c.t = start.Add(time.Duration(i+1) * 10 * time.Second)
		report(t, env, tok, c, o)
	}
	c.t = start.Add(6 * time.Minute)
	m.Poll(t.Context())
	if n := deviceNotes(t, env); len(n) != 1 {
		t.Fatalf("same state as told: %+v", n)
	}
}

func TestDeviceWiredNeedsFiveMinutes(t *testing.T) {
	env, _, c, tok := setupPush(t)
	pc := func(state string) devOpts {
		return devOpts{leases: leasesPhone, wifi: hostapd("wlan0"), neigh: ptr(neighLine("192.168.1.12", macPC, state))}
	}
	report(t, env, tok, c, pc("REACHABLE"))
	if d := findDevice(t, env, macPC); !d.Online || *d.Conn != "wired" {
		t.Fatalf("pc: %+v", d)
	}
	putOptions(t, env, true)
	patchDevice(t, env, macPC, "", true, 0)
	start := c.t
	for _, state := range []string{"STALE", "STALE"} {
		c.t = start.Add(4*time.Minute + 59*time.Second)
		report(t, env, tok, c, pc(state))
		if !findDevice(t, env, macPC).Online {
			t.Fatal("offline before 5 minutes")
		}
	}
	c.t = start.Add(5*time.Minute + time.Second)
	report(t, env, tok, c, pc("STALE"))
	if findDevice(t, env, macPC).Online {
		t.Fatal("still online after 5 minutes")
	}
	notes := deviceNotes(t, env)
	if len(notes) != 1 || notes[0].Title != "pc 离线了" {
		t.Fatalf("notification: %+v", notes)
	}
	// A sign of life in between starts the five minutes over.
	c.t = start.Add(6 * time.Minute)
	report(t, env, tok, c, pc("REACHABLE"))
	c.t = start.Add(10 * time.Minute)
	report(t, env, tok, c, pc("STALE"))
	if !findDevice(t, env, macPC).Online {
		t.Fatal("offline four minutes after the last sign")
	}
}

func TestDeviceWifiToWiredIsNoOffline(t *testing.T) {
	env, _, c, tok := setupPush(t)
	laptop := func(wifi *string) devOpts {
		return devOpts{leases: leasesPhone, wifi: wifi, neigh: ptr(neighLine("192.168.1.12", macPC, "REACHABLE"))}
	}
	report(t, env, tok, c, laptop(hostapd("wlan0", macPC)))
	start := c.t
	// Off the Wi-Fi list, on the cable: the neighbor table keeps answering.
	for _, s := range []time.Duration{3, 30, 65, 90} {
		c.t = start.Add(s * time.Second)
		report(t, env, tok, c, laptop(hostapd("wlan0")))
		if !findDevice(t, env, macPC).Online {
			t.Fatalf("went offline after %ds", s)
		}
	}
	if d := findDevice(t, env, macPC); *d.Conn != "wired" || d.WifiIface != nil {
		t.Fatalf("not moved to the cable: %+v", d)
	}
	if n := eventCount(t, env, "device_offline"); n != 0 {
		t.Fatalf("offline events: %d", n)
	}
	// And back to Wi-Fi.
	c.t = start.Add(100 * time.Second)
	report(t, env, tok, c, laptop(hostapd("wlan1", macPC)))
	if d := findDevice(t, env, macPC); *d.Conn != "wifi" || *d.WifiIface != "wlan1" {
		t.Fatalf("back on Wi-Fi: %+v", d)
	}
}

func TestDeviceStopsReportingIsNoOffline(t *testing.T) {
	env, m, c, tok := setupPush(t)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr(neighLine("192.168.1.12", macPC, "REACHABLE"))})
	putOptions(t, env, true)
	patchDevice(t, env, macPhone, "", true, 0)
	patchDevice(t, env, macPC, "", true, 0)
	for i := 0; i < 12; i++ {
		c.t = c.t.Add(time.Minute)
		m.Poll(t.Context())
	}
	if n := deviceNotes(t, env); len(n) != 0 {
		t.Fatalf("devices notified while the router was quiet: %+v", n)
	}
	if n := eventCount(t, env, "device_offline"); n != 0 {
		t.Fatalf("offline events: %d", n)
	}
	for _, d := range listDevices(t, env).Items {
		if !d.Online {
			t.Fatalf("%s offline", d.Mac)
		}
	}
	// The report that ends the silence decides again.
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr(neighLine("192.168.1.12", macPC, "REACHABLE"))})
	if n := deviceNotes(t, env); len(n) != 0 {
		t.Fatalf("notified on return: %+v", n)
	}
}

func TestDeviceStateSurvivesRestart(t *testing.T) {
	env, m, c, tok := setupPush(t)
	o := devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr(neighLine("192.168.1.12", macPC, "REACHABLE"))}
	report(t, env, tok, c, o)
	putOptions(t, env, true)
	patchDevice(t, env, macPhone, "手机", true, 0)
	before := findDevice(t, env, macPhone)

	router.ForgetDeviceMemory(m)
	c.t = c.t.Add(time.Minute)
	report(t, env, tok, c, o)
	after := findDevice(t, env, macPhone)
	if !after.Online || after.Note != "手机" || !after.Notify || !after.FirstSeen.Equal(before.FirstSeen) || !after.OnlineSince.Equal(*before.OnlineSince) {
		t.Fatalf("after restart: %+v, before: %+v", after, before)
	}
	if n := deviceNotes(t, env); len(n) != 0 {
		t.Fatalf("notified after restart: %+v", n)
	}
	if n := eventCount(t, env, "device_online"); n != 0 {
		t.Fatalf("online events after restart: %d", n)
	}
	// A device that left while the panel was down is offline at the next report.
	router.ForgetDeviceMemory(m)
	c.t = c.t.Add(3 * time.Second)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0"), neigh: ptr("")})
	if findDevice(t, env, macPhone).Online {
		t.Fatal("still online")
	}
	if n := deviceNotes(t, env); len(n) != 1 || n[0].Title != "手机 离线了" {
		t.Fatalf("notes: %+v", n)
	}
}

func TestDeviceOldScript(t *testing.T) {
	env, _, c, tok := setupPush(t)
	// Neither #xc:wifi nor #xc:neigh: the ARP table decides, every device is wired.
	old := func(arp string) devOpts { return devOpts{leases: leasesPhone, arp: arp} }
	report(t, env, tok, c, old("192.168.1.11 0x1 0x2 aa:bb:cc:00:01:01 * br-lan\n"))
	d := findDevice(t, env, macPhone)
	if !d.Online || d.Conn != nil {
		t.Fatalf("device: %+v", d)
	}
	// A lease is not a sign of life.
	if _, ok := tryFind(listDevices(t, env), macPC); ok {
		t.Fatal("a lease alone made a device")
	}
	c.t = c.t.Add(4 * time.Minute)
	report(t, env, tok, c, old(""))
	if !findDevice(t, env, macPhone).Online {
		t.Fatal("offline after 4 minutes")
	}
	c.t = c.t.Add(90 * time.Second)
	report(t, env, tok, c, old(""))
	if findDevice(t, env, macPhone).Online {
		t.Fatal("online after 5 minutes")
	}
}

func tryFind(l deviceList, mac string) (apiDevice, bool) {
	for _, d := range l.Items {
		if d.Mac == mac {
			return d, true
		}
	}
	return apiDevice{}, false
}

func TestDeviceWifiSectionUnreadableChangesNothing(t *testing.T) {
	env, _, c, tok := setupPush(t)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr("")})
	// The ubus call failed: the block is empty, which is not "nobody is connected".
	c.t = c.t.Add(3 * time.Second)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: ptr("=hostapd hostapd.wlan0\n"), neigh: ptr("")})
	if !findDevice(t, env, macPhone).Online {
		t.Fatal("a failed Wi-Fi read made the phone offline")
	}
}

func TestDeviceNoteForgetAndValidation(t *testing.T) {
	env, _, c, tok := setupPush(t)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr("")})

	patchDevice(t, env, macPhone, "  爸爸的手机 ", true, 30)
	d := findDevice(t, env, macPhone)
	if d.Note != "爸爸的手机" || d.DisplayName != "爸爸的手机" || *d.Name != "phone" || !d.Notify || d.CooldownMinutes == nil || *d.CooldownMinutes != 30 {
		t.Fatalf("device: %+v", d)
	}
	// Lower case and dashes find the same device.
	var got apiDevice
	env.MustDo(http.MethodPatch, "/router/devices/aa-bb-cc-00-01-01", map[string]any{"note": "", "notify": false, "cooldownMinutes": nil}, &got)
	if got.Mac != macPhone || got.DisplayName != "phone" || got.CooldownMinutes != nil {
		t.Fatalf("patched: %+v", got)
	}

	bad := map[string]map[string]any{
		"long note":     {"note": strings.Repeat("字", 41), "notify": false, "cooldownMinutes": nil},
		"new line":      {"note": "a\nb", "notify": false, "cooldownMinutes": nil},
		"bad cooldown":  {"note": "", "notify": false, "cooldownMinutes": 7},
		"missing field": {"note": ""},
	}
	for name, body := range bad {
		if code, _ := env.Do(http.MethodPatch, "/router/devices/"+macPhone, body, nil); code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := env.Do(http.MethodPatch, "/router/devices/"+macPhone, map[string]any{"note": strings.Repeat("字", 40), "notify": false, "cooldownMinutes": nil}, nil); code != http.StatusOK {
		t.Errorf("40 characters: %d", code)
	}
	if code, _ := env.Do(http.MethodPatch, "/router/devices/AA:BB:CC:00:09:09", map[string]any{"note": "", "notify": false, "cooldownMinutes": nil}, nil); code != http.StatusNotFound {
		t.Errorf("unknown device: %d", code)
	}
	if code, _ := env.Do(http.MethodPatch, "/router/devices/nonsense", map[string]any{"note": "", "notify": false, "cooldownMinutes": nil}, nil); code != http.StatusBadRequest {
		t.Errorf("bad MAC: %d", code)
	}

	// Forget: the note and the row go, and the audit log has it.
	if code, _ := env.Do(http.MethodDelete, "/router/devices/"+macPhone, nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if _, ok := tryFind(listDevices(t, env), macPhone); ok {
		t.Fatal("still listed")
	}
	if n := countRows(t, env, `SELECT COUNT(*) FROM router_devices`); n != 0 {
		t.Fatalf("rows left: %d", n)
	}
	if code, _ := env.Do(http.MethodDelete, "/router/devices/"+macPhone, nil, nil); code != http.StatusNotFound {
		t.Fatalf("delete twice: %d", code)
	}
	if n := countRows(t, env, `SELECT COUNT(*) FROM audit_log WHERE action IN ('router.device.update', 'router.device.forget')`); n < 3 {
		t.Fatalf("audit entries: %d", n)
	}
}

func TestDeviceNoteInNotificationAndClients(t *testing.T) {
	env, _, c, tok := setupPush(t)
	on := devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr(neighLine("192.168.1.12", macPC, "REACHABLE"))}
	report(t, env, tok, c, on)
	putOptions(t, env, true)
	patchDevice(t, env, macPhone, "爸爸的手机", true, 0)

	c.t = c.t.Add(3 * time.Second)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0"), neigh: ptr(neighLine("192.168.1.12", macPC, "REACHABLE"))})
	if n := deviceNotes(t, env); len(n) != 1 || n[0].Title != "爸爸的手机 离线了" {
		t.Fatalf("notification: %+v", n)
	}

	// GET /router/clients lists the devices that are online, with the note.
	c.t = c.t.Add(3 * time.Second)
	report(t, env, tok, c, on)
	var clients struct {
		Items []struct{ Mac, Name, Note, IP string }
	}
	env.MustDo(http.MethodGet, "/router/clients", nil, &clients)
	if len(clients.Items) != 2 || clients.Items[0].Mac != macPhone || clients.Items[0].Note != "爸爸的手机" || clients.Items[0].Name != "phone" || clients.Items[1].Mac != macPC || clients.Items[1].Note != "" {
		t.Fatalf("clients: %+v", clients.Items)
	}
}

func TestDeviceListOrderAndStatusCounts(t *testing.T) {
	env, _, c, tok := setupPush(t)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone),
		neigh: ptr(neighLine("192.168.1.13", macTV, "REACHABLE") + neighLine("192.168.1.12", macPC, "REACHABLE") + neighLine("192.168.1.11", macPhone, "REACHABLE"))})
	var st struct {
		ClientCount, KnownDevices, WifiClients, WiredClients int
	}
	env.MustDo(http.MethodGet, "/router/status", nil, &st)
	if st.ClientCount != 3 || st.KnownDevices != 3 || st.WifiClients != 1 || st.WiredClients != 2 {
		t.Fatalf("status: %+v", st)
	}
	// The TV goes away for good, the others stay.
	c.t = c.t.Add(10 * time.Minute)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone),
		neigh: ptr(neighLine("192.168.1.12", macPC, "REACHABLE") + neighLine("192.168.1.11", macPhone, "REACHABLE"))})
	l := listDevices(t, env)
	order := []string{}
	for _, d := range l.Items {
		order = append(order, d.Mac)
	}
	if strings.Join(order, ",") != strings.Join([]string{macPhone, macPC, macTV}, ",") || l.Items[2].Online || l.TrafficAvailable {
		t.Fatalf("order: %v", order)
	}
	env.MustDo(http.MethodGet, "/router/status", nil, &st)
	if st.ClientCount != 2 || st.KnownDevices != 3 {
		t.Fatalf("status after: %+v", st)
	}
}

func TestDeviceListNotPush(t *testing.T) {
	env, _, _, _ := setup(t) // direct mode
	if code, raw := env.Do(http.MethodGet, "/router/devices", nil, nil); code != http.StatusConflict || !strings.Contains(string(raw), "router_not_push") {
		t.Fatalf("devices while direct: %d %s", code, raw)
	}
}

func TestOptions(t *testing.T) {
	env := testutil.New(t)
	var o struct {
		DeviceNotify, NewDeviceNotify  bool
		NotifyCooldownMinutes, LogDays int
	}
	env.MustDo(http.MethodGet, "/router/options", nil, &o)
	if o.DeviceNotify || !o.NewDeviceNotify || o.NotifyCooldownMinutes != 15 || o.LogDays != 7 {
		t.Fatalf("defaults: %+v", o)
	}
	body := map[string]any{"deviceNotify": true, "newDeviceNotify": false, "notifyCooldownMinutes": 30, "logDays": 3}
	env.MustDo(http.MethodPut, "/router/options", body, &o)
	if !o.DeviceNotify || o.NewDeviceNotify || o.NotifyCooldownMinutes != 30 || o.LogDays != 3 {
		t.Fatalf("saved: %+v", o)
	}
	var again = o
	env.MustDo(http.MethodGet, "/router/options", nil, &again)
	if again != o {
		t.Fatalf("read back: %+v", again)
	}
	for name, change := range map[string]map[string]any{
		"cooldown 7":  {"notifyCooldownMinutes": 7},
		"cooldown 90": {"notifyCooldownMinutes": 90},
		"days 0":      {"logDays": 0},
		"days 16":     {"logDays": 16},
	} {
		bad := map[string]any{}
		for k, v := range body {
			bad[k] = v
		}
		for k, v := range change {
			bad[k] = v
		}
		if code, _ := env.Do(http.MethodPut, "/router/options", bad, nil); code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	env.MustDo(http.MethodGet, "/router/options", nil, &again)
	if again != o {
		t.Fatalf("a refused change was kept: %+v", again)
	}
}

func TestParseWifi(t *testing.T) {
	text := "=hostapd hostapd.wlan0\n{\n\t\"freq\": 5180,\n\t\"clients\": {\n\t\t\"aa:bb:cc:00:01:01\": {\"auth\": true, \"assoc\": true},\n\t\t\"AA:BB:CC:00:01:02\": {\"assoc\": false}\n\t}\n}\n" +
		"=iwinfo ra0\nAA:BB:CC:00:01:03  -52 dBm / -95 dBm (SNR 43)  1000 ms ago\n\tRX: 6.0 MBit/s\nNo station connected.\n"
	got, ok := router.ParseWifi(text)
	if !ok || len(got) != 2 || got["AA:BB:CC:00:01:01"] != "wlan0" || got["AA:BB:CC:00:01:03"] != "ra0" {
		t.Fatalf("parse: %v %v", got, ok)
	}
	if got, ok := router.ParseWifi(""); !ok || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, ok)
	}
	if got, ok := router.ParseWifi("=hostapd hostapd.wlan0\nnot json\n=iwinfo ra0\naa:bb:cc:00:01:03 -50 dBm\n"); ok || got["AA:BB:CC:00:01:03"] != "ra0" {
		t.Fatalf("broken block: %v %v", got, ok)
	}
}

func TestParseNeigh(t *testing.T) {
	text := "192.168.1.10 dev br-lan lladdr aa:bb:cc:dd:ee:ff REACHABLE\n" +
		"192.168.1.11 dev br-lan lladdr aa:bb:cc:dd:ee:00 STALE\n" +
		"192.168.1.12 dev br-lan  FAILED\n" +
		"fe80::1 dev br-lan lladdr aa:bb:cc:dd:ee:ff router STALE\n" +
		"2409::5 dev br-lan lladdr aa:bb:cc:dd:ee:ff DELAY\n" +
		"garbage\n"
	got := router.ParseNeigh(text)
	if len(got) != 4 {
		t.Fatalf("entries: %+v", got)
	}
	if got[0].MAC() != "AA:BB:CC:DD:EE:FF" || !got[0].Reachable() || got[1].Reachable() || got[3].State() != "DELAY" {
		t.Fatalf("entries: %+v", got)
	}
}

func TestDeviceMAC(t *testing.T) {
	for in, want := range map[string]string{
		"aa:bb:cc:dd:ee:ff": "AA:BB:CC:DD:EE:FF",
		"AA-BB-CC-DD-EE-FF": "AA:BB:CC:DD:EE:FF",
		"aabbccddeeff":      "AA:BB:CC:DD:EE:FF",
		"00:00:00:00:00:00": "",
		"ff:ff:ff:ff:ff:ff": "",
		"33:33:00:00:00:01": "", // multicast
		"aa:bb:cc:dd:ee":    "",
		"hello":             "",
	} {
		if got := router.DeviceMAC(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}
