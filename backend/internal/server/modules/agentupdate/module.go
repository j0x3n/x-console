// Package agentupdate keeps the agents on the version the panel carries
// (B233). When an agent connects with another version, the panel sends it
// the program from XC_AGENTS_DIR and the agent swaps itself. State lives in
// memory; only the per-machine switch is stored. See docs/specs/B233.md.
package agentupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/agenthub"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/core"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/agentupdate/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

const (
	// SettingDisabled holds the ids of the machines with automatic updating
	// switched off.
	SettingDisabled = "agentupdate.disabled"
	// ServiceKey is where the module registers itself, for tests.
	ServiceKey = "agentupdate.updates"

	maxParallel = 2
	// retryAfter is how long one machine is left alone after an automatic
	// try at one target version.
	retryAfter = time.Hour
	// transferTimeout bounds sending the program and the agent's checks.
	transferTimeout = 5 * time.Minute
	// restartWait is how long the new version has to connect after the agent
	// said it swapped the program.
	restartWait = 2 * time.Minute
	// endWait is how long to watch the stream for a late error (a restart
	// that failed) after the agent said it swapped the program.
	endWait = 5 * time.Second
	chunk   = 32 << 10
)

// EventChanged is published when the update state of a machine changes.
const EventChanged = "agentupdate.changed"

type status struct {
	state      api.AgentUpdateState
	err        string
	at         time.Time
	token      int64 // tells one update from the next on the same machine
	restarting bool  // the agent swapped the program and is coming back
	timer      *time.Timer
}

type attempt struct {
	version string
	at      time.Time
}

// Module implements api.ServerInterface.
type Module struct {
	d *module.Deps

	// setMu serializes changes of the stored switch list.
	setMu sync.Mutex

	mu              sync.Mutex
	ctx             context.Context
	states          map[string]*status
	tried           map[string]attempt
	pending         map[string]struct{} // want an automatic update, no free slot
	seq             int64
	version         func() string
	nowFn           func() time.Time
	transferTimeout time.Duration
	restartWait     time.Duration
	endWait         time.Duration
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{
		d: d, ctx: context.Background(),
		states: map[string]*status{}, tried: map[string]attempt{}, pending: map[string]struct{}{},
		version:         func() string { return core.Version },
		nowFn:           time.Now,
		transferTimeout: transferTimeout, restartWait: restartWait, endWait: endWait,
	}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "agentupdate" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start follows the agents coming and going.
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	events, cancel := m.d.Bus.Subscribe("agent.", 64)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				id := agentID(ev.Data)
				switch ev.Topic {
				case "agent.online":
					m.onOnline(ctx, id)
				case "agent.revoked":
					m.onRevoked(ctx, id)
				}
			}
		}
	}()
	return nil
}

func agentID(data any) string {
	if v, ok := data.(map[string]string); ok {
		return v["agentId"]
	}
	return ""
}

// latest is the version of the agent programs the panel carries.
func (m *Module) latest() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.version()
}

func (m *Module) baseCtx() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ctx
}

// ---- the stored switch

func (m *Module) disabledSet(ctx context.Context) (map[string]bool, error) {
	var ids []string
	if err := m.d.Settings.Get(ctx, SettingDisabled, &ids); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return nil, err
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// setAuto changes the switch of one machine. Switching on drops its entry
// from the list, which is also how a removed machine is forgotten.
func (m *Module) setAuto(ctx context.Context, id string, on bool) error {
	m.setMu.Lock()
	defer m.setMu.Unlock()
	set, err := m.disabledSet(ctx)
	if err != nil {
		return err
	}
	if set[id] == !on {
		return nil
	}
	if on {
		delete(set, id)
	} else {
		set[id] = true
	}
	ids := make([]string, 0, len(set))
	for k := range set {
		ids = append(ids, k)
	}
	slices.Sort(ids)
	return m.d.Settings.Set(ctx, SettingDisabled, ids)
}

// ---- events

func (m *Module) onOnline(ctx context.Context, id string) {
	if id == "" {
		return
	}
	a, err := m.d.Agents.Get(ctx, id)
	if err != nil {
		return
	}
	latest := m.latest()
	m.mu.Lock()
	st := m.states[id]
	if st != nil && st.state == api.Updating {
		if st.restarting {
			if a.Version == latest {
				m.clearLocked(id)
			} else {
				m.setLocked(id, api.Failed, fmt.Sprintf("重启后版本还是 %s，没有更新成功", a.Version))
			}
		}
		// Not restarting yet: the stream reports the failure.
		m.mu.Unlock()
		return
	}
	if a.Version == latest && st != nil {
		m.clearLocked(id)
	}
	m.mu.Unlock()
	m.maybeAuto(ctx, a)
}

func (m *Module) onRevoked(ctx context.Context, id string) {
	if id == "" {
		return
	}
	m.mu.Lock()
	if st := m.states[id]; st != nil && st.timer != nil {
		st.timer.Stop()
	}
	delete(m.states, id)
	delete(m.tried, id)
	delete(m.pending, id)
	m.mu.Unlock()
	if err := m.setAuto(ctx, id, true); err != nil { // drops its entry
		slog.Warn("agentupdate: could not drop the switch of a removed agent", "agent", id, "err", err)
	}
	m.d.Bus.Publish(EventChanged, map[string]string{"agentId": id})
}

// ---- deciding

// eligibleLocked says whether an automatic update may start now, apart from
// the number of running updates.
func (m *Module) eligibleLocked(a agenthub.Agent, disabled bool, latest string) bool {
	if disabled || latest == "" || latest == "dev" || !a.Online || !a.Has(protocol.CapUpdate) || a.Version == latest {
		return false
	}
	if st := m.states[a.ID]; st != nil && st.state == api.Updating {
		return false
	}
	if t, ok := m.tried[a.ID]; ok && t.version == latest && m.nowFn().Sub(t.at) < retryAfter {
		return false
	}
	return true
}

func (m *Module) runningLocked() int {
	n := 0
	for _, st := range m.states {
		if st.state == api.Updating {
			n++
		}
	}
	return n
}

// maybeAuto starts an automatic update of a when the rules allow it, or
// queues it when two updates are already running.
func (m *Module) maybeAuto(ctx context.Context, a agenthub.Agent) {
	set, err := m.disabledSet(ctx)
	if err != nil {
		slog.Warn("agentupdate: could not read the switches", "err", err)
		return
	}
	latest := m.latest()
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.eligibleLocked(a, set[a.ID], latest) {
		delete(m.pending, a.ID)
		return
	}
	if m.runningLocked() >= maxParallel {
		m.pending[a.ID] = struct{}{}
		return
	}
	delete(m.pending, a.ID)
	if err := m.startLocked(a, latest, true); err != nil {
		slog.Warn("agentupdate: not started", "agent", a.ID, "err", err)
	}
}

// drain starts queued machines after a slot became free.
func (m *Module) drain() {
	ctx := m.baseCtx()
	m.mu.Lock()
	ids := make([]string, 0, len(m.pending))
	for id := range m.pending {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	slices.Sort(ids)
	for _, id := range ids {
		a, err := m.d.Agents.Get(ctx, id)
		if err != nil {
			m.mu.Lock()
			delete(m.pending, id)
			m.mu.Unlock()
			continue
		}
		m.maybeAuto(ctx, a)
	}
}

// ---- running an update

var errNoProgram = errors.New("面板没有这个系统的代理程序")

// programPath is the agent program for a machine's system.
func (m *Module) programPath(goos, arch string) (string, error) {
	if (goos != "linux" && goos != "windows") || (arch != "amd64" && arch != "arm64") {
		return "", errNoProgram
	}
	name := "x-console-agent"
	if goos == "windows" {
		name += ".exe"
	}
	if m.d.Config.AgentsDir == "" {
		return "", errNoProgram
	}
	path := filepath.Join(m.d.Config.AgentsDir, goos+"-"+arch, name)
	if st, err := os.Stat(path); err != nil || !st.Mode().IsRegular() {
		return "", errNoProgram
	}
	return path, nil
}

// startLocked marks the machine as updating and runs the update in the
// background. A missing program is recorded as a failure.
func (m *Module) startLocked(a agenthub.Agent, latest string, auto bool) error {
	if auto {
		m.tried[a.ID] = attempt{version: latest, at: m.nowFn()}
	}
	path, err := m.programPath(a.Os, a.Arch)
	if err != nil {
		m.setLocked(a.ID, api.Failed, err.Error())
		return conflict("no_program", err.Error())
	}
	m.seq++
	token := m.seq
	m.states[a.ID] = &status{state: api.Updating, at: m.nowFn(), token: token}
	m.publish(a.ID)
	slog.Info("agentupdate: updating agent", "agent", a.ID, "from", a.Version, "to", latest, "auto", auto)
	go m.run(a, path, latest, token)
	return nil
}

func (m *Module) run(a agenthub.Agent, path, version string, token int64) {
	err := m.transfer(a.ID, path, version, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		st := m.states[a.ID]
		if st == nil || st.token != token || st.state != api.Updating {
			return
		}
		st.restarting = true
		st.timer = time.AfterFunc(m.restartWait, func() { m.expire(a.ID, token) })
	})
	if err == nil {
		return // the new version connecting settles it
	}
	slog.Warn("agentupdate: update failed", "agent", a.ID, "err", err)
	m.mu.Lock()
	defer m.mu.Unlock()
	if st := m.states[a.ID]; st != nil && st.token == token && st.state == api.Updating {
		m.setLocked(a.ID, api.Failed, userMessage(err))
	}
}

// expire fails an update whose new version never connected.
func (m *Module) expire(id string, token int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st := m.states[id]; st != nil && st.token == token && st.state == api.Updating {
		m.setLocked(id, api.Failed, "更新后代理没有以新版本连上来")
	}
}

// transfer sends the program and waits for the agent to say it is in place.
// onReady runs at that moment, before the agent restarts.
func (m *Module) transfer(id, path, version string, onReady func()) error {
	f, err := os.Open(path)
	if err != nil {
		return errNoProgram
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	m.mu.Lock()
	timeout, wait, parent := m.transferTimeout, m.endWait, m.ctx
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	s, err := m.d.Agents.Open(ctx, id, protocol.MethodAgentUpdate,
		protocol.AgentUpdateParams{Version: version, Size: size, SHA256: hex.EncodeToString(h.Sum(nil))})
	if err != nil {
		return err
	}
	defer s.Close(nil)
	buf := make([]byte, chunk)
	for s.Context().Err() == nil {
		n, rerr := f.Read(buf)
		if n > 0 {
			if serr := s.Send(ctx, append([]byte(nil), buf[:n]...)); serr != nil {
				break // the agent's answer below says why
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	// There is no half-close: the agent counts bytes and answers when the
	// program is checked, tried and swapped in.
	if _, err := s.Recv(ctx); err != nil {
		return err
	}
	onReady()
	wctx, wcancel := context.WithTimeout(ctx, wait)
	defer wcancel()
	if _, err := s.Recv(wctx); err != nil {
		var pe *protocol.Error
		if errors.As(err, &pe) {
			return err // for example the restart failed
		}
	}
	return nil
}

// userMessage turns a failure into a short Chinese sentence.
func userMessage(err error) string {
	var pe *protocol.Error
	switch {
	case errors.As(err, &pe):
		if pe.Code == protocol.CodeUnknownMethod {
			return "这台代理太旧，要手动更新一次"
		}
		if pe.Message != "" {
			return pe.Message
		}
		return "代理拒绝了更新"
	case errors.Is(err, httpx.ErrAgentOffline), errors.Is(err, rpc.ErrClosed):
		return "更新时代理断线了"
	case errors.Is(err, context.DeadlineExceeded):
		return "更新超时了"
	case errors.Is(err, context.Canceled):
		return "面板停止了更新"
	}
	var he *httpx.Error
	if errors.As(err, &he) {
		return he.Message
	}
	if errors.Is(err, errNoProgram) {
		return err.Error()
	}
	return "更新失败：" + err.Error()
}

// ---- state

func (m *Module) publish(id string) {
	m.d.Bus.Publish(EventChanged, map[string]string{"agentId": id})
}

// setLocked records a new state. Leaving "updating" frees a slot.
func (m *Module) setLocked(id string, state api.AgentUpdateState, msg string) {
	st := m.states[id]
	if st == nil {
		st = &status{}
		m.states[id] = st
	}
	was := st.state
	if st.timer != nil {
		st.timer.Stop()
		st.timer = nil
	}
	st.state, st.err, st.at, st.restarting = state, msg, m.nowFn(), false
	m.publish(id)
	if was == api.Updating && state != api.Updating {
		go m.drain()
	}
}

// clearLocked forgets the state of a machine that is up to date.
func (m *Module) clearLocked(id string) {
	st := m.states[id]
	if st == nil {
		return
	}
	if st.timer != nil {
		st.timer.Stop()
	}
	delete(m.states, id)
	m.publish(id)
	if st.state == api.Updating {
		go m.drain()
	}
}

// ---- HTTP

// ListAgentUpdates implements api.ServerInterface.
func (m *Module) ListAgentUpdates(w http.ResponseWriter, r *http.Request) {
	agents, err := m.d.Agents.List(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	set, err := m.disabledSet(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	latest := m.latest()
	m.mu.Lock()
	items := make([]api.AgentUpdate, 0, len(agents))
	for _, a := range agents {
		it := api.AgentUpdate{AgentId: a.ID, Version: a.Version, AutoUpdate: !set[a.ID], CanUpdate: a.Has(protocol.CapUpdate), State: api.Idle}
		if st := m.states[a.ID]; st != nil {
			at := st.at
			it.State, it.UpdatedAt = st.state, &at
			if st.state == api.Failed {
				msg := st.err
				it.Error = &msg
			}
		}
		items = append(items, it)
	}
	m.mu.Unlock()
	httpx.JSON(w, http.StatusOK, api.AgentUpdateList{Latest: latest, Items: items})
}

func conflict(code, msg string) error { return httpx.NewError(http.StatusConflict, code, msg) }

// UpdateAgentNow implements api.ServerInterface.
func (m *Module) UpdateAgentNow(w http.ResponseWriter, r *http.Request, agentId api.AgentId) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a, err := m.d.Agents.Get(r.Context(), agentId)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	latest := m.latest()
	err = m.updateNow(a, latest)
	m.d.Audit.Record(r.Context(), "agent_update.run", a.ID, map[string]any{"name": a.Name, "from": a.Version, "to": latest}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (m *Module) updateNow(a agenthub.Agent, latest string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.states[a.ID] != nil && m.states[a.ID].state == api.Updating:
		return conflict("updating", "这台机器正在更新")
	case !a.Online:
		return conflict("offline", "这台机器不在线")
	case !a.Has(protocol.CapUpdate):
		return conflict("unsupported", "这台代理太旧，或者配置里关掉了更新，要手动更新一次")
	case a.Version == latest:
		return conflict("up_to_date", "已经是最新版本了")
	case m.runningLocked() >= maxParallel:
		return conflict("busy", "现在有两台机器在更新，等一下再试")
	}
	delete(m.pending, a.ID)
	return m.startLocked(a, latest, false)
}

// SetAgentAutoUpdate implements api.ServerInterface. Turning it on lets the
// panel send programs to the machine later, so that needs the second factor;
// turning it off does not.
func (m *Module) SetAgentAutoUpdate(w http.ResponseWriter, r *http.Request, agentId api.AgentId) {
	var body api.SetAgentAutoUpdateJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.On {
		if err := auth.RequireElevated(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	a, err := m.d.Agents.Get(r.Context(), agentId)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err = m.setAuto(r.Context(), a.ID, body.On)
	m.d.Audit.Record(r.Context(), "agent_update.auto", a.ID, map[string]any{"name": a.Name, "on": body.On}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.publish(a.ID)
	if body.On {
		go m.maybeAuto(m.baseCtx(), a)
	}
	httpx.NoContent(w)
}
