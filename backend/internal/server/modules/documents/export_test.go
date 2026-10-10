package documents

import (
	"context"
	"time"
)

// Hooks for the integration tests in package documents_test, which cannot
// live in this package because testutil imports app, which imports documents.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func RemindAll(m *Module, ctx context.Context) error { return m.remindAll(ctx) }
