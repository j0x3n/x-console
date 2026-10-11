package me_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/me"
	"github.com/j0x3n/x-console/backend/internal/server/modules/me/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

var cst = time.FixedZone("CST", 8*3600)

// start is the fake "now" of the tests: Sunday 2026-10-11 14:00 in Asia/Shanghai,
// the zone of the default schedule (up at 12:00, bed at 03:30).
var start = time.Date(2026, 10, 11, 14, 0, 0, 0, cst)

type rig struct {
	t   *testing.T
	env *testutil.Env
	m   *me.Module

	mu    sync.Mutex
	now   time.Time
	hosts []me.Host

	ev    <-chan events.Event
	woken []contracts.MeWoke
}

func setup(t *testing.T) *rig {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*me.Module](env.App.Deps.Registry, me.ServiceKey)
	if !ok {
		t.Fatal("me module not registered")
	}
	r := &rig{t: t, env: env, m: m, now: start}
	me.SetNow(m, func() time.Time { r.mu.Lock(); defer r.mu.Unlock(); return r.now })
	me.SetHosts(m, func() []me.Host { r.mu.Lock(); defer r.mu.Unlock(); return append([]me.Host(nil), r.hosts...) })
	me.Reset(m, context.Background())
	ev, cancel := env.App.Deps.Bus.Subscribe("me.", 128)
	t.Cleanup(cancel)
	r.ev = ev
	return r
}

// set moves the clock to t.
func (r *rig) set(t time.Time) {
	r.mu.Lock()
	r.now = t
	r.mu.Unlock()
}

// advance moves the clock on by d.
func (r *rig) advance(d time.Duration) {
	r.mu.Lock()
	r.now = r.now.Add(d)
	r.mu.Unlock()
}

// pc sets the only work computer: state is "active" or "idle", and idle is
// how long since the last key press.
func (r *rig) pc(state string, idle time.Duration) {
	r.mu.Lock()
	r.hosts = []me.Host{{ID: "pc1", Name: "工作电脑", Online: true, State: state, Idle: idle}}
	r.mu.Unlock()
}

func (r *rig) state() api.MeState {
	r.t.Helper()
	var s api.MeState
	r.env.MustDo(http.MethodGet, "/me", nil, &s)
	return s
}

// want checks the state name now.
func (r *rig) want(name string) api.MeState {
	r.t.Helper()
	s := r.state()
	if string(s.State) != name {
		r.t.Fatalf("at %s: state = %s (source %q, guessed %v, manual %v), want %s", r.nowStr(), s.State, s.Source, s.Guessed, s.Manual, name)
	}
	return s
}

func (r *rig) nowStr() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.now.Format("01-02 15:04")
}

// drain returns the events published so far.
func (r *rig) drain() []events.Event {
	var out []events.Event
	for {
		select {
		case e := <-r.ev:
			out = append(out, e)
		default:
			return out
		}
	}
}

// wokes returns the me.woke payloads published since the last call.
func (r *rig) wokes() []contracts.MeWoke {
	var out []contracts.MeWoke
	for _, e := range r.drain() {
		if e.Topic == "me.woke" {
			out = append(out, e.Data.(contracts.MeWoke))
		}
	}
	return out
}

func (r *rig) token() string {
	r.t.Helper()
	r.env.Elevate()
	var out api.MeReportToken
	r.env.MustDo(http.MethodPost, "/me/report/token", nil, &out)
	return out.Token
}

// report posts to the public route the way a phone does: no session, just
// the token.
func (r *rig) report(token, body string) int {
	r.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, r.env.URL("/me/report"), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func (r *rig) count(query string, args ...any) int {
	r.t.Helper()
	var n int
	if err := r.env.App.Deps.DB.QueryRow(query, args...).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ---------- computers ----------

func TestComputerInUseThenIdleThenGone(t *testing.T) {
	r := setup(t)
	r.want("unknown") // no computer reports anything yet
	r.pc("active", 0)
	s := r.want("working")
	if s.Source != "pc" || s.Guessed || s.Manual || !s.Since.Equal(start) {
		t.Fatalf("working: %+v", s)
	}
	// Idle for 10 minutes: stepped away.
	r.advance(10 * time.Minute)
	r.pc("idle", 10*time.Minute)
	if s := r.want("away"); !s.Since.Equal(start.Add(10 * time.Minute)) {
		t.Fatalf("away since %v", s.Since)
	}
	// 29 minutes after the last time it was in use: still away.
	r.set(start.Add(29 * time.Minute))
	r.want("away")
	// 30 minutes: nothing tells whether the user is at home.
	r.set(start.Add(30 * time.Minute))
	r.want("unknown")
	// The phone says the user is home.
	tok := r.token()
	if code := r.report(tok, `{"event":"home.arrive"}`); code != http.StatusNoContent {
		t.Fatalf("arrive: %d", code)
	}
	s = r.want("home")
	if s.AtHome == nil || !*s.AtHome || s.Source != "phone" {
		t.Fatalf("home: %+v", s)
	}
	// Using the computer again beats everything.
	r.advance(time.Minute)
	r.pc("active", 0)
	r.want("working")
}

func TestOnlyChosenComputersCount(t *testing.T) {
	r := setup(t)
	r.mu.Lock()
	r.hosts = []me.Host{
		{ID: "pc1", Name: "工作电脑", Online: true, State: "idle", Idle: time.Hour},
		{ID: "pc2", Name: "游戏电脑", Online: true, State: "active"},
	}
	r.mu.Unlock()
	r.want("working") // none chosen: every computer counts
	var s api.MeSettings
	r.env.MustDo(http.MethodPut, "/me/settings", map[string]any{"workHosts": []string{"pc1"}, "guessSleep": true, "haHome": "", "haSleep": ""}, &s)
	if len(s.WorkHosts) != 1 || s.WorkHosts[0] != "pc1" || len(s.Hosts) != 2 {
		t.Fatalf("settings: %+v", s)
	}
	r.advance(31 * time.Minute) // the game computer's use is past the 30 minutes
	r.mu.Lock()
	r.hosts[0].State, r.hosts[0].Idle = "idle", time.Hour
	r.mu.Unlock()
	r.want("unknown") // the game computer is not a work computer
	r.mu.Lock()
	r.hosts[0].State, r.hosts[0].Idle = "active", 0
	r.mu.Unlock()
	r.want("working")
	// A computer that cannot report idle time cannot be chosen.
	status, _ := r.env.Do(http.MethodPut, "/me/settings", map[string]any{"workHosts": []string{"ghost"}, "guessSleep": true, "haHome": "", "haSleep": ""}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("unknown host: %d", status)
	}
}

func TestHostsComeFromAgentsWithThePresenceCapability(t *testing.T) {
	env := testutil.New(t)
	m, _ := module.Lookup[*me.Module](env.App.Deps.Registry, me.ServiceKey)
	me.SetNow(m, func() time.Time { return start })
	me.Reset(m, context.Background())
	withIdle := env.Agent("desktop", []string{protocol.CapSystemInfo, protocol.CapPresence}, nil)
	env.Agent("desktop", []string{protocol.CapSystemInfo}, nil) // cannot report idle time
	idle := int64(0)
	fake := &fakePresence{states: map[string]contracts.HostPresence{withIdle: {HostID: withIdle, State: "active", Online: true, Known: true, IdleSeconds: &idle}}}
	module.Provide[contracts.Presence](env.App.Deps.Registry, contracts.PresenceKey, fake)
	var s api.MeSettings
	env.MustDo(http.MethodGet, "/me/settings", nil, &s)
	if len(s.Hosts) != 1 || s.Hosts[0].Id != withIdle || s.Hosts[0].State == nil || *s.Hosts[0].State != "active" {
		t.Fatalf("hosts: %+v", s.Hosts)
	}
	var st api.MeState
	env.MustDo(http.MethodGet, "/me", nil, &st)
	if st.State != "working" {
		t.Fatalf("state: %+v", st)
	}
	fake.set(withIdle, "idle")
	env.MustDo(http.MethodGet, "/me", nil, &st)
	if st.State != "away" {
		t.Fatalf("after idle: %+v", st)
	}
}

type fakePresence struct {
	mu     sync.Mutex
	states map[string]contracts.HostPresence
}

func (f *fakePresence) State(id string) contracts.HostPresence {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.states[id]
}
func (f *fakePresence) Subscribe() (<-chan contracts.HostPresence, func()) {
	return make(chan contracts.HostPresence), func() {}
}
func (f *fakePresence) set(id, state string) {
	f.mu.Lock()
	p := f.states[id]
	p.State = state
	f.states[id] = p
	f.mu.Unlock()
}

// ---------- phone reports ----------

func TestSleepAndWakeFromThePhone(t *testing.T) {
	r := setup(t)
	tok := r.token()
	r.set(time.Date(2026, 10, 12, 3, 40, 0, 0, cst))
	if code := r.report(tok, `{"event":"sleep.start"}`); code != http.StatusNoContent {
		t.Fatalf("sleep.start: %d", code)
	}
	s := r.want("sleeping")
	if s.Source != "phone" || s.Guessed || !s.Since.Equal(time.Date(2026, 10, 12, 3, 40, 0, 0, cst)) {
		t.Fatalf("sleeping: %+v", s)
	}
	if got := r.wokes(); len(got) != 0 {
		t.Fatalf("woke while going to sleep: %+v", got)
	}
	r.set(time.Date(2026, 10, 12, 11, 55, 0, 0, cst))
	r.want("sleeping") // nothing ended it
	if code := r.report(tok, `{"event":"sleep.end"}`); code != http.StatusNoContent {
		t.Fatalf("sleep.end: %d", code)
	}
	r.want("unknown")
	woke := r.wokes()
	if len(woke) != 1 || !woke[0].SleptAt.Equal(time.Date(2026, 10, 12, 3, 40, 0, 0, cst)) || !woke[0].WokeAt.Equal(time.Date(2026, 10, 12, 11, 55, 0, 0, cst)) || woke[0].Guessed {
		t.Fatalf("woke: %+v", woke)
	}
	// The status page learns when the last report came.
	var st api.MeReportStatus
	r.env.MustDo(http.MethodGet, "/me/report", nil, &st)
	if !st.Enabled || st.LastReportAt == nil || st.LastEvent == nil || *st.LastEvent != "sleep.end" || !strings.HasSuffix(st.ReportUrl, "/api/v1/me/report") {
		t.Fatalf("status: %+v", st)
	}
}

func TestShortSleepIsNotWakingUp(t *testing.T) {
	r := setup(t)
	tok := r.token()
	r.report(tok, `{"event":"sleep.start"}`)
	r.want("sleeping")
	r.advance(2*time.Hour + 59*time.Minute)
	r.report(tok, `{"event":"sleep.end"}`)
	r.want("unknown")
	if got := r.wokes(); len(got) != 0 {
		t.Fatalf("a nap counted: %+v", got)
	}
	// Exactly three hours counts.
	r.advance(time.Minute)
	r.report(tok, `{"event":"sleep.start"}`)
	r.advance(3 * time.Hour)
	r.report(tok, `{"event":"sleep.end"}`)
	if got := r.wokes(); len(got) != 1 {
		t.Fatalf("3 hours: %+v", got)
	}
}

func TestReportChecks(t *testing.T) {
	r := setup(t)
	if code := r.report("anything", `{"event":"sleep.start"}`); code != http.StatusUnauthorized {
		t.Fatalf("no token set up: %d", code)
	}
	tok := r.token()
	cases := []struct {
		name, token, body string
		want              int
	}{
		{"right token", tok, `{"event":"home.arrive"}`, http.StatusNoContent},
		{"no token", "", `{"event":"home.arrive"}`, http.StatusUnauthorized},
		{"wrong token", "nope", `{"event":"home.arrive"}`, http.StatusUnauthorized},
		{"unknown event", tok, `{"event":"wake"}`, http.StatusBadRequest},
		{"unknown field", tok, `{"event":"home.arrive","lat":31.2}`, http.StatusBadRequest},
		{"not json", tok, `event=home.arrive`, http.StatusBadRequest},
		{"empty", tok, ``, http.StatusBadRequest},
		{"at too late", tok, `{"event":"home.leave","at":"2026-10-11T14:06:00+08:00"}`, http.StatusBadRequest},
		{"at too early", tok, `{"event":"home.leave","at":"2026-10-11T01:59:00+08:00"}`, http.StatusBadRequest},
		{"at is not a time", tok, `{"event":"home.leave","at":"now"}`, http.StatusBadRequest},
		{"at 5 minutes ahead", tok, `{"event":"home.leave","at":"2026-10-11T14:05:00+08:00"}`, http.StatusNoContent},
		{"at 12 hours back", tok, `{"event":"home.arrive","at":"2026-10-11T02:00:01+08:00"}`, http.StatusNoContent},
		{"more than 1 KB", tok, `{"event":"home.arrive","pad":"` + strings.Repeat("x", 1100) + `"}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		if code := r.report(c.token, c.body); code != c.want {
			t.Errorf("%s: %d, want %d", c.name, code, c.want)
		}
	}
	// The rejected requests left nothing behind, and the token is never stored.
	if n := r.count(`SELECT COUNT(*) FROM me_reports`); n != 3 {
		t.Errorf("reports stored: %d", n)
	}
	var stored string
	if err := r.env.App.Deps.DB.QueryRow(`SELECT value FROM settings WHERE key = 'me.report'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, tok) {
		t.Fatal("the token is stored in clear")
	}
	// Audit records the good reports and never the token.
	if n := r.count(`SELECT COUNT(*) FROM audit_log WHERE action = 'me.report'`); n != 3 {
		t.Errorf("audit entries: %d", n)
	}
	if n := r.count(`SELECT COUNT(*) FROM audit_log WHERE detail LIKE ?`, "%"+tok+"%"); n != 0 {
		t.Errorf("the token is in the audit log")
	}
}

func TestTokenLifecycle(t *testing.T) {
	r := setup(t)
	// Making a token needs a fresh two-factor check.
	if status, _ := r.env.Do(http.MethodPost, "/me/report/token", nil, nil); status != http.StatusForbidden {
		t.Fatalf("token without elevation: %d", status)
	}
	var st api.MeReportStatus
	r.env.MustDo(http.MethodGet, "/me/report", nil, &st)
	if st.Enabled {
		t.Fatal("enabled before a token exists")
	}
	first := r.token()
	var second api.MeReportToken
	r.env.MustDo(http.MethodPost, "/me/report/token", nil, &second)
	if first == second.Token || len(first) < 32 {
		t.Fatalf("tokens: %q %q", first, second.Token)
	}
	if !strings.Contains(second.Example, "Authorization: Bearer "+second.Token) || !strings.Contains(second.Example, second.ReportUrl) {
		t.Fatalf("example: %s", second.Example)
	}
	if code := r.report(first, `{"event":"home.arrive"}`); code != http.StatusUnauthorized {
		t.Fatalf("old token after a new one: %d", code)
	}
	if code := r.report(second.Token, `{"event":"home.arrive"}`); code != http.StatusNoContent {
		t.Fatalf("new token: %d", code)
	}
	if status, _ := r.env.Do(http.MethodDelete, "/me/report", nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if code := r.report(second.Token, `{"event":"home.arrive"}`); code != http.StatusUnauthorized {
		t.Fatalf("token after closing: %d", code)
	}
	r.env.MustDo(http.MethodGet, "/me/report", nil, &st)
	if st.Enabled {
		t.Fatal("still enabled after closing")
	}
}

// The report route is public, and a public prefix also covers the other
// routes under /me/report. They must still need a login.
func TestOtherReportRoutesNeedALogin(t *testing.T) {
	r := setup(t)
	tok := r.token()
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/me/report"},
		{http.MethodDelete, "/me/report"},
		{http.MethodPost, "/me/report/token"},
	} {
		req, _ := http.NewRequest(c.method, r.env.URL(c.path), nil)
		req.Header.Set("Authorization", "Bearer "+tok) // the report token opens nothing else
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without a session: %d", c.method, c.path, resp.StatusCode)
		}
	}
	// With a session but without the CSRF header, writes are refused.
	req, _ := http.NewRequest(http.MethodDelete, r.env.URL("/me/report"), nil)
	resp, err := r.env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("delete without the CSRF header: %d", resp.StatusCode)
	}
	if code := r.report(tok, `{"event":"home.arrive"}`); code != http.StatusNoContent {
		t.Errorf("the token stopped working: %d", code)
	}
	// Nothing else is public.
	for _, p := range []string{"/me", "/me/history", "/me/settings"} {
		req, _ := http.NewRequest(http.MethodGet, r.env.URL(p), nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s without a session: %d", p, resp.StatusCode)
		}
	}
}

func TestRepeatedReportsCountOnce(t *testing.T) {
	r := setup(t)
	tok := r.token()
	r.report(tok, `{"event":"sleep.start"}`)
	r.advance(time.Minute + 59*time.Second)
	if code := r.report(tok, `{"event":"sleep.start"}`); code != http.StatusNoContent {
		t.Fatalf("repeat: %d", code)
	}
	if n := r.count(`SELECT COUNT(*) FROM me_reports WHERE event = 'sleep.start'`); n != 1 {
		t.Fatalf("stored %d", n)
	}
	// Another kind of event is not a repeat.
	r.report(tok, `{"event":"home.leave"}`)
	if n := r.count(`SELECT COUNT(*) FROM me_reports`); n != 2 {
		t.Fatalf("stored %d", n)
	}
	// Two minutes later it is a new event.
	r.advance(2 * time.Minute)
	r.report(tok, `{"event":"sleep.start"}`)
	if n := r.count(`SELECT COUNT(*) FROM me_reports WHERE event = 'sleep.start'`); n != 2 {
		t.Fatalf("stored %d", n)
	}
	// A report with its own time is compared by that time.
	r.report(tok, `{"event":"home.arrive","at":"2026-10-11T14:06:00+08:00"}`)
	r.report(tok, `{"event":"home.arrive","at":"2026-10-11T14:07:00+08:00"}`)
	if n := r.count(`SELECT COUNT(*) FROM me_reports WHERE event = 'home.arrive'`); n != 1 {
		t.Fatalf("stored %d", n)
	}
}

func TestNewestEventWinsWhateverTheArrivalOrder(t *testing.T) {
	r := setup(t)
	tok := r.token()
	r.report(tok, `{"event":"sleep.start","at":"2026-10-11T13:30:00+08:00"}`)
	r.report(tok, `{"event":"sleep.end","at":"2026-10-11T13:00:00+08:00"}`) // told late, happened first
	r.want("sleeping")
}

// ---------- the computer ends a sleep ----------

func TestUsingTheComputerEndsASleep(t *testing.T) {
	r := setup(t)
	tok := r.token()
	r.set(time.Date(2026, 10, 12, 0, 30, 0, 0, cst))
	r.report(tok, `{"event":"sleep.start"}`)
	r.want("sleeping")
	r.set(time.Date(2026, 10, 12, 8, 0, 0, 0, cst))
	r.pc("active", 0)
	s := r.want("working")
	if !s.Since.Equal(time.Date(2026, 10, 12, 8, 0, 0, 0, cst)) {
		t.Fatalf("working since %v", s.Since)
	}
	woke := r.wokes()
	if len(woke) != 1 || !woke[0].SleptAt.Equal(time.Date(2026, 10, 12, 0, 30, 0, 0, cst)) || !woke[0].WokeAt.Equal(time.Date(2026, 10, 12, 8, 0, 0, 0, cst)) {
		t.Fatalf("woke: %+v", woke)
	}
	// The sleep is over for good: when the computer goes idle the state does
	// not fall back to sleeping.
	r.set(time.Date(2026, 10, 12, 9, 0, 0, 0, cst))
	r.pc("idle", time.Hour)
	r.want("unknown")
	if n := r.count(`SELECT COUNT(*) FROM me_reports WHERE event = 'sleep.end' AND source = 'pc'`); n != 1 {
		t.Fatalf("pc sleep.end reports: %d", n)
	}
	// The phone's own "sleep ended" a minute later is the same news.
	r.report(tok, `{"event":"sleep.end"}`)
	if n := r.count(`SELECT COUNT(*) FROM me_reports WHERE event = 'sleep.end'`); n != 2 {
		t.Fatalf("sleep.end reports: %d", n)
	}
}

// Getting into bed a few minutes after the last key press: the computer still
// counts as in use for up to 5 minutes, but that input came before the sleep.
func TestIdleTimeBeforeBedDoesNotEndTheSleep(t *testing.T) {
	r := setup(t)
	tok := r.token()
	r.set(time.Date(2026, 10, 12, 3, 32, 0, 0, cst))
	r.pc("active", 4*time.Minute)
	r.report(tok, `{"event":"sleep.start"}`)
	r.want("working") // the computer still counts as in use for a minute
	if got := r.wokes(); len(got) != 0 {
		t.Fatalf("woke: %+v", got)
	}
	r.advance(2 * time.Minute)
	r.pc("idle", 6*time.Minute)
	s := r.want("sleeping")
	if s.Source != "phone" || !s.Since.Equal(time.Date(2026, 10, 12, 3, 32, 0, 0, cst)) {
		t.Fatalf("sleeping: %+v", s)
	}
	if got := r.wokes(); len(got) != 0 {
		t.Fatalf("woke: %+v", got)
	}
	// Key presses after the sleep began do end it.
	r.advance(time.Hour)
	r.pc("active", 10*time.Second)
	r.want("working")
	if got := r.wokes(); len(got) != 0 {
		t.Fatalf("one hour of sleep is not waking up: %+v", got)
	}
}

// ---------- schedule guess ----------

func TestGuessedSleepFromTheSchedule(t *testing.T) {
	r := setup(t)
	// The default schedule: up at 12:00, bed at 03:30. Working until 01:00.
	r.set(time.Date(2026, 10, 12, 1, 0, 0, 0, cst))
	r.pc("active", 0)
	r.want("working")
	r.advance(20 * time.Minute)
	r.pc("idle", 20*time.Minute)
	r.want("away")
	// 02:29 is before the window (02:30), and 01:31 is 31 minutes after use.
	r.set(time.Date(2026, 10, 12, 2, 29, 0, 0, cst))
	r.want("unknown")
	r.set(time.Date(2026, 10, 12, 2, 30, 0, 0, cst))
	s := r.want("sleeping")
	if !s.Guessed || s.Source != "schedule" || s.Manual || !s.Since.Equal(time.Date(2026, 10, 12, 2, 30, 0, 0, cst)) {
		t.Fatalf("guessed sleep: %+v", s)
	}
	// Using the computer in the morning ends it; a long guessed sleep is a wake-up.
	r.set(time.Date(2026, 10, 12, 10, 0, 0, 0, cst))
	r.pc("active", 0)
	r.want("working")
	woke := r.wokes()
	if len(woke) != 1 || !woke[0].Guessed || !woke[0].SleptAt.Equal(time.Date(2026, 10, 12, 2, 30, 0, 0, cst)) {
		t.Fatalf("woke: %+v", woke)
	}
	// Away for 40 minutes before the 12:00 wake time: not asleep again.
	r.set(time.Date(2026, 10, 12, 11, 20, 0, 0, cst))
	r.pc("idle", 40*time.Minute)
	r.set(time.Date(2026, 10, 12, 11, 30, 0, 0, cst))
	r.want("unknown")
	// The switch turns the guess off.
	r.set(time.Date(2026, 10, 13, 3, 0, 0, 0, cst))
	r.want("sleeping")
	r.env.MustDo(http.MethodPut, "/me/settings", map[string]any{"workHosts": []string{}, "guessSleep": false, "haHome": "", "haSleep": ""}, nil)
	r.want("unknown")
}

func TestPhoneConfirmsAGuessedSleep(t *testing.T) {
	r := setup(t)
	tok := r.token()
	r.set(time.Date(2026, 10, 12, 3, 0, 0, 0, cst))
	r.want("sleeping")
	r.advance(10 * time.Minute)
	r.report(tok, `{"event":"sleep.start"}`)
	s := r.want("sleeping")
	if s.Guessed || s.Source != "phone" || !s.Since.Equal(time.Date(2026, 10, 12, 3, 0, 0, 0, cst)) {
		t.Fatalf("after the phone's report: %+v", s)
	}
	r.set(time.Date(2026, 10, 12, 11, 0, 0, 0, cst))
	r.report(tok, `{"event":"sleep.end"}`)
	woke := r.wokes()
	if len(woke) != 1 || woke[0].Guessed {
		t.Fatalf("woke: %+v", woke)
	}
}

// ---------- manual ----------

func TestManualStateRunsOut(t *testing.T) {
	r := setup(t)
	tok := r.token()
	r.report(tok, `{"event":"home.arrive"}`)
	r.want("home")
	var s api.MeState
	r.env.MustDo(http.MethodPut, "/me/manual", map[string]any{"state": "out", "duration": "1h"}, &s)
	if s.State != "out" || !s.Manual || s.Source != "manual" || s.ManualUntil == nil || !s.ManualUntil.Equal(start.Add(time.Hour)) {
		t.Fatalf("manual: %+v", s)
	}
	// Even a computer in use does not change it.
	r.pc("active", 0)
	r.advance(59 * time.Minute)
	r.want("out")
	r.advance(time.Minute)
	if s := r.want("working"); s.Manual || s.ManualUntil != nil {
		t.Fatalf("after it ran out: %+v", s)
	}
	// And it stays gone after the setting ran out.
	var got string
	if err := r.env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM settings WHERE key = 'me.manual'`).Scan(&got); err != nil || got != "0" {
		t.Fatalf("setting kept: %v %v", got, err)
	}
}

func TestManualDurations(t *testing.T) {
	r := setup(t)
	put := func(state, duration string) api.MeState {
		var s api.MeState
		r.env.MustDo(http.MethodPut, "/me/manual", map[string]any{"state": state, "duration": duration}, &s)
		return s
	}
	if s := put("away", "2h"); s.ManualUntil == nil || !s.ManualUntil.Equal(start.Add(2*time.Hour)) {
		t.Fatalf("2h: %+v", s)
	}
	// "wake" is the next 12:00: tomorrow when asked at 14:00 ...
	if s := put("home", "wake"); s.ManualUntil == nil || !s.ManualUntil.Equal(time.Date(2026, 10, 12, 12, 0, 0, 0, cst)) {
		t.Fatalf("wake at 14:00: %+v", s)
	}
	// ... and today when asked at 03:00.
	r.set(time.Date(2026, 10, 12, 3, 0, 0, 0, cst))
	if s := put("sleeping", "wake"); s.ManualUntil == nil || !s.ManualUntil.Equal(time.Date(2026, 10, 12, 12, 0, 0, 0, cst)) {
		t.Fatalf("wake at 03:00: %+v", s)
	}
	s := put("working", "forever")
	if s.ManualUntil != nil || !s.Manual {
		t.Fatalf("forever: %+v", s)
	}
	r.advance(30*24*time.Hour + 10*time.Hour)
	r.want("working")
	// Back to automatic.
	var back api.MeState
	r.env.MustDo(http.MethodDelete, "/me/manual", nil, &back)
	if back.State != "unknown" || back.Manual {
		t.Fatalf("back to automatic: %+v", back)
	}
	// Clearing when nothing is set is fine.
	r.env.MustDo(http.MethodDelete, "/me/manual", nil, &back)
	for _, bad := range []map[string]any{{"state": "dancing", "duration": "1h"}, {"state": "out", "duration": "3h"}, {"state": "out"}} {
		if status, _ := r.env.Do(http.MethodPut, "/me/manual", bad, nil); status != http.StatusBadRequest {
			t.Errorf("%v: %d", bad, status)
		}
	}
}

func TestManualSleepThatRunsOutIsAWakeUp(t *testing.T) {
	r := setup(t)
	r.set(time.Date(2026, 10, 12, 2, 0, 0, 0, cst))
	var s api.MeState
	r.env.MustDo(http.MethodPut, "/me/manual", map[string]any{"state": "sleeping", "duration": "wake"}, &s)
	if s.State != "sleeping" || s.Guessed || !s.Manual {
		t.Fatalf("manual sleep: %+v", s)
	}
	r.pc("idle", 5*time.Hour)
	r.set(time.Date(2026, 10, 12, 11, 59, 0, 0, cst))
	r.want("sleeping")
	if got := r.wokes(); len(got) != 0 {
		t.Fatalf("woke early: %+v", got)
	}
	r.set(time.Date(2026, 10, 12, 12, 0, 0, 0, cst))
	r.want("unknown")
	woke := r.wokes()
	if len(woke) != 1 || woke[0].Guessed || !woke[0].SleptAt.Equal(time.Date(2026, 10, 12, 2, 0, 0, 0, cst)) || !woke[0].WokeAt.Equal(time.Date(2026, 10, 12, 12, 0, 0, 0, cst)) {
		t.Fatalf("woke: %+v", woke)
	}
}

func TestManualAwakeEndsAReportedSleep(t *testing.T) {
	r := setup(t)
	tok := r.token()
	r.set(time.Date(2026, 10, 12, 1, 0, 0, 0, cst))
	r.report(tok, `{"event":"sleep.start"}`)
	r.advance(6 * time.Hour)
	r.want("sleeping") // the sleep focus mode is still on
	r.env.MustDo(http.MethodPut, "/me/manual", map[string]any{"state": "home", "duration": "1h"}, nil)
	r.want("home")
	if got := r.wokes(); len(got) != 1 {
		t.Fatalf("woke: %+v", got)
	}
	// When the hour is up the state does not fall back to sleeping.
	r.advance(2 * time.Hour)
	r.pc("idle", time.Hour)
	r.want("unknown")
}

// ---------- Home Assistant ----------

type fakeHA struct {
	mu      sync.Mutex
	watched []string
	states  map[string]contracts.HAState
}

func (f *fakeHA) State(_ context.Context, id string) (contracts.HAState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st, ok := f.states[id]; ok {
		return st, nil
	}
	return contracts.HAState{}, httpx.ErrNotFound
}
func (f *fakeHA) CallService(context.Context, string, string, map[string]any) error { return nil }
func (f *fakeHA) WatchEntity(id string) {
	f.mu.Lock()
	f.watched = append(f.watched, id)
	f.mu.Unlock()
}
func (f *fakeHA) isWatched(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.watched {
		if w == id {
			return true
		}
	}
	return false
}

func (r *rig) withHA(states map[string]contracts.HAState) *fakeHA {
	ha := &fakeHA{states: states}
	module.Provide[contracts.HomeAssistant](r.env.App.Deps.Registry, contracts.HomeAssistantKey, ha)
	return ha
}

func (r *rig) chooseEntities(home, sleep string) {
	r.env.MustDo(http.MethodPut, "/me/settings", map[string]any{"workHosts": []string{}, "guessSleep": false, "haHome": home, "haSleep": sleep}, nil)
}

func (r *rig) haChanged(entity, state string) {
	r.env.App.Deps.Bus.Publish("ha.state_changed", contracts.HAState{EntityID: entity, State: state, LastChanged: time.Now()})
}

func TestHomeAssistantLocation(t *testing.T) {
	r := setup(t)
	ha := r.withHA(nil)
	r.chooseEntities("person.me", "")
	waitFor(t, "the entity to be watched", func() bool { return ha.isWatched("person.me") })
	r.haChanged("person.me", "home")
	waitFor(t, "arrival", func() bool { return r.state().State == "home" })
	if s := r.state(); s.Source != "ha" || s.AtHome == nil || !*s.AtHome {
		t.Fatalf("home: %+v", s)
	}
	r.advance(time.Minute)
	r.haChanged("person.me", "unavailable") // says nothing
	r.haChanged("light.desk", "on")         // not chosen
	r.haChanged("person.me", "not_home")
	waitFor(t, "leaving", func() bool { return r.state().State == "out" })
	if s := r.state(); s.Source != "ha" || s.AtHome == nil || *s.AtHome {
		t.Fatalf("out: %+v", s)
	}
	// An attribute change that keeps the state is not news.
	before := r.count(`SELECT COUNT(*) FROM me_reports`)
	r.haChanged("person.me", "not_home")
	time.Sleep(100 * time.Millisecond)
	if n := r.count(`SELECT COUNT(*) FROM me_reports`); n != before {
		t.Fatalf("reports %d -> %d", before, n)
	}
}

func TestHomeAssistantSleepSensor(t *testing.T) {
	r := setup(t)
	r.withHA(nil)
	r.chooseEntities("", "binary_sensor.phone_focus")
	r.haChanged("binary_sensor.phone_focus", "on")
	waitFor(t, "sleep", func() bool { return r.state().State == "sleeping" })
	if s := r.state(); s.Source != "ha" {
		t.Fatalf("source %q", s.Source)
	}
	r.advance(4 * time.Hour)
	r.haChanged("binary_sensor.phone_focus", "off")
	waitFor(t, "waking", func() bool { return r.state().State != "sleeping" })
	if got := r.wokes(); len(got) != 1 {
		t.Fatalf("woke: %+v", got)
	}
}

func TestPhoneAndHomeAssistantSaySameThing(t *testing.T) {
	r := setup(t)
	tok := r.token()
	r.withHA(nil)
	r.chooseEntities("person.me", "binary_sensor.phone_focus")
	r.report(tok, `{"event":"sleep.start"}`)
	r.advance(30 * time.Second)
	r.haChanged("binary_sensor.phone_focus", "on")
	time.Sleep(100 * time.Millisecond)
	if n := r.count(`SELECT COUNT(*) FROM me_reports WHERE event = 'sleep.start'`); n != 1 {
		t.Fatalf("sleep.start reports: %d", n)
	}
	r.want("sleeping")
	// Different words: the last one received wins.
	r.advance(10 * time.Minute)
	r.haChanged("binary_sensor.phone_focus", "off")
	waitFor(t, "the sensor's word to win", func() bool { return r.state().State != "sleeping" })
	r.advance(10 * time.Minute)
	r.report(tok, `{"event":"sleep.start"}`)
	r.want("sleeping")
	r.advance(10 * time.Minute)
	r.report(tok, `{"event":"home.arrive"}`)
	r.haChanged("person.me", "not_home")
	waitFor(t, "leaving", func() bool { return r.state().AtHome != nil && !*r.state().AtHome })
}

func TestHomeAssistantStateAtStartCountsWhenNewer(t *testing.T) {
	r := setup(t)
	ha := r.withHA(map[string]contracts.HAState{
		"person.me": {EntityID: "person.me", State: "not_home", LastChanged: start.Add(-time.Hour)},
	})
	r.chooseEntities("person.me", "")
	waitFor(t, "the first read", func() bool { return r.state().State == "out" })
	// A second read with nothing newer adds nothing.
	n := r.count(`SELECT COUNT(*) FROM me_reports`)
	me.WatchHA(r.m, context.Background())
	if m := r.count(`SELECT COUNT(*) FROM me_reports`); m != n {
		t.Fatalf("reports %d -> %d", n, m)
	}
	// Home Assistant knows an older state than the phone's report: the phone wins.
	tok := r.token()
	r.report(tok, `{"event":"home.arrive"}`)
	r.want("home")
	ha.mu.Lock()
	ha.states["person.me"] = contracts.HAState{EntityID: "person.me", State: "not_home", LastChanged: start.Add(-time.Hour)}
	ha.mu.Unlock()
	me.WatchHA(r.m, context.Background())
	r.want("home")
	// Bad entity names are refused.
	for _, bad := range []map[string]any{
		{"workHosts": []string{}, "guessSleep": true, "haHome": "light.desk", "haSleep": ""},
		{"workHosts": []string{}, "guessSleep": true, "haHome": "", "haSleep": "person.me"},
		{"workHosts": []string{}, "guessSleep": true, "haHome": "Person.Me", "haSleep": ""},
	} {
		if status, _ := r.env.Do(http.MethodPut, "/me/settings", bad, nil); status != http.StatusBadRequest {
			t.Errorf("%v: %d", bad, status)
		}
	}
}

// ---------- history, clean-up, contract, action ----------

func TestHistoryAndSleepHours(t *testing.T) {
	r := setup(t)
	tok := r.token()
	// Sunday 14:00 to Monday 03:00 at the computer.
	r.pc("active", 0)
	r.want("working")
	r.set(time.Date(2026, 10, 12, 3, 0, 0, 0, cst))
	r.pc("active", 0)
	r.want("working")
	r.pc("idle", 5*time.Minute)
	r.advance(10 * time.Minute)
	r.want("away")
	// Bed at 03:40 by the phone (the guess started at 03:30).
	r.set(time.Date(2026, 10, 12, 3, 40, 0, 0, cst))
	r.want("sleeping")
	r.report(tok, `{"event":"sleep.start"}`)
	// Up at 11:55 by the computer.
	r.set(time.Date(2026, 10, 12, 11, 55, 0, 0, cst))
	r.pc("active", 0)
	r.want("working")

	var h api.MeHistory
	r.env.MustDo(http.MethodGet, "/me/history?days=2", nil, &h)
	if len(h.Days) != 2 || h.Days[0].Date != "2026-10-12" || h.Days[1].Date != "2026-10-11" {
		t.Fatalf("days: %+v", h.Days)
	}
	monday, sunday := h.Days[0], h.Days[1]
	if monday.SleptAt == nil || monday.WokeAt == nil || monday.SleepMinutes == nil || monday.SleepGuessed == nil {
		t.Fatalf("no sleep on Monday: %+v", monday)
	}
	// The guess began at 03:10 + 30 minutes of no use = 03:40 at the latest;
	// the sleep is whatever the rows say, and must end at 11:55.
	if !monday.WokeAt.Equal(time.Date(2026, 10, 12, 11, 55, 0, 0, cst)) || *monday.SleepGuessed {
		t.Fatalf("monday sleep: %+v", monday)
	}
	wantMinutes := int(monday.WokeAt.Sub(*monday.SleptAt) / time.Minute)
	if *monday.SleepMinutes != wantMinutes || wantMinutes < 7*60 || wantMinutes > 9*60 {
		t.Fatalf("sleep minutes %d (from %v)", *monday.SleepMinutes, monday.SleptAt)
	}
	if sunday.SleptAt != nil {
		t.Fatalf("Sunday has a sleep: %+v", sunday)
	}
	// Sunday: working from 14:00 to midnight, then the same row goes on
	// Monday from 00:00.
	first := sunday.Segments[0]
	if first.State != "working" || !first.StartedAt.Equal(start) || first.EndedAt == nil || !first.EndedAt.Equal(time.Date(2026, 10, 12, 0, 0, 0, 0, cst)) {
		t.Fatalf("Sunday's first segment: %+v", first)
	}
	if monday.Segments[0].State != "working" || !monday.Segments[0].StartedAt.Equal(time.Date(2026, 10, 12, 0, 0, 0, 0, cst)) {
		t.Fatalf("Monday's first segment: %+v", monday.Segments[0])
	}
	last := monday.Segments[len(monday.Segments)-1]
	if last.State != "working" || last.EndedAt != nil {
		t.Fatalf("Monday's last segment: %+v", last)
	}
	for _, days := range []string{"0", "181", "x"} {
		if status, _ := r.env.Do(http.MethodGet, "/me/history?days="+days, nil, nil); status != http.StatusBadRequest {
			t.Errorf("days=%s: %d", days, status)
		}
	}
}

func TestOnlyLongSleepsAreCountedInHistory(t *testing.T) {
	r := setup(t)
	tok := r.token()
	r.report(tok, `{"event":"sleep.start"}`)
	r.advance(2 * time.Hour)
	r.report(tok, `{"event":"sleep.end"}`) // a nap
	r.want("unknown")
	r.advance(time.Hour)
	r.report(tok, `{"event":"sleep.start"}`)
	r.advance(5 * time.Hour)
	r.report(tok, `{"event":"sleep.end"}`)
	r.advance(time.Hour)
	r.want("unknown")
	var h api.MeHistory
	r.env.MustDo(http.MethodGet, "/me/history", nil, &h)
	if len(h.Days) != 7 {
		t.Fatalf("default days: %d", len(h.Days))
	}
	slept := 0
	for _, d := range h.Days {
		if d.SleepMinutes != nil {
			slept++
			if *d.SleepMinutes != 5*60 {
				t.Fatalf("sleep minutes: %d", *d.SleepMinutes)
			}
		}
	}
	if slept != 1 {
		t.Fatalf("days with a sleep: %d", slept)
	}
}

func TestCleanUpOldRows(t *testing.T) {
	r := setup(t)
	db := r.env.App.Deps.DB
	old := start.Add(-181 * 24 * time.Hour).UTC()
	recent := start.Add(-100 * 24 * time.Hour).UTC()
	for _, c := range []struct{ s, e time.Time }{{old, old.Add(time.Hour)}, {recent, recent.Add(time.Hour)}} {
		if _, err := db.Exec(`INSERT INTO me_states (state, source, started_at, ended_at) VALUES ('home', 'phone', ?, ?)`, c.s, c.e); err != nil {
			t.Fatal(err)
		}
	}
	for _, at := range []time.Time{start.Add(-31 * 24 * time.Hour), start.Add(-29 * 24 * time.Hour)} {
		if _, err := db.Exec(`INSERT INTO me_reports (event, source, at, created_at) VALUES ('home.arrive', 'phone', ?, ?)`, at.UTC(), at.UTC()); err != nil {
			t.Fatal(err)
		}
	}
	r.state() // makes the open row
	if err := me.Cleanup(r.m, context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := r.count(`SELECT COUNT(*) FROM me_states`); n != 2 { // the 100 days old one and the open one
		t.Fatalf("states left: %d", n)
	}
	if n := r.count(`SELECT COUNT(*) FROM me_reports`); n != 1 {
		t.Fatalf("reports left: %d", n)
	}
	// The newest report of each kind stays however old it is: it still says
	// where the user is.
	if _, err := db.Exec(`DELETE FROM me_reports`); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		event string
		ago   time.Duration
	}{{"home.arrive", 90 * 24 * time.Hour}, {"home.leave", 100 * 24 * time.Hour}, {"sleep.end", 50 * 24 * time.Hour}, {"sleep.start", 60 * 24 * time.Hour}} {
		at := start.Add(-c.ago).UTC()
		if _, err := db.Exec(`INSERT INTO me_reports (event, source, at, created_at) VALUES (?, 'phone', ?, ?)`, c.event, at, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := me.Cleanup(r.m, context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := r.count(`SELECT COUNT(*) FROM me_reports WHERE event IN ('home.arrive', 'sleep.end')`); n != 2 {
		t.Fatalf("newest reports left: %d", n)
	}
	if n := r.count(`SELECT COUNT(*) FROM me_reports`); n != 2 {
		t.Fatalf("reports left: %d", n)
	}
}

func TestContractAndAction(t *testing.T) {
	r := setup(t)
	service, ok := module.Lookup[contracts.Me](r.env.App.Deps.Registry, contracts.MeKey)
	if !ok {
		t.Fatal("contracts.Me is not provided")
	}
	if got := service.Current(); got.State != "" && got.State != contracts.MeUnknown {
		t.Fatalf("current before anything: %+v", got)
	}
	ch, cancel := service.Subscribe()
	defer cancel()
	r.pc("active", 0)
	r.want("working")
	select {
	case got := <-ch:
		if got.State != contracts.MeWorking || got.Source != "pc" || !got.Since.Equal(start) {
			t.Fatalf("pushed: %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no change pushed")
	}
	if got := service.Current(); got.State != contracts.MeWorking {
		t.Fatalf("current: %+v", got)
	}
	// me.state is published for the page and carries the same words.
	var topics []string
	for _, e := range r.drain() {
		topics = append(topics, e.Topic)
	}
	if !bytes.Contains([]byte(strings.Join(topics, ",")), []byte("me.state")) {
		t.Fatalf("events: %v", topics)
	}
	out, err := r.env.App.Deps.Actions.Run(context.Background(), "me.state", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), `"state":"working"`) {
		t.Fatalf("action: %s", raw)
	}
	a, ok := r.env.App.Deps.Actions.Get(context.Background(), "me.state")
	if !ok || a.Effect != "read" {
		t.Fatalf("action effect: %+v", a)
	}
}

func TestStateSurvivesARestart(t *testing.T) {
	r := setup(t)
	r.pc("active", 0)
	r.want("working")
	r.advance(10 * time.Minute)
	r.pc("idle", 10*time.Minute)
	r.want("away")
	// A fresh module instance reads the open row and the last time of use.
	me.Forget(r.m)
	s := r.want("away")
	if !s.Since.Equal(start.Add(10 * time.Minute)) {
		t.Fatalf("since after a restart: %v", s.Since)
	}
	// The restart cannot know the exact last use, only when the "working" row
	// ended (10 minutes in): away goes on until 30 minutes after that.
	r.set(start.Add(31 * time.Minute))
	r.want("away")
	r.set(start.Add(41 * time.Minute))
	r.want("unknown")
}
