// Package cards is the review cards module (B130). The user turns a passage
// of a note or a saved article into a question and an answer. Cards are
// scheduled with FSRS, reviewed on the /cards page or one by one in Telegram,
// and counted in the daily brief. See docs/specs/B130.md.
package cards

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/cards/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/cards/db"
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "cards.module"

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
	_ contracts.CardsDue  = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	module.Provide[contracts.CardsDue](d.Registry, contracts.CardsDueKey, m)
	module.Provide[contracts.KnowledgeSource](d.Registry, contracts.KnowledgeSourcePrefix+"cards", knowledgeSource{m}) // B139
	m.registerActions()
	// "开始"、"显示答案" and the four rating buttons of the Telegram review.
	d.Notify.OnAction(actionPrefix, m.handleAction)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "cards" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the daily push. The job runs every minute and sends once a
// day, at the time the user chose.
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("cards.push", time.Minute, m.pushJob)
	return nil
}

func (m *Module) now() time.Time { return m.nowFn() }

func (m *Module) location() *time.Location {
	if loc := m.d.Config.Location; loc != nil {
		return loc
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
