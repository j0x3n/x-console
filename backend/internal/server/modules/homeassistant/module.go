// Package homeassistant is M9: a live connection to Home Assistant over its
// WebSocket API, an in-memory cache of every entity state, favorites, and
// service calls. See docs/specs/M9.md.
package homeassistant

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/homeassistant/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/homeassistant/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Settings keys.
const (
	keyURL     = "ha.url"
	keyToken   = "ha.token" // encrypted
	keyAgentID = "ha.agent_id"
)

// Module implements the HTTP API and contracts.HomeAssistant.
type Module struct {
	d      *module.Deps
	q      *db.Queries
	direct *direct
	c      *client
}

var (
	_ api.ServerInterface     = (*Module)(nil)
	_ contracts.HomeAssistant = (*Module)(nil)
	_ module.Starter          = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), direct: newDirect()}
	m.c = newClient(d.Log.With("module", "homeassistant"), d.Bus, m.loadConfig, m.transport)
	module.Provide[contracts.HomeAssistant](d.Registry, contracts.HomeAssistantKey, m)
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "homeassistant" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start loads favorites and runs the connection in the background.
func (m *Module) Start(ctx context.Context) error {
	favs, err := m.q.ListFavorites(ctx)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(favs))
	for _, f := range favs {
		ids = append(ids, f.EntityID)
	}
	m.c.setFavorites(ids)
	go m.c.run(ctx)
	m.watchAgents(ctx)
	return nil
}

// transport picks how to reach HA for cfg.
func (m *Module) transport(cfg config) transport {
	if cfg.AgentID != "" {
		return &viaAgent{hub: m.d.Agents, agentID: cfg.AgentID}
	}
	return m.direct
}

// config is the stored connection setup.
type config struct {
	URL     string
	Token   string
	AgentID string // empty means direct
}

func (c config) configured() bool { return c.URL != "" && c.Token != "" }

func (c config) mode() api.HAMode {
	if c.AgentID != "" {
		return api.Agent
	}
	return api.Direct
}

func (m *Module) loadConfig(ctx context.Context) (config, error) {
	var c config
	for key, dst := range map[string]*string{keyURL: &c.URL, keyToken: &c.Token, keyAgentID: &c.AgentID} {
		if err := m.d.Settings.Get(ctx, key, dst); err != nil && !errors.Is(err, settings.ErrNotSet) {
			return config{}, err
		}
	}
	return c, nil
}

// requireConfigured returns ErrIntegrationMissing when HA is not set up.
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

// normalizeURL validates the base URL and strips a trailing slash.
func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", httpx.Invalid("地址格式不对，应该像 http://homeassistant.local:8123")
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", httpx.Invalid("地址里不要带查询参数、锚点或用户名")
	}
	return strings.TrimRight(u.String(), "/"), nil
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
