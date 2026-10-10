// Package screentime keeps the minute-by-minute record of which program was in
// front on the user's Windows computers and shows where the time went (B116).
// The agent sends one sample per minute; this module classifies it and keeps
// the program name and category. The window title is dropped unless the user
// asked to keep it. See docs/specs/B116.md.
package screentime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/screentime/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/screentime/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	// SettingsKey is where the switches are kept.
	SettingsKey = "screentime.settings"

	keepMinutesFor = 400 * 24 * 60 // records
	keepTitlesFor  = 30 * 24 * 60  // window titles
	maxPast        = 24 * 60       // samples older than a day are dropped
	maxFuture      = 5             // so are samples more than 5 minutes ahead
	maxApp         = 120
	maxTitle       = 300
	maxPattern     = 100
	maxRules       = 200
	maxHosts       = 100
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "screentime.module"

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now}
	d.Agents.OnEvent(protocol.EventScreenSample, m.onSample)
	m.registerActions()
	module.Provide[*Module](d.Registry, ServiceKey, m)
	module.Provide[contracts.ActivitySource](d.Registry, contracts.ActivitySourcePrefix+"screentime", activitySource{m}) // B118
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "screentime" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the clean-up.
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("screentime.cleanup", time.Hour, m.cleanup)
	return nil
}

func (m *Module) now() time.Time { return m.nowFn() }

func (m *Module) location() *time.Location {
	if loc := m.d.Scheduler.Location(); loc != nil {
		return loc
	}
	return time.Local
}

// ---------- settings ----------

type state struct {
	Enabled       bool     `json:"enabled"`
	KeepTitles    bool     `json:"keepTitles"`
	DisabledHosts []string `json:"disabledHosts"`
}

func (m *Module) loadState(ctx context.Context) (state, error) {
	st := state{Enabled: true, DisabledHosts: []string{}}
	if err := m.d.Settings.Get(ctx, SettingsKey, &st); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return st, err
	}
	if st.DisabledHosts == nil {
		st.DisabledHosts = []string{}
	}
	return st, nil
}

func (m *Module) loadRules(ctx context.Context) ([]rule, error) {
	rows, err := m.q.ListScreenRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rule, len(rows))
	for i, r := range rows {
		out[i] = rule{field: r.Field, pattern: r.Pattern, category: r.Category}
	}
	return out, nil
}

// ---------- ingest ----------

func clip(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

// onSample stores one minute reported by an agent.
func (m *Module) onSample(agentID string, raw json.RawMessage) {
	ctx := context.Background()
	var s protocol.ScreenSample
	if json.Unmarshal(raw, &s) != nil {
		return
	}
	app := strings.TrimSpace(s.App)
	if app == "" {
		return
	}
	nowMinute := m.now().Unix() / 60
	if s.Minute < nowMinute-maxPast || s.Minute > nowMinute+maxFuture {
		return
	}
	st, err := m.loadState(ctx)
	if err != nil || !st.Enabled || slices.Contains(st.DisabledHosts, agentID) {
		return
	}
	a, err := m.d.Agents.Get(ctx, agentID)
	if err != nil || !a.Has(protocol.CapScreenTime) {
		return
	}
	rules, err := m.loadRules(ctx)
	if err != nil {
		return
	}
	title := ""
	if st.KeepTitles {
		title = clip(s.Title, maxTitle)
	}
	// The category is decided on the full title even when it is not kept.
	category := classify(rules, clip(app, maxApp), clip(s.Title, maxTitle))
	_ = m.q.SaveScreenMinute(ctx, db.SaveScreenMinuteParams{HostID: agentID, Minute: s.Minute, App: clip(app, maxApp), Category: category, Title: title})
}

func (m *Module) cleanup(ctx context.Context) error {
	now := m.now().Unix() / 60
	if err := m.q.DeleteScreenMinutesBefore(ctx, now-keepMinutesFor); err != nil {
		return err
	}
	return m.q.ClearScreenTitlesBefore(ctx, now-keepTitlesFor)
}

// reclassify gives stored minutes the category the current rules would give
// them. appScope (a normalised program name) limits it to one program.
// titlesOnly leaves minutes without a title alone: their category may have
// come from the title, which is gone.
func (m *Module) reclassify(ctx context.Context, appScope string, titlesOnly bool) error {
	rules, err := m.loadRules(ctx)
	if err != nil {
		return err
	}
	rows, err := m.q.ScreenDistinctApps(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		n := normApp(row.App)
		if appScope != "" && n != appScope {
			continue
		}
		var category string
		if row.Title != "" {
			category = classify(rules, row.App, row.Title)
		} else {
			if titlesOnly {
				continue
			}
			c, ok := customApp(rules, row.App)
			if !ok {
				c, ok = builtinApp(row.App)
			}
			if !ok && !browsers[n] {
				c, ok = catOther, true
			}
			if !ok {
				continue
			}
			category = c
		}
		if err := m.q.ReclassifyScreenMinutes(ctx, db.ReclassifyScreenMinutesParams{Category: category, App: row.App, Title: row.Title, Category_2: category}); err != nil {
			return err
		}
	}
	return nil
}

// ---------- settings API ----------

func (m *Module) settingsView(ctx context.Context) (api.ScreenTimeSettings, error) {
	st, err := m.loadState(ctx)
	if err != nil {
		return api.ScreenTimeSettings{}, err
	}
	agents, err := m.d.Agents.List(ctx)
	if err != nil {
		return api.ScreenTimeSettings{}, err
	}
	hosts := []api.ScreenTimeHost{}
	for _, a := range agents {
		if !a.Has(protocol.CapScreenTime) {
			continue
		}
		hosts = append(hosts, api.ScreenTimeHost{Id: a.ID, Name: a.Name, Online: a.Online, Supported: true, Enabled: !slices.Contains(st.DisabledHosts, a.ID)})
	}
	return api.ScreenTimeSettings{Enabled: st.Enabled, KeepTitles: st.KeepTitles, Hosts: hosts}, nil
}

// GetScreenTimeSettings implements api.ServerInterface.
func (m *Module) GetScreenTimeSettings(w http.ResponseWriter, r *http.Request) {
	v, err := m.settingsView(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

// SetScreenTimeSettings implements api.ServerInterface.
func (m *Module) SetScreenTimeSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body api.SetScreenTimeSettingsJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(body.DisabledHosts) > maxHosts {
		httpx.Fail(w, r, httpx.Invalid("关掉的电脑太多"))
		return
	}
	old, err := m.loadState(ctx)
	if err == nil {
		next := state{Enabled: body.Enabled, KeepTitles: body.KeepTitles, DisabledHosts: body.DisabledHosts}
		err = m.d.Settings.Set(ctx, SettingsKey, next)
		if err == nil && old.KeepTitles && !body.KeepTitles {
			err = m.q.ClearScreenTitles(ctx)
		}
	}
	m.d.Audit.Record(ctx, "screentime.settings", "", map[string]any{"enabled": body.Enabled, "keepTitles": body.KeepTitles}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.GetScreenTimeSettings(w, r)
}

// ---------- rules API ----------

func ruleView(r db.ScreenRule) api.ScreenTimeRule {
	return api.ScreenTimeRule{Id: r.ID, Field: api.ScreenTimeRuleField(r.Field), Pattern: r.Pattern, Category: api.ScreenCategory(r.Category), CreatedAt: r.CreatedAt}
}

// ListScreenTimeRules implements api.ServerInterface.
func (m *Module) ListScreenTimeRules(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListScreenRules(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := make([]api.ScreenTimeRule, len(rows))
	for i, row := range rows {
		items[i] = ruleView(row)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// CreateScreenTimeRule implements api.ServerInterface.
func (m *Module) CreateScreenTimeRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body api.CreateScreenTimeRuleJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	pattern := strings.TrimSpace(body.Pattern)
	switch {
	case body.Field != "app" && body.Field != "title":
		httpx.Fail(w, r, httpx.Invalid("规则要按程序名或标题"))
		return
	case pattern == "" || utf8.RuneCountInString(pattern) > maxPattern:
		httpx.Fail(w, r, httpx.Invalid("匹配的文字要写，最长 100 个字"))
		return
	case !validCategory(string(body.Category)):
		httpx.Fail(w, r, httpx.Invalid("不认识这个类别"))
		return
	}
	existing, err := m.q.ListScreenRules(ctx)
	if err == nil && len(existing) >= maxRules {
		httpx.Fail(w, r, httpx.Invalid("规则最多 200 条"))
		return
	}
	var row db.ScreenRule
	if err == nil {
		row, err = m.q.InsertScreenRule(ctx, db.InsertScreenRuleParams{Field: string(body.Field), Pattern: pattern, Category: string(body.Category), CreatedAt: m.now()})
	}
	if err == nil {
		err = m.reclassifyFor(ctx, row.Field, row.Pattern)
	}
	m.d.Audit.Record(ctx, "screentime.rule.create", strconv.FormatInt(row.ID, 10), map[string]any{"field": row.Field, "pattern": row.Pattern, "category": row.Category}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, ruleView(row))
}

// reclassifyFor reclassifies what a rule on field and pattern can touch.
func (m *Module) reclassifyFor(ctx context.Context, field, pattern string) error {
	if field == "app" {
		return m.reclassify(ctx, normApp(pattern), false)
	}
	return m.reclassify(ctx, "", true)
}

// DeleteScreenTimeRule implements api.ServerInterface.
func (m *Module) DeleteScreenTimeRule(w http.ResponseWriter, r *http.Request, ruleId int64) {
	ctx := r.Context()
	rows, err := m.q.ListScreenRules(ctx)
	var gone *db.ScreenRule
	if err == nil {
		for i := range rows {
			if rows[i].ID == ruleId {
				gone = &rows[i]
			}
		}
		if gone == nil {
			err = httpx.ErrNotFound
		}
	}
	if err == nil {
		_, err = m.q.DeleteScreenRule(ctx, ruleId)
	}
	if err == nil {
		err = m.reclassifyFor(ctx, gone.Field, gone.Pattern)
	}
	m.d.Audit.Record(ctx, "screentime.rule.delete", strconv.FormatInt(ruleId, 10), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// ClearScreenTimeData implements api.ServerInterface.
func (m *Module) ClearScreenTimeData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err := m.q.DeleteAllScreenMinutes(ctx)
	m.d.Audit.Record(ctx, "screentime.clear", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}
