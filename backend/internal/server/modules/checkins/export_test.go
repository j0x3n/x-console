package checkins

import (
	"context"
	"time"
)

// Hooks for the integration tests in package checkins_test, which cannot live
// in this package because testutil imports app, which imports checkins.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func AskTick(m *Module, ctx context.Context) error { return m.askTick(ctx) }

func Loc(m *Module) *time.Location { return m.loc() }
