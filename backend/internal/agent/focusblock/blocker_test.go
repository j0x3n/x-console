package focusblock

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// fakeProcs is a process table for the tests.
type fakeProcs struct {
	mu       sync.Mutex
	procs    []Proc
	stubborn map[int32]bool // survive the first request
	calls    []endCall
}

type endCall struct {
	PID   int32
	Force bool
}

func (f *fakeProcs) List(context.Context) ([]Proc, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Proc(nil), f.procs...), nil
}

func (f *fakeProcs) End(_ context.Context, pid int32, force bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, endCall{pid, force})
	if f.stubborn[pid] && !force {
		return nil
	}
	for i, p := range f.procs {
		if p.PID == pid {
			f.procs = append(f.procs[:i], f.procs[i+1:]...)
			break
		}
	}
	return nil
}

func (f *fakeProcs) start(pid int32, name string) {
	f.mu.Lock()
	f.procs = append(f.procs, Proc{PID: pid, Name: name})
	f.mu.Unlock()
}

func (f *fakeProcs) ended() []endCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]endCall(nil), f.calls...)
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type rig struct {
	b     *Blocker
	hosts string
	state string
	procs *fakeProcs
	clock *clock
	flush atomic.Int32
}

func newRig(t *testing.T, original string) *rig {
	t.Helper()
	dir := t.TempDir()
	r := &rig{hosts: filepath.Join(dir, "hosts"), state: filepath.Join(dir, "state", "focusblock-state.json"),
		procs: &fakeProcs{stubborn: map[int32]bool{}}, clock: &clock{t: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}}
	if original != "" {
		if err := os.WriteFile(r.hosts, []byte(original), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r.b = New(r.hosts, r.state)
	r.b.Procs = r.procs
	r.b.Now = r.clock.now
	r.b.Tick = 5 * time.Millisecond
	r.b.HostsCheck = 10 * time.Millisecond
	r.b.RetryClean = 10 * time.Millisecond
	r.b.FlushDNS = func() { r.flush.Add(1) }
	t.Cleanup(r.b.Close)
	return r
}

func (r *rig) read(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(r.hosts)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func apply(sites, apps []string) protocol.FocusBlockApplyParams {
	return protocol.FocusBlockApplyParams{SessionID: 7, TTLSeconds: 600, Domains: sites, Apps: apps}
}

func TestApplyWritesBlockAndClearRestores(t *testing.T) {
	r := newRig(t, sampleHosts)
	st, err := r.b.Apply(apply([]string{"https://www.YouTube.com/watch?v=1", "bilibili.com"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !st.Active || st.Hosts != protocol.FocusBlockHostsOK || st.Sites != 6 || st.SessionID != 7 || st.RemainingSeconds != 600 {
		t.Fatalf("status %+v", st)
	}
	got := r.read(t)
	for _, want := range []string{"127.0.0.1 youtube.com", "127.0.0.1 www.youtube.com", "127.0.0.1 m.youtube.com", "127.0.0.1 bilibili.com"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "::1 youtube.com") {
		t.Error("IPv6 lines without being asked")
	}
	if !strings.HasPrefix(got, sampleHosts) {
		t.Fatalf("original part changed:\n%s", got)
	}
	if _, err := os.Stat(r.state); err != nil {
		t.Fatalf("state file: %v", err)
	}
	if r.flush.Load() == 0 {
		t.Error("DNS cache was not flushed")
	}

	cleared := r.b.Clear()
	if cleared.Active || cleared.Hosts != protocol.FocusBlockHostsOK {
		t.Fatalf("cleared %+v", cleared)
	}
	if r.read(t) != sampleHosts {
		t.Fatalf("not restored:\n%s", r.read(t))
	}
	if _, err := os.Stat(r.state); !os.IsNotExist(err) {
		t.Fatalf("state file still there: %v", err)
	}
	// Clearing again is harmless and writes nothing.
	if again := r.b.Clear(); again.Active || r.read(t) != sampleHosts {
		t.Fatalf("second clear: %+v", again)
	}
}

func TestApplyTwiceAndIPv6(t *testing.T) {
	r := newRig(t, sampleHosts)
	p := apply([]string{"a.com"}, nil)
	p.IPv6 = true
	for range 2 {
		if _, err := r.b.Apply(p); err != nil {
			t.Fatal(err)
		}
	}
	got := r.read(t)
	if strings.Count(got, beginMarker) != 1 || strings.Count(got, "::1 a.com\n") != 1 {
		t.Fatalf("got:\n%s", got)
	}
	// A new list replaces the old one.
	if _, err := r.b.Apply(apply([]string{"b.com"}, nil)); err != nil {
		t.Fatal(err)
	}
	got = r.read(t)
	if strings.Contains(got, "a.com") || !strings.Contains(got, "127.0.0.1 b.com") || strings.Count(got, beginMarker) != 1 {
		t.Fatalf("got:\n%s", got)
	}
	// Only programs now: the hosts block goes.
	st, _ := r.b.Apply(apply(nil, []string{"steam"}))
	if st.Hosts != protocol.FocusBlockHostsNone || r.read(t) != sampleHosts {
		t.Fatalf("status %+v\n%s", st, r.read(t))
	}
}

func TestApplyWithNothingToBlockIsAClear(t *testing.T) {
	r := newRig(t, sampleHosts)
	if _, err := r.b.Apply(apply([]string{"a.com"}, nil)); err != nil {
		t.Fatal(err)
	}
	st, err := r.b.Apply(apply([]string{"nonsense"}, []string{"explorer"}))
	if err != nil || st.Active || len(st.Refused) != 2 {
		t.Fatalf("status %+v err %v", st, err)
	}
	if r.read(t) != sampleHosts {
		t.Fatalf("block stayed:\n%s", r.read(t))
	}
}

func TestApplyChecksParams(t *testing.T) {
	r := newRig(t, sampleHosts)
	for _, ttl := range []int{0, -1, protocol.FocusBlockMaxTTLSeconds + 1} {
		p := apply([]string{"a.com"}, nil)
		p.TTLSeconds = ttl
		if _, err := r.b.Apply(p); err == nil {
			t.Errorf("ttl %d accepted", ttl)
		}
	}
	many := make([]string, protocol.FocusBlockMaxApps+1)
	if _, err := r.b.Apply(apply(nil, many)); err == nil {
		t.Error("too many programs accepted")
	}
	if r.read(t) != sampleHosts {
		t.Fatal("file touched by a refused request")
	}
}

func TestProtectedAndPathNamesAreRefused(t *testing.T) {
	r := newRig(t, sampleHosts)
	r.procs.start(100, "explorer.exe")
	r.procs.start(101, "systemd-logind")
	r.procs.start(102, "steam.exe")
	st, err := r.b.Apply(apply(nil, []string{"explorer.exe", "SystemD-Logind", `C:\Games\steam.exe`, "/usr/bin/steam", "steam"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Refused) != 4 || st.Killed != 1 {
		t.Fatalf("status %+v", st)
	}
	calls := r.procs.ended()
	if len(calls) != 1 || calls[0].PID != 102 {
		t.Fatalf("ended %+v", calls)
	}
}

func TestOnlyTheNameMatches(t *testing.T) {
	r := newRig(t, sampleHosts)
	for i, name := range []string{"chrome-helper", "mychrome.exe", "chrome_proxy", "chromedriver.exe", "chr"} {
		r.procs.start(int32(200+i), name)
	}
	r.procs.start(300, "Chrome.EXE")
	if _, err := r.b.Apply(apply(nil, []string{"chrome"})); err != nil {
		t.Fatal(err)
	}
	calls := r.procs.ended()
	if len(calls) != 1 || calls[0].PID != 300 {
		t.Fatalf("ended %+v", calls)
	}
}

func TestLinuxTruncatedNameMatches(t *testing.T) {
	r := newRig(t, sampleHosts)
	// The kernel shows the first 15 characters of a longer name.
	r.procs.start(400, "very-long-progra")
	r.procs.start(401, "very-long-prog") // 14 characters: not a cut-off name
	r.procs.start(402, "very-long-progr")
	if _, err := r.b.Apply(apply(nil, []string{"very-long-program"})); err != nil {
		t.Fatal(err)
	}
	calls := r.procs.ended()
	if len(calls) != 1 || calls[0].PID != 402 {
		t.Fatalf("ended %+v", calls)
	}
}

func TestEndsProgramsThatStartAgain(t *testing.T) {
	r := newRig(t, sampleHosts)
	r.procs.start(500, "Steam.exe")
	st, err := r.b.Apply(apply(nil, []string{"steam"}))
	if err != nil || st.Killed != 1 || len(st.LastKilled) != 1 {
		t.Fatalf("status %+v err %v", st, err)
	}
	r.procs.start(501, "steam.exe")
	eventually(t, "second start ended", func() bool { return len(r.procs.ended()) == 2 })
	if got := r.b.Status().Killed; got != 2 {
		t.Fatalf("killed %d", got)
	}
}

func TestStubbornProgramGetsForced(t *testing.T) {
	r := newRig(t, sampleHosts)
	r.procs.stubborn[600] = true
	r.procs.start(600, "game")
	if _, err := r.b.Apply(apply(nil, []string{"game"})); err != nil {
		t.Fatal(err)
	}
	eventually(t, "forced", func() bool {
		c := r.procs.ended()
		return len(c) >= 2 && c[len(c)-1].Force
	})
	if got := r.b.Status().Killed; got != 1 {
		t.Fatalf("one program counted %d times", got)
	}
}

func TestDeadlineRestoresWithoutTheServer(t *testing.T) {
	r := newRig(t, sampleHosts)
	p := apply([]string{"a.com"}, []string{"steam"})
	p.TTLSeconds = 120
	if _, err := r.b.Apply(p); err != nil {
		t.Fatal(err)
	}
	r.clock.add(119 * time.Second)
	time.Sleep(40 * time.Millisecond)
	if !r.b.Status().Active || !strings.Contains(r.read(t), "a.com") {
		t.Fatal("restored too early")
	}
	r.clock.add(2 * time.Second)
	eventually(t, "restored at the deadline", func() bool { return !r.b.Status().Active && r.read(t) == sampleHosts })
	if _, err := os.Stat(r.state); !os.IsNotExist(err) {
		t.Fatalf("state file still there: %v", err)
	}
	// The program loop is over too.
	r.procs.start(700, "steam")
	time.Sleep(40 * time.Millisecond)
	if len(r.procs.ended()) != 0 {
		t.Fatal("still ending programs after the deadline")
	}
}

func TestHostsBlockIsPutBackWhenSomeoneRemovesIt(t *testing.T) {
	r := newRig(t, sampleHosts)
	if _, err := r.b.Apply(apply([]string{"a.com"}, nil)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.hosts, []byte(sampleHosts), 0o644); err != nil {
		t.Fatal(err)
	}
	eventually(t, "block back", func() bool { return strings.Contains(r.read(t), "127.0.0.1 a.com") })
}

func TestRecoverCleansALeftoverBlock(t *testing.T) {
	left := withBlock(sampleHosts, []string{"a.com"}, true)
	r := newRig(t, left)
	r.b.Recover() // no state file: nobody is focusing as far as this agent knows
	if r.read(t) != sampleHosts {
		t.Fatalf("not cleaned:\n%s", r.read(t))
	}
	if r.b.Status().Active {
		t.Fatal("active after recover")
	}
}

func TestRecoverCleansAnExpiredBlock(t *testing.T) {
	r := newRig(t, sampleHosts)
	if _, err := r.b.Apply(apply([]string{"a.com"}, nil)); err != nil {
		t.Fatal(err)
	}
	r.b.Close() // the agent dies here
	r.clock.add(time.Hour)

	again := New(r.hosts, r.state)
	again.Now = r.clock.now
	defer again.Close()
	again.Recover()
	if r.read(t) != sampleHosts || again.Status().Active {
		t.Fatalf("not cleaned:\n%s", r.read(t))
	}
	if _, err := os.Stat(r.state); !os.IsNotExist(err) {
		t.Fatalf("state file still there: %v", err)
	}
}

func TestRecoverCarriesOnWithinTheDeadline(t *testing.T) {
	r := newRig(t, sampleHosts)
	if _, err := r.b.Apply(apply([]string{"a.com"}, []string{"steam"})); err != nil {
		t.Fatal(err)
	}
	r.b.Close()
	if err := os.WriteFile(r.hosts, []byte(sampleHosts), 0o644); err != nil { // someone cleaned it meanwhile
		t.Fatal(err)
	}
	r.clock.add(time.Minute)

	again := New(r.hosts, r.state)
	again.Now = r.clock.now
	again.Procs = r.procs
	again.Tick = 5 * time.Millisecond
	defer again.Close()
	again.Recover()
	st := again.Status()
	if !st.Active || st.SessionID != 7 || st.RemainingSeconds != 540 || !strings.Contains(r.read(t), "127.0.0.1 a.com") {
		t.Fatalf("status %+v\n%s", st, r.read(t))
	}
	r.procs.start(800, "steam")
	eventually(t, "program ended after recover", func() bool { return len(r.procs.ended()) == 1 })
}

func TestRecoverIgnoresACorruptStateFile(t *testing.T) {
	r := newRig(t, withBlock(sampleHosts, []string{"a.com"}, false))
	if err := os.MkdirAll(filepath.Dir(r.state), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.state, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.b.Recover()
	if r.read(t) != sampleHosts {
		t.Fatalf("not cleaned:\n%s", r.read(t))
	}
}

func TestPanelAddressIsNeverBlocked(t *testing.T) {
	r := newRig(t, sampleHosts)
	r.b.Reserved = []string{"console.example.com"}
	st, err := r.b.Apply(apply([]string{"console.example.com", "example.com", "a.com"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	got := r.read(t)
	if strings.Contains(got, "console.example.com") || !strings.Contains(got, "a.com") || !strings.Contains(got, "127.0.0.1 example.com") {
		t.Fatalf("got:\n%s", got)
	}
	if len(st.Refused) != 1 || !strings.Contains(st.Refused[0], "console.example.com") {
		t.Fatalf("refused %v", st.Refused)
	}
}

func TestHostsFailureIsReportedAndProgramsStillEnd(t *testing.T) {
	r := newRig(t, "")
	// A directory where the hosts file should be: reading it fails.
	if err := os.Mkdir(r.hosts, 0o755); err != nil {
		t.Fatal(err)
	}
	r.procs.start(900, "steam")
	st, err := r.b.Apply(apply([]string{"a.com"}, []string{"steam"}))
	if err != nil {
		t.Fatal(err)
	}
	if !st.Active || st.Hosts != protocol.FocusBlockHostsFailed || st.HostsError == "" || st.Killed != 1 {
		t.Fatalf("status %+v", st)
	}
}

func TestFailedCleanUpIsRetriedAndRemembered(t *testing.T) {
	r := newRig(t, "")
	// The block is in the file, but the file cannot be read right now.
	moved := r.hosts + ".real"
	if err := os.WriteFile(moved, []byte(withBlock(sampleHosts, []string{"a.com"}, false)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(r.hosts, 0o755); err != nil {
		t.Fatal(err)
	}
	st := r.b.Clear()
	if st.Active || st.Hosts != protocol.FocusBlockHostsFailed {
		t.Fatalf("status %+v", st)
	}
	var s saved
	data, err := os.ReadFile(r.state)
	if err != nil || json.Unmarshal(data, &s) != nil || !s.Cleanup {
		t.Fatalf("state file %q err %v", data, err)
	}
	// The file comes back readable: the retry takes the block out.
	if err := os.Remove(r.hosts); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(moved, r.hosts); err != nil {
		t.Fatal(err)
	}
	eventually(t, "retry cleaned", func() bool {
		b, _ := os.ReadFile(r.hosts)
		return string(b) == sampleHosts
	})
	eventually(t, "state removed", func() bool { _, err := os.Stat(r.state); return os.IsNotExist(err) })
}

func TestNewApplyReplacesTheOldRun(t *testing.T) {
	r := newRig(t, sampleHosts)
	r.procs.start(1000, "steam")
	if _, err := r.b.Apply(apply(nil, []string{"steam"})); err != nil {
		t.Fatal(err)
	}
	p := apply([]string{"a.com"}, []string{"vlc"})
	p.SessionID = 8
	st, err := r.b.Apply(p)
	if err != nil || st.SessionID != 8 {
		t.Fatalf("status %+v err %v", st, err)
	}
	// Steam is no longer in the list.
	r.procs.start(1001, "steam")
	time.Sleep(40 * time.Millisecond)
	if n := len(r.procs.ended()); n != 1 {
		t.Fatalf("ended %d programs", n)
	}
}
