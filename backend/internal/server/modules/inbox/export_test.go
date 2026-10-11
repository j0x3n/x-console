package inbox

import (
	"context"
	"time"
)

// Hooks for the integration tests in package inbox_test, which cannot live in
// this package because testutil imports app, which imports inbox.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func Loc(m *Module) *time.Location { return m.loc() }

// WaitSuggestions blocks until the background suggestions are done.
func WaitSuggestions(m *Module) { m.async.Wait() }

func ReviewTick(m *Module, ctx context.Context) error { return m.reviewTick(ctx) }
