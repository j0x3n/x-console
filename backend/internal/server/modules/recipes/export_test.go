package recipes

import (
	"context"
	"time"
)

// Hooks for the integration tests in package recipes_test, which cannot
// live in this package because testutil imports app, which imports recipes.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func CheckTimers(m *Module, ctx context.Context, now time.Time) error { return m.checkTimers(ctx, now) }
