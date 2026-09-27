package dashboard_test

import (
	"net/http"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/dashboard/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestLayout(t *testing.T) {
	env := testutil.New(t)
	var layout api.DashboardLayout
	env.MustDo(http.MethodGet, "/dashboard/layout", nil, &layout)
	if len(layout.Cards) != 9 || layout.Cards[0].Id != "greeting" {
		t.Fatalf("default layout: %+v", layout)
	}
	layout.Cards[0].Visible = false
	layout.Cards[0].Order, layout.Cards[1].Order = layout.Cards[1].Order, layout.Cards[0].Order
	env.MustDo(http.MethodPut, "/dashboard/layout", layout, nil)
	var saved api.DashboardLayout
	env.MustDo(http.MethodGet, "/dashboard/layout", nil, &saved)
	if saved.Cards[0].Id != "issues" || saved.Cards[1].Visible {
		t.Fatalf("saved layout: %+v", saved)
	}
	saved.Cards[0].Id = "unknown"
	if status, _ := env.Do(http.MethodPut, "/dashboard/layout", saved, nil); status != http.StatusBadRequest {
		t.Fatalf("unknown card status: %d", status)
	}
	if status, _ := env.Do(http.MethodPut, "/dashboard/layout", api.DashboardLayout{}, nil); status != http.StatusBadRequest {
		t.Fatalf("empty layout status: %d", status)
	}
}
