// Package focusblock blocks distracting sites and programs on the chosen
// computers while a pomodoro runs (B131). It follows the focus module's
// events and tells the agents what to block. The agents change their hosts
// file and end the listed programs; they restore everything by themselves
// when the deadline the panel gave them passes. See docs/specs/B131.md.
package focusblock

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/agenthub"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/focusblock/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/focusblock/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	// grace is how long an agent keeps a block after the time it should have
	// ended, in case the panel cannot tell it to stop.
	grace = 5 * time.Minute
	// staleAfter is how long the focus module leaves a session open after its
	// planned end before it closes it.
	staleAfter = time.Hour
	// callTimeout limits one call to an agent.
	callTimeout = 15 * time.Second
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "focusblock.module"

// Module implements api.ServerInterface.
type Module struct {
	d *module.Deps
	q *db.Queries

	wake chan struct{} // a reconcile is wanted

	mu      sync.Mutex
	pending map[string]bool // agents to bring to the right state whatever the records say
	timer   *time.Timer
	haSess  int64  // the session whose start scene was triggered
	haMsg   string // result of the last Home Assistant call

	// haCall runs a scene or script, and nowFn tells the time. Tests replace them.
	haCall func(ctx context.Context, entity string) error
	nowFn  func() time.Time
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), wake: make(chan struct{}, 1), pending: map[string]bool{}, nowFn: time.Now}
	m.haCall = m.callHA
	module.Provide[*Module](d.Registry, ServiceKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "focusblock" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start follows the pomodoro and the agents.
func (m *Module) Start(ctx context.Context) error {
	focusEvents, cancelFocus := m.d.Bus.Subscribe("focus.", 64)
	agentEvents, cancelAgent := m.d.Bus.Subscribe("agent.online", 64)
	m.d.Scheduler.Every("focusblock.sweep", time.Minute, func(context.Context) error {
		m.kick()
		return nil
	})
	// A session that is already running when the server starts has had its
	// start scene before.
	if cur, ok := m.focusCurrent(ctx); ok && m.options(ctx).Enabled {
		m.mu.Lock()
		m.haSess = cur.ID
		m.mu.Unlock()
	}
	go func() {
		defer cancelFocus()
		defer cancelAgent()
		defer m.stopTimer()
		m.reconcile(ctx, nil)
		for {
			select {
			case <-ctx.Done():
				return
			case <-focusEvents:
				m.reconcile(ctx, nil)
			case ev := <-agentEvents:
				var id string
				switch v := ev.Data.(type) {
				case map[string]string:
					id = v["agentId"]
				}
				m.reconcile(ctx, []string{id})
			case <-m.wake:
				m.reconcile(ctx, nil)
			}
		}
	}()
	return nil
}

func (m *Module) now() time.Time {
	m.mu.Lock()
	f := m.nowFn
	m.mu.Unlock()
	return f()
}

// kick asks for a reconcile soon.
func (m *Module) kick() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Module) stopTimer() {
	m.mu.Lock()
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	m.mu.Unlock()
}

// armTimer wakes the module at t, so a block ends on time even when the focus
// module's own event does not arrive.
func (m *Module) armTimer(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	if t.IsZero() {
		return
	}
	m.timer = time.AfterFunc(max(time.Until(t), 0)+time.Second, m.kick)
}

func (m *Module) focusCurrent(ctx context.Context) (contracts.FocusRunning, bool) {
	f, ok := module.Lookup[contracts.FocusCurrent](m.d.Registry, contracts.FocusCurrentKey)
	if !ok {
		return contracts.FocusRunning{}, false
	}
	return f.Current(ctx)
}

// ---- reconcile ----

// plan is what should be true now.
type plan struct {
	opt     options
	session contracts.FocusRunning
	active  bool // the focus is on and the block is switched on
	until   time.Time
}

func (m *Module) plan(ctx context.Context, now time.Time) plan {
	p := plan{opt: m.options(ctx)}
	if !p.opt.Enabled {
		return p
	}
	cur, ok := m.focusCurrent(ctx)
	if !ok {
		return p
	}
	p.session = cur
	p.until = cur.EndsAt
	if p.opt.BlockInBreak {
		p.until = cur.EndsAt.Add(staleAfter)
	}
	p.active = now.Before(p.until)
	return p
}

// ttl is how long an agent keeps the block when it hears nothing more.
func (p plan) ttl(now time.Time) int {
	s := int(p.until.Sub(now)/time.Second) + int(grace/time.Second)
	return min(max(s, 60), protocol.FocusBlockMaxTTLSeconds)
}

// reconcile brings every computer to the state the settings and the pomodoro
// ask for. force lists computers that just connected: they are told the
// state whatever the records say, because an agent may have restarted.
func (m *Module) reconcile(ctx context.Context, force []string) {
	if ctx.Err() != nil {
		return
	}
	now := m.now()
	p := m.plan(ctx, now)
	if p.active {
		m.armTimer(p.until)
	} else {
		m.armTimer(time.Time{})
	}
	changed := m.reconcileHA(ctx, p)
	agents, err := m.d.Agents.List(ctx)
	if err != nil {
		m.d.Log.Warn("focus block: list agents", "err", err)
		return
	}
	rows := map[string]db.FocusBlockAgent{}
	if list, err := m.q.ListFocusBlockAgents(ctx); err == nil {
		for _, r := range list {
			rows[r.AgentID] = r
		}
	}
	forced := map[string]bool{}
	for _, id := range force {
		forced[id] = true
	}
	var wg sync.WaitGroup
	var cm sync.Mutex
	for _, a := range agents {
		if !a.Online || !a.Has(protocol.CapFocusBlock) {
			continue
		}
		row, has := rows[a.ID]
		selected := contains(p.opt.HostIDs, a.ID)
		block := p.active && selected && p.opt.lists()
		switch {
		case block:
			if !forced[a.ID] && has && row.State == stateBlocking && row.SessionID == p.session.ID && row.ConfigHash == p.opt.hash() {
				continue
			}
		case forced[a.ID]:
			// Tell it to restore, in case it carried a block through a restart.
		case has && row.State != stateIdle:
		default:
			continue
		}
		wg.Add(1)
		go func(a agenthub.Agent) {
			defer wg.Done()
			var did bool
			if block {
				did = m.applyTo(ctx, a, p, now)
			} else {
				did = m.clearOn(ctx, a, has)
			}
			if did {
				cm.Lock()
				changed = true
				cm.Unlock()
			}
		}(a)
	}
	wg.Wait()
	if changed {
		m.d.Bus.Publish("focusblock.updated", m.statusNow(ctx))
	}
}

const (
	stateBlocking = "blocking"
	stateIdle     = "idle"
	stateError    = "error"
)

func (m *Module) applyTo(ctx context.Context, a agenthub.Agent, p plan, now time.Time) bool {
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	var st protocol.FocusBlockStatus
	err := m.d.Agents.Call(cctx, a.ID, protocol.MethodFocusBlockApply, protocol.FocusBlockApplyParams{
		SessionID: p.session.ID, TTLSeconds: p.ttl(now), Domains: p.opt.Domains, Apps: p.opt.Apps, IPv6: p.opt.IPv6,
	}, &st)
	row := db.SaveFocusBlockAgentParams{AgentID: a.ID, State: stateBlocking, SessionID: p.session.ID, ConfigHash: p.opt.hash(), UpdatedAt: time.Now().UTC()}
	if err != nil {
		row.State, row.SessionID, row.ConfigHash, row.Message = stateError, 0, "", err.Error()
		row.Refused = "[]"
	} else {
		row.HostsStatus, row.HostsError, row.Sites, row.Killed = st.Hosts, st.HostsError, int64(st.Sites), int64(st.Killed)
		row.Refused = refusedJSON(st.Refused)
		if !st.Active {
			row.State = stateIdle
		}
	}
	if serr := m.q.SaveFocusBlockAgent(ctx, row); serr != nil {
		m.d.Log.Warn("focus block: save state", "err", serr)
	}
	m.d.Audit.Record(ctx, "focusblock.apply", a.ID, map[string]any{"session": p.session.ID, "hosts": row.HostsStatus, "killed": row.Killed}, err)
	return true
}

// clearOn tells an agent to restore. known is false when the panel has no
// record of the agent, which then needs no record afterwards.
func (m *Module) clearOn(ctx context.Context, a agenthub.Agent, known bool) bool {
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	var st protocol.FocusBlockStatus
	err := m.d.Agents.Call(cctx, a.ID, protocol.MethodFocusBlockClear, nil, &st)
	row := db.SaveFocusBlockAgentParams{AgentID: a.ID, State: stateIdle, Refused: "[]", UpdatedAt: time.Now().UTC()}
	if err != nil {
		row.State, row.Message = stateError, err.Error()
	} else if st.Hosts != protocol.FocusBlockHostsOK {
		// The block could not be taken out of the hosts file. The agent keeps trying.
		row.State, row.HostsStatus, row.HostsError = stateError, st.Hosts, st.HostsError
		row.Message = "hosts 文件没能恢复，代理会继续重试"
	}
	if known || err != nil || row.State == stateError {
		if serr := m.q.SaveFocusBlockAgent(ctx, row); serr != nil {
			m.d.Log.Warn("focus block: save state", "err", serr)
		}
	}
	if known {
		m.d.Audit.Record(ctx, "focusblock.clear", a.ID, nil, err)
	}
	return known
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func refusedJSON(list []string) string {
	if list == nil {
		list = []string{}
	}
	b, _ := json.Marshal(list)
	return string(b)
}

// ---- Home Assistant ----

// reconcileHA runs the start scene once per session and the end scene when the
// block is over. It reports whether it did anything.
func (m *Module) reconcileHA(ctx context.Context, p plan) bool {
	m.mu.Lock()
	sess := m.haSess
	m.mu.Unlock()
	switch {
	case p.active && sess != p.session.ID:
		m.mu.Lock()
		m.haSess = p.session.ID
		m.mu.Unlock()
		if p.opt.HAStart != "" {
			m.runHA(ctx, p.opt.HAStart)
			return true
		}
	case !p.active && sess != 0:
		m.mu.Lock()
		m.haSess = 0
		m.mu.Unlock()
		if p.opt.HAEnd != "" {
			m.runHA(ctx, p.opt.HAEnd)
			return true
		}
	}
	return false
}

func (m *Module) runHA(ctx context.Context, entity string) {
	m.mu.Lock()
	call := m.haCall
	m.mu.Unlock()
	err := call(ctx, entity)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.haMsg = fmt.Sprintf("%s 没有打开：%v", entity, err)
		m.d.Log.Warn("focus block: home assistant", "entity", entity, "err", err)
		return
	}
	m.haMsg = fmt.Sprintf("%s 已打开（%s）", entity, time.Now().In(m.d.Config.Location).Format("15:04"))
}

// callHA turns a scene or script on through the Home Assistant action.
func (m *Module) callHA(ctx context.Context, entity string) error {
	domain := entity[:indexDot(entity)]
	in, _ := json.Marshal(map[string]string{"domain": domain, "service": "turn_on", "entityId": entity})
	_, err := m.d.Actions.Run(ctx, "ha.call_service", in)
	return err
}

func indexDot(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return i
		}
	}
	return len(s)
}

// ---- views ----

func (m *Module) computers(ctx context.Context, o options, active bool) ([]api.FocusBlockComputer, error) {
	agents, err := m.d.Agents.List(ctx)
	if err != nil {
		return nil, err
	}
	rows := map[string]db.FocusBlockAgent{}
	list, err := m.q.ListFocusBlockAgents(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range list {
		rows[r.AgentID] = r
	}
	out := make([]api.FocusBlockComputer, 0, len(agents))
	for _, a := range agents {
		c := api.FocusBlockComputer{Id: a.ID, Name: a.Name, Os: a.Os, Online: a.Online, Supported: a.Has(protocol.CapFocusBlock),
			Selected: contains(o.HostIDs, a.ID), Refused: []string{}}
		row, has := rows[a.ID]
		if has {
			c.Sites, c.Killed = int(row.Sites), int(row.Killed)
			t := row.UpdatedAt
			c.UpdatedAt = &t
			_ = json.Unmarshal([]byte(row.Refused), &c.Refused)
			if c.Refused == nil {
				c.Refused = []string{}
			}
		}
		c.State, c.Message = computerState(c, row, has, active)
		out = append(out, c)
	}
	return out, nil
}

// computerState says in one word and one sentence how a computer is doing.
func computerState(c api.FocusBlockComputer, row db.FocusBlockAgent, has, active bool) (api.FocusBlockComputerState, string) {
	switch {
	case !c.Supported:
		return api.Unsupported, "代理版本太旧，更新代理后才能用"
	case !c.Selected:
		return api.Off, ""
	case !c.Online:
		if has && row.State == stateBlocking {
			return api.Offline, "不在线。代理会在预定时间自己恢复"
		}
		return api.Offline, "不在线"
	case !has || row.State == stateIdle:
		if active {
			return api.Idle, "等待生效"
		}
		return api.Idle, "专注开始时生效"
	case row.State == stateError:
		return api.Failed, errorText(row)
	}
	switch row.HostsStatus {
	case protocol.FocusBlockHostsPermission:
		return api.Permission, "没有改 hosts 文件的权限，网站没有屏蔽。请用管理员（Windows）或 root（Linux）运行代理。程序名单仍然有效"
	case protocol.FocusBlockHostsFailed:
		return api.Failed, "hosts 文件写入失败：" + row.HostsError + "。程序名单仍然有效"
	case protocol.FocusBlockHostsNone:
		return api.Active, fmt.Sprintf("已生效。只结束程序，已结束 %d 个", row.Killed)
	}
	return api.Active, fmt.Sprintf("已生效。已屏蔽 %d 个网址名，已结束 %d 个程序", row.Sites, row.Killed)
}

func errorText(row db.FocusBlockAgent) string {
	if row.Message != "" && row.HostsStatus == "" {
		return "代理没有回应：" + row.Message
	}
	if row.HostsError != "" {
		return row.Message + "：" + row.HostsError
	}
	return row.Message
}

func (m *Module) settingsView(ctx context.Context) (api.FocusBlockSettings, error) {
	o := m.options(ctx)
	p := m.plan(ctx, m.now())
	cs, err := m.computers(ctx, o, p.active)
	if err != nil {
		return api.FocusBlockSettings{}, err
	}
	m.mu.Lock()
	haMsg := m.haMsg
	m.mu.Unlock()
	return api.FocusBlockSettings{Enabled: o.Enabled, HostIds: o.HostIDs, Domains: o.Domains, Apps: o.Apps, Ipv6: o.IPv6,
		BlockInBreak: o.BlockInBreak, HaStart: o.HAStart, HaEnd: o.HAEnd, HaMessage: haMsg, Computers: cs,
		MaxDomains: protocol.FocusBlockMaxDomains, MaxApps: protocol.FocusBlockMaxApps}, nil
}

func (m *Module) statusNow(ctx context.Context) api.FocusBlockStatus {
	o := m.options(ctx)
	p := m.plan(ctx, m.now())
	out := api.FocusBlockStatus{}
	cs, err := m.computers(ctx, o, p.active)
	if err != nil {
		return out
	}
	for _, c := range cs {
		switch c.State {
		case api.Active:
			out.ActiveComputers++
		case api.Permission:
			out.ActiveComputers++
			out.ProblemComputers++
		case api.Failed, api.Offline:
			if p.active && p.opt.lists() {
				out.ProblemComputers++
			}
		}
	}
	out.Blocking = out.ActiveComputers > 0
	return out
}

// ---- HTTP handlers ----

func (m *Module) GetFocusBlockSettings(w http.ResponseWriter, r *http.Request) {
	out, err := m.settingsView(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) PutFocusBlockSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.PutFocusBlockSettingsJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	o, err := m.merge(ctx, m.options(ctx), body)
	if err == nil {
		err = m.d.Settings.Set(ctx, SettingsKey, o)
	}
	m.d.Audit.Record(ctx, "focusblock.settings", "", map[string]any{"enabled": o.Enabled, "hosts": len(o.HostIDs), "domains": len(o.Domains), "apps": len(o.Apps)}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.kick()
	m.d.Bus.Publish("focusblock.updated", map[string]any{})
	out, err := m.settingsView(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) GetFocusBlockStatus(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, m.statusNow(r.Context()))
}

func (m *Module) GetFocusBlockSuggestions(w http.ResponseWriter, r *http.Request) {
	out := []string{}
	src, ok := module.Lookup[contracts.ScreenTimeApps](m.d.Registry, contracts.ScreenTimeAppsKey)
	if !ok {
		httpx.JSON(w, http.StatusOK, map[string]any{"apps": out})
		return
	}
	apps, err := src.AppsOfCategory(r.Context(), "entertainment")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	seen := map[string]bool{}
	for _, a := range apps {
		n, err := protocol.NormalizeBlockApp(a)
		if err != nil || protocol.IsProtectedProcess(n) || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"apps": out})
}
