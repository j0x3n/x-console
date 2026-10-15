package router_test

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/router"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// B229: speed and traffic of each device, from nlbwmon.

// nlbwJSON is what `nlbw -c json -g mac` prints. Each device is MAC, rx, tx.
func nlbwJSON(rows ...[3]any) *string {
	var data []string
	for _, r := range rows {
		data = append(data, fmt.Sprintf(`["%v",3,%v,10,%v,10]`, strings.ToLower(r[0].(string)), r[1], r[2]))
	}
	return ptr(fmt.Sprintf(`{"columns":["mac","conns","rx_bytes","rx_pkts","tx_bytes","tx_pkts"],"data":[%s]}`, strings.Join(data, ",")))
}

type trafficDevice struct {
	apiDevice
	RxRate, TxRate *float64
	TodayRx        *int64
	TodayTx        *int64
}

func trafficList(t *testing.T, env *testutil.Env) (map[string]trafficDevice, bool) {
	t.Helper()
	var l struct {
		Items            []trafficDevice
		TrafficAvailable bool
	}
	env.MustDo(http.MethodGet, "/router/devices", nil, &l)
	out := map[string]trafficDevice{}
	for _, d := range l.Items {
		out[d.Mac] = d
	}
	return out, l.TrafficAvailable
}

func pcOnly(nlbw *string) devOpts {
	return devOpts{leases: leasesPhone, wifi: hostapd("wlan0"), neigh: ptr(neighLine("192.168.1.12", macPC, "REACHABLE")), nlbw: nlbw}
}

func TestDeviceSpeedAverageOverTenSeconds(t *testing.T) {
	env, _, c, tok := setupPush(t)
	rx, tx := 1000, 500
	send := func() {
		report(t, env, tok, c, pcOnly(nlbwJSON([3]any{macPC, rx, tx})))
	}
	send()
	list, avail := trafficList(t, env)
	if !avail || list[macPC].RxRate != nil || list[macPC].TodayRx == nil || *list[macPC].TodayRx != 0 {
		t.Fatalf("first report: %v %+v", avail, list[macPC])
	}
	// nlbwmon refreshes every 5 seconds, reports come every 3: the counter
	// moves in steps. The average over about 10 seconds smooths it out.
	steps := []struct{ at, drx int }{{3, 0}, {6, 6000}, {9, 0}, {12, 6000}}
	for _, s := range steps {
		c.t = c.t.Add(3 * time.Second)
		rx += s.drx
		tx += s.drx / 10
		send()
	}
	list, _ = trafficList(t, env)
	d := list[macPC]
	if d.RxRate == nil || math.Abs(*d.RxRate-1000) > 1 || math.Abs(*d.TxRate-100) > 1 {
		t.Fatalf("rate: %v %v", d.RxRate, d.TxRate)
	}
	if *d.TodayRx != 12000 || *d.TodayTx != 1200 {
		t.Fatalf("today: %d %d", *d.TodayRx, *d.TodayTx)
	}
}

func TestDeviceCounterResetCountsAsNew(t *testing.T) {
	env, _, c, tok := setupPush(t)
	report(t, env, tok, c, pcOnly(nlbwJSON([3]any{macPC, 5000, 2000})))
	c.t = c.t.Add(10 * time.Second)
	report(t, env, tok, c, pcOnly(nlbwJSON([3]any{macPC, 7000, 2500})))
	// nlbwmon restarted: the counters are small again, and that is new traffic.
	c.t = c.t.Add(10 * time.Second)
	report(t, env, tok, c, pcOnly(nlbwJSON([3]any{macPC, 300, 100})))
	list, _ := trafficList(t, env)
	if d := list[macPC]; *d.TodayRx != 2300 || *d.TodayTx != 600 {
		t.Fatalf("today: %d %d", *d.TodayRx, *d.TodayTx)
	}
}

func TestDeviceTrafficToday(t *testing.T) {
	env, _, c, tok := setupPush(t)
	// 12:00 UTC is 20:00 in Shanghai. Five hours later it is the next day.
	send := func(rx int) {
		report(t, env, tok, c, pcOnly(nlbwJSON([3]any{macPC, rx, rx / 10})))
	}
	send(1000)
	c.t = c.t.Add(3 * time.Second)
	send(4000)
	list, _ := trafficList(t, env)
	if *list[macPC].TodayRx != 3000 {
		t.Fatalf("day one: %d", *list[macPC].TodayRx)
	}
	c.t = c.t.Add(5 * time.Hour)
	send(9000) // a long gap: no delta, the counters are only read
	c.t = c.t.Add(3 * time.Second)
	send(15000)
	list, _ = trafficList(t, env)
	if *list[macPC].TodayRx != 6000 || *list[macPC].TodayTx != 600 {
		t.Fatalf("day two: %d %d", *list[macPC].TodayRx, *list[macPC].TodayTx)
	}

	// Both days are in the hourly series; the sums are written to the database.
	var tr struct {
		Range       string
		StepSeconds int
		RxBytes     int64
		Points      []struct {
			At     time.Time
			RxRate float64
		}
	}
	env.MustDo(http.MethodGet, "/router/devices/"+macPC+"/traffic?range=24h", nil, &tr)
	if tr.Range != "24h" || tr.StepSeconds != 3600 || tr.RxBytes != 9000 || len(tr.Points) != 2 {
		t.Fatalf("24h: %+v", tr)
	}
	// 6000 bytes in the 6 seconds of the current hour that have passed.
	if last := tr.Points[1]; math.Abs(last.RxRate-1000) > 1 {
		t.Fatalf("current hour rate: %v", last.RxRate)
	}
	env.MustDo(http.MethodGet, "/router/devices/"+macPC+"/traffic?range=7d", nil, &tr)
	if tr.Range != "7d" || len(tr.Points) != 2 {
		t.Fatalf("7d: %+v", tr)
	}
	c.t = c.t.Add(30 * time.Hour)
	env.MustDo(http.MethodGet, "/router/devices/"+macPC+"/traffic?range=24h", nil, &tr)
	if len(tr.Points) != 0 || tr.RxBytes != 0 {
		t.Fatalf("old data in 24h: %+v", tr)
	}
	if code, _ := env.Do(http.MethodGet, "/router/devices/AA:BB:CC:00:09:09/traffic", nil, nil); code != http.StatusNotFound {
		t.Fatalf("unknown device: %d", code)
	}
}

func TestDeviceTrafficWrittenInBatches(t *testing.T) {
	env, _, c, tok := setupPush(t)
	rx := 0
	rows := func() int { return countRows(t, env, `SELECT COALESCE(SUM(rx), 0) FROM router_device_traffic`) }
	for i := 0; i < 5; i++ {
		rx += 3000
		report(t, env, tok, c, pcOnly(nlbwJSON([3]any{macPC, rx, 0})))
		c.t = c.t.Add(3 * time.Second)
	}
	// 12 seconds in: 12000 bytes counted, nothing written yet, and the page still has them.
	if n := rows(); n != 0 {
		t.Fatalf("written too early: %d", n)
	}
	list, _ := trafficList(t, env)
	if *list[macPC].TodayRx != 12000 {
		t.Fatalf("today: %d", *list[macPC].TodayRx)
	}
	c.t = c.t.Add(30 * time.Second)
	rx += 3000
	report(t, env, tok, c, pcOnly(nlbwJSON([3]any{macPC, rx, 0})))
	if n := rows(); n != 15000 {
		t.Fatalf("written: %d", n)
	}
}

func TestDeviceNoNlbw(t *testing.T) {
	env, _, c, tok := setupPush(t)
	report(t, env, tok, c, pcOnly(nil))
	c.t = c.t.Add(3 * time.Second)
	report(t, env, tok, c, pcOnly(nil))
	list, avail := trafficList(t, env)
	d := list[macPC]
	if avail || d.RxRate != nil || d.TodayRx != nil || !d.Online {
		t.Fatalf("without nlbw: %v %+v", avail, d)
	}
	var tr map[string]any
	env.MustDo(http.MethodGet, "/router/traffic", nil, &tr)
	for _, k := range []string{"devicesTodayRx", "wanTodayRx"} {
		if _, ok := tr[k]; ok {
			t.Fatalf("%s without nlbw", k)
		}
	}
	// nlbwmon stopped answering: the section is empty.
	body := devReport(c.t, pcOnly(ptr("")))
	if code := post(t, env, tok.Token, body); code != http.StatusOK {
		t.Fatalf("empty nlbw: %d", code)
	}
	if _, avail := trafficList(t, env); avail {
		t.Fatal("traffic available with an empty nlbw section")
	}
}

func TestDeviceIdleHasZeroSpeed(t *testing.T) {
	env, _, c, tok := setupPush(t)
	// nlbwmon has no row for a device that has moved nothing.
	report(t, env, tok, c, pcOnly(nlbwJSON([3]any{macPhone, 1, 1})))
	c.t = c.t.Add(3 * time.Second)
	report(t, env, tok, c, pcOnly(nlbwJSON([3]any{macPhone, 1, 1})))
	list, _ := trafficList(t, env)
	if d := list[macPC]; d.RxRate == nil || *d.RxRate != 0 || *d.TodayRx != 0 {
		t.Fatalf("idle device: %+v", d)
	}
}

func TestDeviceTrafficTotalsNextToWAN(t *testing.T) {
	env, _, c, tok := setupPush(t)
	rep := func(wanRx int, pcRx int) {
		body := devReport(c.t, pcOnly(nlbwJSON([3]any{macPC, pcRx, 0})))
		body = strings.Replace(body, `"rx_bytes":1000,"tx_bytes":500`, fmt.Sprintf(`"rx_bytes":%d,"tx_bytes":500`, wanRx), 1)
		if code := post(t, env, tok.Token, body); code != http.StatusOK {
			t.Fatalf("report: %d", code)
		}
	}
	rep(1000, 100)
	c.t = c.t.Add(time.Minute)
	rep(601000, 20100)
	var tr struct {
		DevicesTodayRx, DevicesTodayTx, WanTodayRx, WanTodayTx *int64
	}
	env.MustDo(http.MethodGet, "/router/traffic", nil, &tr)
	if tr.DevicesTodayRx == nil || *tr.DevicesTodayRx != 20000 || *tr.DevicesTodayTx != 0 || *tr.WanTodayRx != 600000 || *tr.WanTodayTx != 0 {
		t.Fatalf("totals: %+v", tr)
	}
}

func TestDeviceTrafficKeepsWiredOnline(t *testing.T) {
	env, _, c, tok := setupPush(t)
	rx := 1000
	stale := func(grow bool) devOpts {
		if grow {
			rx += 5000
		}
		return devOpts{leases: leasesPhone, wifi: hostapd("wlan0"), neigh: ptr(neighLine("192.168.1.12", macPC, "REACHABLE")), nlbw: nlbwJSON([3]any{macPC, rx, 0})}
	}
	report(t, env, tok, c, stale(true))
	// The neighbor table lost it (STALE), but its counter keeps going.
	quiet := func(grow bool) {
		o := stale(grow)
		o.neigh = ptr(neighLine("192.168.1.12", macPC, "STALE"))
		report(t, env, tok, c, o)
	}
	for i := 0; i < 8; i++ {
		c.t = c.t.Add(time.Minute)
		quiet(true)
		if !findDevice(t, env, macPC).Online {
			t.Fatalf("offline at minute %d with traffic", i+1)
		}
	}
	// Traffic stops: five minutes from the last sign.
	for i := 0; i < 4; i++ {
		c.t = c.t.Add(time.Minute)
		quiet(false)
	}
	if !findDevice(t, env, macPC).Online {
		t.Fatal("offline after 4 quiet minutes")
	}
	c.t = c.t.Add(90 * time.Second)
	quiet(false)
	if findDevice(t, env, macPC).Online {
		t.Fatal("online after 5 quiet minutes")
	}
}

func TestDeviceTrafficDoesNotKeepWifiOnline(t *testing.T) {
	env, _, c, tok := setupPush(t)
	rx := 1000
	on := devOpts{leases: leasesPhone, wifi: hostapd("wlan0", macPhone), neigh: ptr(neighLine("192.168.1.11", macPhone, "REACHABLE")), nlbw: nlbwJSON([3]any{macPhone, rx, 0})}
	report(t, env, tok, c, on)
	// The phone leaves. Packets from the internet still pass the router for a while.
	c.t = c.t.Add(3 * time.Second)
	gone := devOpts{leases: leasesPhone, wifi: hostapd("wlan0"), neigh: ptr(neighLine("192.168.1.11", macPhone, "STALE")), nlbw: nlbwJSON([3]any{macPhone, rx + 9000, 0})}
	report(t, env, tok, c, gone)
	if findDevice(t, env, macPhone).Online {
		t.Fatal("a Wi-Fi device that left stayed online because its counter moved")
	}
}

func TestDeviceForgetDropsTraffic(t *testing.T) {
	env, _, c, tok := setupPush(t)
	report(t, env, tok, c, pcOnly(nlbwJSON([3]any{macPC, 1000, 0})))
	c.t = c.t.Add(3 * time.Second)
	report(t, env, tok, c, pcOnly(nlbwJSON([3]any{macPC, 4000, 0})))
	c.t = c.t.Add(40 * time.Second)
	report(t, env, tok, c, pcOnly(nlbwJSON([3]any{macPC, 4500, 0})))
	if n := countRows(t, env, `SELECT COUNT(*) FROM router_device_traffic`); n != 1 {
		t.Fatalf("rows: %d", n)
	}
	env.MustDo(http.MethodDelete, "/router/devices/"+macPC, nil, nil)
	if n := countRows(t, env, `SELECT COUNT(*) FROM router_device_traffic`); n != 0 {
		t.Fatalf("traffic left: %d", n)
	}
}

func TestParseNlbw(t *testing.T) {
	// Columns are found by name, so another order or extra columns do not matter.
	text := `{"columns":["family","mac","tx_pkts","rx_bytes","conns","tx_bytes","rx_pkts"],"data":[
		[4,"AA:BB:CC:00:01:02",7,123456789012,1,300,5],
		[4,"aa:bb:cc:00:01:03",7,2.5e3,1,0,5],
		[4,"00:00:00:00:00:00",1,1,1,1,1],
		[4,"aa:bb:cc:00:01:02",1,1,1,1,1]]}`
	got, ok := router.ParseNlbw(text)
	if !ok || len(got) != 2 {
		t.Fatalf("parse: %v %v", got, ok)
	}
	a, b := got["AA:BB:CC:00:01:02"], got["AA:BB:CC:00:01:03"]
	if a.RX() != 123456789013 || a.TX() != 301 || b.RX() != 2500 || b.TX() != 0 {
		t.Fatalf("rows: %+v %+v", a, b)
	}
	for name, bad := range map[string]string{
		"empty":       "",
		"not json":    "Unable to connect to nlbwmon",
		"no mac":      `{"columns":["rx_bytes","tx_bytes"],"data":[]}`,
		"no columns":  `{"data":[]}`,
		"no tx bytes": `{"columns":["mac","rx_bytes"],"data":[]}`,
	} {
		if _, ok := router.ParseNlbw(bad); ok {
			t.Errorf("%s accepted", name)
		}
	}
	if got, ok := router.ParseNlbw(`{"columns":["mac","rx_bytes","tx_bytes"],"data":[]}`); !ok || len(got) != 0 {
		t.Errorf("no devices yet: %v %v", got, ok)
	}
}

// The install script turns nlbwmon's refresh down to 5 seconds and keeps the old value once.
func TestNlbwSetupScript(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("nlbw", "#!/bin/sh\n")
	write("uci", `#!/bin/sh
[ "$1" = "-q" ] && shift
case "$1" in
get) case "$2" in *refresh_interval) cat "$XC_INTERVAL" ;; esac ;;
set) echo "${2#*=}" > "$XC_INTERVAL" ;;
esac
`)
	write("restart", "#!/bin/sh\necho x >> \""+dir+"/restarts\"\n")
	for _, name := range []string{"cat", "echo"} {
		if p, err := exec.LookPath(name); err == nil {
			_ = os.Symlink(p, filepath.Join(bin, name))
		}
	}
	script := strings.NewReplacer("/etc/xc-report.nlbw.bak", filepath.Join(dir, "bak"), "/etc/init.d/nlbwmon restart", filepath.Join(bin, "restart")).Replace(router.NlbwSetup)
	interval := filepath.Join(dir, "interval")
	run := func(value string) (string, string) {
		if err := os.WriteFile(interval, []byte(value+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("/bin/sh", "-c", script)
		cmd.Env = []string{"PATH=" + bin, "XC_INTERVAL=" + interval}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup with %q: %v\n%s", value, err, out)
		}
		got, _ := os.ReadFile(interval)
		bak, _ := os.ReadFile(filepath.Join(dir, "bak"))
		return strings.TrimSpace(string(got)), strings.TrimSpace(string(bak))
	}
	restarts := func() int {
		b, _ := os.ReadFile(filepath.Join(dir, "restarts"))
		return strings.Count(string(b), "x")
	}
	if got, bak := run("30s"); got != "5s" || bak != "30s" || restarts() != 1 {
		t.Fatalf("30s: %q %q %d", got, bak, restarts())
	}
	// Already fast: nothing to do. Another slow value does not replace the first backup.
	if got, _ := run("5s"); got != "5s" || restarts() != 1 {
		t.Fatalf("5s: %q %d", got, restarts())
	}
	if got, bak := run("1m"); got != "5s" || bak != "30s" || restarts() != 2 {
		t.Fatalf("1m: %q %q %d", got, bak, restarts())
	}
	if got, _ := run("3s"); got != "3s" || restarts() != 2 {
		t.Fatalf("3s: %q %d", got, restarts())
	}
	// A value in a format nobody expects is left alone.
	if got, _ := run("often"); got != "often" || restarts() != 2 {
		t.Fatalf("often: %q %d", got, restarts())
	}
}
