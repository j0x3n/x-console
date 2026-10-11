package habits

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// goalMetrics gives the goals module (B128) the number of check-ins of one
// habit and the number of workouts.
type goalMetrics struct{ m *Module }

func (goalMetrics) Kinds() []contracts.GoalMetricKind {
	return []contracts.GoalMetricKind{
		{Kind: contracts.GoalMetricHabitCheckins, Module: "habits", RefKind: contracts.GoalRefHabit},
		{Kind: contracts.GoalMetricWorkouts, Module: "habits", RefKind: contracts.GoalRefNone},
	}
}

// Options lists the habits that are not archived.
func (g goalMetrics) Options(ctx context.Context, kind string) ([]contracts.GoalMetricOption, error) {
	if kind != contracts.GoalMetricHabitCheckins {
		return nil, nil
	}
	rows, err := g.m.d.DB.QueryContext(ctx, `SELECT id, name FROM habits WHERE archived_at IS NULL ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contracts.GoalMetricOption
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out = append(out, contracts.GoalMetricOption{Value: strconv.FormatInt(id, 10), Label: name})
	}
	return out, rows.Err()
}

// Measure counts check-in rows of the habit ref, or workout logs.
func (g goalMetrics) Measure(ctx context.Context, kind, ref string, from, until time.Time) (float64, error) {
	var n int64
	switch kind {
	case contracts.GoalMetricHabitCheckins:
		id, err := strconv.ParseInt(ref, 10, 64)
		if err != nil || id <= 0 {
			return 0, errors.New("没有选习惯")
		}
		err = g.m.d.DB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM habit_logs WHERE habit_id = ? AND at >= ? AND at < ?`, id, from.UTC(), until.UTC()).Scan(&n)
		return float64(n), err
	case contracts.GoalMetricWorkouts:
		err := g.m.d.DB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM workout_logs WHERE created_at >= ? AND created_at < ?`, from.UTC(), until.UTC()).Scan(&n)
		return float64(n), err
	}
	return 0, errors.New("不认识的数据来源")
}
