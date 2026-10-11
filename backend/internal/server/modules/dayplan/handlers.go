package dayplan

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dayplan/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dayplan/db"
)

func (m *Module) dateParam(p *string) (string, error) {
	if p == nil || *p == "" {
		return m.today(), nil
	}
	if _, _, err := m.dayBounds(*p); err != nil {
		return "", errBadDate
	}
	return *p, nil
}

func (m *Module) GetDayPlan(w http.ResponseWriter, r *http.Request, p api.GetDayPlanParams) {
	ctx := r.Context()
	date, err := m.dateParam(p.Date)
	if fail(w, r, err) {
		return
	}
	out := map[string]any{"date": date, "slots": m.freeSlotsFor(ctx, date)}
	plan, err := m.load(ctx, date)
	if err == nil {
		out["plan"] = plan
	} else if !errors.Is(err, httpx.ErrNotFound) {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) GenerateDayPlan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Date    *string `json:"date"`
		Replace *bool   `json:"replace"`
	}
	if r.ContentLength != 0 {
		if err := httpx.Decode(r, &body); fail(w, r, err) {
			return
		}
	}
	date, err := m.dateParam(body.Date)
	if fail(w, r, err) {
		return
	}
	plan, err := m.generate(r.Context(), date, body.Replace != nil && *body.Replace)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, plan)
}

// cleanItems checks the items the user sent. A timed item needs both ends in
// one day, and timed items must not overlap.
func cleanItems(in []api.DayPlanItemInput) ([]itemRow, error) {
	if len(in) > maxItems {
		return nil, httpx.Invalid("一天最多 8 件事")
	}
	rows := make([]itemRow, 0, len(in))
	var timed []span
	for _, it := range in {
		title := strings.TrimSpace(it.Title)
		if title == "" || utf8.RuneCountInString(title) > 200 {
			return nil, httpx.Invalid("每件事要写 1 到 200 个字")
		}
		row := itemRow{Title: title, Kind: "manual"}
		if it.Kind != nil && it.Kind.Valid() {
			row.Kind = string(*it.Kind)
		}
		if it.Ref != nil {
			row.Ref = strings.TrimSpace(*it.Ref)
		}
		if it.Reason != nil {
			row.Reason = strings.TrimSpace(*it.Reason)
		}
		row.Done = it.Done != nil && *it.Done
		start, end := "", ""
		if it.Start != nil {
			start = *it.Start
		}
		if it.End != nil {
			end = *it.End
		}
		if start != "" || end != "" {
			s, ok1 := parseHM(start)
			e, ok2 := parseHM(end)
			if !ok1 || !ok2 {
				return nil, httpx.Invalid("开始和结束时间都要写成 HH:MM")
			}
			if e <= s {
				return nil, httpx.Invalid("结束时间要晚于开始时间")
			}
			for _, t := range timed {
				if s < t.End && t.Start < e {
					return nil, httpx.Invalid("两件事的时间重叠了")
				}
			}
			timed = append(timed, span{s, e})
			row.Start, row.End = start, end
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (m *Module) PutDayPlanItems(w http.ResponseWriter, r *http.Request, date string) {
	ctx := r.Context()
	var body struct {
		Items []api.DayPlanItemInput `json:"items"`
	}
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	if _, _, err := m.dayBounds(date); err != nil {
		httpx.Fail(w, r, errBadDate)
		return
	}
	rows, err := cleanItems(body.Items)
	if fail(w, r, err) {
		return
	}
	status := "draft"
	cur, err := m.q.GetDayPlan(ctx, date)
	switch {
	case err == nil:
		status = cur.Status
	case errors.Is(err, sql.ErrNoRows):
		if date < m.today() {
			httpx.Fail(w, r, errPast)
			return
		}
	default:
		httpx.Fail(w, r, err)
		return
	}
	// An item that keeps its title and time keeps its "already reminded".
	old, _ := m.q.ListDayPlanItems(ctx, date)
	notified := map[string]bool{}
	for _, o := range old {
		if o.Notified == 1 {
			notified[o.Title+"|"+o.StartTime+"|"+o.EndTime] = true
		}
	}
	for i := range rows {
		rows[i].Notified = notified[rows[i].Title+"|"+rows[i].Start+"|"+rows[i].End]
	}
	if fail(w, r, m.savePlan(ctx, date, status, "manual", rows)) {
		return
	}
	m.d.Audit.Record(ctx, "dayplan.edit", date, map[string]any{"items": len(rows)}, nil)
	m.d.Bus.Publish("dayplan.changed", map[string]any{"date": date})
	plan, err := m.load(ctx, date)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, plan)
}

func (m *Module) ConfirmDayPlan(w http.ResponseWriter, r *http.Request, date string) {
	ctx := r.Context()
	plan, err := m.load(ctx, date)
	if fail(w, r, err) {
		return
	}
	if len(plan.Items) == 0 {
		httpx.Fail(w, r, httpx.Invalid("计划里还没有事，先加一件"))
		return
	}
	n, err := m.q.ConfirmDayPlan(ctx, db.ConfirmDayPlanParams{ConfirmedAt: m.nowUTC(), Date: date})
	if fail(w, r, err) {
		return
	}
	if n == 0 {
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "conflict", "这一天的计划已经确认过了"))
		return
	}
	m.d.Audit.Record(ctx, "dayplan.confirm", date, nil, nil)
	m.d.Bus.Publish("dayplan.changed", map[string]any{"date": date})
	out, err := m.load(ctx, date)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) SetDayPlanItemDone(w http.ResponseWriter, r *http.Request, id int64) {
	var body struct {
		Done bool `json:"done"`
	}
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	date, err := m.setDone(r.Context(), id, body.Done)
	if fail(w, r, err) {
		return
	}
	plan, err := m.load(r.Context(), date)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, plan)
}

func (m *Module) setDone(ctx context.Context, id int64, done bool) (string, error) {
	it, err := m.q.GetDayPlanItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", httpx.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if err := m.q.SetDayPlanItemDone(ctx, doneParams(id, done)); err != nil {
		return "", err
	}
	m.d.Bus.Publish("dayplan.changed", map[string]any{"date": it.PlanDate})
	return it.PlanDate, nil
}
