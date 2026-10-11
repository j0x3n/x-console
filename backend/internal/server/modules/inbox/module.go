// Package inbox is the capture inbox (B134). The user drops a thought in as
// text, as a voice recording or as a Telegram message. The AI suggests where
// it belongs (reminder, board card, note, read later); nothing is created
// until the user confirms. What stays unconfirmed is brought up once every
// evening. See docs/specs/B134.md.
package inbox

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/inbox/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/inbox/db"
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "inbox.module"

const (
	maxText      = 5000
	maxOpen      = 500
	maxVoiceSize = 10 << 20
	maxVoiceSecs = 300
	keepClosed   = 30 * 24 * time.Hour
	// actionPrefix starts the Telegram button ids: "inbox.file:<id>" and "inbox.dismiss:<id>".
	actionPrefix = "inbox."
)

// Module implements api.ServerInterface and contracts.TelegramInbox.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time

	// async tracks the suggestions being made in the background, so tests can wait for them.
	async sync.WaitGroup
}

var (
	_ api.ServerInterface     = (*Module)(nil)
	_ module.Starter          = (*Module)(nil)
	_ contracts.TelegramInbox = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	module.Provide[contracts.TelegramInbox](d.Registry, contracts.TelegramInboxFallbackKey, m)
	m.registerActions()
	d.Notify.OnAction(actionPrefix, m.handleAction)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "inbox" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the evening reminder and the clean-up of old items.
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("inbox.review", time.Minute, m.reviewTick)
	m.d.Scheduler.Every("inbox.prune", 6*time.Hour, func(ctx context.Context) error {
		return m.q.PruneClosedInboxItems(ctx, ptrTime(m.now().Add(-keepClosed).UTC()))
	})
	return nil
}

func ptrTime(t time.Time) *time.Time { return &t }

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
