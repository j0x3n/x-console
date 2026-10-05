package core

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/core/api"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const preferencesKey = "ui.preferences"

var (
	nightModes = []string{"on", "off", "auto"}
	accents    = []string{"indigo", "ocean", "teal", "violet", "rose", "graphite"}
	languages  = []string{"zh", "en"}
	// B98 removed these accents. Old saves and old clients still send them.
	legacyAccents = map[api.PreferencesAccent]api.PreferencesAccent{"ember": "indigo", "mint": "teal"}
)

// currentAccent maps a removed accent to the one that replaced it.
func currentAccent(a api.PreferencesAccent) api.PreferencesAccent {
	if next, ok := legacyAccents[a]; ok {
		return next
	}
	return a
}

func defaultPreferences() api.Preferences {
	return api.Preferences{NightMode: "auto", Accent: "indigo", Language: "zh"}
}

// GetPreferences returns the saved UI preferences, or the defaults without
// updatedAt when nothing has been saved yet.
func (h *Handlers) GetPreferences(w http.ResponseWriter, r *http.Request) {
	out := defaultPreferences()
	var saved api.Preferences
	err := h.Settings.Get(r.Context(), preferencesKey, &saved)
	switch {
	case errors.Is(err, settings.ErrNotSet):
	case err != nil:
		httpx.Fail(w, r, err)
		return
	default:
		// A value that is no longer in the enum falls back to the default.
		if slices.Contains(nightModes, string(saved.NightMode)) {
			out.NightMode = saved.NightMode
		}
		if accent := currentAccent(saved.Accent); slices.Contains(accents, string(accent)) {
			out.Accent = accent
		}
		if slices.Contains(languages, string(saved.Language)) {
			out.Language = saved.Language
		}
		// B88, B89: older saves have neither field.
		if saved.Nickname != nil && utf8.RuneCountInString(*saved.Nickname) <= 20 {
			out.Nickname = saved.Nickname
		}
		if saved.QuoteMode != nil && saved.QuoteMode.Valid() {
			out.QuoteMode = saved.QuoteMode
		}
		out.UpdatedAt = saved.UpdatedAt
	}
	httpx.JSON(w, http.StatusOK, out)
}

// PutPreferences replaces the saved UI preferences.
func (h *Handlers) PutPreferences(w http.ResponseWriter, r *http.Request) {
	var body api.Preferences
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	body.Accent = currentAccent(body.Accent)
	switch {
	case !slices.Contains(nightModes, string(body.NightMode)):
		httpx.Fail(w, r, httpx.Invalid("夜间模式不对"))
		return
	case !slices.Contains(accents, string(body.Accent)):
		httpx.Fail(w, r, httpx.Invalid("主题色不对"))
		return
	case !slices.Contains(languages, string(body.Language)):
		httpx.Fail(w, r, httpx.Invalid("语言不对"))
		return
	case body.QuoteMode != nil && !body.QuoteMode.Valid():
		httpx.Fail(w, r, httpx.Invalid("每日一句的显示方式不对"))
		return
	}
	if body.Nickname != nil {
		name := strings.TrimSpace(*body.Nickname)
		if utf8.RuneCountInString(name) > 20 {
			httpx.Fail(w, r, httpx.Invalid("称呼最多 20 个字"))
			return
		}
		body.Nickname = &name
	}
	now := time.Now().UTC()
	body.UpdatedAt = &now
	if err := h.Settings.Set(r.Context(), preferencesKey, body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.Bus.Publish("ui.preferences_changed", body)
	httpx.JSON(w, http.StatusOK, body)
}
