package experiments

import "time"

// Hooks for the integration tests in package experiments_test, which cannot
// live in this package because testutil imports app, which imports experiments.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func Loc(m *Module) *time.Location { return m.loc() }
