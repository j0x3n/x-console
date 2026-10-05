package github

import (
	"context"
	"time"
)

// Hooks for the external test package, which cannot reach internals
// (testutil imports app, which imports this package).

func (m *Module) ScheduledSync(ctx context.Context) error { return m.scheduledSync(ctx) }

func (m *Module) SetNow(now func() time.Time)            { m.now = now }
func (m *Module) MigrateB62(ctx context.Context) error   { return m.migrateB62(ctx) }
func (m *Module) MigrateB70(ctx context.Context) error   { return m.migrateB70(ctx) }
func (m *Module) FlushNotify(ctx context.Context) error  { return m.flushNotify(ctx) }
func (m *Module) CheckCIQuota(ctx context.Context) error { return m.checkCIQuota(ctx) }
