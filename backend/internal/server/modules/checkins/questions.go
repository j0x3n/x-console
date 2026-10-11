package checkins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/checkins/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/checkins/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// OptionsKey holds the module's switch.
const OptionsKey = "checkins.options"

var clockRe = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

func toQuestion(r db.CheckinQuestion) api.CheckinQuestion {
	times := []string{}
	_ = json.Unmarshal([]byte(r.Times), &times)
	return api.CheckinQuestion{Id: r.ID, Label: r.Label, LowLabel: r.LowLabel, HighLabel: r.HighLabel,
		Times: times, Enabled: r.Enabled == 1, SortOrder: int(r.SortOrder)}
}

// cleanInput checks a question and returns it with trimmed texts and sorted,
// distinct times.
func cleanInput(in api.CheckinQuestionInput) (api.CheckinQuestionInput, error) {
	in.Label = strings.TrimSpace(in.Label)
	in.LowLabel = strings.TrimSpace(in.LowLabel)
	in.HighLabel = strings.TrimSpace(in.HighLabel)
	if in.Label == "" || utf8.RuneCountInString(in.Label) > maxLabel {
		return in, httpx.Invalid("问题要写 1 到 60 个字")
	}
	if utf8.RuneCountInString(in.LowLabel) > maxEnd || utf8.RuneCountInString(in.HighLabel) > maxEnd {
		return in, httpx.Invalid("1 分和 5 分的说明最多 30 个字")
	}
	if len(in.Times) > maxTimes {
		return in, httpx.Invalid("每个问题最多 6 个时间点")
	}
	for _, t := range in.Times {
		if !clockRe.MatchString(t) {
			return in, httpx.Invalid("时间要写成 HH:MM，比如 10:00")
		}
	}
	times := slices.Clone(in.Times)
	slices.Sort(times)
	in.Times = slices.Compact(times)
	return in, nil
}

func (m *Module) ListCheckinQuestions(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListCheckinQuestions(r.Context())
	if fail(w, r, err) {
		return
	}
	items := make([]api.CheckinQuestion, 0, len(rows))
	for _, row := range rows {
		items = append(items, toQuestion(row))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (m *Module) CreateCheckinQuestion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body api.CheckinQuestionInput
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	in, err := cleanInput(body)
	if fail(w, r, err) {
		return
	}
	n, err := m.q.CountCheckinQuestions(ctx)
	if fail(w, r, err) {
		return
	}
	if n >= maxQuestions {
		httpx.Fail(w, r, httpx.Invalid("最多 10 个问题"))
		return
	}
	order, err := m.q.MaxCheckinSort(ctx)
	if fail(w, r, err) {
		return
	}
	times, _ := json.Marshal(in.Times)
	now := m.now().UTC()
	row, err := m.q.InsertCheckinQuestion(ctx, db.InsertCheckinQuestionParams{Label: in.Label, LowLabel: in.LowLabel,
		HighLabel: in.HighLabel, Times: string(times), Enabled: b2i(in.Enabled), SortOrder: order + 1, CreatedAt: now, UpdatedAt: now})
	m.d.Audit.Record(ctx, "checkin.question.create", in.Label, nil, err)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusCreated, toQuestion(row))
}

func (m *Module) UpdateCheckinQuestion(w http.ResponseWriter, r *http.Request, id api.QuestionId) {
	ctx := r.Context()
	var body api.CheckinQuestionInput
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	in, err := cleanInput(body)
	if fail(w, r, err) {
		return
	}
	times, _ := json.Marshal(in.Times)
	row, err := m.q.UpdateCheckinQuestion(ctx, db.UpdateCheckinQuestionParams{Label: in.Label, LowLabel: in.LowLabel,
		HighLabel: in.HighLabel, Times: string(times), Enabled: b2i(in.Enabled), UpdatedAt: m.now().UTC(), ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(ctx, "checkin.question.update", in.Label, nil, err)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, toQuestion(row))
}

func (m *Module) DeleteCheckinQuestion(w http.ResponseWriter, r *http.Request, id api.QuestionId) {
	n, err := m.q.DeleteCheckinQuestion(r.Context(), id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(r.Context(), "checkin.question.delete", "", map[string]any{"id": id}, err)
	if fail(w, r, err) {
		return
	}
	httpx.NoContent(w)
}

func (m *Module) enabled(ctx context.Context) bool {
	var o api.CheckinSettings
	if err := m.d.Settings.Get(ctx, OptionsKey, &o); err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("load check-in settings", "err", err)
	}
	return o.Enabled
}

func (m *Module) GetCheckinSettings(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, api.CheckinSettings{Enabled: m.enabled(r.Context())})
}

func (m *Module) PutCheckinSettings(w http.ResponseWriter, r *http.Request) {
	var body api.CheckinSettings
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	err := m.d.Settings.Set(r.Context(), OptionsKey, body)
	m.d.Audit.Record(r.Context(), "checkin.settings", "", map[string]any{"enabled": body.Enabled}, err)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, body)
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
