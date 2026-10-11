package goals

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals/db"
)

// behindMargin is how many percentage points below the expected progress a
// goal may be before it counts as behind.
const behindMargin = 10

// quarterBounds is the first moment of the quarter and the first moment of the
// next one, in loc.
func quarterBounds(year, quarter int, loc *time.Location) (time.Time, time.Time) {
	first := time.Month((quarter-1)*3 + 1)
	from := time.Date(year, first, 1, 0, 0, 0, 0, loc)
	return from, from.AddDate(0, 3, 0)
}

func quarterOf(t time.Time) (int, int) { return t.Year(), (int(t.Month())-1)/3 + 1 }

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// expectedPercent is how far the quarter has gone at now: 0 before it starts,
// 100 after it ends, linear in between.
func expectedPercent(from, to, now time.Time) float64 {
	if !now.After(from) {
		return 0
	}
	if !now.Before(to) {
		return 100
	}
	return clamp(float64(now.Sub(from))/float64(to.Sub(from))*100, 0, 100)
}

// resultPercent is the share of the way from start to target, between 0 and
// 100. It also works when the target is below the start (a weight to lose).
func resultPercent(value, start, target float64) float64 {
	if target == start {
		if value >= target {
			return 100
		}
		return 0
	}
	p := (value - start) / (target - start) * 100
	if math.IsNaN(p) || math.IsInf(p, 0) {
		return 0
	}
	return clamp(p, 0, 100)
}

// stateOf compares progress with the expected progress. A missing progress is
// never behind.
func stateOf(progress *float64, expected float64, from, to, now time.Time) api.GoalState {
	if progress != nil && *progress >= 100 {
		return api.GoalStateDone
	}
	if now.Before(from) {
		return api.GoalStateUpcoming
	}
	if !now.Before(to) {
		return api.GoalStateMissed
	}
	if progress != nil && *progress+behindMargin < expected {
		return api.GoalStateBehind
	}
	return api.GoalStateOnTrack
}

// average of the progress values that exist, or nil when none does.
func average(values []*float64) *float64 {
	var sum float64
	n := 0
	for _, v := range values {
		if v != nil {
			sum += *v
			n++
		}
	}
	if n == 0 {
		return nil
	}
	out := round1(sum / float64(n))
	return &out
}

func ptr(v float64) *float64 { return &v }

// measured is the value of one key result at one moment.
type measured struct {
	value       *float64
	progress    *float64
	unavailable string
}

// measure reads the value of a key result at the moment at (not after now)
// and turns it into a progress. Auto sources add up from the start of the
// quarter to at. Manual ones take the last value recorded by then.
func (m *Module) measure(ctx context.Context, cat *catalog, hidden func(string) bool, row db.GoalResult, from, to, at, now time.Time) measured {
	var value float64
	if row.Source == string(api.Manual) {
		v, err := m.manualAt(ctx, row, at, now)
		if err != nil {
			m.d.Log.Warn("goal manual value", "result", row.ID, "err", err)
			return measured{unavailable: "error"}
		}
		value = v
	} else {
		kind, ok := cat.kinds[row.Source]
		if !ok {
			return measured{unavailable: "missing"}
		}
		if hidden(kind.Module) {
			return measured{unavailable: "hidden"}
		}
		end := at
		if end.After(to) {
			end = to
		}
		if end.After(from) {
			v, err := cat.sources[row.Source].Measure(ctx, row.Source, row.Ref, from, end)
			if errors.Is(err, contracts.ErrGoalMetricMissing) {
				return measured{unavailable: "missing"}
			}
			if err != nil {
				m.d.Log.Warn("goal metric", "kind", row.Source, "err", err)
				return measured{unavailable: "error"}
			}
			value = v
		}
	}
	p := round1(resultPercent(value, startOf(row), row.Target))
	return measured{value: &value, progress: &p}
}

// startOf is where progress starts. Only manual key results have a start.
func startOf(row db.GoalResult) float64 {
	if row.Source == string(api.Manual) {
		return row.StartValue
	}
	return 0
}

// manualAt is the value of a manual key result at the moment at: the last
// value recorded by then, or the start. For the present it is the stored
// value.
func (m *Module) manualAt(ctx context.Context, row db.GoalResult, at, now time.Time) (float64, error) {
	if !at.Before(now) {
		return row.ManualValue, nil
	}
	v, err := m.q.LatestGoalResultLog(ctx, db.LatestGoalResultLogParams{ResultID: row.ID, At: at.UTC()})
	if errors.Is(err, sql.ErrNoRows) {
		return row.StartValue, nil
	}
	return v, err
}

// resultView builds the API form of a key result as of now.
func (m *Module) resultView(ctx context.Context, cat *catalog, hidden func(string) bool, row db.GoalResult, from, to, now time.Time) api.GoalResult {
	// auto sources stop at the end of the quarter on their own; a manual value
	// is whatever the user entered last, also after the quarter
	r := m.measure(ctx, cat, hidden, row, from, to, now, now)
	expected := expectedPercent(from, to, now)
	out := api.GoalResult{
		Id: row.ID, GoalId: row.GoalID, Name: row.Name, Target: row.Target, Unit: row.Unit, StartValue: startOf(row),
		Source: api.GoalSource(row.Source), Ref: row.Ref, RefLabel: m.refLabel(ctx, cat, row),
		Value: r.value, Progress: r.progress, Unavailable: r.unavailable, UpdatedAt: row.UpdatedAt,
	}
	out.State = stateOf(r.progress, expected, from, to, now)
	return out
}

// refLabel is the name of the habit or category a key result is bound to.
func (m *Module) refLabel(ctx context.Context, cat *catalog, row db.GoalResult) string {
	if row.Source == string(api.Manual) {
		return ""
	}
	src, ok := cat.sources[row.Source]
	if !ok {
		return ""
	}
	opts, err := src.Options(ctx, row.Source)
	if err != nil {
		return ""
	}
	for _, o := range opts {
		if o.Value == row.Ref {
			return o.Label
		}
	}
	return ""
}

// goalView builds the API form of a goal as of now.
func (m *Module) goalView(ctx context.Context, cat *catalog, hidden func(string) bool, g db.Goal, rows []db.GoalResult, now time.Time) api.Goal {
	from, to := quarterBounds(int(g.Year), int(g.Quarter), m.loc())
	expected := round1(expectedPercent(from, to, now))
	results := make([]api.GoalResult, 0, len(rows))
	progress := make([]*float64, 0, len(rows))
	for _, row := range rows {
		r := m.resultView(ctx, cat, hidden, row, from, to, now)
		results = append(results, r)
		progress = append(progress, r.Progress)
	}
	avg := average(progress)
	return api.Goal{
		Id: g.ID, Title: g.Title, Year: int(g.Year), Quarter: int(g.Quarter), Note: g.Note, Status: api.GoalStatus(g.Status),
		Results: results, Progress: avg, Expected: expected, State: stateOf(avg, expected, from, to, now),
		From: from.Format(dateLayout), Until: to.AddDate(0, 0, -1).Format(dateLayout),
		CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt,
	}
}
