package dashboard_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/dashboard/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestDashboardLayout(t *testing.T) {
	env := testutil.New(t)
	var layout api.DashboardLayout
	env.MustDo(http.MethodGet, "/dashboard/layout", nil, &layout)
	if layout.Cards == nil || len(layout.Cards) != 0 {
		t.Fatalf("initial layout: %+v", layout)
	}

	two := 2
	want := api.DashboardLayout{Cards: []api.DashboardCard{
		{Id: "weather", Visible: false, Order: 4},
		{Id: "future-card", Visible: true, Order: 1},
		{Id: "network", Visible: true, Order: 2, Column: &two},
	}}
	env.MustDo(http.MethodPut, "/dashboard/layout", want, &layout)
	if !reflect.DeepEqual(layout, want) {
		t.Fatalf("saved layout: %+v", layout)
	}
	env.MustDo(http.MethodGet, "/dashboard/layout", nil, &layout)
	if !reflect.DeepEqual(layout, want) {
		t.Fatalf("read layout: %+v", layout)
	}

	env.MustDo(http.MethodPut, "/dashboard/layout", map[string]any{"cards": []any{}}, &layout)
	if layout.Cards == nil || len(layout.Cards) != 0 {
		t.Fatalf("cleared layout: %+v", layout)
	}
}

func TestDashboardLayoutInvalid(t *testing.T) {
	env := testutil.New(t)
	card := api.DashboardCard{Id: "weather", Visible: true, Order: 0}
	cases := []struct {
		name string
		body any
	}{
		{"duplicate", api.DashboardLayout{Cards: []api.DashboardCard{card, card}}},
		{"too many", api.DashboardLayout{Cards: make([]api.DashboardCard, 51)}},
		{"long id", api.DashboardLayout{Cards: []api.DashboardCard{{Id: strings.Repeat("界", 41), Visible: true, Order: 0}}}},
		{"missing cards", map[string]any{}},
		{"missing visible", map[string]any{"cards": []any{map[string]any{"id": "weather", "order": 0}}}},
		{"null visible", map[string]any{"cards": []any{map[string]any{"id": "weather", "visible": nil, "order": 0}}}},
		{"null order", map[string]any{"cards": []any{map[string]any{"id": "weather", "visible": true, "order": nil}}}},
		{"invalid order", map[string]any{"cards": []any{map[string]any{"id": "weather", "visible": true, "order": "first"}}}},
		{"column too big", map[string]any{"cards": []any{map[string]any{"id": "weather", "visible": true, "order": 0, "column": 4}}}},
		{"null column", map[string]any{"cards": []any{map[string]any{"id": "weather", "visible": true, "order": 0, "column": nil}}}},
		{"unknown field", map[string]any{"cards": []any{map[string]any{"id": "weather", "visible": true, "order": 0, "width": 2}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := env.Do(http.MethodPut, "/dashboard/layout", tc.body, nil)
			if status != http.StatusBadRequest {
				t.Fatalf("status %d: %s", status, body)
			}
		})
	}
	var layout api.DashboardLayout
	env.MustDo(http.MethodGet, "/dashboard/layout", nil, &layout)
	if len(layout.Cards) != 0 {
		t.Fatalf("invalid layout was stored: %+v", layout)
	}
}
