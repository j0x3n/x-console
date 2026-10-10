package contacts

import (
	"context"
	"time"
)

// Hooks for the integration tests in package contacts_test, which cannot
// live in this package because testutil imports app, which imports contacts.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func RemindAll(m *Module, ctx context.Context) error { return m.remindAll(ctx) }
