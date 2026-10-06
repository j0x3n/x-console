// Package reminders is M7: user reminders plus delivery of notifications to
// external channels (Web Push, Telegram, Bark, ServerChan).
//
// It plugs into notify.Service: it registers the channels, a Router that
// applies notification_routes and quiet hours, and the button handlers for
// "reminder.*" actions.
package reminders

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/db"
)

// scanInterval is how often due reminders are checked.
const scanInterval = 30 * time.Second

// Module implements api.ServerInterface and contracts.Reminders.
type Module struct {
	d         *module.Deps
	q         *db.Queries
	http      *http.Client
	push      *webPushChannel
	iconMu    sync.Mutex
	icons     map[string][]byte
	iconOrder []string
	iconDraws int
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ contracts.Reminders = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
	_ module.PublicPather = (*Module)(nil)
)

// New builds the module, registers channels, the router, action handlers and
// the actions catalog entries.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), http: &http.Client{Timeout: 15 * time.Second}, icons: map[string][]byte{}}
	m.push = &webPushChannel{m: m}
	d.Notify.RegisterChannel(m.push)
	d.Notify.RegisterChannel(&telegramChannel{m: m})
	d.Notify.RegisterChannel(&barkChannel{m: m})
	d.Notify.RegisterChannel(&serverChanChannel{m: m})
	d.Notify.SetRouter(&router{m: m})
	d.Notify.OnAction("reminder.", m.handleAction)
	module.Provide[contracts.Reminders](d.Registry, contracts.RemindersKey, m)
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "reminders" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// PublicPaths exposes the Telegram webhook. It checks its own secret.
func (m *Module) PublicPaths() []string { return []string{telegramWebhookPath, "/notify/icons/"} }

// Start creates the VAPID keys and schedules the reminder scan.
func (m *Module) Start(ctx context.Context) error {
	if _, err := m.push.keys(ctx); err != nil {
		return err
	}
	m.watchScopeRemoved(ctx)
	m.d.Scheduler.Every("reminders.scan", scanInterval, func(ctx context.Context) error {
		return m.scan(ctx, time.Now())
	})
	return nil
}
