package quotas

import (
	"context"
	"time"
)

// Hooks for the integration tests in package quotas_test, which cannot live
// in this package because testutil imports app, which imports quotas.

func Tick(m *Module, ctx context.Context) error { return m.tick(ctx) }

func SetDeepSeekURL(m *Module, url string) { m.deepseekURL = url }

func SetNow(m *Module, now func() time.Time) { m.now = now }
