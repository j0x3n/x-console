package cards

import (
	"context"
	"time"
)

// Hooks for the integration tests in package cards_test, which cannot live
// in this package because testutil imports app, which imports cards.

// SetNow replaces the clock.
func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

// PushJob runs the daily push once.
func PushJob(m *Module, ctx context.Context) error { return m.pushJob(ctx) }

// HandleAction presses a Telegram button.
func HandleAction(m *Module, ctx context.Context, id string) error { return m.handleAction(ctx, id) }
