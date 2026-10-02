// Package hosts is the backend of M2 (servers) and M3 (this PC): metrics,
// processes, services, terminals, files, exec, desktop quick actions,
// SSH-only hosts and alert rules. See docs/specs/M2-M3.md.
package hosts

import (
	"context"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Module implements api.ServerInterface and contracts.Hosts.
type Module struct {
	d   *module.Deps
	q   *db.Queries
	now func() time.Time

	presence *presenceStore
	metrics  *metricStore
	ssh      *sshPool
	alerts   *alertState
	traffic  *trafficTracker
	briefs   *briefCache
	units    unitCache

	infoMu  sync.Mutex
	info    map[string]protocol.SystemInfo
	geo     *geoResolver
	baseCtx context.Context
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ contracts.Hosts     = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module, subscribes to agent metrics and registers the
// contracts.Hosts provider and the actions.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{
		d: d, q: db.New(d.DB), now: func() time.Time { return time.Now().UTC() },
		presence: newPresenceStore(), metrics: newMetricStore(rawCapacity), ssh: newSSHPool(), alerts: newAlertState(),
		traffic: newTrafficTracker(), briefs: &briefCache{from: map[string]cachedBrief{}},
		info: map[string]protocol.SystemInfo{},
		geo:  newGeoResolver(d.Config.DataDir), baseCtx: context.Background(),
	}
	if err := m.initializeHostInfo(context.Background()); err != nil {
		m.geo.close()
		return nil, err
	}
	d.Agents.SetPairingInfo(m)
	module.Provide[contracts.HostPairingInfo](d.Registry, contracts.HostPairingInfoKey, m)
	d.Agents.OnEvent(protocol.EventMetrics, m.onMetrics)
	d.Agents.OnEvent(protocol.EventPresenceUpdate, m.onPresence)
	module.Provide[contracts.Presence](d.Registry, contracts.PresenceKey, m)
	module.Provide[contracts.Hosts](d.Registry, contracts.HostsKey, m)
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "hosts" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start registers the background jobs.
func (m *Module) Start(ctx context.Context) error {
	m.baseCtx = ctx
	m.d.Scheduler.Every("hosts.addresses", 5*time.Minute, m.refreshAddresses)
	go func() { _ = m.refreshAddresses(ctx) }()
	m.d.Scheduler.Every("hosts.rollup", time.Minute, m.rollup)
	m.d.Scheduler.Every("hosts.alerts", 15*time.Second, m.evaluateAlerts)
	m.d.Scheduler.Every("hosts.ssh_metrics", time.Minute, m.pollSSH)
	m.d.Scheduler.Every("hosts.cleanup", time.Hour, m.cleanup)
	m.d.Scheduler.Every("hosts.traffic_flush", time.Minute, m.flushTraffic)
	m.d.Scheduler.Every("hosts.traffic_alerts", 5*time.Minute, m.checkTraffic)
	go func() {
		<-ctx.Done()
		m.ssh.closeAll()
		m.geo.close()
	}()
	m.followPresence(ctx)
	m.followIntervals(ctx)
	return nil
}
