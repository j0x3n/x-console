package vault

import (
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/vault/api"
)

type Module struct {
	d        *module.Deps
	mu       sync.Mutex
	hidden   map[string]struct{}
	hiddenOK bool
}

var _ api.ServerInterface = (*Module)(nil)

func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d}
	module.Provide[contracts.HiddenModules](d.Registry, contracts.HiddenModulesKey, m)
	d.Actions.SetHidden(m)
	return m, nil
}

func (m *Module) Name() string { return "vault" }

func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

func (m *Module) status(r *http.Request) (api.VaultStatus, error) {
	configured, until, err := m.d.Auth.VaultStatus(r.Context())
	if err != nil {
		return api.VaultStatus{}, err
	}
	return api.VaultStatus{Configured: configured, Unlocked: until != nil, UnlockedUntil: until}, nil
}

func (m *Module) GetVaultStatus(w http.ResponseWriter, r *http.Request) {
	status, err := m.status(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, status)
}

func (m *Module) SetupVault(w http.ResponseWriter, r *http.Request) {
	var body api.VaultPassword
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := m.d.Auth.SetupVault(r.Context(), body.Password); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.GetVaultStatus(w, r)
}

func (m *Module) UnlockVault(w http.ResponseWriter, r *http.Request) {
	var body api.VaultPassword
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := m.d.Auth.UnlockVault(r.Context(), r, body.Password); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.GetVaultStatus(w, r)
}

func (m *Module) LockVault(w http.ResponseWriter, r *http.Request) {
	if err := m.d.Auth.LockVault(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) ChangeVaultPassword(w http.ResponseWriter, r *http.Request) {
	var body api.ChangeVaultPasswordJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := m.d.Auth.ChangeVaultPassword(r.Context(), body.OldPassword, body.NewPassword); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}
