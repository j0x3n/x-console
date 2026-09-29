package core_test

import (
	"net/http"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type prefs struct {
	NightMode string  `json:"nightMode"`
	Accent    string  `json:"accent"`
	Language  string  `json:"language"`
	UpdatedAt *string `json:"updatedAt"`
}

func TestPreferences(t *testing.T) {
	env := testutil.New(t)

	var got prefs
	env.MustDo(http.MethodGet, "/me/preferences", nil, &got)
	if got.NightMode != "auto" || got.Accent != "ember" || got.Language != "zh" || got.UpdatedAt != nil {
		t.Fatalf("defaults: %+v", got)
	}

	var saved prefs
	env.MustDo(http.MethodPut, "/me/preferences", prefs{NightMode: "on", Accent: "ocean", Language: "en"}, &saved)
	if saved.UpdatedAt == nil {
		t.Fatalf("put should set updatedAt: %+v", saved)
	}
	env.MustDo(http.MethodGet, "/me/preferences", nil, &got)
	if got.NightMode != "on" || got.Accent != "ocean" || got.Language != "en" || got.UpdatedAt == nil {
		t.Fatalf("after put: %+v", got)
	}

	status, _ := env.Do(http.MethodPut, "/me/preferences", prefs{NightMode: "on", Accent: "pink", Language: "zh"}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("bad accent: %d", status)
	}

	env.MustDo(http.MethodPost, "/auth/logout", nil, nil)
	if status, _ := env.Do(http.MethodGet, "/me/preferences", nil, nil); status != http.StatusUnauthorized {
		t.Fatalf("logged out: %d", status)
	}
}
