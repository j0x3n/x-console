package dayplan

import (
	"context"
	"time"
)

// Hooks for the integration tests in package dayplan_test, which cannot live
// in this package because testutil imports app, which imports dayplan.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func Loc(m *Module) *time.Location { return m.loc() }

func Tick(m *Module, ctx context.Context) error { return m.tick(ctx) }
