// Package monitoring is M10 (ops monitoring): Docker containers on agents,
// a script library that runs on one or many hosts, website / certificate /
// domain monitors, and subscriptions with renewal reminders.
// See docs/modules/M10.md.
package monitoring

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/db"
)

// selfKey lets tests find the module instance in the registry.
const selfKey = "monitoring.module"

const (
	checkTick       = 15 * time.Second
	subscriptionJob = 30 * time.Minute
	resultRetention = 30 * 24 * time.Hour
	// checkWorkers caps how many monitors are checked at the same time.
	checkWorkers = 8
)

// Module implements api.ServerInterface.
type Module struct {
	d   *module.Deps
	q   *db.Queries
	now func() time.Time

	// base is the context of background work (script runs); it ends when
	// the server stops. Set in Start.
	baseMu sync.Mutex
	base   context.Context

	// tlsRoots replaces the system roots for certificate checks (tests).
	tlsRoots atomic.Pointer[x509.CertPool]

	// busy holds a lock per monitor so a manual check and the scheduler
	// never process the same monitor at once.
	busyMu sync.Mutex
	busy   map[int64]*sync.Mutex
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module and registers its actions.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), now: func() time.Time { return time.Now().UTC() }, busy: map[int64]*sync.Mutex{}}
	module.Provide(d.Registry, selfKey, m)
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "monitoring" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start marks interrupted script runs and registers the background jobs.
func (m *Module) Start(ctx context.Context) error {
	m.baseMu.Lock()
	m.base = ctx
	m.baseMu.Unlock()
	if _, err := m.q.AbortUnfinishedRuns(ctx, db.AbortUnfinishedRunsParams{FinishedAt: ptr(m.now()), Error: "服务重启，运行被中断"}); err != nil {
		return err
	}
	m.d.Scheduler.Every("monitoring.check", checkTick, func(ctx context.Context) error {
		return m.checkDue(ctx, m.now())
	})
	m.d.Scheduler.Every("monitoring.subscriptions", subscriptionJob, func(ctx context.Context) error {
		return m.scanSubscriptions(ctx, m.now())
	})
	m.d.Scheduler.Every("monitoring.cleanup", time.Hour, func(ctx context.Context) error {
		return m.cleanup(ctx, m.now())
	})
	return nil
}

// background returns the context for work that outlives a request.
func (m *Module) background() context.Context {
	m.baseMu.Lock()
	defer m.baseMu.Unlock()
	if m.base == nil {
		return context.Background()
	}
	return m.base
}

// cleanup drops monitor results older than the retention.
func (m *Module) cleanup(ctx context.Context, now time.Time) error {
	_, err := m.q.DeleteMonitorResultsBefore(ctx, now.Add(-resultRetention))
	return err
}

// ---- helpers ----

func ptr[T any](v T) *T { return &v }

// decodeOptional decodes a JSON body that may be empty.
func decodeOptional(r *http.Request, v any) error {
	if r.ContentLength == 0 {
		return nil
	}
	err := httpx.Decode(r, v)
	var he *httpx.Error
	if errors.As(err, &he) && strings.Contains(he.Message, io.EOF.Error()) {
		return nil
	}
	return err
}

// notFound maps sql.ErrNoRows-style misses; callers pass the row count.
func notFound(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// clip shortens s to at most n bytes on a UTF-8 boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && cut < len(s) && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut] + "\n…(truncated)"
}

// errText is the message of err for storing and showing.
func errText(err error) string {
	var he *httpx.Error
	if errors.As(err, &he) {
		return he.Message
	}
	return err.Error()
}
