package forecasts

import (
	"context"
	"time"
)

// Hooks for the integration tests in package forecasts_test, which cannot live
// in this package because testutil imports app, which imports forecasts.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func Loc(m *Module) *time.Location { return m.loc() }

func DueTick(m *Module, ctx context.Context) error { return m.dueTick(ctx) }
