package goals

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals/db"
)

// refreshDays is how long after a week ends its review still takes new
// progress numbers when it is saved. After that the stored snapshot stays.
const refreshDays = 7

// weekStart is the Monday 00:00 of the week t falls in.
func weekStart(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	back := (int(t.Weekday()) + 6) % 7
	return time.Date(t.Year(), t.Month(), t.Day()-back, 0, 0, 0, 0, loc)
}

// parseWeek turns "current" or any date into the Monday of that week. Weeks
// that have not started are refused.
func (m *Module) parseWeek(week string) (time.Time, error) {
	loc := m.loc()
	this := weekStart(m.now(), loc)
	if week == "current" {
		return this, nil
	}
	t, err := time.ParseInLocation(dateLayout, week, loc)
	if err != nil {
		return time.Time{}, httpx.Invalid("周要写成 current 或 YYYY-MM-DD")
	}
	ws := weekStart(t, loc)
	if ws.After(this) {
		return time.Time{}, httpx.Invalid("还没到这一周")
	}
	return ws, nil
}

// weekGoals works out how each active goal moved in the week that starts at
// ws. Goals whose quarter does not touch the week are left out. The week is
// measured up to now when it is not over yet.
func (m *Module) weekGoals(ctx context.Context, ws, now time.Time) ([]api.GoalWeekGoal, error) {
	we := ws.AddDate(0, 0, 7)
	end := we
	if now.Before(end) {
		end = now
	}
	goals, err := m.q.ListGoals(ctx)
	if err != nil {
		return nil, err
	}
	all, err := m.q.ListAllGoalResults(ctx)
	if err != nil {
		return nil, err
	}
	byGoal := map[int64][]db.GoalResult{}
	for _, r := range all {
		byGoal[r.GoalID] = append(byGoal[r.GoalID], r)
	}
	cat, hidden := m.catalog(), m.hiddenFn(ctx)
	out := []api.GoalWeekGoal{}
	// the list is newest quarter first; a review reads better oldest first
	for i := len(goals) - 1; i >= 0; i-- {
		g := goals[i]
		if g.Status != string(api.GoalStatusActive) {
			continue
		}
		from, to := quarterBounds(int(g.Year), int(g.Quarter), m.loc())
		if !from.Before(we) || !to.After(ws) {
			continue
		}
		wg := api.GoalWeekGoal{GoalId: g.ID, Title: g.Title, Year: int(g.Year), Quarter: int(g.Quarter), Results: []api.GoalWeekResult{}}
		var before, after []*float64
		for _, row := range byGoal[g.ID] {
			a := m.measure(ctx, cat, hidden, row, from, to, end, now)
			b := m.measure(ctx, cat, hidden, row, from, to, ws, now)
			wr := api.GoalWeekResult{
				Id: row.ID, Name: row.Name, Unit: row.Unit, Source: api.GoalSource(row.Source), Target: row.Target,
				Value: a.value, Progress: a.progress, Unavailable: a.unavailable,
				State: stateOf(a.progress, expectedPercent(from, to, end), from, to, end),
			}
			if a.value != nil && b.value != nil {
				c := round1(*a.value - *b.value)
				wr.Change = &c
			}
			wg.Results = append(wg.Results, wr)
			after = append(after, a.progress)
			if a.progress != nil {
				before = append(before, b.progress)
			}
		}
		wg.Progress = average(after)
		wg.ProgressBefore = average(before)
		if wg.Progress != nil && wg.ProgressBefore != nil {
			c := round1(*wg.Progress - *wg.ProgressBefore)
			wg.Change = &c
		}
		wg.Expected = round1(expectedPercent(from, to, end))
		wg.State = stateOf(wg.Progress, wg.Expected, from, to, end)
		out = append(out, wg)
	}
	return out, nil
}

// maskHidden takes the numbers of hidden modules out of a stored snapshot, and
// then the goal's own numbers too, since they were averaged with them.
func (m *Module) maskHidden(ctx context.Context, goals []api.GoalWeekGoal) {
	cat, hidden := m.catalog(), m.hiddenFn(ctx)
	for gi := range goals {
		g := &goals[gi]
		masked := false
		var progress []*float64
		for ri := range g.Results {
			r := &g.Results[ri]
			if r.Source != api.Manual {
				if kind, ok := cat.kinds[string(r.Source)]; ok && hidden(kind.Module) {
					r.Value, r.Change, r.Progress, r.Unavailable = nil, nil, nil, "hidden"
					masked = true
				}
			}
			progress = append(progress, r.Progress)
		}
		if masked {
			g.Progress = average(progress)
			g.ProgressBefore, g.Change = nil, nil
		}
	}
}

func answered(r db.GoalReview) bool {
	return strings.TrimSpace(r.WentWell+r.FellShort+r.NextWeek) != ""
}

// review builds the review page of one week.
func (m *Module) review(ctx context.Context, week string) (api.GoalReview, error) {
	ws, err := m.parseWeek(week)
	if err != nil {
		return api.GoalReview{}, err
	}
	now := m.now()
	out := api.GoalReview{
		WeekStart: ws.Format(dateLayout), WeekEnd: ws.AddDate(0, 0, 6).Format(dateLayout),
		Current: ws.Equal(weekStart(now, m.loc())), Goals: []api.GoalWeekGoal{},
	}
	row, err := m.q.GetGoalReview(ctx, out.WeekStart)
	switch {
	case err == nil:
		out.Saved = true
		out.WentWell, out.FellShort, out.NextWeek = row.WentWell, row.FellShort, row.NextWeek
		t := row.UpdatedAt
		out.UpdatedAt = &t
		var goals []api.GoalWeekGoal
		if json.Unmarshal([]byte(row.Snapshot), &goals) == nil && goals != nil {
			out.Goals = goals
			m.maskHidden(ctx, out.Goals)
		} else if out.Goals, err = m.weekGoals(ctx, ws, now); err != nil {
			return api.GoalReview{}, err
		}
	case errors.Is(err, sql.ErrNoRows):
		if out.Goals, err = m.weekGoals(ctx, ws, now); err != nil {
			return api.GoalReview{}, err
		}
	default:
		return api.GoalReview{}, err
	}
	prev, err := m.q.GetGoalReview(ctx, ws.AddDate(0, 0, -7).Format(dateLayout))
	if err == nil {
		out.LastPlan = prev.NextWeek
	} else if !errors.Is(err, sql.ErrNoRows) {
		return api.GoalReview{}, err
	}
	return out, nil
}

// GetGoalReview implements api.ServerInterface.
func (m *Module) GetGoalReview(w http.ResponseWriter, r *http.Request, week string) {
	out, err := m.review(r.Context(), week)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// SaveGoalReview implements api.ServerInterface.
func (m *Module) SaveGoalReview(w http.ResponseWriter, r *http.Request, week string) {
	var body api.GoalReviewInput
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	out, err := m.saveReview(r.Context(), week, body)
	m.d.Audit.Record(r.Context(), "goal.review", week, nil, err)
	if fail(w, r, err) {
		return
	}
	m.d.Bus.Publish("goal.review_saved", map[string]any{"weekStart": out.WeekStart})
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) saveReview(ctx context.Context, week string, in api.GoalReviewInput) (api.GoalReview, error) {
	ws, err := m.parseWeek(week)
	if err != nil {
		return api.GoalReview{}, err
	}
	key := ws.Format(dateLayout)
	old, err := m.q.GetGoalReview(ctx, key)
	saved := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return api.GoalReview{}, err
	}
	pick := func(label string, v *string, cur string) (string, error) {
		if v == nil {
			return cur, nil
		}
		return checkTitle(label, *v, maxNote, false)
	}
	wentWell, err := pick("“做得好”的回答", in.WentWell, old.WentWell)
	if err != nil {
		return api.GoalReview{}, err
	}
	fellShort, err := pick("“没做到”的回答", in.FellShort, old.FellShort)
	if err != nil {
		return api.GoalReview{}, err
	}
	nextWeek, err := pick("“下周安排”的回答", in.NextWeek, old.NextWeek)
	if err != nil {
		return api.GoalReview{}, err
	}

	now := m.now()
	snapshot := old.Snapshot
	var goals []api.GoalWeekGoal
	if !saved || now.Before(ws.AddDate(0, 0, 7+refreshDays)) || json.Unmarshal([]byte(snapshot), &goals) != nil {
		// the snapshot keeps the real numbers, also while hidden content is
		// locked; reading it masks them again
		if goals, err = m.weekGoals(contracts.IgnoreHidden(ctx), ws, now); err != nil {
			return api.GoalReview{}, err
		}
		raw, err := json.Marshal(goals)
		if err != nil {
			return api.GoalReview{}, err
		}
		snapshot = string(raw)
	}
	var progress []*float64
	for _, g := range goals {
		progress = append(progress, g.Progress)
	}
	stamp := now.UTC()
	if _, err := m.q.UpsertGoalReview(ctx, db.UpsertGoalReviewParams{
		WeekStart: key, WentWell: wentWell, FellShort: fellShort, NextWeek: nextWeek, Snapshot: snapshot,
		Progress: average(progress), CreatedAt: stamp, UpdatedAt: stamp,
	}); err != nil {
		return api.GoalReview{}, err
	}
	return m.review(ctx, week)
}

// ListGoalReviews implements api.ServerInterface.
func (m *Module) ListGoalReviews(w http.ResponseWriter, r *http.Request, params api.ListGoalReviewsParams) {
	limit := 50
	if params.Limit != nil {
		limit = int(*params.Limit)
	}
	rows, err := m.q.ListGoalReviews(r.Context(), int64(limit))
	if fail(w, r, err) {
		return
	}
	items := make([]api.GoalReviewSummary, 0, len(rows))
	for _, row := range rows {
		start, perr := time.ParseInLocation(dateLayout, row.WeekStart, m.loc())
		if perr != nil {
			continue
		}
		items = append(items, api.GoalReviewSummary{
			WeekStart: row.WeekStart, WeekEnd: start.AddDate(0, 0, 6).Format(dateLayout),
			Answered: answered(row), Progress: row.Progress, UpdatedAt: row.UpdatedAt,
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}
