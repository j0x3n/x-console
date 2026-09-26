package monitoring

import (
	"context"
	"crypto/x509"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/db"
)

// Hooks for the integration tests in package monitoring_test, which cannot
// live in this package because testutil imports app, which imports this
// package.

const SelfKey = selfKey

func (m *Module) Queries() *db.Queries                              { return m.q }
func (m *Module) SetTLSRoots(p *x509.CertPool)                      { m.tlsRoots.Store(p) }
func (m *Module) CheckDue(ctx context.Context, now time.Time) error { return m.checkDue(ctx, now) }
func (m *Module) Cleanup(ctx context.Context, now time.Time) error  { return m.cleanup(ctx, now) }

func (m *Module) CheckNow(ctx context.Context, id int64, now time.Time) (db.MonitorResult, error) {
	return m.checkNow(ctx, id, now)
}

func (m *Module) ScanSubscriptions(ctx context.Context, now time.Time) error {
	return m.scanSubscriptions(ctx, now)
}
