package automations

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/automations/api"
	"github.com/j0x3n/x-console/backend/internal/server/scheduler"
)

type Module struct {
	d    *module.Deps
	mu   sync.Mutex
	jobs map[int64]scheduler.EntryID
	last map[int64]time.Time
	ctx  context.Context
	// eventRules are the enabled rules that bus events can start. reload
	// refreshes them, so handling an event does not read the database.
	eventRules []api.Automation
	// eventRuns holds recent event-started run times per rule (see loopLimits).
	eventRuns map[int64][]time.Time
	// metricHigh: "rule:host" whose metric condition held on the last sample.
	metricHigh map[string]bool
}

var _ api.ServerInterface = (*Module)(nil)
var _ module.Starter = (*Module)(nil)
var _ module.PublicPather = (*Module)(nil)

func New(d *module.Deps) (module.Module, error) {
	return &Module{d: d, jobs: map[int64]scheduler.EntryID{}, last: map[int64]time.Time{}, eventRuns: map[int64][]time.Time{}, metricHigh: map[string]bool{}}, nil
}
func (m *Module) Name() string          { return "automations" }
func (m *Module) PublicPaths() []string { return []string{"/hooks"} }
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
	r.Post("/hooks/{token}", m.hook)
}
func (m *Module) Start(ctx context.Context) error {
	m.ctx = ctx
	if err := m.reload(ctx); err != nil {
		return err
	}
	ch, cancel := m.d.Bus.Subscribe("", 512)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				m.onEvent(ctx, ev)
			}
		}
	}()
	return nil
}
func (m *Module) fail(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	httpx.Fail(w, r, err)
	return true
}
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.ErrNotFound
	}
	return err
}
func (m *Module) onEvent(ctx context.Context, ev events.Event) { m.handleEvent(ctx, ev) }
