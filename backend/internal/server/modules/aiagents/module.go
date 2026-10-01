// Package aiagents is B47: AI agents (Claude Code, Codex or the built-in
// assistant with a fixed brief) and the Git connections (GitHub, Forgejo)
// they use. The coding module runs their tasks; see docs/specs/B47.md.
//
// "Agent" here is the AI agent. The paired machine programs are called
// agents in the rest of the code (table agents, agenthub); this module calls
// them runners.
package aiagents

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/db"
)

// Module implements api.ServerInterface, contracts.GitConnections and
// contracts.AIAgents.
type Module struct {
	d   *module.Deps
	q   *db.Queries
	hc  *http.Client
	now func() time.Time
}

var (
	_ api.ServerInterface      = (*Module)(nil)
	_ contracts.GitConnections = (*Module)(nil)
	_ contracts.AIAgents       = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), hc: &http.Client{Timeout: 30 * time.Second},
		now: func() time.Time { return time.Now().UTC() }}
	module.Provide[contracts.GitConnections](d.Registry, contracts.GitConnectionsKey, m)
	module.Provide[contracts.AIAgents](d.Registry, contracts.AIAgentsKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "aiagents" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}
