// Package module defines how feature modules plug into the server.
//
// A module lives in internal/server/modules/<name>/ and exposes
//
//	func New(d *module.Deps) (*Module, error)
//
// returning a value that implements Module. It is registered with one line in
// internal/server/app/modules.go. See docs/03-backend.md.
package module

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/agenthub"
	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/scheduler"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Deps are the shared services every module may use.
type Deps struct {
	Config    config.Config
	DB        *sql.DB
	Log       *slog.Logger
	Bus       *events.Bus
	Audit     *audit.Log
	Auth      *auth.Service
	Secrets   *secrets.Box
	Settings  *settings.Store
	Notify    *notify.Service
	Agents    *agenthub.Hub
	Scheduler *scheduler.Scheduler
	// Actions is the catalog used by the AI assistant and automations (M12).
	// Register every operation that makes sense to trigger by name.
	Actions *actions.Registry
	// Registry lets modules find each other's public services (for example
	// the automation engine calls actions offered by other modules).
	Registry *Registry
}

// Module is a feature module.
type Module interface {
	// Name is the short module name, for example "projects".
	Name() string
	// Mount registers HTTP routes. r is the /api/v1 router with auth applied.
	Mount(r chi.Router)
}

// Starter is implemented by modules with background work. Start is called
// once after every module was built; register scheduler jobs and event
// subscriptions here. Stop work when ctx is canceled.
type Starter interface {
	Start(ctx context.Context) error
}

// PublicPather is implemented by modules that need unauthenticated routes,
// such as incoming webhooks. Paths are relative to /api/v1 and match by prefix.
// Such handlers must verify their own secret.
type PublicPather interface {
	PublicPaths() []string
}
