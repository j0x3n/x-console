package router_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The report script runs here with fake ubus, curl, ip and iwinfo commands.
// What it sends is posted to the panel, so the two halves are tested together.

const fakeUbusScript = `#!/bin/sh
case "$1 $2" in
"list hostapd.*") [ -n "$FAKE_HOSTAPD" ] && printf 'hostapd.wlan0\nhostapd.wlan1\n' ;;
"call hostapd.wlan0") echo '{"freq":2437,"clients":{"aa:bb:cc:00:01:01":{"assoc":true,"auth":true}}}' ;;
"call hostapd.wlan1") echo '{"freq":5180,"clients":{}}' ;;
"call system") case "$3" in
	board) echo '{"hostname":"OpenWrt","model":"Fake","release":{"description":"OpenWrt 24.10.0"}}' ;;
	info) echo '{"uptime":100,"load":[0,0,0],"memory":{"total":1000,"available":600}}' ;;
	esac ;;
"call network.interface") echo '{"interface":[{"interface":"lan","up":true,"device":"br-lan","l3_device":"br-lan"},{"interface":"wan","up":true,"device":"eth1","l3_device":"eth1","ipv4-address":[{"address":"100.64.1.2","mask":32}]}]}' ;;
"call network.device") echo '{"eth1":{"statistics":{"rx_bytes":1000,"tx_bytes":500}}}' ;;
esac
`

// fakeCurl keeps the body it is asked to post and answers like the panel.
const fakeCurl = `#!/bin/sh
while [ $# -gt 0 ]; do
	case "$1" in
	--data-binary) cp "${2#@}" "$XC_FAKE_OUT/body" ;;
	-o) out=$2 ;;
	esac
	shift
done
[ -n "$FAKE_FAIL" ] && exit 22
echo 'interval=60' > "$out"
`

const fakeIP = `#!/bin/sh
echo '192.168.1.11 dev br-lan lladdr aa:bb:cc:00:01:01 REACHABLE'
echo '192.168.1.12 dev br-lan lladdr aa:bb:cc:00:01:02 STALE'
`

const fakeIwinfo = `#!/bin/sh
echo 'AA:BB:CC:00:01:01  -52 dBm / -95 dBm (SNR 43)  1000 ms ago'
`

// runScript runs report.sh once and returns the body it posted.
type scriptEnv struct {
	t    *testing.T
	root string
	bin  string
	out  string
}

func newScriptEnv(t *testing.T, token string) *scriptEnv {
	t.Helper()
	root := t.TempDir()
	e := &scriptEnv{t: t, root: root, bin: filepath.Join(root, "bin"), out: filepath.Join(root, "out")}
	for _, d := range []string{e.bin, e.out, filepath.Join(root, "net")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	src, err := os.ReadFile("report.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := strings.NewReplacer("@URL@", "http://panel.invalid/report", "@TOKEN@", token).Replace(string(src))
	if err := os.WriteFile(filepath.Join(root, "report.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"date", "cat", "rm", "sleep", "wc", "tail", "sed", "grep", "cut", "mv", "head", "cp"} {
		if p, err := exec.LookPath(name); err == nil {
			_ = os.Symlink(p, filepath.Join(e.bin, name))
		}
	}
	e.fake("ubus", fakeUbusScript)
	e.fake("curl", fakeCurl)
	e.fake("logger", "#!/bin/sh\n")
	return e
}

func (e *scriptEnv) fake(name, body string) {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(e.bin, name), []byte(body), 0o755); err != nil {
		e.t.Fatal(err)
	}
}

func (e *scriptEnv) run(extraEnv ...string) string {
	e.t.Helper()
	body, out, err := e.try(extraEnv...)
	if err != nil {
		e.t.Fatalf("script: %v\n%s", err, out)
	}
	return body
}

// try runs the script and returns the body it posted, its output and its error.
func (e *scriptEnv) try(extraEnv ...string) (string, []byte, error) {
	e.t.Helper()
	_ = os.Remove(filepath.Join(e.out, "body"))
	cmd := exec.Command("/bin/sh", filepath.Join(e.root, "report.sh"))
	cmd.Env = append([]string{"PATH=" + e.bin, "XC_REPORT_DIR=" + e.root, "XC_FAKE_OUT=" + e.out, "XC_SYS_NET=" + filepath.Join(e.root, "net")}, extraEnv...)
	out, err := cmd.CombinedOutput()
	body, _ := os.ReadFile(filepath.Join(e.out, "body"))
	return string(body), out, err
}

func TestScriptWifiAndNeighSections(t *testing.T) {
	env, _, _, tok := setupPush(t)
	e := newScriptEnv(t, tok.Token)
	e.fake("ip", fakeIP)
	e.fake("nlbw", "#!/bin/sh\n[ \"$*\" = \"-c json -g mac\" ] && echo '{\"columns\":[\"mac\",\"rx_bytes\",\"tx_bytes\"],\"data\":[[\"aa:bb:cc:00:01:01\",4000,300]]}'\n")
	body := e.run("FAKE_HOSTAPD=1")
	for _, want := range []string{"#xc:wifi\n=hostapd hostapd.wlan0\n", "=hostapd hostapd.wlan1\n", "#xc:neigh\n192.168.1.11 dev br-lan lladdr aa:bb:cc:00:01:01 REACHABLE", "#xc:nlbw\n{\"columns\""} {
		if !strings.Contains(body, want) {
			t.Fatalf("body misses %q:\n%s", want, body)
		}
	}
	// The panel takes what the script sends.
	if code := post(t, env, tok.Token, body); code != 200 {
		t.Fatalf("panel: %d", code)
	}
	d := findDevice(t, env, macPhone)
	if !d.Online || *d.Conn != "wifi" || *d.WifiIface != "wlan0" {
		t.Fatalf("phone: %+v", d)
	}
	if _, avail := trafficList(t, env); !avail {
		t.Fatal("the nlbw section did not arrive")
	}
	// A STALE neighbor is not a device that is there.
	if _, ok := tryFind(listDevices(t, env), macPC); ok {
		t.Fatal("a stale neighbor became a device")
	}
}

func TestScriptWithoutHostapdUsesIwinfo(t *testing.T) {
	e := newScriptEnv(t, "tok")
	e.fake("iwinfo", fakeIwinfo)
	// The phy80211 directory marks a wireless interface.
	if err := os.MkdirAll(filepath.Join(e.root, "net", "wlan0", "phy80211"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(e.root, "net", "eth0"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := e.run()
	if !strings.Contains(body, "#xc:wifi\n=iwinfo wlan0\nAA:BB:CC:00:01:01") || strings.Contains(body, "=iwinfo eth0") {
		t.Fatalf("body:\n%s", body)
	}
	// No ip command: the neighbor section is left out and the ARP table stays.
	if strings.Contains(body, "#xc:neigh") || strings.Contains(body, "#xc:nlbw") || !strings.Contains(body, "#xc:arp") {
		t.Fatalf("body:\n%s", body)
	}
}

// logread prints the file FAKE_LOG.
const fakeLogread = "#!/bin/sh\ncat \"$FAKE_LOG\"\n"

func logLines(from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		fmt.Fprintf(&b, "Thu Oct  9 20:00:%02d 2026 daemon.info dnsmasq[%d]: line %d\n", i%60, i, i)
	}
	return b.String()
}

// sentLines are the lines under #xc:log in a body.
func sentLines(body string) []string {
	_, rest, ok := strings.Cut(body, "#xc:log\n")
	if !ok {
		return nil
	}
	rest, _, _ = strings.Cut(rest, "\n#xc:")
	return strings.Split(strings.TrimRight(rest, "\n"), "\n")
}

func TestScriptSendsOnlyNewLogLines(t *testing.T) {
	e := newScriptEnv(t, "tok")
	e.fake("logread", fakeLogread)
	logFile := filepath.Join(e.root, "log")
	write := func(text string) {
		if err := os.WriteFile(logFile, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(extra ...string) []string {
		return sentLines(e.run(append([]string{"FAKE_LOG=" + logFile}, extra...)...))
	}

	// The first time, no position is known: everything goes.
	write(logLines(1, 5))
	if got := run(); len(got) != 5 || !strings.HasSuffix(got[0], "line 1") || !strings.HasSuffix(got[4], "line 5") {
		t.Fatalf("first: %v", got)
	}
	pos, _ := os.ReadFile(filepath.Join(e.root, "xc-report.logpos"))
	if !strings.HasSuffix(strings.TrimSpace(string(pos)), "line 5") {
		t.Fatalf("position: %q", pos)
	}
	// Two more lines: only those.
	write(logLines(1, 7))
	if got := run(); len(got) != 2 || !strings.HasSuffix(got[0], "line 6") {
		t.Fatalf("second: %v", got)
	}
	// Nothing new: no section at all.
	if got := run(); got != nil {
		t.Fatalf("third: %v", got)
	}
	// The router restarted: its log began again and the last line is not in it.
	write(logLines(20, 22))
	if got := run(); len(got) != 3 || !strings.HasSuffix(got[0], "line 20") {
		t.Fatalf("after restart: %v", got)
	}
	// The panel did not take the report: the same lines go again.
	write(logLines(20, 24))
	if _, _, err := e.try("FAKE_LOG="+logFile, "FAKE_FAIL=1"); err == nil {
		t.Fatal("the script ignored a failed post")
	}
	pos, _ = os.ReadFile(filepath.Join(e.root, "xc-report.logpos"))
	if !strings.HasSuffix(strings.TrimSpace(string(pos)), "line 22") {
		t.Fatalf("position moved on a failed report: %q", pos)
	}
	if got := run(); len(got) != 2 || !strings.HasSuffix(got[0], "line 23") {
		t.Fatalf("after a failed report: %v", got)
	}
}

func TestScriptLogLimit(t *testing.T) {
	e := newScriptEnv(t, "tok")
	e.fake("logread", fakeLogread)
	logFile := filepath.Join(e.root, "log")
	if err := os.WriteFile(logFile, []byte(logLines(1, 1200)), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func() []string { return sentLines(e.run("FAKE_LOG=" + logFile)) }
	// 500 at a time; the position is the 500th line, so the next run goes on from there.
	first, second, third := run(), run(), run()
	if len(first) != 500 || !strings.HasSuffix(first[0], "line 1") || !strings.HasSuffix(first[499], "line 500") {
		t.Fatalf("first: %d lines", len(first))
	}
	if len(second) != 500 || !strings.HasSuffix(second[0], "line 501") || !strings.HasSuffix(second[499], "line 1000") {
		t.Fatalf("second: %d lines", len(second))
	}
	if len(third) != 200 || !strings.HasSuffix(third[0], "line 1001") {
		t.Fatalf("third: %d lines", len(third))
	}
}

func TestScriptSendsTimeZone(t *testing.T) {
	e := newScriptEnv(t, "tok")
	body := e.run("TZ=Asia/Shanghai")
	if !strings.Contains(body, "#xc:tz\n") {
		t.Fatalf("no time zone:\n%s", body)
	}
}
