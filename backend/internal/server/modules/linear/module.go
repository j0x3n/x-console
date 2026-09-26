// Package linear is the Linear half of M13: two-way sync between Linear
// teams and local projects. It reads and writes local issues only through
// contracts.IssueSync. See docs/specs/M13.md.
package linear

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/linear/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/linear/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Settings keys.
const (
	keyAPIKey      = "linear.api_key" // encrypted
	keyAPIURL      = "linear.api_url"
	keyLastSync    = "linear.last_sync"
	keyLocalCursor = "linear.local_cursor"
)

const (
	defaultAPIURL = "https://api.linear.app/graphql"
	syncInterval  = 5 * time.Minute
	source        = "linear"
)

// Module implements the HTTP API.
type Module struct {
	d   *module.Deps
	q   *db.Queries
	log *slog.Logger
	hc  *http.Client
	now func() time.Time

	mu      sync.Mutex // one sync or push at a time
	stateMu sync.Mutex
	syncing bool
	states  map[string][]lnState // team id → workflow states
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	return &Module{
		d: d, q: db.New(d.DB), log: d.Log.With("module", "linear"),
		hc:     &http.Client{Timeout: 15 * time.Second},
		now:    func() time.Time { return time.Now().UTC() },
		states: map[string][]lnState{},
	}, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "linear" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start pulls every five minutes and pushes local changes as they happen.
func (m *Module) Start(ctx context.Context) error {
	m.d.Scheduler.Every("linear.sync", syncInterval, func(ctx context.Context) error {
		_, err := m.sync(ctx)
		if errors.Is(err, httpx.ErrIntegrationMissing) {
			return nil
		}
		return err
	})
	ch, cancel := m.d.Bus.Subscribe("issue.", 256)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-ch:
				m.onIssueEvent(ctx, ev.Topic, ev.Data)
			}
		}
	}()
	return nil
}

// issueEvent is the part of the M5 event payloads the push needs.
type issueEvent struct {
	Key            string `json:"key"`
	ExternalSource string `json:"externalSource"`
	ExternalID     string `json:"externalId"`
}

// onIssueEvent pushes a local change to Linear. issue.synced is what our
// own Upsert publishes, so it is ignored: pushing it back would echo.
func (m *Module) onIssueEvent(ctx context.Context, topic string, data any) {
	switch topic {
	case "issue.updated", "issue.created", "issue.status_changed":
	default:
		return
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	var ev issueEvent
	if err := json.Unmarshal(raw, &ev); err != nil || ev.Key == "" {
		return
	}
	if topic != "issue.status_changed" && ev.ExternalSource != source {
		return
	}
	if err := m.pushKey(ctx, ev.Key, ev.ExternalID); err != nil && !errors.Is(err, httpx.ErrIntegrationMissing) {
		m.log.Warn("linear push failed", "issue", ev.Key, "err", err)
		m.recordError(ctx, ev.Key+"："+err.Error())
	}
}

// config is the stored setup.
type config struct {
	Key    string
	APIURL string
}

func (c config) configured() bool { return c.Key != "" }

func (m *Module) loadConfig(ctx context.Context) (config, error) {
	var c config
	for key, dst := range map[string]*string{keyAPIKey: &c.Key, keyAPIURL: &c.APIURL} {
		if err := m.d.Settings.Get(ctx, key, dst); err != nil && !errors.Is(err, settings.ErrNotSet) {
			return config{}, err
		}
	}
	if c.APIURL == "" {
		c.APIURL = defaultAPIURL
	}
	return c, nil
}

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

func (m *Module) client(cfg config) *gqlClient {
	return &gqlClient{url: cfg.APIURL, key: cfg.Key, hc: m.hc}
}

func (m *Module) issueSync() (contracts.IssueSync, error) {
	s, ok := module.Lookup[contracts.IssueSync](m.d.Registry, contracts.IssueSyncKey)
	if !ok {
		return nil, httpx.NewError(501, "feature_unavailable", "项目模块未启用")
	}
	return s, nil
}

func normalizeAPIURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultAPIURL, nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", httpx.Invalid("API 地址格式不对，应该像 https://api.linear.app/graphql")
	}
	return u.String(), nil
}

func maskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) < 12 {
		return "••••••••"
	}
	return "••••••••" + key[len(key)-4:]
}

func (m *Module) setSyncing(v bool) {
	m.stateMu.Lock()
	m.syncing = v
	m.stateMu.Unlock()
}

func (m *Module) isSyncing() bool {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	return m.syncing
}
