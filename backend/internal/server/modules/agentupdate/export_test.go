package agentupdate

import "time"

// Hooks for the integration tests in package agentupdate_test, which cannot
// live in this package because testutil imports app, which imports it.

// SetVersion sets the version of the agent programs the panel carries.
func SetVersion(m *Module, v string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.version = func() string { return v }
}

// SetNow sets the clock used for the one-hour rule.
func SetNow(m *Module, now func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nowFn = now
}

// SetWaits sets how long to wait for the new version to connect and how long
// to watch the stream after the agent said it swapped the program.
func SetWaits(m *Module, restart, end time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartWait, m.endWait = restart, end
}
