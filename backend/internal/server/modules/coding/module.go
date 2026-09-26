// Package coding is M4: coding tasks. It keeps the registered repositories
// and the task queue, runs tasks on an agent (coding.run stream), stores and
// forwards their output, and lets you review, commit, push, open a pull
// request or discard the result. See docs/modules/M4.md.
package coding

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/db"
)

// Module implements api.ServerInterface and contracts.Coding.
type Module struct {
	d   *module.Deps
	q   *db.Queries
	now func() time.Time

	// flushEvery is how often output events are stored and published.
	flushEvery time.Duration
	// cancelWait is how long a canceled task may take to stop on the agent
	// (interrupt, 10 s grace, kill, worktree cleanup) before the server
	// closes the stream itself.
	cancelWait time.Duration
	// overtime is added to a task's timeout before the server gives up on
	// an agent that never reports the end.
	overtime time.Duration
	// timeoutUnit is what one "timeout minute" means; tests use seconds.
	timeoutUnit time.Duration

	mu   sync.Mutex
	ctx  context.Context // from Start; running tasks live as long as it
	runs map[int64]*taskRun
	wg   sync.WaitGroup

	dispatchMu sync.Mutex
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ contracts.Coding    = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module and registers contracts.Coding and the actions.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{
		d: d, q: db.New(d.DB), now: func() time.Time { return time.Now().UTC() },
		flushEvery: 200 * time.Millisecond, cancelWait: 30 * time.Second, overtime: 2 * time.Minute, timeoutUnit: time.Minute,
		runs: map[int64]*taskRun{},
	}
	module.Provide[contracts.Coding](d.Registry, contracts.CodingKey, m)
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "coding" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start fails tasks a previous server process left running, starts queued
// tasks and starts more whenever an agent comes online.
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	if err := m.failStale(ctx); err != nil {
		return err
	}
	events, cancel := m.d.Bus.Subscribe("agent.online", 16)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-events:
				if !ok {
					return
				}
				m.dispatch()
			}
		}
	}()
	go m.dispatch()
	return nil
}

// failStale marks tasks that were running when the server stopped. Their
// agent lost the stream and removed the worktree already.
func (m *Module) failStale(ctx context.Context) error {
	now := m.now()
	ids, err := m.q.FailRunning(ctx, db.FailRunningParams{Error: "服务重启了，任务被中断。", Now: &now})
	if err != nil {
		return err
	}
	for _, id := range ids {
		if t, err := m.task(ctx, id); err == nil {
			m.d.Bus.Publish("coding_task.updated", t)
		}
	}
	if len(ids) > 0 {
		slog.Info("coding: marked interrupted tasks as failed", "count", len(ids))
	}
	return nil
}

// runCtx is the context running tasks use; nil before Start.
func (m *Module) runCtx() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ctx
}
