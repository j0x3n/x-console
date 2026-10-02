// Package aiagents is B47: AI agents (Claude Code, Codex or the built-in
// assistant with a fixed brief) and the Git connections (GitHub, Forgejo)
// they use. The coding module runs their tasks; see docs/specs/B47.md.
//
// "Agent" here is the AI agent. The paired machine programs are called
// agents in the rest of the code (table agents, agenthub); this module calls
// them runners.
package aiagents

import (
	"context"
	"net/http"
	"sync"
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

	mu         sync.Mutex
	ctx        context.Context // from Start
	running    map[int64]int   // built-in agent jobs by agent id
	wg         sync.WaitGroup
	runMu      sync.Mutex
	activeRuns map[int64]context.CancelFunc
	finishMu   sync.Mutex // finishRun
	decisionMu sync.Mutex
	decisions  map[int64]chan decisionReply
}

var (
	_ api.ServerInterface      = (*Module)(nil)
	_ contracts.GitConnections = (*Module)(nil)
	_ contracts.AIAgents       = (*Module)(nil)
	_ module.Starter           = (*Module)(nil)
	_ module.PublicPather      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), hc: &http.Client{Timeout: 30 * time.Second},
		now: func() time.Time { return time.Now().UTC() }, running: map[int64]int{}, activeRuns: map[int64]context.CancelFunc{}, decisions: map[int64]chan decisionReply{}}
	module.Provide[contracts.GitConnections](d.Registry, contracts.GitConnectionsKey, m)
	module.Provide[contracts.AIAgents](d.Registry, contracts.AIAgentsKey, m)
	module.Provide[contracts.GitAccounts](d.Registry, contracts.GitAccountsKey, m) // B62
	module.Provide[contracts.GitIssues](d.Registry, contracts.GitIssuesKey, m)
	module.Provide[contracts.AgentRuns](d.Registry, contracts.AgentRunsKey, m)
	d.Notify.OnAction("ai_agent.", m.notificationAction)
	module.Provide[contracts.CodingQuestions](d.Registry, contracts.CodingQuestionsKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "aiagents" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
	r.Post("/hooks/git/{connectionId}", m.hook)
}

// PublicPaths implements module.PublicPather: Git services call the
// webhook without a session; it checks their signature.
func (m *Module) PublicPaths() []string { return []string{"/hooks/git/"} }

// Start follows coding tasks to comment on their cards.
func (m *Module) Start(ctx context.Context) error {
	if err := m.recoverRuns(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	go m.follow(ctx)
	return nil
}

func (m *Module) base() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ctx
}
