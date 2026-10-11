package focusblock_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	focusapi "github.com/j0x3n/x-console/backend/internal/server/modules/focus/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/focusblock"
	"github.com/j0x3n/x-console/backend/internal/server/modules/focusblock/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// fakeAgent stands in for the computer's agent.
type fakeAgent struct {
	mu      sync.Mutex
	applies []protocol.FocusBlockApplyParams
	clears  int
	result  protocol.FocusBlockStatus // answer to apply
	failure *protocol.Error           // when set, apply fails
}

func (f *fakeAgent) register(c *conn.Client) {
	c.Handle(protocol.MethodFocusBlockApply, func(_ context.Context, raw json.RawMessage) (any, error) {
		var p protocol.FocusBlockApplyParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.applies = append(f.applies, p)
		if f.failure != nil {
			return nil, f.failure
		}
		st := f.result
		st.Active, st.SessionID = true, p.SessionID
		if st.Hosts == "" {
			st.Hosts = protocol.FocusBlockHostsOK
			st.Sites = len(p.Domains) * 3
		}
		return st, nil
	})
	c.Handle(protocol.MethodFocusBlockClear, func(context.Context, json.RawMessage) (any, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.clears++
		return protocol.FocusBlockStatus{Hosts: protocol.FocusBlockHostsOK}, nil
	})
	c.Handle(protocol.MethodFocusBlockStatus, func(context.Context, json.RawMessage) (any, error) {
		return protocol.FocusBlockStatus{Hosts: protocol.FocusBlockHostsOK}, nil
	})
}

func (f *fakeAgent) counts() (applies, clears int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.applies), f.clears
}

func (f *fakeAgent) lastApply() protocol.FocusBlockApplyParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.applies[len(f.applies)-1]
}

type rig struct {
	env   *testutil.Env
	m     *focusblock.Module
	agent *fakeAgent
	host  string
	base  int // clears from the agent's own connection, before the test starts
}

// cleared counts the clear calls since the agent connected.
func (r *rig) cleared() int {
	_, c := r.agent.counts()
	return c - r.base
}

func setup(t *testing.T) *rig {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*focusblock.Module](env.App.Deps.Registry, focusblock.ServiceKey)
	if !ok {
		t.Fatal("focusblock module not registered")
	}
	r := &rig{env: env, m: m, agent: &fakeAgent{}}
	r.host = env.Agent("desktop", []string{protocol.CapSystemInfo, protocol.CapFocusBlock}, r.agent.register)
	// A connecting agent is told the state: nothing to block, so it hears "clear".
	eventually(t, "first clear", func() bool { _, c := r.agent.counts(); return c >= 1 })
	r.base = 1
	return r
}

func (r *rig) settings(t *testing.T) api.FocusBlockSettings {
	t.Helper()
	var out api.FocusBlockSettings
	r.env.MustDo(http.MethodGet, "/focus-block/settings", nil, &out)
	return out
}

func (r *rig) put(t *testing.T, body map[string]any) api.FocusBlockSettings {
	t.Helper()
	var out api.FocusBlockSettings
	r.env.MustDo(http.MethodPut, "/focus-block/settings", body, &out)
	return out
}

// configure switches the block on for the fake computer.
func (r *rig) configure(t *testing.T, extra map[string]any) {
	t.Helper()
	body := map[string]any{"enabled": true, "hostIds": []string{r.host}, "domains": []string{"youtube.com", "https://www.bilibili.com/video/1"}, "apps": []string{"Steam.exe"}}
	for k, v := range extra {
		body[k] = v
	}
	r.put(t, body)
}

func (r *rig) startFocus(t *testing.T, minutes int) focusapi.FocusSession {
	t.Helper()
	var s focusapi.FocusSession
	r.env.MustDo(http.MethodPost, "/focus/start", map[string]any{"minutes": minutes}, &s)
	return s
}

func (r *rig) stopFocus(t *testing.T, id int64) {
	t.Helper()
	r.env.MustDo(http.MethodPost, fmt.Sprintf("/focus/%d/stop", id), nil, nil)
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (r *rig) computer(t *testing.T) api.FocusBlockComputer {
	t.Helper()
	for _, c := range r.settings(t).Computers {
		if c.Id == r.host {
			return c
		}
	}
	t.Fatal("computer not listed")
	return api.FocusBlockComputer{}
}

func TestSettingsNeedElevationAndAreChecked(t *testing.T) {
	env := testutil.New(t)
	// Before anything is elevated: read works, write does not.
	var s api.FocusBlockSettings
	env.MustDo(http.MethodGet, "/focus-block/settings", nil, &s)
	if s.Enabled || len(s.Domains) != 0 || len(s.Apps) != 0 || s.BlockInBreak || s.Ipv6 || s.MaxDomains != protocol.FocusBlockMaxDomains {
		t.Fatalf("defaults: %+v", s)
	}
	if status, _ := env.Do(http.MethodPut, "/focus-block/settings", map[string]any{"enabled": true}, nil); status != http.StatusForbidden {
		t.Fatalf("without elevation: %d", status)
	}
	env.Elevate()
	host := env.Agent("desktop", []string{protocol.CapFocusBlock}, nil)

	bad := map[string]map[string]any{
		"ip":        {"domains": []string{"127.0.0.1"}},
		"one part":  {"domains": []string{"localhost"}},
		"spaces":    {"domains": []string{"a b.com"}},
		"explorer":  {"apps": []string{"explorer.exe"}},
		"systemd":   {"apps": []string{"systemd-logind"}},
		"path":      {"apps": []string{`C:\Games\steam.exe`}},
		"unix path": {"apps": []string{"/usr/bin/steam"}},
		"empty app": {"apps": []string{" "}},
		"no host":   {"hostIds": []string{"nope"}},
		"bad scene": {"haStart": "light.living_room"},
		"too many":  {"apps": make([]string, protocol.FocusBlockMaxApps+1)},
	}
	for name, body := range bad {
		if status, raw := env.Do(http.MethodPut, "/focus-block/settings", body, nil); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, status, raw)
		}
	}
	// A refused request saves nothing, not even its valid parts.
	if status, _ := env.Do(http.MethodPut, "/focus-block/settings", map[string]any{"enabled": true, "domains": []string{"a.com", "oops"}}, nil); status != http.StatusBadRequest {
		t.Fatalf("mixed: %d", status)
	}
	env.MustDo(http.MethodGet, "/focus-block/settings", nil, &s)
	if s.Enabled || len(s.Domains) != 0 {
		t.Fatalf("saved a refused request: %+v", s)
	}

	env.MustDo(http.MethodPut, "/focus-block/settings", map[string]any{
		"enabled": true, "hostIds": []string{host, host}, "ipv6": true, "blockInBreak": true,
		"domains": []string{"https://WWW.YouTube.com/watch?v=1", "youtube.com", "www.youtube.com:443", "bilibili.com"},
		"apps":    []string{"Steam.exe", "steam", " VLC ", "chrome"},
		"haStart": "scene.focus_on", "haEnd": "script.focus_off",
	}, &s)
	if !s.Enabled || !s.Ipv6 || !s.BlockInBreak || !slices.Equal(s.HostIds, []string{host}) ||
		!slices.Equal(s.Domains, []string{"www.youtube.com", "youtube.com", "bilibili.com"}) ||
		!slices.Equal(s.Apps, []string{"steam", "vlc", "chrome"}) || s.HaStart != "scene.focus_on" || s.HaEnd != "script.focus_off" {
		t.Fatalf("saved: %+v", s)
	}
	// Only the sent fields change.
	env.MustDo(http.MethodPut, "/focus-block/settings", map[string]any{"enabled": false}, &s)
	if s.Enabled || len(s.Domains) != 3 || len(s.Apps) != 3 || !s.Ipv6 {
		t.Fatalf("partial update: %+v", s)
	}
	// The computer list shows the paired agent.
	if len(s.Computers) != 1 || s.Computers[0].Id != host || !s.Computers[0].Supported || !s.Computers[0].Online || !s.Computers[0].Selected {
		t.Fatalf("computers: %+v", s.Computers)
	}
}

func TestBlockFollowsTheFocusSession(t *testing.T) {
	r := setup(t)
	r.configure(t, map[string]any{"ipv6": true})
	if c := r.computer(t); c.State != api.Idle {
		t.Fatalf("before the focus: %+v", c)
	}

	s := r.startFocus(t, 25)
	eventually(t, "apply", func() bool { a, _ := r.agent.counts(); return a == 1 })
	p := r.agent.lastApply()
	if p.SessionID != s.Id || !p.IPv6 || !slices.Equal(p.Domains, []string{"youtube.com", "www.bilibili.com"}) || !slices.Equal(p.Apps, []string{"steam"}) {
		t.Fatalf("apply params: %+v", p)
	}
	// 25 minutes of focus plus the grace time. The agent counts it from when it hears it.
	if p.TTLSeconds < 25*60+290 || p.TTLSeconds > 25*60+300+5 {
		t.Fatalf("ttl %d", p.TTLSeconds)
	}
	eventually(t, "state active", func() bool { return r.computer(t).State == api.Active })
	c := r.computer(t)
	if c.Sites != 6 || !strings.Contains(c.Message, "已生效") {
		t.Fatalf("computer: %+v", c)
	}
	var st api.FocusBlockStatus
	r.env.MustDo(http.MethodGet, "/focus-block/status", nil, &st)
	if !st.Blocking || st.ActiveComputers != 1 || st.ProblemComputers != 0 {
		t.Fatalf("status: %+v", st)
	}

	r.stopFocus(t, s.Id)
	eventually(t, "clear", func() bool { return r.cleared() >= 1 })
	eventually(t, "state idle", func() bool { return r.computer(t).State == api.Idle })
	r.env.MustDo(http.MethodGet, "/focus-block/status", nil, &st)
	if st.Blocking {
		t.Fatalf("still blocking: %+v", st)
	}
	if a, _ := r.agent.counts(); a != 1 {
		t.Fatalf("applied %d times", a)
	}
}

func TestOffSwitchAndUnselectedComputerDoNothing(t *testing.T) {
	r := setup(t)
	r.configure(t, map[string]any{"enabled": false})
	s := r.startFocus(t, 25)
	time.Sleep(300 * time.Millisecond)
	if a, _ := r.agent.counts(); a != 0 {
		t.Fatalf("applied while switched off")
	}
	r.stopFocus(t, s.Id)

	r.put(t, map[string]any{"enabled": true, "hostIds": []string{}})
	s = r.startFocus(t, 25)
	time.Sleep(300 * time.Millisecond)
	if a, _ := r.agent.counts(); a != 0 {
		t.Fatalf("applied to a computer nobody chose")
	}
	if c := r.computer(t); c.State != api.Off {
		t.Fatalf("computer: %+v", c)
	}
	r.stopFocus(t, s.Id)

	// Empty lists have nothing to block.
	r.put(t, map[string]any{"hostIds": []string{r.host}, "domains": []string{}, "apps": []string{}})
	s = r.startFocus(t, 25)
	time.Sleep(300 * time.Millisecond)
	if a, _ := r.agent.counts(); a != 0 {
		t.Fatalf("applied with empty lists")
	}
	r.stopFocus(t, s.Id)
}

func TestSettingsChangedDuringFocusTakeEffectAtOnce(t *testing.T) {
	r := setup(t)
	r.configure(t, nil)
	s := r.startFocus(t, 25)
	eventually(t, "apply", func() bool { a, _ := r.agent.counts(); return a == 1 })

	r.put(t, map[string]any{"domains": []string{"twitter.com"}})
	eventually(t, "second apply with the new list", func() bool { a, _ := r.agent.counts(); return a == 2 })
	if p := r.agent.lastApply(); !slices.Equal(p.Domains, []string{"twitter.com"}) {
		t.Fatalf("params: %+v", p)
	}
	// Switching off restores.
	r.put(t, map[string]any{"enabled": false})
	eventually(t, "clear", func() bool { return r.cleared() >= 1 })
	eventually(t, "idle", func() bool { return r.computer(t).State != api.Active })
	r.stopFocus(t, s.Id)
}

func TestBlockEndsWhenTimeIsUpUnlessBreakIsBlocked(t *testing.T) {
	for _, inBreak := range []bool{false, true} {
		t.Run(fmt.Sprintf("blockInBreak=%v", inBreak), func(t *testing.T) {
			r := setup(t)
			r.configure(t, map[string]any{"blockInBreak": inBreak})
			s := r.startFocus(t, 25)
			eventually(t, "apply", func() bool { a, _ := r.agent.counts(); return a == 1 })
			ttl := r.agent.lastApply().TTLSeconds
			if inBreak && ttl < 25*60+3600+290 || !inBreak && ttl > 25*60+305 {
				t.Fatalf("ttl %d", ttl)
			}
			at := func(d time.Duration) {
				focusblock.SetNow(r.m, func() time.Time { return s.EndsAt.Add(d) })
				focusblock.Reconcile(r.m, context.Background())
			}
			at(-time.Minute) // still focusing
			if r.cleared() != 0 {
				t.Fatal("cleared before the planned end")
			}
			at(30 * time.Second) // time is up, the session is still open
			if inBreak {
				time.Sleep(100 * time.Millisecond)
				if r.cleared() != 0 {
					t.Fatal("cleared at the planned end although the break is blocked")
				}
				at(61 * time.Minute) // the focus module closes it an hour after the end
			}
			eventually(t, "clear", func() bool { return r.cleared() >= 1 })
			eventually(t, "idle", func() bool { return r.computer(t).State == api.Idle })
			r.stopFocus(t, s.Id)
		})
	}
}

func TestAgentReconnectGetsTheCurrentState(t *testing.T) {
	r := setup(t)
	r.configure(t, nil)
	s := r.startFocus(t, 25)
	eventually(t, "apply", func() bool { a, _ := r.agent.counts(); return a == 1 })

	// The agent came back after a restart: it hears the state again.
	focusblock.Reconcile(r.m, context.Background(), r.host)
	if a, _ := r.agent.counts(); a != 2 {
		t.Fatalf("applied %d times after reconnect", a)
	}
	r.stopFocus(t, s.Id)
	eventually(t, "clear", func() bool { return r.cleared() >= 1 })
	before := r.cleared()
	focusblock.Reconcile(r.m, context.Background(), r.host)
	if c := r.cleared(); c != before+1 {
		t.Fatalf("cleared %d times, want %d: a reconnecting agent is told to restore", c, before+1)
	}
}

func TestPermissionProblemIsShown(t *testing.T) {
	r := setup(t)
	r.agent.result = protocol.FocusBlockStatus{Hosts: protocol.FocusBlockHostsPermission, HostsError: "access denied", Killed: 2}
	r.configure(t, nil)
	r.startFocus(t, 25)
	eventually(t, "permission", func() bool { return r.computer(t).State == api.Permission })
	c := r.computer(t)
	if !strings.Contains(c.Message, "权限") || c.Killed != 2 {
		t.Fatalf("computer: %+v", c)
	}
	var st api.FocusBlockStatus
	r.env.MustDo(http.MethodGet, "/focus-block/status", nil, &st)
	if !st.Blocking || st.ProblemComputers != 1 {
		t.Fatalf("status: %+v", st)
	}
}

func TestAgentErrorIsShownAndRetried(t *testing.T) {
	r := setup(t)
	r.agent.failure = &protocol.Error{Code: protocol.CodeFailed, Message: "boom"}
	r.configure(t, nil)
	s := r.startFocus(t, 25)
	eventually(t, "failed", func() bool { return r.computer(t).State == api.Failed })
	if c := r.computer(t); !strings.Contains(c.Message, "boom") {
		t.Fatalf("message %q", c.Message)
	}
	// The next check (the minute sweep) tries again and works.
	r.agent.mu.Lock()
	r.agent.failure = nil
	r.agent.mu.Unlock()
	focusblock.Reconcile(r.m, context.Background())
	eventually(t, "active", func() bool { return r.computer(t).State == api.Active })
	r.stopFocus(t, s.Id)
}

func TestOldAgentWithoutTheFeatureIsLeftAlone(t *testing.T) {
	env := testutil.New(t)
	m, _ := module.Lookup[*focusblock.Module](env.App.Deps.Registry, focusblock.ServiceKey)
	calls := 0
	old := env.Agent("server", []string{protocol.CapSystemInfo}, func(c *conn.Client) {
		c.Handle(protocol.MethodFocusBlockApply, func(context.Context, json.RawMessage) (any, error) { calls++; return nil, nil })
		c.Handle(protocol.MethodFocusBlockClear, func(context.Context, json.RawMessage) (any, error) { calls++; return nil, nil })
	})
	env.MustDo(http.MethodPut, "/focus-block/settings", map[string]any{"enabled": true, "hostIds": []string{old}, "apps": []string{"steam"}}, nil)
	env.MustDo(http.MethodPost, "/focus/start", map[string]any{"minutes": 25}, nil)
	focusblock.Reconcile(m, context.Background(), old)
	var s api.FocusBlockSettings
	env.MustDo(http.MethodGet, "/focus-block/settings", nil, &s)
	if calls != 0 || len(s.Computers) != 1 || s.Computers[0].State != api.Unsupported || s.Computers[0].Supported {
		t.Fatalf("calls %d, computers %+v", calls, s.Computers)
	}
}

func TestHomeAssistantSceneIsOptional(t *testing.T) {
	r := setup(t)
	var mu sync.Mutex
	var called []string
	focusblock.SetHACall(r.m, func(_ context.Context, entity string) error {
		mu.Lock()
		defer mu.Unlock()
		called = append(called, entity)
		return nil
	})
	got := func() []string { mu.Lock(); defer mu.Unlock(); return slices.Clone(called) }

	// Nothing is set: nothing is called.
	r.configure(t, nil)
	s := r.startFocus(t, 25)
	eventually(t, "apply", func() bool { a, _ := r.agent.counts(); return a == 1 })
	r.stopFocus(t, s.Id)
	eventually(t, "clear", func() bool { return r.cleared() >= 1 })
	if len(got()) != 0 {
		t.Fatalf("called %v", got())
	}

	r.put(t, map[string]any{"haStart": "scene.focus_on", "haEnd": "script.focus_off"})
	s = r.startFocus(t, 25)
	eventually(t, "start scene", func() bool { return slices.Equal(got(), []string{"scene.focus_on"}) })
	// Changing a list during the session does not run the scene again.
	r.put(t, map[string]any{"apps": []string{"vlc"}})
	eventually(t, "second apply", func() bool { a, _ := r.agent.counts(); return a == 3 })
	r.stopFocus(t, s.Id)
	eventually(t, "end scene", func() bool { return slices.Equal(got(), []string{"scene.focus_on", "script.focus_off"}) })
	eventually(t, "message", func() bool { return strings.Contains(r.settings(t).HaMessage, "script.focus_off") })
}

func TestHomeAssistantFailureIsShown(t *testing.T) {
	r := setup(t)
	focusblock.SetHACall(r.m, func(context.Context, string) error { return errors.New("not configured") })
	r.configure(t, map[string]any{"haStart": "scene.focus_on"})
	s := r.startFocus(t, 25)
	eventually(t, "message", func() bool { return strings.Contains(r.settings(t).HaMessage, "没有打开") })
	r.stopFocus(t, s.Id)
	// The block itself still works.
	if a, _ := r.agent.counts(); a != 1 {
		t.Fatalf("applied %d times", a)
	}
}

func TestRealHomeAssistantCallReportsMissingSetup(t *testing.T) {
	r := setup(t)
	r.configure(t, map[string]any{"haStart": "scene.focus_on"})
	s := r.startFocus(t, 25)
	eventually(t, "message", func() bool { return strings.Contains(r.settings(t).HaMessage, "scene.focus_on 没有打开") })
	r.stopFocus(t, s.Id)
}

func TestSuggestionsComeFromScreenTimeEntertainment(t *testing.T) {
	r := setup(t)
	db := r.env.App.Deps.DB
	for i, row := range []struct{ app, category string }{
		{"Spotify.exe", "entertainment"}, // built-in rule
		{"steam.exe", "entertainment"},   // built-in rule
		{"chrome.exe", "entertainment"},  // a browser sorted by its window title
		{"explorer.exe", "other"},
		{"Code.exe", "coding"},
	} {
		if _, err := db.Exec(`INSERT INTO screen_minutes (host_id, minute, app, category, title) VALUES ('h', ?, ?, ?, '')`, 1000+i, row.app, row.category); err != nil {
			t.Fatal(err)
		}
	}
	r.env.MustDo(http.MethodPost, "/screentime/rules", map[string]any{"field": "app", "pattern": "MyGame.exe", "category": "entertainment"}, nil)
	r.env.MustDo(http.MethodPost, "/screentime/rules", map[string]any{"field": "app", "pattern": "explorer", "category": "entertainment"}, nil)
	r.env.MustDo(http.MethodPost, "/screentime/rules", map[string]any{"field": "title", "pattern": "anime", "category": "entertainment"}, nil)
	var out struct{ Apps []string }
	r.env.MustDo(http.MethodGet, "/focus-block/suggestions", nil, &out)
	want := []string{"mygame", "spotify", "steam"}
	if !slices.Equal(out.Apps, want) {
		t.Fatalf("suggestions %v, want %v (no browsers, no system programs)", out.Apps, want)
	}
}
