package core

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/core/api"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const preferencesKey = "ui.preferences"

var (
	nightModes = []string{"on", "off", "auto"}
	accents    = []string{"ember", "violet", "mint", "ocean", "rose", "graphite"}
	languages  = []string{"zh", "en"}
)

func defaultPreferences() api.Preferences {
	return api.Preferences{NightMode: "auto", Accent: "ember", Language: "zh"}
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
		if slices.Contains(accents, string(saved.Accent)) {
			out.Accent = saved.Accent
		}
		if slices.Contains(languages, string(saved.Language)) {
			out.Language = saved.Language
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
