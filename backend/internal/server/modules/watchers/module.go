// Package watchers lets AI watch a web page for the user (B206). The user
// gives a site, login details and a goal in one sentence. The first time an
// AI agent drives the browser (internal/server/browser) to the value and
// records the steps. Later checks replay the steps without AI; AI is called
// only when the value changed, when the steps no longer work, or in "ai"
// mode. Captchas and QR logins are handed to the user through the takeover
// view. See docs/specs/B205-B208.md.
package watchers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/browser"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/db"
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "watchers.module"

// OptionsKey holds the module's settings.
const OptionsKey = "watchers.options"

const (
	maxWatchers      = 100
	defaultInterval  = 60
	minInterval      = 15
	maxInterval      = 7 * 24 * 60
	minKeepalive     = 5
	maxGoal          = 1000
	maxName          = 100
	defaultBudgetUSD = 1.0
	// failLimit is how many failed checks in a row stop the automatic repair
	// by AI; the user has to start a new exploration.
	failLimit = 3
	// askLimit is how many times a day one watcher may ask the user.
	askLimit = 3
)

// defaultDangerWords are the button texts that need approval before a click.
var defaultDangerWords = []string{"提交", "撤销", "撤回", "删除", "支付", "付款", "确认", "取消申请", "注销", "退出登录"}

// Module implements api.ServerInterface.
type Module struct {
	d       *module.Deps
	q       *db.Queries
	browser *browser.Service
	nowFn   func() time.Time
	// chatOverride and llmOverride replace the ai module in tests.
	chatOverride contracts.ToolChat
	llmOverride  contracts.LLM
	xOverride    contracts.XGraphQL
	xLimit       xLimiter
	waitFor      time.Duration // how long a run waits for the user

	mu      sync.Mutex
	running map[int64]*runState
	bg      sync.WaitGroup
	ctx     context.Context
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), browser: browser.FromEnv(d.Log), nowFn: time.Now,
		running: map[int64]*runState{}, waitFor: 30 * time.Minute, ctx: context.Background()}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	m.registerActions()
	d.Notify.OnAction("watcher.", m.onNotifyAction)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "watchers" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start marks runs cut off by a restart and schedules the checks.
func (m *Module) Start(ctx context.Context) error {
	m.ctx = ctx
	now := m.now()
	if err := m.q.FinishStaleRuns(ctx, db.FinishStaleRunsParams{Detail: "服务重启，没跑完", FinishedAt: &now}); err != nil {
		return err
	}
	if err := m.q.ResetBusyWatchers(ctx, "服务重启，上次没跑完"); err != nil {
		return err
	}
	m.d.Scheduler.Every("watchers.due", time.Minute, m.tick)
	m.d.Scheduler.Every("watchers.cleanup", 6*time.Hour, m.cleanup)
	go func() {
		<-ctx.Done()
		m.browser.Close()
	}()
	return nil
}

func (m *Module) now() time.Time { return m.nowFn() }

func (m *Module) chat() contracts.ToolChat {
	if m.chatOverride != nil {
		return m.chatOverride
	}
	c, _ := module.Lookup[contracts.ToolChat](m.d.Registry, contracts.ToolChatKey)
	return c
}

func (m *Module) llm() contracts.LLM {
	if m.llmOverride != nil {
		return m.llmOverride
	}
	c, _ := module.Lookup[contracts.LLM](m.d.Registry, contracts.LLMKey)
	return c
}

func (m *Module) timezone() string {
	if l := m.d.Config.Location; l != nil && l.String() != "Local" && l.String() != "" {
		return l.String()
	}
	return "Asia/Shanghai"
}

func (m *Module) startOfDay() time.Time {
	loc := m.d.Config.Location
	if loc == nil {
		loc = time.Local
	}
	now := m.now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
}

func fail(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	httpx.Fail(w, r, err)
	return true
}

func ptr[T any](v T) *T { return &v }

// ---- stored types ----

// Login is what the user gave for the site.
type Login struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Pending is what a watcher waits for.
type Pending struct {
	Kind    string    `json:"kind"`
	Message string    `json:"message"`
	Since   time.Time `json:"since"`
}

// Options are the module settings.
type Options struct {
	BudgetUSD   *float64          `json:"budgetUsd,omitempty"`
	DangerWords []string          `json:"dangerWords,omitempty"`
	XQueryIDs   map[string]string `json:"xQueryIds,omitempty"`
}

func (m *Module) options(ctx context.Context) (budget float64, words []string) {
	var o Options
	_ = m.d.Settings.Get(ctx, OptionsKey, &o)
	budget = defaultBudgetUSD
	if o.BudgetUSD != nil {
		budget = *o.BudgetUSD
	}
	words = o.DangerWords
	if o.DangerWords == nil {
		words = defaultDangerWords
	}
	return budget, words
}

func (m *Module) seal(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return m.d.Secrets.Seal(string(b))
}

func (m *Module) open(sealed string, v any) error {
	if sealed == "" {
		return nil
	}
	raw, err := m.d.Secrets.Open(sealed)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), v)
}

func (m *Module) login(w db.Watcher) *Login {
	var l Login
	if m.open(w.LoginSealed, &l) != nil || (l.Username == "" && l.Password == "") {
		return nil
	}
	return &l
}

func (m *Module) proxy(w db.Watcher) string {
	var p string
	_ = m.open(w.ProxySealed, &p)
	return p
}

func (m *Module) state(w db.Watcher) []byte {
	var s string
	_ = m.open(w.StateSealed, &s)
	return []byte(s)
}

func parseHosts(raw string) []string {
	var h []string
	_ = json.Unmarshal([]byte(raw), &h)
	return h
}

func (m *Module) get(ctx context.Context, id int64) (db.Watcher, error) {
	w, err := m.q.GetWatcher(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return w, httpx.ErrNotFound
	}
	return w, err
}

func (m *Module) publish(id int64) {
	m.d.Bus.Publish("watcher.updated", map[string]any{"id": id})
}

// ---- conversions ----

func (m *Module) toAPI(ctx context.Context, w db.Watcher) api.Watcher {
	out := api.Watcher{
		Id: w.ID, Kind: api.WatcherKind(w.Kind), Name: w.Name, Url: w.Url, Goal: w.Goal, AllowedHosts: parseHosts(w.AllowedHosts),
		IntervalMinutes: int(w.IntervalMinutes), KeepaliveMinutes: int(w.KeepaliveMinutes), Mode: api.WatcherMode(w.Mode),
		Enabled: w.Enabled == 1, Status: api.WatcherStatus(w.Status), StatusDetail: w.StatusDetail, CurrentValue: w.CurrentValue,
		HasLogin: w.LoginSealed != "", HasProxy: w.ProxySealed != "", HasRecipe: w.Recipe != "" && w.Kind != "x",
		LastCheckedAt: w.LastCheckedAt, LastChangeAt: w.LastChangeAt, NextCheckAt: w.NextCheckAt, CreatedAt: w.CreatedAt,
	}
	if out.AllowedHosts == nil {
		out.AllowedHosts = []string{}
	}
	if !out.Enabled {
		out.Status = api.WatcherStatusPaused
	}
	if w.Pending != "" {
		var p Pending
		if json.Unmarshal([]byte(w.Pending), &p) == nil {
			out.Pending = &api.WatcherPending{Kind: api.WatcherPendingKind(p.Kind), Message: p.Message, Since: p.Since,
				Live: ptr(m.browser.Live(sessionKey(w.ID)) != nil)}
		}
	}
	if c := m.chat(); c != nil {
		if v, err := c.Spent(ctx, "watchers", strconv.FormatInt(w.ID, 10), m.now().AddDate(0, 0, -30)); err == nil {
			out.Spent30d = &v
		}
	}
	return out
}

func sessionKey(id int64) string { return "watcher:" + strconv.FormatInt(id, 10) }

// ---- validation ----

type fields struct {
	kind             string
	name, url, goal  string
	hosts            []string
	interval, keepAl int
	mode             string
}

func (f *fields) check() error {
	f.name = strings.TrimSpace(f.name)
	f.goal = strings.TrimSpace(f.goal)
	f.url = strings.TrimSpace(f.url)
	if f.name == "" || utf8.RuneCountInString(f.name) > maxName {
		return httpx.Invalid("名字要写，最多 100 个字")
	}
	if f.kind == "x" {
		return f.checkX()
	}
	if f.goal == "" || utf8.RuneCountInString(f.goal) > maxGoal {
		return httpx.Invalid("目标要写一句话，最多 1000 个字")
	}
	u, err := url.Parse(f.url)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return httpx.Invalid("网址要以 http:// 或 https:// 开头")
	}
	var hosts []string
	for _, h := range f.hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		hosts = []string{baseDomain(u.Hostname())}
	}
	if len(hosts) > 20 {
		return httpx.Invalid("允许的域名最多 20 个")
	}
	if !browser.HostAllowed(hosts, f.url) {
		return httpx.Invalid("网址的域名不在允许的域名里")
	}
	f.hosts = hosts
	if f.interval == 0 {
		f.interval = defaultInterval
	}
	if f.interval < minInterval || f.interval > maxInterval {
		return httpx.Invalid("检查间隔最少 15 分钟，最多 7 天")
	}
	if f.keepAl != 0 && (f.keepAl < minKeepalive || f.keepAl > maxInterval) {
		return httpx.Invalid("保活间隔最少 5 分钟")
	}
	if f.mode == "" {
		f.mode = string(api.Replay)
	}
	if f.mode != string(api.Replay) && f.mode != string(api.Ai) {
		return httpx.Invalid("模式只能是 replay 或 ai")
	}
	return nil
}

// checkX validates an X watcher (B207). The goal is an optional filter.
func (f *fields) checkX() error {
	if utf8.RuneCountInString(f.goal) > maxGoal {
		return httpx.Invalid("过滤条件最多 1000 个字")
	}
	_, canonical, err := parseXTarget(f.url)
	if err != nil {
		return err
	}
	f.url, f.hosts, f.mode = canonical, []string{"x.com"}, string(api.Replay)
	if f.interval == 0 {
		f.interval = xDefaultInterval
	}
	if f.interval < minInterval || f.interval > maxInterval {
		return httpx.Invalid("检查间隔最少 15 分钟，最多 7 天")
	}
	f.keepAl = 0
	return nil
}

// baseDomain keeps the last two labels of a host, or three for the common
// two-part suffixes such as gov.cn, so www.example.gov.cn allows
// example.gov.cn and its subdomains.
func baseDomain(host string) string {
	host = strings.ToLower(host)
	if net := strings.Count(host, "."); net < 2 || isIP(host) {
		return host
	}
	parts := strings.Split(host, ".")
	n := 2
	second := parts[len(parts)-2]
	switch second {
	case "gov", "com", "net", "org", "edu", "co", "ac":
		if len(parts[len(parts)-1]) == 2 {
			n = 3
		}
	}
	if n > len(parts) {
		n = len(parts)
	}
	return strings.Join(parts[len(parts)-n:], ".")
}

func isIP(h string) bool {
	for _, c := range h {
		if (c < '0' || c > '9') && c != '.' {
			return false
		}
	}
	return true
}

func checkProxy(p string) error {
	if p == "" {
		return nil
	}
	u, err := url.Parse(p)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") {
		return httpx.Invalid("代理地址要像 http://主机:端口 或 socks5://主机:端口")
	}
	return nil
}

var queryIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// ---- handlers ----

// ListWatchers implements api.ServerInterface.
func (m *Module) ListWatchers(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListWatchers(r.Context())
	if fail(w, r, err) {
		return
	}
	items := make([]api.Watcher, 0, len(rows))
	for _, row := range rows {
		items = append(items, m.toAPI(r.Context(), row))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// CreateWatcher implements api.ServerInterface.
func (m *Module) CreateWatcher(w http.ResponseWriter, r *http.Request) {
	var body api.WatcherInput
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if body.Username != nil || body.Password != nil || (body.Proxy != nil && *body.Proxy != "") {
		if fail(w, r, auth.RequireElevated(r.Context())) {
			return
		}
	}
	row, err := m.create(r.Context(), body)
	m.d.Audit.Record(r.Context(), "watcher.create", "", map[string]any{"url": body.Url, "login": body.Username != nil}, err)
	if fail(w, r, err) {
		return
	}
	m.publish(row.ID)
	m.startRun(row.ID, "explore")
	httpx.JSON(w, http.StatusCreated, m.toAPI(r.Context(), row))
}

func (m *Module) create(ctx context.Context, body api.WatcherInput) (db.Watcher, error) {
	f := fields{kind: "web", name: body.Name, url: body.Url, goal: body.Goal, mode: ""}
	if body.Kind != nil {
		if *body.Kind != api.Web && *body.Kind != api.X {
			return db.Watcher{}, httpx.Invalid("kind 只能是 web 或 x")
		}
		f.kind = string(*body.Kind)
	}
	if body.AllowedHosts != nil {
		f.hosts = *body.AllowedHosts
	}
	if body.IntervalMinutes != nil {
		f.interval = *body.IntervalMinutes
	}
	if body.KeepaliveMinutes != nil {
		f.keepAl = *body.KeepaliveMinutes
	}
	if body.Mode != nil {
		f.mode = string(*body.Mode)
	}
	if err := f.check(); err != nil {
		return db.Watcher{}, err
	}
	rows, err := m.q.ListWatchers(ctx)
	if err != nil {
		return db.Watcher{}, err
	}
	if len(rows) >= maxWatchers {
		return db.Watcher{}, httpx.Invalid("最多 100 个盯梢任务")
	}
	var loginSealed, proxySealed string
	if body.Username != nil || body.Password != nil {
		l := Login{}
		if body.Username != nil {
			l.Username = *body.Username
		}
		if body.Password != nil {
			l.Password = *body.Password
		}
		if loginSealed, err = m.seal(l); err != nil {
			return db.Watcher{}, err
		}
	}
	if body.Proxy != nil && *body.Proxy != "" {
		if err := checkProxy(*body.Proxy); err != nil {
			return db.Watcher{}, err
		}
		if proxySealed, err = m.seal(*body.Proxy); err != nil {
			return db.Watcher{}, err
		}
	}
	hosts, _ := json.Marshal(f.hosts)
	now := m.now()
	if f.kind == "x" && (loginSealed != "" || proxySealed != "") {
		return db.Watcher{}, httpx.Invalid("X 用稍后读里存的 Cookie，不用填登录资料和代理")
	}
	return m.q.CreateWatcher(ctx, db.CreateWatcherParams{Kind: f.kind, Name: f.name, Url: f.url, Goal: f.goal,
		AllowedHosts: string(hosts), IntervalMinutes: int64(f.interval), KeepaliveMinutes: int64(f.keepAl), Mode: f.mode,
		LoginSealed: loginSealed, ProxySealed: proxySealed, NextCheckAt: &now, CreatedAt: now, UpdatedAt: now})
}

// GetWatcher implements api.ServerInterface.
func (m *Module) GetWatcher(w http.ResponseWriter, r *http.Request, id api.WatcherId) {
	row, err := m.get(r.Context(), id)
	if fail(w, r, err) {
		return
	}
	base := m.toAPI(r.Context(), row)
	var out api.WatcherDetail
	b, _ := json.Marshal(base)
	_ = json.Unmarshal(b, &out)
	out.Changes = []api.WatcherChange{}
	out.Runs = []api.WatcherRun{}
	out.Recipe = []api.WatcherRecipeStep{}
	changes, err := m.q.ListChanges(r.Context(), id)
	if fail(w, r, err) {
		return
	}
	for _, c := range changes {
		out.Changes = append(out.Changes, api.WatcherChange{Id: c.ID, OldValue: c.OldValue, NewValue: c.NewValue,
			Summary: c.Summary, Important: c.Important == 1, ShotId: c.ShotID, CreatedAt: c.CreatedAt})
	}
	runs, err := m.q.ListRuns(r.Context(), id)
	if fail(w, r, err) {
		return
	}
	for _, run := range runs {
		item := api.WatcherRun{Id: run.ID, Kind: api.WatcherRunKind(run.Kind), StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
			Outcome: api.WatcherRunOutcome(run.Outcome), Detail: run.Detail, Tokens: run.Tokens, Steps: []api.WatcherStep{}}
		_ = json.Unmarshal([]byte(run.Steps), &item.Steps)
		if item.Steps == nil {
			item.Steps = []api.WatcherStep{}
		}
		out.Runs = append(out.Runs, item)
	}
	if rec, ok := parseRecipe(row.Recipe); ok {
		for _, st := range rec.Steps {
			out.Recipe = append(out.Recipe, api.WatcherRecipeStep{Text: st.Note})
		}
		out.TargetNote = ptr(rec.TargetNote)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// UpdateWatcher implements api.ServerInterface.
func (m *Module) UpdateWatcher(w http.ResponseWriter, r *http.Request, id api.WatcherId) {
	var body api.WatcherPatch
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if body.Username != nil || body.Password != nil || body.Proxy != nil || (body.ClearLogin != nil && *body.ClearLogin) {
		if fail(w, r, auth.RequireElevated(r.Context())) {
			return
		}
	}
	row, err := m.update(r.Context(), id, body)
	m.d.Audit.Record(r.Context(), "watcher.update", strconv.FormatInt(id, 10),
		map[string]any{"login": body.Username != nil || body.Password != nil, "proxy": body.Proxy != nil}, err)
	if fail(w, r, err) {
		return
	}
	m.publish(id)
	httpx.JSON(w, http.StatusOK, m.toAPI(r.Context(), row))
}

func (m *Module) update(ctx context.Context, id int64, body api.WatcherPatch) (db.Watcher, error) {
	cur, err := m.get(ctx, id)
	if err != nil {
		return cur, err
	}
	if m.busy(id) && (body.Url != nil || body.Goal != nil || body.Username != nil || body.Password != nil || body.ClearLogin != nil) {
		return cur, httpx.NewError(http.StatusConflict, "busy", "任务正在跑，等它跑完再改")
	}
	f := fields{kind: cur.Kind, name: cur.Name, url: cur.Url, goal: cur.Goal, hosts: parseHosts(cur.AllowedHosts),
		interval: int(cur.IntervalMinutes), keepAl: int(cur.KeepaliveMinutes), mode: cur.Mode}
	if body.Name != nil {
		f.name = *body.Name
	}
	if body.Url != nil {
		f.url = *body.Url
		if body.AllowedHosts == nil {
			f.hosts = nil // follow the new address
		}
	}
	if body.Goal != nil {
		f.goal = *body.Goal
	}
	if body.AllowedHosts != nil {
		f.hosts = *body.AllowedHosts
	}
	if body.IntervalMinutes != nil {
		f.interval = *body.IntervalMinutes
	}
	if body.KeepaliveMinutes != nil {
		f.keepAl = *body.KeepaliveMinutes
	}
	if body.Mode != nil {
		f.mode = string(*body.Mode)
	}
	if err := f.check(); err != nil {
		return cur, err
	}
	loginSealed, proxySealed, stateSealed, recipe := cur.LoginSealed, cur.ProxySealed, cur.StateSealed, cur.Recipe
	if body.ClearLogin != nil && *body.ClearLogin {
		loginSealed, stateSealed = "", ""
	}
	if body.Username != nil || body.Password != nil {
		l := Login{}
		if old := m.login(cur); old != nil {
			l = *old
		}
		if body.Username != nil {
			l.Username = *body.Username
		}
		if body.Password != nil {
			l.Password = *body.Password
		}
		if loginSealed, err = m.seal(l); err != nil {
			return cur, err
		}
	}
	if body.Proxy != nil {
		if err := checkProxy(*body.Proxy); err != nil {
			return cur, err
		}
		proxySealed = ""
		if *body.Proxy != "" {
			if proxySealed, err = m.seal(*body.Proxy); err != nil {
				return cur, err
			}
		}
	}
	next := cur.NextCheckAt
	if f.url != cur.Url || (f.goal != cur.Goal && cur.Kind != "x") {
		// The old steps lead to the old value; explore again soon.
		recipe = ""
		now := m.now()
		next = &now
	}
	enabled := cur.Enabled
	if body.Enabled != nil {
		enabled = 0
		if *body.Enabled {
			enabled = 1
			if cur.Enabled == 0 {
				now := m.now()
				next = &now
			}
		}
	}
	hosts, _ := json.Marshal(f.hosts)
	err = m.q.UpdateWatcherSettings(ctx, db.UpdateWatcherSettingsParams{Name: f.name, Url: f.url, Goal: f.goal,
		AllowedHosts: string(hosts), IntervalMinutes: int64(f.interval), KeepaliveMinutes: int64(f.keepAl), Mode: f.mode,
		Enabled: enabled, LoginSealed: loginSealed, ProxySealed: proxySealed, StateSealed: stateSealed, Recipe: recipe,
		NextCheckAt: next, UpdatedAt: m.now(), ID: id})
	if err != nil {
		return cur, err
	}
	return m.get(ctx, id)
}

// DeleteWatcher implements api.ServerInterface.
func (m *Module) DeleteWatcher(w http.ResponseWriter, r *http.Request, id api.WatcherId) {
	if _, err := m.get(r.Context(), id); fail(w, r, err) {
		return
	}
	m.cancelRun(id)
	err := m.q.DeleteWatcher(r.Context(), id)
	m.d.Audit.Record(r.Context(), "watcher.delete", strconv.FormatInt(id, 10), nil, err)
	if fail(w, r, err) {
		return
	}
	m.publish(id)
	w.WriteHeader(http.StatusNoContent)
}

// RunWatcher implements api.ServerInterface.
func (m *Module) RunWatcher(w http.ResponseWriter, r *http.Request, id api.WatcherId) {
	var body struct {
		Kind string `json:"kind"`
	}
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	switch body.Kind {
	case "check", "explore", "manual":
	default:
		fail(w, r, httpx.Invalid("kind 只能是 check、explore 或 manual"))
		return
	}
	if _, err := m.get(r.Context(), id); fail(w, r, err) {
		return
	}
	if !m.startRun(id, body.Kind) {
		fail(w, r, httpx.NewError(http.StatusConflict, "busy", "任务正在跑"))
		return
	}
	m.d.Audit.Record(r.Context(), "watcher.run", strconv.FormatInt(id, 10), map[string]any{"kind": body.Kind}, nil)
	w.WriteHeader(http.StatusAccepted)
}

// AnswerWatcher implements api.ServerInterface.
func (m *Module) AnswerWatcher(w http.ResponseWriter, r *http.Request, id api.WatcherId) {
	var body struct {
		Answer string `json:"answer"`
	}
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	switch body.Answer {
	case "done", "approve", "deny", "cancel":
	default:
		fail(w, r, httpx.Invalid("answer 只能是 done、approve、deny 或 cancel"))
		return
	}
	if !m.answer(id, body.Answer) {
		fail(w, r, httpx.NewError(http.StatusConflict, "not_waiting", "这个任务现在没有在等你"))
		return
	}
	m.d.Audit.Record(r.Context(), "watcher.answer", strconv.FormatInt(id, 10), map[string]any{"answer": body.Answer}, nil)
	w.WriteHeader(http.StatusNoContent)
}

// GetWatcherShot implements api.ServerInterface.
func (m *Module) GetWatcherShot(w http.ResponseWriter, r *http.Request, shotID int64) {
	data, err := m.q.GetShot(r.Context(), shotID)
	if errors.Is(err, sql.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	if fail(w, r, err) {
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(data)
}

// GetWatchersStatus implements api.ServerInterface.
func (m *Module) GetWatchersStatus(w http.ResponseWriter, r *http.Request) {
	budget, _ := m.options(r.Context())
	out := api.WatchersStatus{BrowserConfigured: m.browser.Configured(), BudgetUsd: budget}
	if out.BrowserConfigured {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		_, err := m.browser.Check(ctx)
		cancel()
		out.Browser = err == nil
		if err != nil {
			out.BrowserError = ptr(err.Error())
		}
	}
	if c := m.chat(); c != nil {
		out.Ai = c.Available(r.Context(), "agent")
		out.SpentToday, _ = c.Spent(r.Context(), "watchers", "", m.startOfDay())
	}
	if x := m.xClient(); x != nil {
		out.X = x.Ready(r.Context())
	}
	httpx.JSON(w, http.StatusOK, out)
}

// GetWatchersSettings implements api.ServerInterface.
func (m *Module) GetWatchersSettings(w http.ResponseWriter, r *http.Request) {
	budget, words := m.options(r.Context())
	ids := m.xQueryIDs(r.Context())
	httpx.JSON(w, http.StatusOK, api.WatchersSettings{BudgetUsd: budget, DangerWords: words, XQueryIds: &ids})
}

// PutWatchersSettings implements api.ServerInterface.
func (m *Module) PutWatchersSettings(w http.ResponseWriter, r *http.Request) {
	var body api.WatchersSettings
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if body.BudgetUsd < 0 || body.BudgetUsd > 100 {
		fail(w, r, httpx.Invalid("每天的花费上限在 0 到 100 美元之间"))
		return
	}
	words := []string{}
	for _, x := range body.DangerWords {
		if x = strings.TrimSpace(x); x != "" && len(words) < 50 {
			words = append(words, x)
		}
	}
	opts := Options{BudgetUSD: &body.BudgetUsd, DangerWords: words}
	if body.XQueryIds != nil {
		opts.XQueryIDs = map[string]string{}
		for k, v := range *body.XQueryIds {
			v = strings.TrimSpace(v)
			if _, known := defaultXQueryIDs[k]; !known || v == "" || v == defaultXQueryIDs[k] {
				continue
			}
			if !queryIDRe.MatchString(v) {
				fail(w, r, httpx.Invalid(k+" 的 queryId 格式不对"))
				return
			}
			opts.XQueryIDs[k] = v
		}
	} else {
		var old Options
		_ = m.d.Settings.Get(r.Context(), OptionsKey, &old)
		opts.XQueryIDs = old.XQueryIDs
	}
	err := m.d.Settings.Set(r.Context(), OptionsKey, opts)
	m.d.Audit.Record(r.Context(), "watcher.settings", "", map[string]any{"budget": body.BudgetUsd}, err)
	if fail(w, r, err) {
		return
	}
	ids := m.xQueryIDs(r.Context())
	httpx.JSON(w, http.StatusOK, api.WatchersSettings{BudgetUsd: body.BudgetUsd, DangerWords: words, XQueryIds: &ids})
}

// cleanup drops old runs and screenshots that no change points at.
func (m *Module) cleanup(ctx context.Context) error {
	before := m.now().AddDate(0, 0, -30)
	if _, err := m.d.DB.ExecContext(ctx, `DELETE FROM watcher_runs WHERE started_at < ? AND id NOT IN (
 SELECT id FROM (SELECT id, ROW_NUMBER() OVER (PARTITION BY watcher_id ORDER BY id DESC) AS n FROM watcher_runs) WHERE n <= 20)`, before); err != nil {
		return err
	}
	_, err := m.d.DB.ExecContext(ctx, `DELETE FROM watcher_shots WHERE created_at < ?
 AND id NOT IN (SELECT shot_id FROM watcher_changes WHERE shot_id IS NOT NULL)`, m.now().AddDate(0, 0, -7))
	return err
}
