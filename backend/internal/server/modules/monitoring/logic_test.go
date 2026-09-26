package monitoring

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"
)

func TestNetError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	_, err = http.Get("http://" + addr + "/")
	if got := netError(err); got != "连接被拒绝" {
		t.Fatalf("refused: %q", got)
	}
	if got := netError(&url.Error{Op: "Get", URL: "x", Err: &net.DNSError{Name: "nope.invalid"}}); got != "域名解析失败：nope.invalid" {
		t.Fatalf("dns: %q", got)
	}
	if got := netError(&url.Error{Op: "Get", URL: "x", Err: errors.New("boom")}); got != "boom" {
		t.Fatalf("url error: %q", got)
	}
	if got := netError(context.DeadlineExceeded); got != "超时" {
		t.Fatalf("timeout: %q", got)
	}
}

func TestNextState(t *testing.T) {
	s := monitorState{Status: statusUnknown}
	var changes []transition
	for _, ok := range []bool{true, false, false, false, true, true, false, true} {
		var c transition
		s, c = nextState(s, ok)
		changes = append(changes, c)
	}
	want := []transition{noChange, noChange, wentDown, noChange, cameUp, noChange, noChange, noChange}
	if !slices.Equal(changes, want) {
		t.Fatalf("changes %v, want %v", changes, want)
	}
	if s.Status != statusUp || s.Failures != 0 {
		t.Fatalf("final %+v", s)
	}
	// A new monitor that fails twice goes down without ever being up.
	s, _ = nextState(monitorState{Status: statusUnknown}, false)
	if s.Status != statusUnknown {
		t.Fatalf("one failure: %+v", s)
	}
	if s, c := nextState(s, false); s.Status != statusDown || c != wentDown {
		t.Fatalf("two failures: %+v %v", s, c)
	}
}

func TestExpiryDue(t *testing.T) {
	var notified []int
	var sent []int
	for _, left := range []float64{20, 13.5, 13, 6.9, 6, 2.5, 1, -1} {
		th, due, marked := expiryDue(left, tlsThresholds, notified)
		notified = marked
		if due {
			sent = append(sent, th)
		}
	}
	if !slices.Equal(sent, []int{14, 7, 3}) {
		t.Fatalf("sent %v", sent)
	}
	// First seen with 5 days left: one reminder, and the 14 day one never follows.
	th, due, marked := expiryDue(5, tlsThresholds, nil)
	if !due || th != 7 || !slices.Equal(marked, []int{7, 14}) {
		t.Fatalf("5 days: %d %v %v", th, due, marked)
	}
	if _, due, _ := expiryDue(4, tlsThresholds, marked); due {
		t.Fatal("repeated")
	}
}

func TestSameExpiry(t *testing.T) {
	a := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if sameExpiry(nil, a) || !sameExpiry(&a, a.Add(2*time.Hour)) || sameExpiry(&a, a.Add(48*time.Hour)) {
		t.Fatal("sameExpiry")
	}
}

func TestIsDue(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	last := now.Add(-56 * time.Second)
	if isDue(&last, time.Minute, now) != true || isDue(nil, time.Minute, now) != true {
		t.Fatal("should be due")
	}
	last = now.Add(-30 * time.Second)
	if isDue(&last, time.Minute, now) {
		t.Fatal("not due yet")
	}
}

func TestDownsample(t *testing.T) {
	since := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	var pts []resultPoint
	for i := range 1440 { // one per minute for a day
		code := int64(200)
		pts = append(pts, resultPoint{At: since.Add(time.Duration(i) * time.Minute), OK: i != 100, StatusCode: &code, LatencyMs: int64(i % 10)})
	}
	out, step := downsample(pts, since, 24*time.Hour, 500)
	if step != 173 || len(out) > 500 || len(out) < 400 {
		t.Fatalf("step %d, %d points", step, len(out))
	}
	bad := 0
	for _, p := range out {
		if !p.OK {
			bad++
		}
	}
	if bad != 1 {
		t.Fatalf("%d failed buckets", bad)
	}
	same, step := downsample(pts[:10], since, time.Hour, 500)
	if step != 0 || len(same) != 10 {
		t.Fatal("small input changed")
	}
	up, avg := uptime(pts[:10])
	if up != 100 || avg != 4.5 {
		t.Fatalf("uptime %v %v", up, avg)
	}
}

func TestScriptCommand(t *testing.T) {
	if got := scriptCommand(shellBash, "echo 'hi'"); got != `bash -c 'echo '\''hi'\'''` {
		t.Fatalf("bash: %s", got)
	}
	if got := scriptCommand(shellSh, "uptime"); got != "uptime" {
		t.Fatalf("sh: %s", got)
	}
	if got := scriptCommand(shellPowerShell, "Get-Date"); got != "Get-Date" {
		t.Fatalf("powershell: %s", got)
	}
}

func date(s string) time.Time {
	t, err := parseDate(s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestAdvance(t *testing.T) {
	cases := []struct {
		from, cycle string
		days        int
		want        string
	}{
		{"2026-01-31", cycleMonthly, 0, "2026-02-28"},
		{"2028-01-31", cycleMonthly, 0, "2028-02-29"},
		{"2026-12-15", cycleMonthly, 0, "2027-01-15"},
		{"2028-02-29", cycleYearly, 0, "2029-02-28"},
		{"2026-10-01", cycleCustom, 90, "2026-12-30"},
	}
	for _, c := range cases {
		if got := advance(date(c.from), c.cycle, c.days).Format(dateLayout); got != c.want {
			t.Errorf("%s %s: got %s, want %s", c.from, c.cycle, got, c.want)
		}
	}
	if got := renewUntil(date("2026-01-10"), date("2026-04-02"), cycleMonthly, 0).Format(dateLayout); got != "2026-04-10" {
		t.Fatalf("renewUntil: %s", got)
	}
	if got := renewUntil(date("2026-05-10"), date("2026-04-02"), cycleMonthly, 0).Format(dateLayout); got != "2026-05-10" {
		t.Fatalf("renewUntil future: %s", got)
	}
}

func TestToday(t *testing.T) {
	sh, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 9, 30, 20, 0, 0, 0, time.UTC) // Oct 1, 04:00 in Shanghai
	if got := today(now, sh).Format(dateLayout); got != "2026-10-01" {
		t.Fatal(got)
	}
	if daysBetween(date("2026-10-01"), date("2026-10-08")) != 7 {
		t.Fatal("daysBetween")
	}
}

func TestReminderDue(t *testing.T) {
	remind := []int{7, 1}
	var reminded []int
	sent := 0
	for _, left := range []int{10, 8, 7, 7, 6, 2, 1, 1, 0, -1} {
		due, marked := reminderDue(left, remind, reminded)
		reminded = marked
		if due {
			sent++
		}
	}
	if sent != 2 || !slices.Equal(reminded, []int{1, 7}) {
		t.Fatalf("sent %d, reminded %v", sent, reminded)
	}
	if due, _ := reminderDue(-2, remind, nil); due {
		t.Fatal("overdue should not remind")
	}
}

func TestMonthlyCost(t *testing.T) {
	if monthlyCost(120, cycleYearly, 0) != 10 || monthlyCost(9, cycleMonthly, 0) != 9 || monthlyCost(1, cycleCustom, 0) != 0 {
		t.Fatal("monthly cost")
	}
	if got := round2(monthlyCost(30, cycleCustom, 30)); got != 30.44 {
		t.Fatalf("custom: %v", got)
	}
}

func TestNormalizeTarget(t *testing.T) {
	ok := map[[2]string]string{
		{kindHTTP, "https://example.com/health"}: "https://example.com/health",
		{kindTLS, "example.com"}:                 "example.com",
		{kindTLS, "https://example.com/x"}:       "example.com",
		{kindTLS, "example.com:8443"}:            "example.com:8443",
		{kindTLS, "example.com:443"}:             "example.com",
		{kindDomain, "https://www.Example.COM/"}: "example.com",
		{kindDomain, "example.org."}:             "example.org",
	}
	for in, want := range ok {
		got, err := normalizeTarget(in[0], in[1])
		if err != nil || got != want {
			t.Errorf("%v: got %q %v, want %q", in, got, err, want)
		}
	}
	for _, in := range [][2]string{{kindHTTP, "ftp://x"}, {kindHTTP, "example.com"}, {kindTLS, "a b"}, {kindTLS, "x:99999"}, {kindDomain, "localhost"}, {"ping", "x"}} {
		if _, err := normalizeTarget(in[0], in[1]); err == nil {
			t.Errorf("%v accepted", in)
		}
	}
}

func TestClip(t *testing.T) {
	if clip("héllo", 2) != "h\n…(truncated)" || clip("abc", 5) != "abc" {
		t.Fatalf("%q", clip("héllo", 2))
	}
}
