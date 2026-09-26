package habits

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
)

// Hooks for the external test package (testutil imports app, which imports
// this package, so integration tests live in habits_test).

func Tick(m *Module, ctx context.Context, now time.Time) error { return m.tick(ctx, now) }

func CheckinAt(m *Module, ctx context.Context, id int64, amount float64, source string, now time.Time) error {
	_, _, err := m.checkin(ctx, id, amount, "", source, now)
	return err
}

func TodayAt(m *Module, ctx context.Context, now time.Time) ([]api.HabitToday, error) {
	return m.today(ctx, now)
}
