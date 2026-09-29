package hosts

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	rawCapacity     = 720 // one hour of 5 second detail samples
	publishInterval = 4 * time.Second
	keep1m          = 7 * 24 * time.Hour
	keep1h          = 90 * 24 * time.Hour
	keepAlerts      = 90 * 24 * time.Hour
)

// metricStore keeps the raw samples of every host in memory: a ring buffer
// per host that holds the last hour.
type metricStore struct {
	mu    sync.RWMutex
	cap   int
	hosts map[string]*ring
	fast  map[string]bool // hosts in the 1 second mode: every sample is pushed
}

type ring struct {
	buf         []protocol.MetricsSample
	next        int
	slotAt      time.Time // when the newest slot of buf was started
	lastPublish time.Time
}

// slotGap is the shortest distance between two slots of the ring in the
// 1 second mode. Samples closer together replace the newest slot, so the ring
// still holds an hour of history.
const slotGap = 4500 * time.Millisecond

func newMetricStore(capacity int) *metricStore {
	return &metricStore{cap: capacity, hosts: map[string]*ring{}, fast: map[string]bool{}}
}

// add stores a sample and reports whether it should be pushed to browsers:
// at most one host.metrics event per host every 4 seconds, or every sample
// while the host runs in the 1 second mode.
func (s *metricStore) add(id string, x protocol.MetricsSample) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.hosts[id]
	if r == nil {
		r = &ring{}
		s.hosts[id] = r
	}
	switch {
	case s.fast[id] && len(r.buf) > 0 && x.At.After(r.slotAt) && x.At.Sub(r.slotAt) < slotGap:
		newest := len(r.buf) - 1
		if len(r.buf) >= s.cap {
			newest = (r.next - 1 + s.cap) % s.cap
		}
		r.buf[newest] = x
	case len(r.buf) < s.cap:
		r.buf = append(r.buf, x)
		r.slotAt = x.At
	default:
		r.buf[r.next] = x
		r.next = (r.next + 1) % s.cap
		r.slotAt = x.At
	}
	if s.fast[id] || x.At.Sub(r.lastPublish) >= publishInterval {
		r.lastPublish = x.At
		return true
	}
	return false
}

// setFast turns the every-sample push on or off for a host.
func (s *metricStore) setFast(id string, fast bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fast {
		s.fast[id] = true
	} else {
		delete(s.fast, id)
	}
}

// ordered returns the samples of r oldest first. Caller holds the lock.
func (r *ring) ordered() []protocol.MetricsSample {
	out := make([]protocol.MetricsSample, 0, len(r.buf))
	out = append(out, r.buf[r.next:]...)
	return append(out, r.buf[:r.next]...)
}

func (s *metricStore) latest(id string) (protocol.MetricsSample, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r := s.hosts[id]
	if r == nil || len(r.buf) == 0 {
		return protocol.MetricsSample{}, false
	}
	i := len(r.buf) - 1
	if len(r.buf) == s.cap {
		i = (r.next - 1 + s.cap) % s.cap
	}
	return r.buf[i], true
}

// between returns the samples of one host with from <= At < to.
func (s *metricStore) between(id string, from, to time.Time) []protocol.MetricsSample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r := s.hosts[id]
	if r == nil {
		return nil
	}
	var out []protocol.MetricsSample
	for _, x := range r.ordered() {
		if !x.At.Before(from) && x.At.Before(to) {
			out = append(out, x)
		}
	}
	return out
}

func (s *metricStore) ids() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.hosts))
	for id := range s.hosts {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (s *metricStore) drop(id string) {
	s.mu.Lock()
	delete(s.hosts, id)
	delete(s.fast, id)
	s.mu.Unlock()
}

// hostMetricsEvent is the payload of host.metrics.
type hostMetricsEvent struct {
	HostID string            `json:"hostId"`
	Sample api.MetricsSample `json:"sample"`
}

// recordSample stores a sample (agents every 5s or 30s, SSH every minute)
// and publishes host.metrics when due.
func (m *Module) recordSample(hostID string, x protocol.MetricsSample) {
	if x.At.IsZero() {
		x.At = m.now()
	}
	m.traffic.observe(hostID, x, m.d.Config.Location)
	if m.metrics.add(hostID, x) {
		m.d.Bus.Publish("host.metrics", hostMetricsEvent{HostID: hostID, Sample: toAPISample(x)})
	}
}

// onMetrics runs in the agent connection's read loop; it only touches memory.
func (m *Module) onMetrics(agentID string, raw json.RawMessage) {
	var x protocol.MetricsSample
	if err := json.Unmarshal(raw, &x); err != nil {
		m.d.Log.Warn("bad metrics event", "agent", agentID, "err", err)
		return
	}
	// The server clock decides, so charts line up even if an agent's clock drifts.
	x.At = m.now()
	m.recordSample(agentID, x)
}

func toAPISample(x protocol.MetricsSample) api.MetricsSample {
	disks := make([]api.DiskUsage, 0, len(x.Disks))
	for _, d := range x.Disks {
		disks = append(disks, api.DiskUsage{Mount: d.Mount, FsType: d.FSType, Used: int64(d.Used), Total: int64(d.Total)})
	}
	cores := x.CPUPerCore
	if cores == nil {
		cores = []float64{}
	}
	out := api.MetricsSample{
		At: x.At, Cpu: x.CPU, CpuPerCore: cores, MemUsed: int64(x.MemUsed), MemTotal: int64(x.MemTotal),
		SwapUsed: int64(x.SwapUsed), SwapTotal: int64(x.SwapTotal), Disks: disks, NetRx: x.NetRxRate, NetTx: x.NetTxRate,
		DiskRead: x.DiskReadRate, DiskWrite: x.DiskWriteRate, Load1: x.Load1, Load5: x.Load5, Load15: x.Load15,
		UptimeSeconds: int64(x.UptimeSeconds), Procs: x.Procs,
	}
	if len(x.NetInterfaces) > 0 {
		interfaces := make([]api.NetInterface, 0, len(x.NetInterfaces))
		for _, nic := range x.NetInterfaces {
			interfaces = append(interfaces, api.NetInterface{Name: nic.Name, Rx: nic.RxRate, Tx: nic.TxRate})
		}
		out.NetInterfaces = &interfaces
	}
	return out
}

func percent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return round1(float64(used) / float64(total) * 100)
}

// fullestDisk returns the highest usage percent and its mount point.
func fullestDisk(disks []protocol.DiskUsage) (float64, string) {
	var best float64
	var mount string
	for _, d := range disks {
		if p := percent(d.Used, d.Total); p > best || mount == "" {
			best, mount = p, d.Mount
		}
	}
	return best, mount
}

func round1(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }

func pointFromSample(x protocol.MetricsSample) api.MetricsPoint {
	disk, _ := fullestDisk(x.Disks)
	return api.MetricsPoint{At: x.At, Cpu: x.CPU, Memory: percent(x.MemUsed, x.MemTotal), MemUsed: int64(x.MemUsed),
		MemTotal: int64(x.MemTotal), Disk: disk, NetRx: x.NetRxRate, NetTx: x.NetTxRate, Load1: x.Load1}
}

// rollupRow is one aggregated row of host_metrics_1m or host_metrics_1h.
type rollupRow struct {
	At       time.Time
	CPU      float64
	MemUsed  int64
	MemTotal int64
	DiskJSON string
	NetRx    float64
	NetTx    float64
	Load1    float64
}

func (r rollupRow) point() api.MetricsPoint {
	var disks []protocol.DiskUsage
	_ = json.Unmarshal([]byte(r.DiskJSON), &disks)
	disk, _ := fullestDisk(disks)
	return api.MetricsPoint{At: r.At, Cpu: round1(r.CPU), Memory: percent(uint64(r.MemUsed), uint64(r.MemTotal)), MemUsed: r.MemUsed,
		MemTotal: r.MemTotal, Disk: disk, NetRx: r.NetRx, NetTx: r.NetTx, Load1: r.Load1}
}

// averageSamples folds raw samples into one row. Counters are averaged;
// totals and disks come from the last sample.
func averageSamples(at time.Time, xs []protocol.MetricsSample) rollupRow {
	row := rollupRow{At: at}
	for _, x := range xs {
		row.CPU += x.CPU
		row.MemUsed += int64(x.MemUsed)
		row.NetRx += x.NetRxRate
		row.NetTx += x.NetTxRate
		row.Load1 += x.Load1
	}
	n := float64(len(xs))
	last := xs[len(xs)-1]
	row.CPU /= n
	row.MemUsed = int64(float64(row.MemUsed) / n)
	row.NetRx /= n
	row.NetTx /= n
	row.Load1 /= n
	row.MemTotal = int64(last.MemTotal)
	disks := last.Disks
	if disks == nil {
		disks = []protocol.DiskUsage{}
	}
	raw, _ := json.Marshal(disks)
	row.DiskJSON = string(raw)
	return row
}

// averageRows folds rows into one row with the same rules.
func averageRows(at time.Time, rows []rollupRow) rollupRow {
	out := rollupRow{At: at}
	for _, r := range rows {
		out.CPU += r.CPU
		out.MemUsed += r.MemUsed
		out.NetRx += r.NetRx
		out.NetTx += r.NetTx
		out.Load1 += r.Load1
	}
	n := float64(len(rows))
	last := rows[len(rows)-1]
	out.CPU /= n
	out.MemUsed = int64(float64(out.MemUsed) / n)
	out.NetRx /= n
	out.NetTx /= n
	out.Load1 /= n
	out.MemTotal = last.MemTotal
	out.DiskJSON = last.DiskJSON
	return out
}

func row1m(r db.HostMetrics1m) rollupRow {
	return rollupRow{At: r.At, CPU: r.Cpu, MemUsed: r.MemUsed, MemTotal: r.MemTotal, DiskJSON: r.DiskJson, NetRx: r.NetRx, NetTx: r.NetTx, Load1: r.Load1}
}

func row1h(r db.HostMetrics1h) rollupRow {
	return rollupRow{At: r.At, CPU: r.Cpu, MemUsed: r.MemUsed, MemTotal: r.MemTotal, DiskJSON: r.DiskJson, NetRx: r.NetRx, NetTx: r.NetTx, Load1: r.Load1}
}

// rollup writes the 1m rows of the last two complete minutes from memory and
// the 1h rows of the previous and the current hour from the 1m rows. Rows
// are upserted, so running it late or twice is harmless.
func (m *Module) rollup(ctx context.Context) error {
	now := m.now()
	minute := now.Truncate(time.Minute)
	for _, start := range []time.Time{minute.Add(-2 * time.Minute), minute.Add(-time.Minute)} {
		for _, id := range m.metrics.ids() {
			xs := m.metrics.between(id, start, start.Add(time.Minute))
			if len(xs) == 0 {
				continue
			}
			r := averageSamples(start, xs)
			if err := m.q.UpsertMetric1m(ctx, db.UpsertMetric1mParams{HostID: id, At: start, Cpu: r.CPU, MemUsed: r.MemUsed,
				MemTotal: r.MemTotal, DiskJson: r.DiskJSON, NetRx: r.NetRx, NetTx: r.NetTx, Load1: r.Load1}); err != nil {
				return err
			}
		}
	}
	hour := now.Truncate(time.Hour)
	rows, err := m.q.ListMetrics1mWindow(ctx, db.ListMetrics1mWindowParams{Since: hour.Add(-time.Hour), Until: hour.Add(time.Hour)})
	if err != nil {
		return err
	}
	groups := map[string]map[time.Time][]rollupRow{}
	for _, r := range rows {
		h := r.At.UTC().Truncate(time.Hour)
		if groups[r.HostID] == nil {
			groups[r.HostID] = map[time.Time][]rollupRow{}
		}
		groups[r.HostID][h] = append(groups[r.HostID][h], row1m(r))
	}
	for id, byHour := range groups {
		for h, rs := range byHour {
			a := averageRows(h, rs)
			if err := m.q.UpsertMetric1h(ctx, db.UpsertMetric1hParams{HostID: id, At: h, Cpu: a.CPU, MemUsed: a.MemUsed,
				MemTotal: a.MemTotal, DiskJson: a.DiskJSON, NetRx: a.NetRx, NetTx: a.NetTx, Load1: a.Load1}); err != nil {
				return err
			}
		}
	}
	return nil
}

// cleanup drops rollups and resolved alerts past their retention.
func (m *Module) cleanup(ctx context.Context) error {
	now := m.now()
	if err := m.q.DeleteMetrics1mBefore(ctx, now.Add(-keep1m)); err != nil {
		return err
	}
	if err := m.q.DeleteMetrics1hBefore(ctx, now.Add(-keep1h)); err != nil {
		return err
	}
	if err := m.q.DeleteTrafficHourlyBefore(ctx, now.Add(-keepTrafficHours).Format(hourLayout)); err != nil {
		return err
	}
	return m.q.DeleteAlertEventsBefore(ctx, now.Add(-keepAlerts))
}

// series builds the chart data of one host.
func (m *Module) series(ctx context.Context, hostID, rng string) (api.MetricsSeries, error) {
	now := m.now()
	out := api.MetricsSeries{Range: rng, Points: []api.MetricsPoint{}}
	switch rng {
	case "", "1h":
		out.Range, out.StepSeconds = "1h", 5
		xs := m.metrics.between(hostID, now.Add(-time.Hour), now.Add(time.Second))
		if len(xs) >= 2 {
			if gap := xs[len(xs)-1].At.Sub(xs[len(xs)-2].At); gap > 45*time.Second {
				out.StepSeconds = 60 // SSH hosts are polled every minute
			} else if gap > 20*time.Second {
				out.StepSeconds = 30
			}
			for _, x := range xs {
				out.Points = append(out.Points, pointFromSample(x))
			}
			return out, nil
		}
		// Just restarted: fall back to the minute rows.
		out.StepSeconds = 60
		rows, err := m.q.ListMetrics1m(ctx, db.ListMetrics1mParams{HostID: hostID, Since: now.Add(-time.Hour), Until: now})
		if err != nil {
			return out, err
		}
		for _, r := range rows {
			out.Points = append(out.Points, row1m(r).point())
		}
	case "24h":
		out.StepSeconds = 300
		rows, err := m.q.ListMetrics1m(ctx, db.ListMetrics1mParams{HostID: hostID, Since: now.Add(-24 * time.Hour), Until: now})
		if err != nil {
			return out, err
		}
		rs := make([]rollupRow, 0, len(rows))
		for _, r := range rows {
			rs = append(rs, row1m(r))
		}
		for _, r := range bucket(rs, 5*time.Minute) {
			out.Points = append(out.Points, r.point())
		}
	case "7d":
		out.StepSeconds = 3600
		rows, err := m.q.ListMetrics1h(ctx, db.ListMetrics1hParams{HostID: hostID, Since: now.Add(-7 * 24 * time.Hour), Until: now.Add(time.Hour)})
		if err != nil {
			return out, err
		}
		for _, r := range rows {
			out.Points = append(out.Points, row1h(r).point())
		}
	default:
		return out, httpx.Invalid("range 只能是 1h、24h 或 7d")
	}
	return out, nil
}

// bucket averages sorted rows into buckets of size step.
func bucket(rows []rollupRow, step time.Duration) []rollupRow {
	var out []rollupRow
	var cur []rollupRow
	var start time.Time
	for _, r := range rows {
		b := r.At.UTC().Truncate(step)
		if len(cur) > 0 && !b.Equal(start) {
			out = append(out, averageRows(start, cur))
			cur = nil
		}
		start = b
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		out = append(out, averageRows(start, cur))
	}
	return out
}
