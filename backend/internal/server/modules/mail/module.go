// Package mail is B53: read Gmail, Aliyun enterprise mail and other IMAP
// inboxes, and push new mail. The first version only receives.
package mail

import (
	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
)

type Module struct {
	d *module.Deps
}

var _ api.ServerInterface = (*Module)(nil)

func New(d *module.Deps) (module.Module, error) {
	return &Module{d: d}, nil
}

func (m *Module) Name() string { return "mail" }

func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}
