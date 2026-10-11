package knowledge

import (
	"context"
	"time"
)

// Hooks for the integration tests in package knowledge_test, which cannot live
// in this package because testutil imports app, which imports knowledge.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func Sync(m *Module, ctx context.Context) { m.sync(ctx) }

// Status returns the sync state kept in memory.
func LastError(m *Module) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr
}

func SetBudget(m *Module, n int) { m.budget = n }
