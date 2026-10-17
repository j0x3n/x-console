// Package feeds is B241: the news and tweets page. The data stays on a VM in
// the user's network. The panel asks a small read-only service there, through
// a paired agent's http.proxy. See docs/specs/B241.md.
package feeds

import (
	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/feeds/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/feeds/db"
)

// Module implements the HTTP API.
type Module struct {
	d *module.Deps
	q *db.Queries
}

var _ api.ServerInterface = (*Module)(nil)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	return &Module{d: d, q: db.New(d.DB)}, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "feeds" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}
