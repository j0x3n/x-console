// Package checkins asks the user two or three short questions a day (energy,
// stress) and keeps the 1 to 5 answers (B132). The answers come from buttons
// in a notification (Telegram) or from the page, and other modules read them
// to compare days (B133). See docs/specs/B132.md.
package checkins

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/checkins/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/checkins/db"
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "checkins.module"

const (
	dateLayout = "2006-01-02"

	maxQuestions = 10
	maxTimes     = 6
	maxLabel     = 60
	maxEnd       = 30

	// askWindow is how long after its time a question is still asked (the
	// server was down at 10:00 and came back at 10:20), and how long a
	// slot counts as "pending" on the page.
	askWindow = 60 * time.Minute
)

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time
}

var (
	_ api.ServerInterface     = (*Module)(nil)
	_ module.Starter          = (*Module)(nil)
	_ contracts.CheckinSeries = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	module.Provide[contracts.CheckinSeries](d.Registry, contracts.CheckinSeriesKey, m)
	m.registerActions()
	d.Notify.OnAction(actionPrefix, m.handleAction)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "checkins" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start checks every minute whether a question is due.
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("checkins.ask", time.Minute, m.askTick)
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
