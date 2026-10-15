package router_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/router"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// B232: a device the panel has never seen is announced once.

func newNotes(t *testing.T, env *testutil.Env) []struct{ Kind, Title, Body, Priority, Link string } {
	t.Helper()
	var n struct {
		Items []struct{ Kind, Title, Body, Priority, Link string }
	}
	if code, raw := env.Do(http.MethodGet, "/notifications", nil, &n); code != http.StatusOK {
		t.Fatalf("notifications: %d %s", code, raw)
	}
	var out []struct{ Kind, Title, Body, Priority, Link string }
	for _, it := range n.Items {
		if it.Kind == "router.device_new" {
			out = append(out, it)
		}
	}
	return out
}

func TestNewDeviceFirstReportIsSilent(t *testing.T) {
	env, _, c, tok := setupPush(t)
	home := devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr(neighLine("192.168.1.12", macPC, "REACHABLE"))}
	report(t, env, tok, c, home)
	if n := newNotes(t, env); len(n) != 0 {
		t.Fatalf("first report notified: %+v", n)
	}
	if len(listDevices(t, env).Items) != 2 {
		t.Fatal("devices not taken in")
	}
	if n := eventCount(t, env, "devices_seeded"); n != 1 {
		t.Fatalf("seeded events: %d", n)
	}
	var title string
	if err := env.App.Deps.DB.QueryRow(`SELECT title FROM router_events WHERE kind = 'devices_seeded'`).Scan(&title); err != nil || title != "第一次上报，记下了 2 台设备" {
		t.Fatalf("title %q: %v", title, err)
	}

	// The same devices again, and a new token with a new script: nothing new.
	c.t = c.t.Add(time.Minute)
	report(t, env, tok, c, home)
	tok2 := newInstallToken(t, env)
	c.t = c.t.Add(time.Minute)
	if code := post(t, env, tok2.Token, devReport(c.t, home)); code != http.StatusOK {
		t.Fatalf("report after reinstall: %d", code)
	}
	if n := newNotes(t, env); len(n) != 0 {
		t.Fatalf("notified after reinstall: %+v", n)
	}
	if n := eventCount(t, env, "devices_seeded"); n != 1 || eventCount(t, env, "device_new") != 0 {
		t.Fatalf("events: seeded %d, new %d", n, eventCount(t, env, "device_new"))
	}
}

func TestNewDeviceNotifiedOnce(t *testing.T) {
	env, _, c, tok := setupPush(t)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr("")})

	// A third device joins. Everything about it is in the text.
	guest := devOpts{leases: leasesPhone, wifi: hostapd("wlan1", macPhone, macTV), neigh: ptr("")}
	c.t = c.t.Add(time.Minute)
	report(t, env, tok, c, guest)
	n := newNotes(t, env)
	if len(n) != 1 || n[0].Title != "新设备连上了：tv" || n[0].Priority != "high" || n[0].Link != "/router?device="+macTV {
		t.Fatalf("notification: %+v", n)
	}
	for _, want := range []string{"IP 192.168.1.13", "MAC " + macTV, "Wi-Fi", "wlan1"} {
		if !strings.Contains(n[0].Body, want) {
			t.Fatalf("body misses %q: %s", want, n[0].Body)
		}
	}
	if eventCount(t, env, "device_new") != 1 {
		t.Fatal("no device_new event")
	}
	// Going away and coming back is not new.
	c.t = c.t.Add(time.Minute)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr("")})
	c.t = c.t.Add(time.Minute)
	report(t, env, tok, c, guest)
	if n := newNotes(t, env); len(n) != 1 {
		t.Fatalf("notified again: %+v", n)
	}
	// The cool-down and the device notification switch do not matter.
	putOptions(t, env, false)
	patchDevice(t, env, macTV, "", false, 60)
	if n := newNotes(t, env); len(n) != 1 {
		t.Fatalf("notes: %+v", n)
	}
}

func TestNewDeviceSwitchOff(t *testing.T) {
	env, _, c, tok := setupPush(t)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr("")})
	env.MustDo(http.MethodPut, "/router/options", map[string]any{"deviceNotify": false, "newDeviceNotify": false, "notifyCooldownMinutes": 15, "logDays": 7}, nil)
	c.t = c.t.Add(time.Minute)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone, macTV), neigh: ptr("")})
	if n := newNotes(t, env); len(n) != 0 {
		t.Fatalf("notified with the switch off: %+v", n)
	}
	// The event list has it anyway.
	if n := eventCount(t, env, "device_new"); n != 1 {
		t.Fatalf("device_new events: %d", n)
	}
}

func TestNewDeviceAfterForget(t *testing.T) {
	env, _, c, tok := setupPush(t)
	home := devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone, macTV), neigh: ptr("")}
	report(t, env, tok, c, home)
	if code, _ := env.Do(http.MethodDelete, "/router/devices/"+macTV, nil, nil); code != http.StatusNoContent {
		t.Fatalf("forget: %d", code)
	}
	c.t = c.t.Add(time.Minute)
	report(t, env, tok, c, home)
	if n := newNotes(t, env); len(n) != 1 || n[0].Title != "新设备连上了：tv" {
		t.Fatalf("after forgetting: %+v", n)
	}
}

func TestNewDeviceNotAfterSilence(t *testing.T) {
	env, m, c, tok := setupPush(t)
	home := devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr(neighLine("192.168.1.12", macPC, "REACHABLE"))}
	report(t, env, tok, c, home)
	for i := 0; i < 10; i++ {
		c.t = c.t.Add(time.Minute)
		m.Poll(t.Context())
	}
	c.t = c.t.Add(time.Minute)
	report(t, env, tok, c, home)
	if n := newNotes(t, env); len(n) != 0 {
		t.Fatalf("notified when the router came back: %+v", n)
	}
	if n := eventCount(t, env, "device_new"); n != 0 {
		t.Fatalf("device_new events: %d", n)
	}
}

func TestNewDeviceKeepsSeededAcrossRestart(t *testing.T) {
	env, m, c, tok := setupPush(t)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr("")})
	router.ForgetDeviceMemory(m)
	c.t = c.t.Add(time.Minute)
	report(t, env, tok, c, devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone, macTV), neigh: ptr("")})
	if n := newNotes(t, env); len(n) != 1 {
		t.Fatalf("after restart: %+v", n)
	}
	if eventCount(t, env, "devices_seeded") != 1 {
		t.Fatal("seeded twice")
	}
}
