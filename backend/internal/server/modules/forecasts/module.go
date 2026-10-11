// Package forecasts keeps predictions and decisions (B137). Each record has a
// statement, an optional confidence and a due date. On the due date the user
// is reminded to fill in the outcome. The stats show how the stated
// confidence compares with what happened. See docs/specs/B137.md.
package forecasts

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/forecasts/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/forecasts/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "forecasts.module"

// OptionsKey holds the module's settings.
const OptionsKey = "forecasts.options"

const (
	maxRecords   = 2000
	maxStatement = 500
	maxReason    = 2000
	maxNote      = 1000
	maxYears     = 5
)

var clockRe = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "forecasts" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the due-date reminder.
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("forecasts.due", time.Minute, m.dueTick)
	return nil
}

func (m *Module) now() time.Time { return m.nowFn() }

func (m *Module) loc() *time.Location {
	if l := m.d.Scheduler.Location(); l != nil {
		return l
	}
	return time.Local
}

func (m *Module) today() string { return m.now().In(m.loc()).Format("2006-01-02") }

func fail(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	httpx.Fail(w, r, err)
	return true
}

func ptr[T any](v T) *T { return &v }

func toForecast(r db.Forecast, today string) api.Forecast {
	f := api.Forecast{
		Id: r.ID, Kind: api.ForecastKind(r.Kind), Statement: r.Statement, DueDate: r.DueDate, Reason: r.Reason,
		Note: r.Note, CreatedAt: r.CreatedAt, ResolvedAt: r.ResolvedAt,
		Overdue: r.Outcome == nil && r.DueDate < today,
	}
	if r.Probability != nil {
		f.Probability = ptr(int(*r.Probability))
	}
	if r.Outcome != nil {
		f.Outcome = ptr(api.ForecastOutcome(*r.Outcome))
	}
	return f
}

func (m *Module) get(ctx context.Context, id int64) (db.Forecast, error) {
	row, err := m.q.GetForecast(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, httpx.ErrNotFound
	}
	return row, err
}

// checkDue parses a due date and keeps it between today and five years ahead.
func (m *Module) checkDue(s string) (string, error) {
	s = strings.TrimSpace(s)
	d, err := time.ParseInLocation("2006-01-02", s, m.loc())
	if err != nil {
		return "", httpx.Invalid("到期日要写成 YYYY-MM-DD，比如 2026-12-31")
	}
	now := m.now().In(m.loc())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, m.loc())
	if d.Before(today) {
		return "", httpx.Invalid("到期日不能早于今天")
	}
	if d.After(today.AddDate(maxYears, 0, 0)) {
		return "", httpx.Invalid("到期日最远 5 年")
	}
	return d.Format("2006-01-02"), nil
}

func checkStatement(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > maxStatement {
		return "", httpx.Invalid("内容要写 1 到 500 个字")
	}
	return s, nil
}

func checkReason(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxReason {
		return "", httpx.Invalid("理由最多 2000 个字")
	}
	return s, nil
}

func checkNote(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxNote {
		return "", httpx.Invalid("备注最多 1000 个字")
	}
	return s, nil
}

func checkProbability(p int) error {
	if p < 1 || p > 99 {
		return httpx.Invalid("把握要在 1 到 99 之间")
	}
	return nil
}

func (m *Module) create(ctx context.Context, in api.ForecastInput) (api.Forecast, error) {
	if in.Kind != api.Prediction && in.Kind != api.Decision {
		return api.Forecast{}, httpx.Invalid("类型只能是预测或决策")
	}
	statement, err := checkStatement(in.Statement)
	if err != nil {
		return api.Forecast{}, err
	}
	reason, err := checkReason(deref(in.Reason))
	if err != nil {
		return api.Forecast{}, err
	}
	var prob *int64
	if in.Probability != nil {
		if err := checkProbability(*in.Probability); err != nil {
			return api.Forecast{}, err
		}
		prob = ptr(int64(*in.Probability))
	}
	due, err := m.checkDue(in.DueDate)
	if err != nil {
		return api.Forecast{}, err
	}
	n, err := m.q.CountForecasts(ctx)
	if err != nil {
		return api.Forecast{}, err
	}
	if n >= maxRecords {
		return api.Forecast{}, httpx.Invalid("记录太多了，最多 2000 条，请先删掉一些")
	}
	row, err := m.q.InsertForecast(ctx, db.InsertForecastParams{
		Kind: string(in.Kind), Statement: statement, Probability: prob, DueDate: due, Reason: reason, CreatedAt: m.now().UTC(),
	})
	if err != nil {
		return api.Forecast{}, err
	}
	// Due today and the reminder time has passed: do not remind right after the user wrote it.
	if due == m.today() && !m.beforeRemindTime(ctx) {
		if err := m.q.SetForecastNotified(ctx, row.ID); err != nil {
			return api.Forecast{}, err
		}
		row.Notified = 1
	}
	return toForecast(row, m.today()), nil
}

func (m *Module) resolve(ctx context.Context, id int64, outcome api.ForecastOutcome, note *string) (api.Forecast, error) {
	if outcome != api.Yes && outcome != api.No && outcome != api.Void {
		return api.Forecast{}, httpx.Invalid("结果只能是成了、没成或作废")
	}
	row, err := m.get(ctx, id)
	if err != nil {
		return api.Forecast{}, err
	}
	n := row.Note
	if note != nil {
		if n, err = checkNote(*note); err != nil {
			return api.Forecast{}, err
		}
	}
	row, err = m.q.ResolveForecast(ctx, db.ResolveForecastParams{Outcome: ptr(string(outcome)), Note: n, ResolvedAt: ptr(m.now().UTC()), ID: id})
	if err != nil {
		return api.Forecast{}, err
	}
	return toForecast(row, m.today()), nil
}

func (m *Module) list(ctx context.Context, status api.ListForecastsParamsStatus) ([]api.Forecast, error) {
	today := m.today()
	out := []api.Forecast{}
	if status != api.Resolved {
		rows, err := m.q.ListOpenForecasts(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, toForecast(r, today))
		}
	}
	if status != api.Open {
		rows, err := m.q.ListResolvedForecasts(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, toForecast(r, today))
		}
	}
	return out, nil
}

func (m *Module) ListForecasts(w http.ResponseWriter, r *http.Request, params api.ListForecastsParams) {
	status := api.All
	if params.Status != nil {
		status = *params.Status
		if !status.Valid() {
			httpx.Fail(w, r, httpx.Invalid("status 只能是 open、resolved 或 all"))
			return
		}
	}
	items, err := m.list(r.Context(), status)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "today": m.today()})
}

func (m *Module) CreateForecast(w http.ResponseWriter, r *http.Request) {
	var body api.ForecastInput
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	f, err := m.create(r.Context(), body)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusCreated, f)
}

func (m *Module) UpdateForecast(w http.ResponseWriter, r *http.Request, forecastId api.ForecastId) {
	ctx := r.Context()
	var body api.ForecastPatch
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	row, err := m.get(ctx, forecastId)
	if fail(w, r, err) {
		return
	}
	if row.Outcome != nil {
		if body.Statement != nil || body.Probability != nil || body.DueDate != nil || body.Reason != nil {
			httpx.Fail(w, r, httpx.Invalid("已经有结果的记录只能改备注，要改别的请先撤销结果"))
			return
		}
		if body.Note != nil {
			note, err := checkNote(*body.Note)
			if fail(w, r, err) {
				return
			}
			row, err = m.q.SetForecastNote(ctx, db.SetForecastNoteParams{Note: note, ID: row.ID})
			if fail(w, r, err) {
				return
			}
		}
		httpx.JSON(w, http.StatusOK, toForecast(row, m.today()))
		return
	}
	p := db.UpdateOpenForecastParams{ID: row.ID, Statement: row.Statement, Probability: row.Probability, DueDate: row.DueDate, Reason: row.Reason, Notified: row.Notified}
	if body.Statement != nil {
		if p.Statement, err = checkStatement(*body.Statement); fail(w, r, err) {
			return
		}
	}
	if body.Reason != nil {
		if p.Reason, err = checkReason(*body.Reason); fail(w, r, err) {
			return
		}
	}
	if body.Probability != nil {
		if *body.Probability == 0 {
			p.Probability = nil
		} else {
			if err := checkProbability(*body.Probability); fail(w, r, err) {
				return
			}
			p.Probability = ptr(int64(*body.Probability))
		}
	}
	if body.DueDate != nil && strings.TrimSpace(*body.DueDate) != row.DueDate {
		if p.DueDate, err = m.checkDue(*body.DueDate); fail(w, r, err) {
			return
		}
		// A new date gets its own reminder.
		p.Notified = 0
		if p.DueDate == m.today() && !m.beforeRemindTime(ctx) {
			p.Notified = 1
		}
	}
	row, err = m.q.UpdateOpenForecast(ctx, p)
	if errors.Is(err, sql.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	if fail(w, r, err) {
		return
	}
	if body.Note != nil {
		note, err := checkNote(*body.Note)
		if fail(w, r, err) {
			return
		}
		if row, err = m.q.SetForecastNote(ctx, db.SetForecastNoteParams{Note: note, ID: row.ID}); fail(w, r, err) {
			return
		}
	}
	httpx.JSON(w, http.StatusOK, toForecast(row, m.today()))
}

func (m *Module) DeleteForecast(w http.ResponseWriter, r *http.Request, forecastId api.ForecastId) {
	n, err := m.q.DeleteForecast(r.Context(), forecastId)
	m.d.Audit.Record(r.Context(), "forecasts.delete", strconv.FormatInt(forecastId, 10), nil, err)
	if fail(w, r, err) {
		return
	}
	if n == 0 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) ResolveForecast(w http.ResponseWriter, r *http.Request, forecastId api.ForecastId) {
	var body api.ResolveForecastJSONBody
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	f, err := m.resolve(r.Context(), forecastId, body.Outcome, body.Note)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, f)
}

func (m *Module) ReopenForecast(w http.ResponseWriter, r *http.Request, forecastId api.ForecastId) {
	if _, err := m.get(r.Context(), forecastId); fail(w, r, err) {
		return
	}
	row, err := m.q.ReopenForecast(r.Context(), forecastId)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, toForecast(row, m.today()))
}

func (m *Module) GetForecastStats(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListScoredForecasts(r.Context())
	if fail(w, r, err) {
		return
	}
	items := make([]scored, 0, len(rows))
	for _, row := range rows {
		if row.Probability == nil || row.Outcome == nil {
			continue
		}
		items = append(items, scored{Probability: int(*row.Probability), Yes: *row.Outcome == "yes"})
	}
	httpx.JSON(w, http.StatusOK, calibrate(items))
}

func defaults() api.ForecastSettings { return api.ForecastSettings{Remind: true, RemindTime: "09:00"} }

func (m *Module) options(ctx context.Context) api.ForecastSettings {
	var saved api.ForecastSettings
	err := m.d.Settings.Get(ctx, OptionsKey, &saved)
	if err == nil && clockRe.MatchString(saved.RemindTime) {
		return saved
	}
	if err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("load forecasts settings", "err", err)
	}
	return defaults()
}

func (m *Module) GetForecastSettings(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, m.options(r.Context()))
}

func (m *Module) PutForecastSettings(w http.ResponseWriter, r *http.Request) {
	var body api.ForecastSettings
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	if !clockRe.MatchString(body.RemindTime) {
		httpx.Fail(w, r, httpx.Invalid("时间要写成 HH:MM，比如 09:00"))
		return
	}
	err := m.d.Settings.Set(r.Context(), OptionsKey, body)
	m.d.Audit.Record(r.Context(), "forecasts.settings", "", map[string]any{"remind": body.Remind}, err)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, body)
}

// beforeRemindTime says whether the reminder time of today has not come yet.
func (m *Module) beforeRemindTime(ctx context.Context) bool {
	opts := m.options(ctx)
	now := m.now().In(m.loc())
	hh, _ := strconv.Atoi(opts.RemindTime[:2])
	mm, _ := strconv.Atoi(opts.RemindTime[3:])
	return now.Hour()*60+now.Minute() < hh*60+mm
}

// dueTick reminds once for every record whose due date has come and that has
// no outcome. Records that fell due while the server was off are included.
func (m *Module) dueTick(ctx context.Context) error {
	if !m.options(ctx).Remind || m.beforeRemindTime(ctx) {
		return nil
	}
	rows, err := m.q.ListDueUnnotified(ctx, m.today())
	if err != nil {
		return err
	}
	for _, row := range rows {
		// Marked before sending: a failed send is not retried every minute.
		if err := m.q.SetForecastNotified(ctx, row.ID); err != nil {
			return err
		}
		title := "到期了，填一下结果：" + clip(row.Statement, 40)
		if _, err := m.d.Notify.Send(ctx, notify.Notification{Kind: "forecast.due", Title: title,
			Body: "点开选成了、没成或作废。", Link: "/forecasts", Source: "forecasts", Immediate: true}); err != nil {
			return err
		}
	}
	return nil
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
