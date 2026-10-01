// Package router is B65: an OpenWrt main router, read over its ubus HTTP
// endpoint. Nothing is installed on the router. See docs/specs/B65.md.
package router

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Settings keys.
const (
	keyURL      = "router.url"
	keyUsername = "router.username"
	keyPassword = "router.password" // encrypted
	keyAgentID  = "router.agent_id"
)

// ServiceKey finds the module in module.Registry, for tests.
const ServiceKey = "router.service"

// Module implements the HTTP API.
type Module struct {
	d      *module.Deps
	q      *db.Queries
	direct *direct
	now    func() time.Time

	mu     sync.Mutex
	client *ubus  // built from the saved config, reset when it changes
	live   sample // last counters read by GET /router/status, for the rate
	poll   sample // last counters read by the poller, for the traffic table
	seen   map[string]time.Time
	cached *clientsCache
	watch  wanWatch
	quiet  time.Time // no outage notification before this, set by a reboot
	pruned time.Time
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), direct: newDirect(), now: time.Now, seen: map[string]time.Time{}}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "router" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start runs the poller: traffic every minute and WAN alerts.
func (m *Module) Start(ctx context.Context) error {
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.Poll(ctx)
			}
		}
	}()
	return nil
}

// config is the stored connection setup.
type config struct {
	URL      string
	Username string
	Password string
	AgentID  string // empty means direct
}

func (c config) configured() bool { return c.URL != "" && c.Username != "" }

func (c config) mode() api.RouterMode {
	if c.AgentID != "" {
		return api.Agent
	}
	return api.Direct
}

func (m *Module) loadConfig(ctx context.Context) (config, error) {
	var c config
	for key, dst := range map[string]*string{keyURL: &c.URL, keyUsername: &c.Username, keyPassword: &c.Password, keyAgentID: &c.AgentID} {
		if err := m.d.Settings.Get(ctx, key, dst); err != nil && !errors.Is(err, settings.ErrNotSet) {
			return config{}, err
		}
	}
	return c, nil
}

// newClient builds a ubus client for cfg.
func (m *Module) newClient(cfg config) *ubus {
	var t transport = m.direct
	if cfg.AgentID != "" {
		t = &viaAgent{hub: m.d.Agents, agentID: cfg.AgentID}
	}
	return &ubus{url: cfg.URL + "/ubus", username: cfg.Username, password: cfg.Password, t: t}
}

// ubus returns the client for the saved config, or ErrIntegrationMissing.
func (m *Module) ubus(ctx context.Context) (*ubus, error) {
	m.mu.Lock()
	c := m.client
	m.mu.Unlock()
	if c != nil {
		return c, nil
	}
	cfg, err := m.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.configured() {
		return nil, httpx.ErrIntegrationMissing
	}
	c = m.newClient(cfg)
	m.mu.Lock()
	if m.client == nil {
		m.client = c
	}
	c = m.client
	m.mu.Unlock()
	return c, nil
}

// reset drops the client and every remembered reading after a config change.
func (m *Module) reset() {
	m.mu.Lock()
	m.client = nil
	m.live = sample{}
	m.poll = sample{}
	m.seen = map[string]time.Time{}
	m.cached = nil
	m.watch = wanWatch{}
	m.mu.Unlock()
}

// normalizeURL validates the router address and strips a trailing slash.
func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw != "" && !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", httpx.Invalid("地址格式不对，应该像 http://192.168.1.1")
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", httpx.Invalid("地址里不要带查询参数、锚点或用户名")
	}
	return strings.TrimRight(strings.TrimSuffix(strings.TrimRight(u.String(), "/"), "/ubus"), "/"), nil
}

// checkAgent validates the agent used in agent mode.
func (m *Module) checkAgent(ctx context.Context, agentID string) error {
	if agentID == "" {
		return httpx.Invalid("请选择一个代理")
	}
	a, err := m.d.Agents.Get(ctx, agentID)
	if errors.Is(err, httpx.ErrNotFound) {
		return httpx.Invalid("这个代理不存在或已吊销")
	}
	if err != nil {
		return err
	}
	if !a.Has(protocol.CapProxy) {
		return httpx.Invalid("这个代理不支持转发。请把代理程序升级到新版本")
	}
	return nil
}
