package core_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type prefs struct {
	NightMode     string    `json:"nightMode"`
	Accent        string    `json:"accent"`
	Language      string    `json:"language"`
	UpdatedAt     *string   `json:"updatedAt"`
	Nickname      *string   `json:"nickname,omitempty"`
	QuoteMode     *string   `json:"quoteMode,omitempty"`
	NavPins       *[]string `json:"navPins,omitempty"`
	TerminalLeave *string   `json:"terminalLeave,omitempty"`
}

func TestPreferencesTerminalLeave(t *testing.T) {
	env := testutil.New(t)

	var initial map[string]any
	env.MustDo(http.MethodGet, "/me/preferences", nil, &initial)
	if _, ok := initial["terminalLeave"]; ok {
		t.Fatalf("defaults should omit terminalLeave: %+v", initial)
	}

	for _, mode := range []string{"ask", "keep", "close"} {
		t.Run(mode, func(t *testing.T) {
			body := prefs{NightMode: "on", Accent: "ocean", Language: "zh", TerminalLeave: &mode}
			var saved, got prefs
			env.MustDo(http.MethodPut, "/me/preferences", body, &saved)
			env.MustDo(http.MethodGet, "/me/preferences", nil, &got)
			if saved.TerminalLeave == nil || *saved.TerminalLeave != mode || got.TerminalLeave == nil || *got.TerminalLeave != mode {
				t.Fatalf("terminalLeave %q: saved %+v, got %+v", mode, saved, got)
			}
		})
	}

	t.Run("omitted", func(t *testing.T) {
		name, quote := "小明", "daily"
		pins := []string{"/notes", "/projects"}
		body := prefs{NightMode: "off", Accent: "teal", Language: "en", Nickname: &name, QuoteMode: &quote, NavPins: &pins}
		var saved map[string]any
		env.MustDo(http.MethodPut, "/me/preferences", body, &saved)
		var got map[string]any
		env.MustDo(http.MethodGet, "/me/preferences", nil, &got)
		for _, result := range []map[string]any{saved, got} {
			if _, ok := result["terminalLeave"]; ok {
				t.Fatalf("omitted terminalLeave should stay absent: %+v", result)
			}
		}
		var read prefs
		env.MustDo(http.MethodGet, "/me/preferences", nil, &read)
		if read.NightMode != body.NightMode || read.Accent != body.Accent || read.Language != body.Language || read.UpdatedAt == nil ||
			read.Nickname == nil || *read.Nickname != name || read.QuoteMode == nil || *read.QuoteMode != quote ||
			read.NavPins == nil || !slices.Equal(*read.NavPins, pins) {
			t.Fatalf("other preferences changed: %+v", read)
		}
	})

	for _, mode := range []string{"invalid", ""} {
		t.Run("invalid "+mode, func(t *testing.T) {
			body := prefs{NightMode: "on", Accent: "ocean", Language: "zh", TerminalLeave: &mode}
			status, raw := env.Do(http.MethodPut, "/me/preferences", body, nil)
			if status != http.StatusBadRequest {
				t.Fatalf("bad terminalLeave: %d, %s", status, raw)
			}
			var failure struct{ Code, Message string }
			if err := json.Unmarshal(raw, &failure); err != nil {
				t.Fatal(err)
			}
			if failure.Code != "validation_failed" || failure.Message != "终端的离开方式不对" {
				t.Fatalf("bad terminalLeave error: %+v", failure)
			}

			if err := env.App.Deps.Settings.Set(t.Context(), "ui.preferences", body); err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			env.MustDo(http.MethodGet, "/me/preferences", nil, &got)
			if _, ok := got["terminalLeave"]; ok || got["accent"] != "ocean" {
				t.Fatalf("invalid saved terminalLeave: %+v", got)
			}
		})
	}
}

func TestPreferencesNavPins(t *testing.T) {
	env := testutil.New(t)

	var initial map[string]any
	env.MustDo(http.MethodGet, "/me/preferences", nil, &initial)
	if _, ok := initial["navPins"]; ok {
		t.Fatalf("defaults should omit navPins: %+v", initial)
	}

	forty := make([]string, 40)
	for i := range forty {
		forty[i] = fmt.Sprintf("/entry-%d", i)
	}
	for _, tc := range []struct {
		name string
		pins []string
		want []string
	}{
		{"round trip", []string{"/notes", "/projects", "/new-module/123"}, []string{"/notes", "/projects", "/new-module/123"}},
		{"duplicates", []string{"/notes", "/projects", "/notes", "/home", "/projects"}, []string{"/notes", "/projects", "/home"}},
		{"empty", []string{}, []string{}},
		{"max length", []string{"/" + strings.Repeat("a", 63), "/"}, []string{"/" + strings.Repeat("a", 63), "/"}},
		{"max items", forty, forty},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := prefs{NightMode: "on", Accent: "ocean", Language: "zh", NavPins: &tc.pins}
			var saved, got prefs
			env.MustDo(http.MethodPut, "/me/preferences", body, &saved)
			env.MustDo(http.MethodGet, "/me/preferences", nil, &got)
			if saved.NavPins == nil || got.NavPins == nil || !slices.Equal(*saved.NavPins, tc.want) || !slices.Equal(*got.NavPins, tc.want) {
				t.Fatalf("navPins: saved %+v, got %+v, want %v", saved, got, tc.want)
			}
		})
	}

	t.Run("omitted", func(t *testing.T) {
		env.MustDo(http.MethodPut, "/me/preferences", prefs{NightMode: "on", Accent: "ocean", Language: "zh"}, nil)
		var got map[string]any
		env.MustDo(http.MethodGet, "/me/preferences", nil, &got)
		if _, ok := got["navPins"]; ok {
			t.Fatalf("omitted navPins should stay absent: %+v", got)
		}
	})

	for _, tc := range []struct {
		name string
		pins []string
	}{
		{"too many", slices.Repeat([]string{"/notes"}, 41)},
		{"empty path", []string{""}},
		{"missing slash", []string{"notes"}},
		{"uppercase", []string{"/Notes"}},
		{"underscore", []string{"/my_notes"}},
		{"query", []string{"/notes?x=1"}},
		{"space", []string{"/my notes"}},
		{"unicode", []string{"/笔记"}},
		{"newline", []string{"/notes\n"}},
		{"too long", []string{"/" + strings.Repeat("a", 64)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := prefs{NightMode: "on", Accent: "ocean", Language: "zh", NavPins: &tc.pins}
			status, raw := env.Do(http.MethodPut, "/me/preferences", body, nil)
			if status != http.StatusBadRequest {
				t.Fatalf("bad navPins: %d, %s", status, raw)
			}
			var failure struct{ Code, Message string }
			if err := json.Unmarshal(raw, &failure); err != nil {
				t.Fatal(err)
			}
			if failure.Code != "validation_failed" || failure.Message != "左栏固定的入口不对" {
				t.Fatalf("bad navPins error: %+v", failure)
			}

			// Old saves and manually edited values must not expose invalid pins.
			if err := env.App.Deps.Settings.Set(t.Context(), "ui.preferences", body); err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			env.MustDo(http.MethodGet, "/me/preferences", nil, &got)
			if _, ok := got["navPins"]; ok || got["accent"] != "ocean" {
				t.Fatalf("invalid saved navPins: %+v", got)
			}
		})
	}

	t.Run("valid saved pins unchanged", func(t *testing.T) {
		pins := []string{"/notes", "/projects", "/notes"}
		body := prefs{NightMode: "on", Accent: "ocean", Language: "zh", NavPins: &pins}
		if err := env.App.Deps.Settings.Set(t.Context(), "ui.preferences", body); err != nil {
			t.Fatal(err)
		}
		var got prefs
		env.MustDo(http.MethodGet, "/me/preferences", nil, &got)
		if got.NavPins == nil || !slices.Equal(*got.NavPins, pins) {
			t.Fatalf("saved navPins changed: %+v", got)
		}
	})
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

	// B175: the cyber theme
	env.MustDo(http.MethodPut, "/me/preferences", prefs{NightMode: "on", Accent: "cyber", Language: "zh"}, &saved)
	env.MustDo(http.MethodGet, "/me/preferences", nil, &got)
	if saved.Accent != "cyber" || got.Accent != "cyber" {
		t.Fatalf("cyber accent: saved %s, got %s", saved.Accent, got.Accent)
	}

	env.MustDo(http.MethodPost, "/auth/logout", nil, nil)
	if status, _ := env.Do(http.MethodGet, "/me/preferences", nil, nil); status != http.StatusUnauthorized {
		t.Fatalf("logged out: %d", status)
	}
}
