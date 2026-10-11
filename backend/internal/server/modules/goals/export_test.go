package goals

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/goals/api"
)

// Hooks for the integration tests in package goals_test, which cannot live in
// this package because testutil imports app, which imports goals.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func RemindTick(m *Module, ctx context.Context, now time.Time) error { return m.remindTick(ctx, now) }

func QuarterBounds(year, quarter int, loc *time.Location) (time.Time, time.Time) {
	return quarterBounds(year, quarter, loc)
}

func ExpectedPercent(from, to, now time.Time) float64 { return expectedPercent(from, to, now) }

func ResultPercent(value, start, target float64) float64 { return resultPercent(value, start, target) }

func StateOf(progress *float64, expected float64, from, to, now time.Time) api.GoalState {
	return stateOf(progress, expected, from, to, now)
}

func WeekStart(t time.Time, loc *time.Location) time.Time { return weekStart(t, loc) }
