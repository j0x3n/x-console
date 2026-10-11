package dayplan

import (
	"context"
	"errors"
	"net/http"
	"regexp"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dayplan/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// OptionsKey holds the module's settings.
const OptionsKey = "dayplan.options"

var clockRe = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

func defaults() api.DayPlanSettings {
	return api.DayPlanSettings{Auto: true, GenerateTime: "07:30", UseAi: true, WorkStart: "09:00", WorkEnd: "18:00", Review: true, ReviewTime: "20:30"}
}

func (m *Module) options(ctx context.Context) api.DayPlanSettings {
	var saved api.DayPlanSettings
	err := m.d.Settings.Get(ctx, OptionsKey, &saved)
	if err == nil && validSettings(saved) == nil {
		return saved
	}
	if err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("load day plan settings", "err", err)
	}
	return defaults()
}

func validSettings(s api.DayPlanSettings) error {
	for _, v := range []string{s.GenerateTime, s.WorkStart, s.WorkEnd, s.ReviewTime} {
		if !clockRe.MatchString(v) {
			return httpx.Invalid("时间要写成 HH:MM，比如 09:00")
		}
	}
	start, _ := parseHM(s.WorkStart)
	end, _ := parseHM(s.WorkEnd)
	if end-start < minSlot {
		return httpx.Invalid("工作时段至少要有 30 分钟")
	}
	return nil
}

func (m *Module) GetDayPlanSettings(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, m.options(r.Context()))
}

func (m *Module) PutDayPlanSettings(w http.ResponseWriter, r *http.Request) {
	var body api.DayPlanSettings
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	if fail(w, r, validSettings(body)) {
		return
	}
	err := m.d.Settings.Set(r.Context(), OptionsKey, body)
	m.d.Audit.Record(r.Context(), "dayplan.settings", "", map[string]any{"auto": body.Auto, "useAi": body.UseAi}, err)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, body)
}
