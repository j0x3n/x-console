// Package goals keeps the quarterly goals and the weekly review (B128). A goal
// has key results. A key result is filled in by hand, or measured from data
// other modules already have: habit check-ins, workouts, focus minutes,
// computer time and GitHub activity. The module compares the progress with how
// far the quarter has gone, reminds the user on Sunday evening to review the
// week, and keeps one review per week.
// See docs/specs/B128.md.
package goals

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals/db"
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "goals.module"

const (
	dateLayout = "2006-01-02"

	maxTitle   = 200
	maxName    = 100
	maxUnit    = 20
	maxNote    = 5000
	maxResults = 20
	maxGoals   = 200
)

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now}
	m.registerActions()
	module.Provide[*Module](d.Registry, ServiceKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "goals" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start checks every minute whether it is time to remind about the review.
// The check itself decides, so a restart late on Sunday still sends it.
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("goals.review", time.Minute, func(ctx context.Context) error {
		return m.remindTick(ctx, m.now())
	})
	return nil
}

func (m *Module) now() time.Time { return m.nowFn() }

func (m *Module) loc() *time.Location {
	if l := m.d.Scheduler.Location(); l != nil {
		return l
	}
	return time.Local
}

func fail(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	httpx.Fail(w, r, err)
	return true
}

// hiddenFn says whether a module's data may not be shown to the caller now.
func (m *Module) hiddenFn(ctx context.Context) func(module string) bool {
	h, _ := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
	cache := map[string]bool{}
	return func(name string) bool {
		if h == nil {
			return false
		}
		if v, ok := cache[name]; ok {
			return v
		}
		v := h.Hidden(ctx, name)
		cache[name] = v
		return v
	}
}

// catalog is what the other modules offer for key results right now.
type catalog struct {
	sources map[string]contracts.GoalMetricSource // by kind
	kinds   map[string]contracts.GoalMetricKind
}

func (m *Module) catalog() *catalog {
	c := &catalog{sources: map[string]contracts.GoalMetricSource{}, kinds: map[string]contracts.GoalMetricKind{}}
	for key, src := range module.All[contracts.GoalMetricSource](m.d.Registry) {
		if len(key) <= len(contracts.GoalMetricPrefix) || key[:len(contracts.GoalMetricPrefix)] != contracts.GoalMetricPrefix {
			continue
		}
		for _, k := range src.Kinds() {
			c.sources[k.Kind] = src
			c.kinds[k.Kind] = k
		}
	}
	return c
}
