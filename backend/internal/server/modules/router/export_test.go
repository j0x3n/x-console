package router

import "time"

// Hooks for the external test package.

func SetNow(m *Module, now func() time.Time) { m.now = now }
