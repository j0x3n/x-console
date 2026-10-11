package skills

import "time"

// Hooks for the integration tests in package skills_test, which cannot live
// in this package because testutil imports app, which imports skills.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func Loc(m *Module) *time.Location { return m.loc() }
