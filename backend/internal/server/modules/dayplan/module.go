// Package dayplan plans the day (B135). Every morning it looks at the free
// stretches in the calendar and at what is due (issues, reminders, what was
// left over yesterday) and proposes the three things that matter most with a
// time for each. The user edits and confirms; the confirmed plan goes into
// the brief, reminds at the start times, and is checked off in the evening.
// See docs/specs/B135.md.
package dayplan

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dayplan/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dayplan/db"
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "dayplan.module"

const (
	dateLayout = "2006-01-02"
	keepDays   = 60
	// actionPrefix starts the button ids: "dayplan.done:<item>".
	actionPrefix = "dayplan."
	// startWindow is how long after its start time an item is still reminded.
	startWindow = 10 * time.Minute
)

// Module implements api.ServerInterface and contracts.DayPlanSource.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time
}

var (
	_ api.ServerInterface     = (*Module)(nil)
	_ module.Starter          = (*Module)(nil)
	_ contracts.DayPlanSource = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	module.Provide[contracts.DayPlanSource](d.Registry, contracts.DayPlanSourceKey, m)
	m.registerActions()
	d.Notify.OnAction(actionPrefix, m.handleAction)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "dayplan" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start runs the minute check: draft, start reminders, evening review.
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("dayplan.tick", time.Minute, m.tick)
	m.d.Scheduler.Every("dayplan.prune", 24*time.Hour, func(ctx context.Context) error {
		return m.q.PruneDayPlans(ctx, m.now().AddDate(0, 0, -keepDays).Format(dateLayout))
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

func (m *Module) today() string { return m.now().In(m.loc()).Format(dateLayout) }

func fail(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	httpx.Fail(w, r, err)
	return true
}
