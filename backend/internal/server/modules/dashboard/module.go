// Package dashboard stores the overview card layout.
package dashboard

import (
	"errors"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dashboard/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const layoutKey = "dashboard.layout"

var cardIDs = []string{"greeting", "issues", "reminders", "habits", "servers", "coding", "weather", "home", "calendar"}

type Module struct{ d *module.Deps }

func New(d *module.Deps) (module.Module, error) { return &Module{d: d}, nil }

func (m *Module) Name() string { return "dashboard" }

func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

func defaultLayout() api.DashboardLayout {
	cards := make([]api.DashboardCard, len(cardIDs))
	for i, id := range cardIDs {
		cards[i] = api.DashboardCard{Id: id, Visible: true, Order: i}
	}
	return api.DashboardLayout{Cards: cards}
}

func validateLayout(layout api.DashboardLayout) error {
	if len(layout.Cards) != len(cardIDs) {
		return httpx.Invalid("卡片列表不完整")
	}
	known := make(map[string]bool, len(cardIDs))
	for _, id := range cardIDs {
		known[id] = true
	}
	seenIDs := make(map[string]bool, len(cardIDs))
	seenOrders := make(map[int]bool, len(cardIDs))
	for _, card := range layout.Cards {
		if !known[card.Id] || seenIDs[card.Id] || card.Order < 0 || card.Order >= len(cardIDs) || seenOrders[card.Order] {
			return httpx.Invalid("卡片布局无效")
		}
		seenIDs[card.Id] = true
		seenOrders[card.Order] = true
	}
	return nil
}

func (m *Module) GetDashboardLayout(w http.ResponseWriter, r *http.Request) {
	layout := defaultLayout()
	if err := m.d.Settings.Get(r.Context(), layoutKey, &layout); err != nil && !errors.Is(err, settings.ErrNotSet) {
		httpx.Fail(w, r, err)
		return
	}
	sort.Slice(layout.Cards, func(i, j int) bool { return layout.Cards[i].Order < layout.Cards[j].Order })
	httpx.JSON(w, http.StatusOK, layout)
}

func (m *Module) PutDashboardLayout(w http.ResponseWriter, r *http.Request) {
	var layout api.DashboardLayout
	if err := httpx.Decode(r, &layout); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := validateLayout(layout); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sort.Slice(layout.Cards, func(i, j int) bool { return layout.Cards[i].Order < layout.Cards[j].Order })
	err := m.d.Settings.Set(r.Context(), layoutKey, layout)
	m.d.Audit.Record(r.Context(), "dashboard.layout.update", layoutKey, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("dashboard.layout.updated", layout)
	httpx.JSON(w, http.StatusOK, layout)
}

var _ api.ServerInterface = (*Module)(nil)
