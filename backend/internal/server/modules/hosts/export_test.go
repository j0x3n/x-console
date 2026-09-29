package hosts

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Hooks for the integration tests in package hosts_test, which cannot live
// in this package because testutil imports app, which imports hosts.

type HostMetricsEvent = hostMetricsEvent

var (
	SSHHostID  = sshHostID
	CannedProc = cannedProc
)

func (m *Module) Queries() *db.Queries                             { return m.q }
func (m *Module) EvaluateAlerts(ctx context.Context) error         { return m.evaluateAlerts(ctx) }
func (m *Module) Rollup(ctx context.Context) error                 { return m.rollup(ctx) }
func (m *Module) Cleanup(ctx context.Context) error                { return m.cleanup(ctx) }
func (m *Module) PollSSH(ctx context.Context) error                { return m.pollSSH(ctx) }
func (m *Module) RecordSample(id string, x protocol.MetricsSample) { m.recordSample(id, x) }
func (m *Module) AddSample(id string, x protocol.MetricsSample)    { m.metrics.add(id, x) }
func (m *Module) DropSSH(id int64)                                 { m.ssh.drop(id) }

func (m *Module) LatestSample(id string) (protocol.MetricsSample, bool) { return m.metrics.latest(id) }

func (m *Module) Series(ctx context.Context, id, rng string) (api.MetricsSeries, error) {
	return m.series(ctx, id, rng)
}

func (m *Module) SetNow(now time.Time)                   { m.now = func() time.Time { return now } }
func (m *Module) FlushTraffic(ctx context.Context) error { return m.flushTraffic(ctx) }
func (m *Module) CheckTraffic(ctx context.Context) error { return m.checkTraffic(ctx) }
