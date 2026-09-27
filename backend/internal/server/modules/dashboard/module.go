package dashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dashboard/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const layoutKey = "dashboard.layout"

type Module struct {
	d *module.Deps
}

var _ api.ServerInterface = (*Module)(nil)

func New(d *module.Deps) (module.Module, error) {
	return &Module{d: d}, nil
}

func (m *Module) Name() string { return "dashboard" }

func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

func (m *Module) GetDashboardLayout(w http.ResponseWriter, r *http.Request) {
	var layout api.DashboardLayout
	err := m.d.Settings.Get(r.Context(), layoutKey, &layout)
	if errors.Is(err, settings.ErrNotSet) {
		layout.Cards = []api.DashboardCard{}
	} else if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, layout)
}

func (m *Module) PutDashboardLayout(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if err := httpx.Decode(r, &raw); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cardsRaw, ok := raw["cards"]
	if !ok || len(raw) != 1 || len(cardsRaw) == 0 || cardsRaw[0] != '[' {
		httpx.Fail(w, r, httpx.Invalid("布局需要 cards 数组"))
		return
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(cardsRaw, &entries); err != nil || len(entries) > 50 {
		httpx.Fail(w, r, httpx.Invalid("cards 最多只能有 50 项"))
		return
	}
	layout := api.DashboardLayout{Cards: make([]api.DashboardCard, 0, len(entries))}
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(entry, &fields); err != nil || len(fields) != 3 || fields["id"] == nil || fields["visible"] == nil || fields["order"] == nil {
			httpx.Fail(w, r, httpx.Invalid("卡片需要 id、visible 和 order"))
			return
		}
		var visible bool
		var order int
		if err := json.Unmarshal(fields["visible"], &visible); err != nil || json.Unmarshal(fields["order"], &order) != nil || bytes.Equal(fields["visible"], []byte("null")) || bytes.Equal(fields["order"], []byte("null")) {
			httpx.Fail(w, r, httpx.Invalid("卡片需要有效的 visible 和 order"))
			return
		}
		var card api.DashboardCard
		dec := json.NewDecoder(bytes.NewReader(entry))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&card); err != nil || card.Id == "" || utf8.RuneCountInString(card.Id) > 40 {
			httpx.Fail(w, r, httpx.Invalid("卡片 id 必须在 1 到 40 个字符之间"))
			return
		}
		if seen[card.Id] {
			httpx.Fail(w, r, httpx.Invalid("卡片 id 不能重复"))
			return
		}
		seen[card.Id] = true
		layout.Cards = append(layout.Cards, card)
	}
	if err := m.d.Settings.Set(r.Context(), layoutKey, layout); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, layout)
}
