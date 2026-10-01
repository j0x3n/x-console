// Package github is the GitHub half of M13: a cache of open pull requests,
// workflow runs and issues of watched repositories, refreshed every minute, CI failure notifications, links from pull requests to local
// issues and coding tasks, and contracts.GitHub for opening pull requests.
// See docs/specs/M13.md.
package github

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Settings keys.
const (
	keyToken    = "github.token" // encrypted
	keyRepos    = "github.repos"
	keyAPIURL   = "github.api_url"
	keyLogin    = "github.login"
	keyLastSync = "github.last_sync"
	// B62: the Git account (aiagents connection) whose token this module uses.
	keyConnectionID = "github.connection_id"
	keyMigratedB62  = "github.migrated_b62"
	keyWatches      = "github.watches"
	keyMigratedB70  = "github.migrated_b70"
)

const (
	defaultAPIURL = "https://api.github.com"
	syncInterval  = time.Minute
	// slowSyncInterval is used while the rate limit is nearly used up.
	slowSyncInterval = 5 * time.Minute
	maxRepos         = 50
)

// Module implements the HTTP API and contracts.GitHub.
type Module struct {
	d    *module.Deps
	q    *db.Queries
	log  *slog.Logger
	hc   *http.Client
	rate *rateState
	etag *etagCache
	now  func() time.Time

	repos       repoCache
	migrationMu sync.Mutex
	accountMu   sync.Mutex
	accounts    map[int64]*accountState
	jobs        jobsCache

	syncMu  sync.Mutex // one sync at a time
	stateMu sync.Mutex
	syncing bool
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ contracts.GitHub    = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module and offers contracts.GitHub.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{
		d: d, q: db.New(d.DB), log: d.Log.With("module", "github"),
		hc:       &http.Client{Timeout: 15 * time.Second},
		rate:     &rateState{remaining: -1},
		etag:     newETagCache(),
		accounts: map[int64]*accountState{},
		jobs:     jobsCache{entries: map[string]jobsEntry{}},
		now:      func() time.Time { return time.Now().UTC() },
	}
	module.Provide[contracts.GitHub](d.Registry, contracts.GitHubKey, m)
	module.Provide[contracts.GitHubCredentials](d.Registry, contracts.GitHubCredentialsKey, m) // B47
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "github" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the sync every minute.
func (m *Module) Start(ctx context.Context) error {
	if err := m.migrateB62(ctx); err != nil {
		m.log.Warn("github: moving the token into a Git account failed", "err", err)
	}
	if err := m.migrateB70(ctx); err != nil {
		return err
	}
	m.d.Scheduler.Every("github.sync", syncInterval, m.scheduledSync)
	return nil
}

// scheduledSync is one tick of the scheduler. While the rate limit is nearly
// used up it syncs at most every five minutes.
func (m *Module) scheduledSync(ctx context.Context) error {
	err := m.syncAccounts(ctx, true)
	if errors.Is(err, httpx.ErrIntegrationMissing) {
		return nil
	}
	return err
}

// lowQuota reports whether less than a tenth of the hourly rate limit is
// left and the limit has not reset yet.
func (m *Module) lowQuota() bool {
	remaining, limit, reset := m.rate.info()
	return remaining >= 0 && limit > 0 && remaining < limit/10 && reset.After(m.now())
}

// currentSyncInterval is how often the scheduled sync effectively runs now.
func (m *Module) currentSyncInterval() time.Duration {
	if m.lowQuota() {
		return slowSyncInterval
	}
	return syncInterval
}

// config is the stored setup. With a Git account (B62) the token, API
// address and login come from that account.
type config struct {
	Token        string
	Repos        []string
	APIURL       string
	Login        string
	ConnectionID int64
	Watches      []api.RepoWatch
	HasWatches   bool
	Forge        string
	Name         string
}

func (c config) configured() bool { return c.Token != "" || len(c.Watches) > 0 }

func (m *Module) loadConfig(ctx context.Context) (config, error) {
	c, err := m.loadLegacyConfig(ctx)
	if err != nil {
		return c, err
	}
	err = m.d.Settings.Get(ctx, keyWatches, &c.Watches)
	if err != nil && !errors.Is(err, settings.ErrNotSet) {
		return c, err
	}
	c.HasWatches = err == nil
	if !c.HasWatches {
		for _, r := range c.Repos {
			c.Watches = append(c.Watches, api.RepoWatch{ConnectionId: c.ConnectionID, Repo: r})
		}
	}
	if c.Watches == nil {
		c.Watches = []api.RepoWatch{}
	}
	return c, nil
}

func (m *Module) loadLegacyConfig(ctx context.Context) (config, error) {
	var c config
	get := func(key string, dst any) error {
		if err := m.d.Settings.Get(ctx, key, dst); err != nil && !errors.Is(err, settings.ErrNotSet) {
			return err
		}
		return nil
	}
	for key, dst := range map[string]any{keyToken: &c.Token, keyRepos: &c.Repos, keyAPIURL: &c.APIURL, keyLogin: &c.Login} {
		if err := get(key, dst); err != nil {
			return config{}, err
		}
	}
	if c.APIURL == "" {
		c.APIURL = defaultAPIURL
	}
	if c.Repos == nil {
		c.Repos = []string{}
	}
	if err := get(keyConnectionID, &c.ConnectionID); err != nil {
		return config{}, err
	}
	if c.ConnectionID != 0 {
		m.fromAccount(ctx, &c)
	}
	return c, nil
}

// fromAccount fills the token, API address and login from the Git account.
// A broken account (deleted, no token) leaves the module unconfigured
// instead of failing every request.
func (m *Module) fromAccount(ctx context.Context, c *config) {
	c.Token, c.Login = "", ""
	accounts, ok := module.Lookup[contracts.GitAccounts](m.d.Registry, contracts.GitAccountsKey)
	if !ok {
		return
	}
	base, token, err := accounts.Credentials(ctx, c.ConnectionID)
	if err != nil {
		m.log.Warn("github account", "connection", c.ConnectionID, "err", err)
		return
	}
	c.APIURL, c.Token = base, token
	if a, err := accounts.Account(ctx, c.ConnectionID); err == nil {
		c.Login = a.Username
	}
}

// migrateB62 turns the token of the old GitHub settings into a Git account
// once (B62). The old setting stays until the next version.
func (m *Module) migrateB62(ctx context.Context) error {
	var done bool
	if err := m.d.Settings.Get(ctx, keyMigratedB62, &done); err == nil && done {
		return nil
	} else if err != nil && !errors.Is(err, settings.ErrNotSet) {
		return err
	}
	accounts, ok := module.Lookup[contracts.GitAccounts](m.d.Registry, contracts.GitAccountsKey)
	if !ok {
		return nil // no aiagents module: keep using the old token
	}
	cfg, err := m.loadConfig(ctx)
	if err != nil {
		return err
	}
	if cfg.ConnectionID == 0 && cfg.Token != "" {
		id, err := accounts.ImportGitHub(ctx, cfg.APIURL, cfg.Token, cfg.Login)
		if err != nil {
			return err
		}
		if err := m.d.Settings.Set(ctx, keyConnectionID, id); err != nil {
			return err
		}
	}
	return m.d.Settings.Set(ctx, keyMigratedB62, true)
}

// requireConfigured returns ErrIntegrationMissing when no token is stored.
func (m *Module) requireConfigured(ctx context.Context) (config, error) {
	cfg, err := m.loadConfig(ctx)
	if err != nil {
		return config{}, err
	}
	if !cfg.configured() {
		return config{}, httpx.ErrIntegrationMissing
	}
	return cfg, nil
}

// Credentials implements contracts.GitHubCredentials (B47).
func (m *Module) Credentials(ctx context.Context) (string, string, error) {
	cfg, err := m.requireConfigured(ctx)
	if err != nil {
		return "", "", err
	}
	return cfg.APIURL, cfg.Token, nil
}

func (m *Module) client(cfg config) *restClient {
	state := m.accountState(cfg)
	return &restClient{base: cfg.APIURL, token: cfg.Token, forge: cfg.Forge, hc: m.hc, rate: state.rate, etag: m.etag, now: m.now}
}

// normalizeAPIURL validates the REST base URL. Empty means the default.
func normalizeAPIURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultAPIURL, nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" {
		return "", httpx.Invalid("API 地址格式不对，应该像 https://api.github.com")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// normalizeRepo accepts owner/name or a github.com URL.
func normalizeRepo(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		s = strings.Trim(u.Path, "/")
	}
	s = strings.TrimSuffix(s, ".git")
	if !repoRe.MatchString(s) {
		return "", httpx.Invalid("仓库要写成 owner/name：" + raw)
	}
	return s, nil
}

func normalizeRepos(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range in {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		r, err := normalizeRepo(raw)
		if err != nil {
			return nil, err
		}
		if k := strings.ToLower(r); !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	if len(out) > maxRepos {
		return nil, httpx.Invalid("最多关注 50 个仓库")
	}
	return out, nil
}

func maskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) < 12 {
		return "••••••••"
	}
	return "••••••••" + token[len(token)-4:]
}

// keepInts turns an empty list into one that matches nothing real, because
// "NOT IN (NULL)" would match nothing at all.
func keepInts(ids []int64) []int64 {
	if len(ids) == 0 {
		return []int64{0}
	}
	return ids
}

func keepStrings(s []string) []string {
	if len(s) == 0 {
		return []string{""}
	}
	return s
}
