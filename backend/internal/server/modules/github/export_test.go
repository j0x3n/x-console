package github

import (
	"context"
	"time"
)

// Hooks for the external test package, which cannot reach internals
// (testutil imports app, which imports this package).

func (m *Module) ScheduledSync(ctx context.Context) error { return m.scheduledSync(ctx) }

func (m *Module) SetNow(now func() time.Time) { m.now = now }
