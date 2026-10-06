// Package quotas shows how much of the user's AI subscriptions is left
// (B110): Claude, Codex and Grok windows read by the agent on the machine the
// account is signed in on, and DeepSeek balances read by the server. Several
// accounts of one service are fine. See docs/specs/B110.md.
package quotas

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/quotas/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/quotas/db"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	// readEvery and claudeEvery are how often the scheduler reads an account.
	// Claude's takes a Claude Code process, so it is read less often.
	readEvery   = 5 * time.Minute
	claudeEvery = 15 * time.Minute
	// refreshAfterOK and refreshAfterFail are the least time between two
	// readings of one account when the user presses refresh.
	refreshAfterOK   = 5 * time.Minute
	refreshAfterFail = 30 * time.Second
	agentTimeout     = 90 * time.Second
	maxParallel      = 4
	maxName          = 60
)

// Error codes shown to the page. They say what to do about it.
const (
	codeOffline     = "offline"
	codeSignedOut   = "signed_out"
	codeUnavailable = "unavailable"
	codeUnsupported = "unsupported"
	codeHostMissing = "host_missing"
)

// ServiceKey is where the module registers itself, for tests and for modules
// that want the readings (B112 notifications).
const ServiceKey = "quotas.accounts"

// Module implements api.ServerInterface.
type Module struct {
	d   *module.Deps
	q   *db.Queries
	now func() time.Time

	// deepseekURL is DeepSeek's balance endpoint. Tests point it elsewhere.
	deepseekURL string
	client      *http.Client
	sem         chan struct{}

	mu    sync.Mutex
	locks map[int64]*sync.Mutex
	ctx   context.Context
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{
		d: d, q: db.New(d.DB), now: time.Now,
		deepseekURL: "https://api.deepseek.com/user/balance",
		client:      &http.Client{Timeout: 15 * time.Second},
		sem:         make(chan struct{}, maxParallel),
		locks:       map[int64]*sync.Mutex{},
		ctx:         context.Background(),
	}
	m.registerActions()
	module.Provide[*Module](d.Registry, ServiceKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "quotas" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the readings.
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	m.d.Scheduler.Every("quotas.read", time.Minute, func(ctx context.Context) error { return m.tick(ctx) })
	return nil
}

func (m *Module) background() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ctx
}

func (m *Module) lock(id int64) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.locks[id]
	if l == nil {
		l = &sync.Mutex{}
		m.locks[id] = l
	}
	return l
}

func interval(kind string) time.Duration {
	if kind == protocol.QuotaKindClaude {
		return claudeEvery
	}
	return readEvery
}

// tick reads every account that is due. An account on a machine that is off is
// marked so once, and read again as soon as the machine is back.
func (m *Module) tick(ctx context.Context) error {
	accts, err := m.q.ListQuotaAccounts(ctx)
	if err != nil {
		return err
	}
	rows, err := m.q.ListQuotaReadings(ctx)
	if err != nil {
		return err
	}
	byID := make(map[int64]db.QuotaReading, len(rows))
	for _, r := range rows {
		byID[r.AccountID] = r
	}
	var wg sync.WaitGroup
	for _, a := range accts {
		r, has := byID[a.ID]
		if a.HostID != "" && !m.d.Agents.Online(a.HostID) {
			if !has || r.ErrorCode != codeOffline {
				m.saveFailure(ctx, a.ID, codeOffline, "机器离线")
			}
			continue
		}
		wg.Add(1)
		go func(a db.QuotaAccount) {
			defer wg.Done()
			m.readIfDue(ctx, a, interval(a.Kind), interval(a.Kind))
		}(a)
	}
	wg.Wait()
	return nil
}

// readIfDue reads the account unless it was tried more recently than the age
// that applies: okAge after a good reading, failAge after a failed one. One
// reading of an account at a time.
func (m *Module) readIfDue(ctx context.Context, a db.QuotaAccount, okAge, failAge time.Duration) {
	l := m.lock(a.ID)
	l.Lock()
	defer l.Unlock()
	if r, err := m.q.GetQuotaReading(ctx, a.ID); err == nil {
		age := m.now().Sub(r.TriedAt)
		switch {
		case r.ErrorCode == codeOffline:
			// the machine is back or this would not be asked: read now
		case r.Ok == 1 && age < okAge, r.Ok == 0 && age < failAge:
			return
		}
	}
	select {
	case m.sem <- struct{}{}:
		defer func() { <-m.sem }()
	case <-ctx.Done():
		return
	}
	res, err := m.fetch(ctx, a)
	if err != nil {
		code, msg := classify(err)
		if code == codeUnavailable && !isKnown(err) {
			m.d.Log.Warn("quota read", "account", a.ID, "kind", a.Kind, "err", err)
		}
		m.saveFailure(ctx, a.ID, code, msg)
		return
	}
	m.saveReading(ctx, a.ID, res)
}

// result is what one reading found.
type result struct {
	plan, user, credits string
	windows             []api.QuotaWindow
	balances            []api.QuotaBalance
}

func (m *Module) fetch(ctx context.Context, a db.QuotaAccount) (result, error) {
	if a.Kind == "deepseek" {
		return m.fetchDeepSeek(ctx, a)
	}
	ag, err := m.d.Agents.Get(ctx, a.HostID)
	if err != nil {
		return result{}, &readError{code: codeHostMissing, msg: "这台机器已经不存在或被吊销了"}
	}
	if !m.d.Agents.Online(a.HostID) {
		return result{}, &readError{code: codeOffline, msg: "机器离线"}
	}
	if !ag.Has(protocol.CapQuota) {
		return result{}, &readError{code: codeUnsupported, msg: "这台机器的代理不支持读取额度，请升级代理"}
	}
	cctx, cancel := context.WithTimeout(ctx, agentTimeout)
	defer cancel()
	var out protocol.QuotaReading
	if err := m.d.Agents.Call(cctx, a.HostID, protocol.MethodQuotaRead, protocol.QuotaReadParams{Kind: a.Kind, Home: a.Home}, &out); err != nil {
		return result{}, err
	}
	res := result{plan: out.Plan, user: out.User, credits: out.Credits, windows: []api.QuotaWindow{}, balances: []api.QuotaBalance{}}
	for _, w := range out.Windows {
		aw := api.QuotaWindow{Name: w.Name, UsedPercent: w.UsedPercent, ResetsAt: w.ResetsAt}
		if w.SpanSecs > 0 {
			aw.SpanSecs = &w.SpanSecs
		}
		if w.Model != "" {
			model := w.Model
			aw.Model = &model
		}
		if w.Aside {
			aside := true
			aw.Aside = &aside
		}
		res.windows = append(res.windows, aw)
	}
	return res, nil
}

// readError is a failure whose code and message are already fit to show.
type readError struct{ code, msg string }

func (e *readError) Error() string { return e.code + ": " + e.msg }

func isKnown(err error) bool {
	var re *readError
	var he *httpx.Error
	return errors.As(err, &re) || errors.As(err, &he)
}

// classify turns an error into the code and message the page shows. The
// agent's messages are written to be shown and hold no token; any other error
// is logged and shown without its details.
func classify(err error) (code, msg string) {
	var re *readError
	if errors.As(err, &re) {
		return re.code, re.msg
	}
	var he *httpx.Error
	if errors.As(err, &he) {
		switch he.Code {
		case "agent_offline":
			return codeOffline, "机器离线"
		case "agent_" + protocol.CodeQuotaSignedOut:
			return codeSignedOut, he.Message
		case "agent_" + protocol.CodeQuotaUnavailable:
			return codeUnavailable, he.Message
		case "agent_" + protocol.CodeUnsupported, "agent_" + protocol.CodeUnknownMethod:
			return codeUnsupported, "这台机器的代理不支持读取额度，请升级代理"
		case "agent_timeout":
			return codeUnavailable, "读取超时"
		case "agent_bad_params":
			return codeUnavailable, "账号目录不合法：" + he.Message
		}
		return codeUnavailable, he.Message
	}
	return codeUnavailable, "读取失败，详情见服务器日志"
}

func (m *Module) saveReading(ctx context.Context, id int64, r result) {
	windows, _ := json.Marshal(nonNilWindows(r.windows))
	balances, _ := json.Marshal(nonNilBalances(r.balances))
	now := m.now().UTC()
	err := m.q.SaveQuotaReading(ctx, db.SaveQuotaReadingParams{
		AccountID: id, Plan: r.plan, User: r.user, Credits: r.credits,
		BalancesJson: string(balances), WindowsJson: string(windows), ReadAt: &now, TriedAt: now,
	})
	if err != nil {
		if ctx.Err() == nil {
			m.d.Log.Warn("quota save", "account", id, "err", err)
		}
		return
	}
	m.d.Bus.Publish("quota.updated", map[string]any{"id": id})
}

func (m *Module) saveFailure(ctx context.Context, id int64, code, msg string) {
	err := m.q.SaveQuotaFailure(ctx, db.SaveQuotaFailureParams{AccountID: id, Error: msg, ErrorCode: code, TriedAt: m.now().UTC()})
	if err != nil {
		if ctx.Err() == nil {
			m.d.Log.Warn("quota save", "account", id, "err", err)
		}
		return
	}
	m.d.Bus.Publish("quota.updated", map[string]any{"id": id})
}

func nonNilWindows(w []api.QuotaWindow) []api.QuotaWindow {
	if w == nil {
		return []api.QuotaWindow{}
	}
	return w
}

func nonNilBalances(b []api.QuotaBalance) []api.QuotaBalance {
	if b == nil {
		return []api.QuotaBalance{}
	}
	return b
}

// ---- views ----

type hostInfo struct {
	name   string
	online bool
	quota  bool
}

func (m *Module) hostMap(ctx context.Context) (map[string]hostInfo, error) {
	agents, err := m.d.Agents.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]hostInfo, len(agents))
	for _, a := range agents {
		out[a.ID] = hostInfo{name: a.Name, online: a.Online, quota: a.Has(protocol.CapQuota)}
	}
	return out, nil
}

func (m *Module) view(a db.QuotaAccount, r *db.QuotaReading, hosts map[string]hostInfo) api.QuotaAccount {
	v := api.QuotaAccount{
		Id: a.ID, Kind: api.QuotaKind(a.Kind), Name: a.Name, HostId: a.HostID, Home: a.Home,
		KeySet: a.ApiKey != "", Status: api.Pending, CreatedAt: a.CreatedAt,
		Windows: []api.QuotaWindow{}, Balances: []api.QuotaBalance{},
	}
	if a.HostID != "" {
		if h, ok := hosts[a.HostID]; ok {
			name, online := h.name, h.online
			v.HostName, v.HostOnline = &name, &online
		}
	}
	if r == nil {
		return v
	}
	if r.Ok == 1 {
		v.Status = api.Ok
	} else {
		v.Status = api.Error
		if r.Error != "" {
			e := r.Error
			v.Error = &e
		}
		if r.ErrorCode != "" {
			c := r.ErrorCode
			v.ErrorCode = &c
		}
	}
	if r.Plan != "" {
		p := r.Plan
		v.Plan = &p
	}
	if r.User != "" {
		u := r.User
		v.User = &u
	}
	if r.Credits != "" {
		c := r.Credits
		v.Credits = &c
	}
	_ = json.Unmarshal([]byte(r.WindowsJson), &v.Windows)
	_ = json.Unmarshal([]byte(r.BalancesJson), &v.Balances)
	v.Windows, v.Balances = nonNilWindows(v.Windows), nonNilBalances(v.Balances)
	v.ReadAt = r.ReadAt
	t := r.TriedAt
	v.TriedAt = &t
	return v
}

func (m *Module) views(ctx context.Context) ([]api.QuotaAccount, error) {
	accts, err := m.q.ListQuotaAccounts(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := m.q.ListQuotaReadings(ctx)
	if err != nil {
		return nil, err
	}
	hosts, err := m.hostMap(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*db.QuotaReading, len(rows))
	for i := range rows {
		byID[rows[i].AccountID] = &rows[i]
	}
	out := make([]api.QuotaAccount, 0, len(accts))
	for _, a := range accts {
		out = append(out, m.view(a, byID[a.ID], hosts))
	}
	return out, nil
}

func (m *Module) viewOne(ctx context.Context, id int64) (api.QuotaAccount, error) {
	a, err := m.q.GetQuotaAccount(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return api.QuotaAccount{}, httpx.ErrNotFound
	}
	if err != nil {
		return api.QuotaAccount{}, err
	}
	var r *db.QuotaReading
	if row, err := m.q.GetQuotaReading(ctx, id); err == nil {
		r = &row
	}
	hosts, err := m.hostMap(ctx)
	if err != nil {
		return api.QuotaAccount{}, err
	}
	return m.view(a, r, hosts), nil
}

// ---- handlers ----

// ListQuotaAccounts implements api.ServerInterface.
func (m *Module) ListQuotaAccounts(w http.ResponseWriter, r *http.Request) {
	items, err := m.views(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// ListQuotaHosts implements api.ServerInterface.
func (m *Module) ListQuotaHosts(w http.ResponseWriter, r *http.Request) {
	agents, err := m.d.Agents.List(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := []api.QuotaHost{}
	for _, a := range agents {
		if a.Has(protocol.CapQuota) {
			items = append(items, api.QuotaHost{Id: a.ID, Name: a.Name, Online: a.Online})
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// CreateQuotaAccount implements api.ServerInterface.
func (m *Module) CreateQuotaAccount(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.CreateQuotaAccountJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := m.create(r.Context(), body)
	m.d.Audit.Record(r.Context(), "quota_account.create", strconv.FormatInt(v.Id, 10), map[string]any{"kind": string(body.Kind), "name": body.Name, "hostId": deref(body.HostId), "home": deref(body.Home)}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, v)
}

func (m *Module) create(ctx context.Context, in api.QuotaAccountInput) (api.QuotaAccount, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > maxName {
		return api.QuotaAccount{}, httpx.Invalid("备注名不能为空，最长 60 个字")
	}
	if !in.Kind.Valid() {
		return api.QuotaAccount{}, httpx.Invalid("不认识的服务类型")
	}
	hostID, home, key := strings.TrimSpace(deref(in.HostId)), strings.TrimSpace(deref(in.Home)), strings.TrimSpace(deref(in.ApiKey))
	sealed, hash, err := m.checkIdentity(ctx, string(in.Kind), hostID, home, key)
	if err != nil {
		return api.QuotaAccount{}, err
	}
	row, err := m.q.InsertQuotaAccount(ctx, db.InsertQuotaAccountParams{
		Kind: string(in.Kind), Name: name, HostID: hostID, Home: home, ApiKey: sealed, KeyHash: hash, CreatedAt: m.now().UTC(),
	})
	if err != nil {
		return api.QuotaAccount{}, dupOr(err)
	}
	m.readSoon(row)
	return m.viewOne(ctx, row.ID)
}

// checkIdentity validates the machine, directory and key of an account and
// returns the key sealed, and its hash for telling duplicates apart.
func (m *Module) checkIdentity(ctx context.Context, kind, hostID, home, key string) (sealed, hash string, err error) {
	if kind == "deepseek" {
		if hostID != "" || home != "" {
			return "", "", httpx.Invalid("DeepSeek 不需要机器和目录")
		}
		if key == "" {
			return "", "", httpx.Invalid("请填写 DeepSeek 的 API Key")
		}
		if sealed, err = m.d.Secrets.Seal(key); err != nil {
			return "", "", err
		}
		return sealed, secrets.Hash(key), nil
	}
	if key != "" {
		return "", "", httpx.Invalid("只有 DeepSeek 需要 API Key")
	}
	if hostID == "" {
		return "", "", httpx.Invalid("请选择机器")
	}
	ag, err := m.d.Agents.Get(ctx, hostID)
	if err != nil {
		if errors.Is(err, httpx.ErrNotFound) {
			return "", "", httpx.Invalid("找不到这台机器")
		}
		return "", "", err
	}
	if !ag.Has(protocol.CapQuota) {
		return "", "", httpx.Invalid("这台机器的代理不能读取额度。请先在它上面安装 claude、codex 或 grok，并升级代理")
	}
	if !validHome(home) {
		return "", "", httpx.Invalid("登录目录要写绝对路径，或以 ~/ 开头，不能含 ..")
	}
	return "", "", nil
}

var winDrive = regexp.MustCompile(`^[A-Za-z]:[\\/]`)

// validHome is the server's first check of a directory. The agent checks it
// again with the rules of its own system.
func validHome(home string) bool {
	if home == "" {
		return true
	}
	for _, part := range strings.FieldsFunc(home, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return false
		}
	}
	return home == "~" || strings.HasPrefix(home, "~/") || strings.HasPrefix(home, `~\`) ||
		strings.HasPrefix(home, "/") || strings.HasPrefix(home, `\\`) || winDrive.MatchString(home)
}

func dupOr(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return httpx.NewError(http.StatusConflict, "conflict", "这个账号已经添加过了")
	}
	return err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// readSoon reads a new or changed account in the background, so its card fills
// in without waiting for the next tick.
func (m *Module) readSoon(a db.QuotaAccount) {
	ctx := m.background()
	go m.readIfDue(ctx, a, 0, 0)
}

// UpdateQuotaAccount implements api.ServerInterface.
func (m *Module) UpdateQuotaAccount(w http.ResponseWriter, r *http.Request, accountId api.AccountId) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.UpdateQuotaAccountJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := m.update(r.Context(), accountId, body)
	m.d.Audit.Record(r.Context(), "quota_account.update", strconv.FormatInt(accountId, 10), map[string]any{"name": deref(body.Name), "hostId": deref(body.HostId), "home": deref(body.Home), "keyChanged": body.ApiKey != nil}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) update(ctx context.Context, id int64, in api.QuotaAccountPatch) (api.QuotaAccount, error) {
	cur, err := m.q.GetQuotaAccount(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return api.QuotaAccount{}, httpx.ErrNotFound
	}
	if err != nil {
		return api.QuotaAccount{}, err
	}
	next := cur
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || len([]rune(name)) > maxName {
			return api.QuotaAccount{}, httpx.Invalid("备注名不能为空，最长 60 个字")
		}
		next.Name = name
	}
	if in.HostId != nil {
		next.HostID = strings.TrimSpace(*in.HostId)
	}
	if in.Home != nil {
		next.Home = strings.TrimSpace(*in.Home)
	}
	key := ""
	if in.ApiKey != nil {
		key = strings.TrimSpace(*in.ApiKey)
		if key == "" {
			return api.QuotaAccount{}, httpx.Invalid("API Key 不能为空")
		}
	}
	changed := next.HostID != cur.HostID || next.Home != cur.Home || key != ""
	if changed {
		// A different machine, directory or key is a different account: check
		// it as a new one. Without a new key the old one stays.
		checkKey := key
		if cur.Kind == "deepseek" && key == "" {
			checkKey = "keep"
		}
		sealed, hash, err := m.checkIdentity(ctx, cur.Kind, next.HostID, next.Home, checkKey)
		if err != nil {
			return api.QuotaAccount{}, err
		}
		if key != "" {
			next.ApiKey, next.KeyHash = sealed, hash
		}
	}
	row, err := m.q.UpdateQuotaAccount(ctx, db.UpdateQuotaAccountParams{
		Name: next.Name, HostID: next.HostID, Home: next.Home, ApiKey: next.ApiKey, KeyHash: next.KeyHash, ID: id,
	})
	if err != nil {
		return api.QuotaAccount{}, dupOr(err)
	}
	if changed {
		_ = m.q.DeleteQuotaReading(ctx, id)
		m.readSoon(row)
	}
	return m.viewOne(ctx, id)
}

// DeleteQuotaAccount implements api.ServerInterface.
func (m *Module) DeleteQuotaAccount(w http.ResponseWriter, r *http.Request, accountId api.AccountId) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err := m.remove(r.Context(), accountId)
	m.d.Audit.Record(r.Context(), "quota_account.delete", strconv.FormatInt(accountId, 10), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) remove(ctx context.Context, id int64) error {
	_ = m.q.DeleteQuotaReading(ctx, id)
	n, err := m.q.DeleteQuotaAccount(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// RefreshQuotaAccount implements api.ServerInterface.
func (m *Module) RefreshQuotaAccount(w http.ResponseWriter, r *http.Request, accountId api.AccountId) {
	a, err := m.q.GetQuotaAccount(r.Context(), accountId)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.readIfDue(r.Context(), a, refreshAfterOK, refreshAfterFail)
	v, err := m.viewOne(r.Context(), accountId)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

// ReorderQuotaAccounts implements api.ServerInterface.
func (m *Module) ReorderQuotaAccounts(w http.ResponseWriter, r *http.Request) {
	var body api.ReorderQuotaAccountsJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := m.reorder(r.Context(), body.Ids); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) reorder(ctx context.Context, ids []int64) error {
	accts, err := m.q.ListQuotaAccounts(ctx)
	if err != nil {
		return err
	}
	exists := make(map[int64]bool, len(accts))
	for _, a := range accts {
		exists[a.ID] = true
	}
	order := make([]int64, 0, len(accts))
	seen := map[int64]bool{}
	for _, id := range ids {
		if exists[id] && !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
	}
	for _, a := range accts {
		if !seen[a.ID] {
			order = append(order, a.ID)
		}
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	for i, id := range order {
		if err := q.SetQuotaOrder(ctx, db.SetQuotaOrderParams{SortOrder: int64(i + 1), ID: id}); err != nil {
			return err
		}
	}
	return tx.Commit()
}
