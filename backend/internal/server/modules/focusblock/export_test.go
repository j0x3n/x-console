package focusblock

import (
	"context"
	"time"
)

// Hooks for the external test package. It cannot live in this package
// because testutil imports app, which imports this module.

// Reconcile brings the computers to the state the settings ask for now.
// force lists agents that are told the state whatever the records say.
func Reconcile(m *Module, ctx context.Context, force ...string) { m.reconcile(ctx, force) }

// SetHACall replaces the call that turns a Home Assistant scene or script on.
func SetHACall(m *Module, f func(ctx context.Context, entity string) error) {
	m.mu.Lock()
	m.haCall = f
	m.mu.Unlock()
}

// SetNow replaces the clock the module plans with.
func SetNow(m *Module, f func() time.Time) {
	m.mu.Lock()
	m.nowFn = f
	m.mu.Unlock()
}
