package vault

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/vault/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// HiddenModulesKey is the setting that lists the hidden modules (B57).
const HiddenModulesKey = "vault.hidden_modules"

// allModules is every module the sidebar can hide, in sidebar order.
var allModules = []api.ModuleId{
	api.Projects, api.Coding, api.Notes, api.Reminders, api.Habits, api.Drive, api.Calendar,
	api.Servers, api.Pc, api.Monitoring, api.Home, api.Automations, api.Github,
}

// hiddenModules reads the saved list. Unknown names are dropped.
func (m *Module) hiddenModules(ctx context.Context) ([]api.ModuleId, error) {
	var saved []string
	if err := m.d.Settings.Get(ctx, HiddenModulesKey, &saved); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return nil, err
	}
	out := []api.ModuleId{}
	for _, id := range allModules {
		if slices.Contains(saved, string(id)) {
			out = append(out, id)
		}
	}
	return out, nil
}

// GetHiddenModules is GET /vault/modules. It looks like a missing route
// while the vault is locked, so nothing hints that modules can be hidden.
func (m *Module) GetHiddenModules(w http.ResponseWriter, r *http.Request) {
	if !auth.VaultUnlocked(r.Context()) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	hidden, err := m.hiddenModules(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, api.HiddenModules{Hidden: hidden})
}

// SetHiddenModules is PUT /vault/modules.
func (m *Module) SetHiddenModules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !auth.VaultUnlocked(ctx) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var body api.HiddenModules
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	save := []string{}
	for _, id := range body.Hidden {
		if !slices.Contains(allModules, id) {
			httpx.Fail(w, r, httpx.Invalid("不认识这个模块："+string(id)))
			return
		}
		if !slices.Contains(save, string(id)) {
			save = append(save, string(id))
		}
	}
	err := m.d.Settings.Set(ctx, HiddenModulesKey, save)
	m.d.Audit.Record(ctx, "vault.modules", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.GetHiddenModules(w, r)
}

// GetAvailableModules is GET /app/modules: every module, minus the hidden
// ones while the vault is locked.
func (m *Module) GetAvailableModules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hidden := []api.ModuleId{}
	if !auth.VaultUnlocked(ctx) {
		var err error
		if hidden, err = m.hiddenModules(ctx); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	out := []api.ModuleId{}
	for _, id := range allModules {
		if !slices.Contains(hidden, id) {
			out = append(out, id)
		}
	}
	httpx.JSON(w, http.StatusOK, api.AvailableModules{Modules: out})
}
