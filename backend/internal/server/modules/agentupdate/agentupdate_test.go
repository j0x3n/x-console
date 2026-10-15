package agentupdate_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/selfupdate"
	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/agentupdate"
	"github.com/j0x3n/x-console/backend/internal/server/modules/agentupdate/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// B233: the panel updates agents whose version differs from its own.

type fixture struct {
	t      *testing.T
	env    *testutil.Env
	m      *agentupdate.Module
	dir    string // XC_AGENTS_DIR
	events chan string
}

func setup(t *testing.T, panelVersion string) *fixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the agent programs in these tests are shell scripts")
	}
	dir := t.TempDir()
	prog := filepath.Join(dir, "linux-amd64")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "x-console-agent"), []byte("#!/bin/sh\necho v2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := testutil.NewWithConfig(t, func(c *config.Config) { c.AgentsDir = dir })
	m, ok := module.Lookup[*agentupdate.Module](env.App.Deps.Registry, agentupdate.ServiceKey)
	if !ok {
		t.Fatal("agentupdate module not registered")
	}
	agentupdate.SetVersion(m, panelVersion)
	f := &fixture{t: t, env: env, m: m, dir: dir, events: make(chan string, 256)}
	ch, cancel := env.App.Deps.Bus.Subscribe(agentupdate.EventChanged, 256)
	t.Cleanup(cancel)
	go func() {
		for ev := range ch {
			if d, ok := ev.Data.(map[string]string); ok {
				select {
				case f.events <- d["agentId"]:
				default:
				}
			}
		}
	}()
	return f
}

// testAgent is a paired agent that can connect, drop and come back.
type testAgent struct {
	f      *fixture
	id     string
	token  string
	osName string
	arch   string
	caps   []string

	mu     sync.Mutex
	cancel context.CancelFunc

	// custom, when set, serves agent.update instead of the real updater.
	custom func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error
	// updater is the real updater; its restart reconnects as nextVersion.
	updater     *selfupdate.Updater
	nextVersion string // "" means the agent does not come back
	exeDir      string
	calls       atomic.Int32
	restarts    atomic.Int32
}

// pair registers an agent without connecting it. withUpdate adds the update
// capability.
func (f *fixture) pair(t *testing.T, withUpdate bool) *testAgent {
	t.Helper()
	code, _, err := f.env.App.Deps.Agents.CreatePairingCode(context.Background(), "test", "server")
	if err != nil {
		t.Fatal(err)
	}
	a := &testAgent{f: f, osName: "linux", arch: "amd64", nextVersion: "v2", exeDir: t.TempDir(), caps: []string{protocol.CapSystemInfo}}
	if withUpdate {
		a.caps = append(a.caps, protocol.CapUpdate)
	}
	a.id, a.token, err = conn.Pair(context.Background(), f.env.Server.URL, code, a.hello("v1"))
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(a.exeDir, "x-console-agent")
	if err := os.WriteFile(exe, []byte("OLD PROGRAM"), 0o755); err != nil {
		t.Fatal(err)
	}
	a.updater = &selfupdate.Updater{Exe: func() (string, error) { return exe, nil }}
	a.updater.Restart = func(string) error {
		a.restarts.Add(1)
		if a.nextVersion != "" {
			go func() {
				a.drop()
				a.connect(a.nextVersion)
			}()
		}
		return nil
	}
	return a
}

func (a *testAgent) hello(version string) protocol.Hello {
	return protocol.Hello{AgentVersion: version, OS: a.osName, Arch: a.arch, Hostname: "h", Capabilities: a.caps}
}

func (a *testAgent) connect(version string) {
	a.f.t.Helper()
	client := conn.New(a.f.env.Server.URL, a.token, a.hello(version))
	client.HandleStream(protocol.MethodAgentUpdate, func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
		a.calls.Add(1)
		if a.custom != nil {
			return a.custom(ctx, raw, s)
		}
		return a.updater.Handle(ctx, raw, s)
	})
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()
	a.f.t.Cleanup(cancel)
	go client.Run(ctx)
	waitFor(a.f.t, "agent online", func() bool { return a.f.env.App.Deps.Agents.Online(a.id) })
}

// drop disconnects the agent and waits until the panel sees it gone.
func (a *testAgent) drop() {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(15 * time.Millisecond)
	}
}

func (f *fixture) list() api.AgentUpdateList {
	f.t.Helper()
	var out api.AgentUpdateList
	f.env.MustDo(http.MethodGet, "/agent-updates", nil, &out)
	return out
}

func (f *fixture) item(a *testAgent) api.AgentUpdate {
	f.t.Helper()
	for _, it := range f.list().Items {
		if it.AgentId == a.id {
			return it
		}
	}
	f.t.Fatalf("agent %s is not in the list", a.id)
	return api.AgentUpdate{}
}

func (f *fixture) until(a *testAgent, what string, ok func(api.AgentUpdate) bool) api.AgentUpdate {
	f.t.Helper()
	var it api.AgentUpdate
	waitFor(f.t, what, func() bool { it = f.item(a); return ok(it) })
	return it
}

// settle waits a moment and then checks nothing started.
func (f *fixture) settle() { time.Sleep(300 * time.Millisecond) }

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAutoUpdateWhenVersionDiffers(t *testing.T) {
	f := setup(t, "v2")
	a := f.pair(t, true)
	a.connect("v1")
	// The agent swaps its program, restarts as v2 and the state clears.
	waitFor(t, "update", func() bool { return a.restarts.Load() == 1 })
	it := f.until(a, "new version", func(it api.AgentUpdate) bool { return it.Version == "v2" && it.State == api.Idle })
	if !it.AutoUpdate || !it.CanUpdate {
		t.Fatalf("item %+v", it)
	}
	if got := readFile(t, filepath.Join(a.exeDir, "x-console-agent")); got != "#!/bin/sh\necho v2\n" {
		t.Fatalf("program %q", got)
	}
	if readFile(t, filepath.Join(a.exeDir, "x-console-agent.old")) != "OLD PROGRAM" {
		t.Fatal("no .old")
	}
	if l := f.list(); l.Latest != "v2" {
		t.Fatalf("latest %q", l.Latest)
	}
	select {
	case id := <-f.events:
		if id != a.id {
			t.Fatalf("event for %s", id)
		}
	default:
		t.Fatal("no agentupdate.changed event")
	}
	f.settle()
	if a.calls.Load() != 1 {
		t.Fatalf("updated %d times", a.calls.Load())
	}
}

func TestNoAutoUpdate(t *testing.T) {
	t.Run("switched off", func(t *testing.T) {
		f := setup(t, "v2")
		a := f.pair(t, true)
		if err := f.env.App.Deps.Settings.Set(context.Background(), agentupdate.SettingDisabled, []string{a.id}); err != nil {
			t.Fatal(err)
		}
		a.connect("v1")
		f.settle()
		it := f.item(a)
		if a.calls.Load() != 0 || it.AutoUpdate || it.State != api.Idle || it.Version != "v1" {
			t.Fatalf("calls %d item %+v", a.calls.Load(), it)
		}
	})
	t.Run("panel is dev", func(t *testing.T) {
		f := setup(t, "dev")
		a := f.pair(t, true)
		a.connect("v1")
		f.settle()
		if a.calls.Load() != 0 || f.item(a).State != api.Idle {
			t.Fatal("updated although the panel is a dev build")
		}
	})
	t.Run("agent cannot update", func(t *testing.T) {
		f := setup(t, "v2")
		a := f.pair(t, false)
		a.connect("v1")
		f.settle()
		it := f.item(a)
		if a.calls.Load() != 0 || it.CanUpdate || it.State != api.Idle {
			t.Fatalf("calls %d item %+v", a.calls.Load(), it)
		}
	})
	t.Run("same version", func(t *testing.T) {
		f := setup(t, "v2")
		a := f.pair(t, true)
		a.connect("v2")
		f.settle()
		if a.calls.Load() != 0 {
			t.Fatal("updated an agent that is current")
		}
	})
}

// failing makes the agent refuse every update with msg.
func failing(a *testAgent, msg string) {
	a.custom = func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
		return &protocol.Error{Code: protocol.CodeFailed, Message: msg}
	}
}

func TestFailureIsShownAndRetriedOnlyAfterAnHour(t *testing.T) {
	f := setup(t, "v2")
	now := time.Now()
	var mu sync.Mutex
	agentupdate.SetNow(f.m, func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
	a := f.pair(t, true)
	failing(a, "校验不对")
	a.connect("v1")
	it := f.until(a, "failure", func(it api.AgentUpdate) bool { return it.State == api.Failed })
	if it.Error == nil || *it.Error != "校验不对" || it.UpdatedAt == nil {
		t.Fatalf("item %+v", it)
	}
	if a.calls.Load() != 1 {
		t.Fatalf("calls %d", a.calls.Load())
	}

	reconnect := func() {
		a.drop()
		waitFor(t, "offline", func() bool { return !f.env.App.Deps.Agents.Online(a.id) })
		a.connect("v1")
		f.settle()
	}
	reconnect() // within the hour: left alone
	if a.calls.Load() != 1 {
		t.Fatalf("tried again within the hour: %d", a.calls.Load())
	}
	// Another target version is a new try.
	agentupdate.SetVersion(f.m, "v3")
	reconnect()
	if a.calls.Load() != 2 {
		t.Fatalf("a new target was not tried: %d", a.calls.Load())
	}
	reconnect() // v3 is within the hour now
	if a.calls.Load() != 2 {
		t.Fatalf("tried v3 twice within the hour: %d", a.calls.Load())
	}
	mu.Lock()
	now = now.Add(61 * time.Minute)
	mu.Unlock()
	reconnect()
	if a.calls.Load() != 3 {
		t.Fatalf("not tried after an hour: %d", a.calls.Load())
	}
}

func TestManualUpdate(t *testing.T) {
	f := setup(t, "v2")
	a := f.pair(t, true)
	if err := f.env.App.Deps.Settings.Set(context.Background(), agentupdate.SettingDisabled, []string{a.id}); err != nil {
		t.Fatal(err)
	}
	url := "/agent-updates/" + a.id

	// Needs the second factor.
	if status, _ := f.env.Do(http.MethodPost, url, nil, nil); status != http.StatusForbidden {
		t.Fatalf("without elevation: %d", status)
	}
	f.env.Elevate()
	status := func() int { s, _ := f.env.Do(http.MethodPost, url, nil, nil); return s }
	if status() != http.StatusConflict { // not online
		t.Fatalf("offline")
	}
	a.connect("v1")
	f.settle()
	if a.calls.Load() != 0 {
		t.Fatal("auto switch off was ignored")
	}
	if s, _ := f.env.Do(http.MethodPost, "/agent-updates/nope", nil, nil); s != http.StatusNotFound {
		t.Fatalf("unknown agent: %d", s)
	}

	// Manual does not care about the switch; hold the update to see "updating".
	release := make(chan struct{})
	a.custom = func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
		<-release
		return &protocol.Error{Code: protocol.CodeFailed, Message: "没有权限写程序文件"}
	}
	if s := status(); s != http.StatusAccepted {
		t.Fatalf("manual update: %d", s)
	}
	f.until(a, "updating", func(it api.AgentUpdate) bool { return it.State == api.Updating })
	if status() != http.StatusConflict {
		t.Fatal("second update while updating")
	}
	close(release)
	it := f.until(a, "failure", func(it api.AgentUpdate) bool { return it.State == api.Failed })
	if *it.Error != "没有权限写程序文件" {
		t.Fatalf("error %q", *it.Error)
	}

	// Manual again (no one-hour rule), this time for real.
	a.custom = nil
	if s := status(); s != http.StatusAccepted {
		t.Fatalf("manual update after a failure: %d", s)
	}
	f.until(a, "updated", func(it api.AgentUpdate) bool { return it.Version == "v2" && it.State == api.Idle })
	if n := f.audited("agent_update.run"); n != 2 {
		t.Fatalf("%d audit entries for manual updates, want 2", n)
	}
	if status() != http.StatusConflict { // up to date
		t.Fatal("update of a current agent")
	}
}

func TestManualUpdateRefusals(t *testing.T) {
	f := setup(t, "v2")
	f.env.Elevate()
	old := f.pair(t, false)
	old.connect("v1")
	if s, body := f.env.Do(http.MethodPost, "/agent-updates/"+old.id, nil, nil); s != http.StatusConflict {
		t.Fatalf("agent without the capability: %d %s", s, body)
	}
	other := f.pair(t, true)
	other.arch = "arm64" // the panel has no program for it
	other.connect("v1")
	f.until(other, "failure", func(it api.AgentUpdate) bool { return it.State == api.Failed })
	if s, _ := f.env.Do(http.MethodPost, "/agent-updates/"+other.id, nil, nil); s != http.StatusConflict {
		t.Fatalf("no program: %d", s)
	}
	if it := f.item(other); *it.Error != "面板没有这个系统的代理程序" {
		t.Fatalf("error %q", *it.Error)
	}
}

func TestOnlyTwoAtATime(t *testing.T) {
	f := setup(t, "v2")
	release := make(chan struct{})
	hold := func(a *testAgent) {
		a.custom = func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
			select {
			case <-release:
			case <-ctx.Done():
			}
			return &protocol.Error{Code: protocol.CodeFailed, Message: "x"}
		}
	}
	var agents []*testAgent
	for i := 0; i < 3; i++ {
		a := f.pair(t, true)
		hold(a)
		agents = append(agents, a)
	}
	for _, a := range agents {
		a.connect("v1")
		time.Sleep(50 * time.Millisecond)
	}
	waitFor(t, "two updating", func() bool {
		n := 0
		for _, it := range f.list().Items {
			if it.State == api.Updating {
				n++
			}
		}
		return n == 2
	})
	f.settle()
	if agents[2].calls.Load() != 0 || f.item(agents[2]).State != api.Idle {
		t.Fatal("a third update started")
	}
	f.env.Elevate()
	if s, _ := f.env.Do(http.MethodPost, "/agent-updates/"+agents[2].id, nil, nil); s != http.StatusConflict {
		t.Fatalf("manual third update: %d", s)
	}
	close(release)
	// A slot frees up and the waiting one goes.
	waitFor(t, "third update", func() bool { return agents[2].calls.Load() == 1 })
}

func TestNewVersionDoesNotConnect(t *testing.T) {
	f := setup(t, "v2")
	agentupdate.SetWaits(f.m, 300*time.Millisecond, time.Second)
	a := f.pair(t, true)
	a.nextVersion = "" // swaps the program, then never comes back
	a.connect("v1")
	it := f.until(a, "failure", func(it api.AgentUpdate) bool { return it.State == api.Failed })
	if *it.Error != "更新后代理没有以新版本连上来" {
		t.Fatalf("error %q", *it.Error)
	}
}

func TestRestartedWithOldVersion(t *testing.T) {
	f := setup(t, "v2")
	a := f.pair(t, true)
	a.nextVersion = "v1" // reconnects, but still as the old version
	a.connect("v1")
	it := f.until(a, "failure", func(it api.AgentUpdate) bool { return it.State == api.Failed })
	if *it.Error != "重启后版本还是 v1，没有更新成功" {
		t.Fatalf("error %q", *it.Error)
	}
}

func TestAutoSwitch(t *testing.T) {
	f := setup(t, "v2")
	a := f.pair(t, true)
	a.connect("v2")
	url := "/agent-updates/" + a.id + "/auto"

	// Off needs no second factor and is remembered.
	if s, body := f.env.Do(http.MethodPut, url, map[string]bool{"on": false}, nil); s != http.StatusNoContent {
		t.Fatalf("off: %d %s", s, body)
	}
	if f.item(a).AutoUpdate {
		t.Fatal("still on")
	}
	var ids []string
	if err := f.env.App.Deps.Settings.Get(context.Background(), agentupdate.SettingDisabled, &ids); err != nil || len(ids) != 1 || ids[0] != a.id {
		t.Fatalf("stored %v %v", ids, err)
	}
	// On does.
	if s, _ := f.env.Do(http.MethodPut, url, map[string]bool{"on": true}, nil); s != http.StatusForbidden {
		t.Fatalf("on without elevation: %d", s)
	}
	f.env.Elevate()
	if s, body := f.env.Do(http.MethodPut, url, map[string]bool{"on": true}, nil); s != http.StatusNoContent {
		t.Fatalf("on: %d %s", s, body)
	}
	if !f.item(a).AutoUpdate {
		t.Fatal("still off")
	}
	if s, _ := f.env.Do(http.MethodPut, "/agent-updates/nope/auto", map[string]bool{"on": false}, nil); s != http.StatusNotFound {
		t.Fatalf("unknown agent: %d", s)
	}
	if n := f.audited("agent_update.auto"); n != 2 {
		t.Fatalf("%d audit entries for the switch, want 2", n)
	}
}

// audited counts the audit entries of an action that went through.
func (f *fixture) audited(action string) int {
	f.t.Helper()
	var n int
	if err := f.env.App.Deps.DB.QueryRow(`SELECT count(*) FROM audit_log WHERE action = ? AND result = 'ok'`, action).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func TestRevokedAgentIsForgotten(t *testing.T) {
	f := setup(t, "v2")
	a := f.pair(t, true)
	failing(a, "校验不对")
	a.connect("v1")
	f.until(a, "failure", func(it api.AgentUpdate) bool { return it.State == api.Failed })
	if s, _ := f.env.Do(http.MethodPut, "/agent-updates/"+a.id+"/auto", map[string]bool{"on": false}, nil); s != http.StatusNoContent {
		t.Fatal("off")
	}
	if err := f.env.App.Deps.Agents.Revoke(context.Background(), a.id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "gone", func() bool {
		for _, it := range f.list().Items {
			if it.AgentId == a.id {
				return false
			}
		}
		return true
	})
	waitFor(t, "switch dropped", func() bool {
		var ids []string
		_ = f.env.App.Deps.Settings.Get(context.Background(), agentupdate.SettingDisabled, &ids)
		return len(ids) == 0
	})
}
