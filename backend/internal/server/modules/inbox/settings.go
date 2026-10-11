package inbox

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/inbox/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// OptionsKey holds the module's settings.
const OptionsKey = "inbox.options"

// lastReviewKey holds the date (YYYY-MM-DD) of the last evening reminder.
const lastReviewKey = "inbox.last_review"

var (
	clockRe    = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)
	languageRe = regexp.MustCompile(`^[a-z]{2,3}$`)
)

func defaults() api.InboxSettings {
	return api.InboxSettings{Suggest: true, ReviewReminder: true, ReviewTime: "21:00"}
}

func (m *Module) options(ctx context.Context) api.InboxSettings {
	o := defaults()
	var saved api.InboxSettings
	err := m.d.Settings.Get(ctx, OptionsKey, &saved)
	if err == nil && clockRe.MatchString(saved.ReviewTime) {
		return saved
	}
	if err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("load inbox settings", "err", err)
	}
	return o
}

func (m *Module) GetInboxSettings(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, m.options(r.Context()))
}

func (m *Module) PutInboxSettings(w http.ResponseWriter, r *http.Request) {
	var body api.InboxSettings
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	body.TranscribeModel = strings.TrimSpace(body.TranscribeModel)
	body.Language = strings.TrimSpace(strings.ToLower(body.Language))
	switch {
	case !clockRe.MatchString(body.ReviewTime):
		httpx.Fail(w, r, httpx.Invalid("时间要写成 HH:MM，比如 21:00"))
		return
	case utf8.RuneCountInString(body.TranscribeModel) > 100:
		httpx.Fail(w, r, httpx.Invalid("转写模型名太长了"))
		return
	case body.Language != "" && !languageRe.MatchString(body.Language):
		httpx.Fail(w, r, httpx.Invalid("语言要写 ISO-639-1 代码，比如 zh 或 en"))
		return
	case body.CardProjectId < 0:
		httpx.Fail(w, r, httpx.Invalid("项目不正确"))
		return
	}
	err := m.d.Settings.Set(r.Context(), OptionsKey, body)
	m.d.Audit.Record(r.Context(), "inbox.settings", "", map[string]any{"suggest": body.Suggest, "reviewReminder": body.ReviewReminder}, err)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, body)
}
