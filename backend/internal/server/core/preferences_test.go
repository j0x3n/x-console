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
	Nickname  *string `json:"nickname,omitempty"`
	QuoteMode *string `json:"quoteMode,omitempty"`
}

func TestPreferences(t *testing.T) {
	env := testutil.New(t)

	var got prefs
	env.MustDo(http.MethodGet, "/me/preferences", nil, &got)
	if got.NightMode != "auto" || got.Accent != "indigo" || got.Language != "zh" || got.UpdatedAt != nil {
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

	// B88, B89: nickname and the daily quote
	name, mode := "  小明 ", "daily"
	env.MustDo(http.MethodPut, "/me/preferences", prefs{NightMode: "on", Accent: "ocean", Language: "zh", Nickname: &name, QuoteMode: &mode}, nil)
	env.MustDo(http.MethodGet, "/me/preferences", nil, &got)
	if got.Nickname == nil || *got.Nickname != "小明" || got.QuoteMode == nil || *got.QuoteMode != "daily" {
		t.Fatalf("nickname and quote: %+v", got)
	}
	long, bad := "一二三四五六七八九十一二三四五六七八九十一", "weekly"
	if status, _ := env.Do(http.MethodPut, "/me/preferences", prefs{NightMode: "on", Accent: "ocean", Language: "zh", Nickname: &long}, nil); status != http.StatusBadRequest {
		t.Fatalf("long nickname: %d", status)
	}
	if status, _ := env.Do(http.MethodPut, "/me/preferences", prefs{NightMode: "on", Accent: "ocean", Language: "zh", QuoteMode: &bad}, nil); status != http.StatusBadRequest {
		t.Fatalf("bad quote mode: %d", status)
	}

	// B98: ember and mint were removed. Old clients still send them.
	for old, want := range map[string]string{"ember": "indigo", "mint": "teal"} {
		env.MustDo(http.MethodPut, "/me/preferences", prefs{NightMode: "off", Accent: old, Language: "zh"}, &saved)
		env.MustDo(http.MethodGet, "/me/preferences", nil, &got)
		if saved.Accent != want || got.Accent != want {
			t.Fatalf("legacy accent %s: saved %s, got %s", old, saved.Accent, got.Accent)
		}
	}

	env.MustDo(http.MethodPost, "/auth/logout", nil, nil)
	if status, _ := env.Do(http.MethodGet, "/me/preferences", nil, nil); status != http.StatusUnauthorized {
		t.Fatalf("logged out: %d", status)
	}
}
