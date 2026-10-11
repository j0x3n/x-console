package me

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/me/api"
)

// Hooks for the integration tests in package me_test, which cannot live in
// this package because testutil imports app, which imports me.

// SetNow replaces the clock.
func SetNow(m *Module, now func() time.Time) {
	m.mu.Lock()
	m.nowFn = now
	m.mu.Unlock()
}

// Host is a computer for tests.
type Host struct {
	ID     string
	Name   string
	Online bool
	State  string
	Idle   time.Duration
}

// SetHosts replaces the computers that report idle time.
func SetHosts(m *Module, hosts func() []Host) {
	m.mu.Lock()
	m.listHosts = func(context.Context) []hostInfo {
		var out []hostInfo
		for _, h := range hosts() {
			idle := h.Idle
			out = append(out, hostInfo{ID: h.ID, Name: h.Name, Online: h.Online, State: h.State, Idle: &idle})
		}
		return out
	}
	m.mu.Unlock()
}

// Recompute decides again, as the minute tick does.
func Recompute(m *Module, ctx context.Context) { m.recompute(ctx) }

// Cleanup runs the clean-up job.
func Cleanup(m *Module, ctx context.Context) error { return m.cleanup(ctx) }

// WatchHA runs the Home Assistant start-up read with the saved settings.
func WatchHA(m *Module, ctx context.Context) {
	m.mu.Lock()
	_ = m.ensureLoaded(ctx)
	cfg := m.cfg
	m.mu.Unlock()
	m.watchHA(ctx, cfg)
}

// Reset forgets everything the module remembered, rows included, so a test
// starts from an empty history at its own clock.
func Reset(m *Module, ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, _ = m.d.DB.ExecContext(ctx, `DELETE FROM me_states`)
	_, _ = m.d.DB.ExecContext(ctx, `DELETE FROM me_reports`)
	m.forgetLocked()
}

// Forget makes the module read its rows again, as after a restart.
func Forget(m *Module) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forgetLocked()
}

func (m *Module) forgetLocked() {
	m.loaded, m.cur, m.manual, m.lastActive, m.lastWoke = false, nil, nil, time.Time{}, time.Time{}
	m.view = api.MeState{}
}
