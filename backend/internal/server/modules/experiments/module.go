// Package experiments is the self-experiment module (B133). The user states
// what they change, which numbers to watch and which days are the baseline
// and which the change. The module reads those numbers from the modules that
// already keep them and compares the two phases, saying plainly when the
// data is too thin to tell a real change from chance.
// See docs/specs/B133.md.
package experiments

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/experiments/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/experiments/db"
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "experiments.module"

const (
	dateLayout = "2006-01-02"

	maxExperiments = 200
	maxMetrics     = 6
	maxTitle       = 100
	maxText        = 5000
	maxSpanDays    = 180
	minPhaseDays   = 3
)

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time
}

var _ api.ServerInterface = (*Module)(nil)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "experiments" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

func (m *Module) now() time.Time { return m.nowFn() }

func (m *Module) loc() *time.Location {
	if l := m.d.Scheduler.Location(); l != nil {
		return l
	}
	return time.Local
}

func (m *Module) today() string { return m.now().In(m.loc()).Format(dateLayout) }

func fail(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	httpx.Fail(w, r, err)
	return true
}

// hidden says whether a module's data may not be shown right now.
func (m *Module) hidden(ctx context.Context) func(name string) bool {
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
