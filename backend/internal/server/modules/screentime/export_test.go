package screentime

import (
	"context"
	"time"
)

// Hooks for the integration tests in package screentime_test, which cannot
// live in this package because testutil imports app, which imports screentime.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func Cleanup(m *Module, ctx context.Context) error { return m.cleanup(ctx) }
