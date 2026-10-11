package focus

import (
	"context"
	"errors"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// goalMetrics gives the goals module (B128) the minutes of finished focus
// sessions. Like the journal, it leaves out sessions shorter than a minute.
type goalMetrics struct{ m *Module }

func (goalMetrics) Kinds() []contracts.GoalMetricKind {
	return []contracts.GoalMetricKind{
		{Kind: contracts.GoalMetricFocusMinutes, Module: "calendar", RefKind: contracts.GoalRefNone},
	}
}

func (goalMetrics) Options(context.Context, string) ([]contracts.GoalMetricOption, error) {
	return nil, nil
}

func (g goalMetrics) Measure(ctx context.Context, kind, _ string, from, until time.Time) (float64, error) {
	if kind != contracts.GoalMetricFocusMinutes {
		return 0, errors.New("不认识的数据来源")
	}
	var seconds int64
	err := g.m.d.DB.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(actual_seconds), 0) FROM focus_sessions
		 WHERE started_at >= ? AND started_at < ? AND ended_at IS NOT NULL AND actual_seconds >= 60`,
		from.UTC(), until.UTC()).Scan(&seconds)
	return float64(seconds) / 60, err
}
