package journal

import (
	"context"
	"time"
)

// Hooks for the integration tests in package journal_test, which cannot live
// in this package because testutil imports app, which imports journal.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func Collect(m *Module, ctx context.Context, from, until time.Time) { m.collect(ctx, from, until) }

func CollectRecent(m *Module, ctx context.Context) { m.collectRecent(ctx) }

func Backfill(m *Module, ctx context.Context) error { return m.backfill(ctx) }
