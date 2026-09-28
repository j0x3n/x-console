// Package metrics collects system metrics with gopsutil, pushes them to the
// server every 30 seconds, or every 5 seconds while its detail is open.
package metrics

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Sampling intervals can be shortened in tests.
var Interval = 30 * time.Second
var DetailInterval = 5 * time.Second

// maxDisks caps the mounts reported per sample.
const maxDisks = 24

// Register pushes a summary on connection and switches cadence on demand.
func Register(c *conn.Client) {
	col := NewCollector()
	var detail atomic.Bool
	changed := make(chan struct{}, 1)
	c.Handle(protocol.MethodMetricsDetail, func(_ context.Context, raw json.RawMessage) (any, error) {
		var p protocol.MetricsDetailParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		detail.Store(p.On)
		select {
		case changed <- struct{}{}:
		default:
		}
		return nil, nil
	})
	c.OnConnect(func(ctx context.Context) {
		defer detail.Store(false)
		push := func() {
			var s protocol.MetricsSample
			var err error
			detailed := detail.Load()
			if detailed {
				s, err = col.Sample(ctx)
			} else {
				s, err = col.SampleSummary(ctx)
			}
			if err != nil {
				slog.Warn("metrics sample failed", "err", err)
				return
			}
			if detailed {
				_ = c.Emit(ctx, protocol.EventMetrics, s)
			} else {
				_ = c.Emit(ctx, protocol.EventMetrics, summaryPayload(s))
			}
		}
		push()
		t := time.NewTimer(Interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-changed:
				if detail.Load() {
					push()
				}
			case <-t.C:
				push()
			}
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			if detail.Load() {
				t.Reset(DetailInterval)
			} else {
				t.Reset(Interval)
			}
		}
	})
}

func summaryPayload(s protocol.MetricsSample) protocol.MetricsSummary {
	return protocol.MetricsSummary{
		At: s.At, CPU: s.CPU, MemUsed: s.MemUsed, MemTotal: s.MemTotal,
		Disks: s.Disks, NetRxRate: s.NetRxRate, NetTxRate: s.NetTxRate,
		Load1: s.Load1, UptimeSeconds: s.UptimeSeconds,
	}
}

// SampleSummary keeps the fields needed for host cards and alert rules.
func (c *Collector) SampleSummary(ctx context.Context) (protocol.MetricsSample, error) {
	s, err := c.sample(ctx, false)
	if err != nil {
		return s, err
	}
	s.CPUPerCore = []float64{}
	if len(s.Disks) > 1 {
		best := 0
		for i := 1; i < len(s.Disks); i++ {
			current := float64(s.Disks[i].Used) / float64(s.Disks[i].Total)
			max := float64(s.Disks[best].Used) / float64(s.Disks[best].Total)
			if current > max {
				best = i
			}
		}
		s.Disks = []protocol.DiskUsage{s.Disks[best]}
	}
	return s, nil
}

// Collector turns cumulative counters into rates between two samples.
type Collector struct {
	mu   sync.Mutex
	last counters
}

type counters struct {
	at       time.Time
	cpuTotal cpu.TimesStat
	cpuCores []cpu.TimesStat
	netRx    uint64
	netTx    uint64
	nics     map[string]nicCounters
	diskR    uint64
	diskW    uint64
}

type nicCounters struct{ rx, tx uint64 }

// NewCollector reads the first set of counters.
func NewCollector() *Collector {
	c := &Collector{}
	c.last.read(context.Background())
	return c
}

func (c *counters) read(ctx context.Context) {
	c.at = time.Now()
	if t, err := cpu.TimesWithContext(ctx, false); err == nil && len(t) > 0 {
		c.cpuTotal = t[0]
	}
	if t, err := cpu.TimesWithContext(ctx, true); err == nil {
		c.cpuCores = t
	}
	c.netRx, c.netTx, c.nics = netCounters(ctx)
	c.diskR, c.diskW = diskCounters(ctx)
}

// Sample reads all metrics. CPU and rates are averages since the previous
// Sample (at least one second apart).
func (c *Collector) Sample(ctx context.Context) (protocol.MetricsSample, error) {
	return c.sample(ctx, true)
}

func (c *Collector) sample(ctx context.Context, detailed bool) (protocol.MetricsSample, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := time.Second - time.Since(c.last.at); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return protocol.MetricsSample{}, ctx.Err()
		}
	}
	prev := c.last
	c.last.read(ctx)
	cur := c.last
	elapsed := cur.at.Sub(prev.at).Seconds()

	s := protocol.MetricsSample{At: cur.at.UTC(), CPUPerCore: []float64{}, Disks: []protocol.DiskUsage{}}
	s.CPU = round1(BusyPercent(prev.cpuTotal, cur.cpuTotal))
	if detailed && len(prev.cpuCores) == len(cur.cpuCores) {
		for i := range cur.cpuCores {
			s.CPUPerCore = append(s.CPUPerCore, round1(BusyPercent(prev.cpuCores[i], cur.cpuCores[i])))
		}
	}
	s.NetRxRate = Rate(prev.netRx, cur.netRx, elapsed)
	s.NetTxRate = Rate(prev.netTx, cur.netTx, elapsed)
	if detailed {
		s.NetInterfaces = interfaceRates(prev.nics, cur.nics, elapsed)
	}
	s.DiskReadRate = Rate(prev.diskR, cur.diskR, elapsed)
	s.DiskWriteRate = Rate(prev.diskW, cur.diskW, elapsed)

	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		s.MemTotal = vm.Total
		s.MemUsed = vm.Total - vm.Available
		if vm.Available > vm.Total {
			s.MemUsed = vm.Used
		}
	}
	if detailed {
		if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
			s.SwapTotal, s.SwapUsed = sw.Total, sw.Used
		}
	}
	if l, err := load.AvgWithContext(ctx); err == nil {
		s.Load1, s.Load5, s.Load15 = round2(l.Load1), round2(l.Load5), round2(l.Load15)
	}
	if up, err := host.UptimeWithContext(ctx); err == nil {
		s.UptimeSeconds = up
	}
	if detailed {
		if pids, err := process.PidsWithContext(ctx); err == nil {
			s.Procs = len(pids)
		}
	}
	s.Disks = Disks(ctx)
	return s, nil
}

// BusyPercent is the non-idle share of CPU time between two readings.
func BusyPercent(a, b cpu.TimesStat) float64 {
	aAll, aBusy := busy(a)
	bAll, bBusy := busy(b)
	if bBusy <= aBusy {
		return 0
	}
	if bAll <= aAll {
		return 100
	}
	return min(100, max(0, (bBusy-aBusy)/(bAll-aAll)*100))
}

func busy(t cpu.TimesStat) (all, busy float64) {
	all = t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
	return all, all - t.Idle - t.Iowait
}

// Rate is the per-second increase of a counter; counter resets give 0.
func Rate(prev, cur uint64, seconds float64) float64 {
	if cur < prev || seconds <= 0 {
		return 0
	}
	return float64(int64(float64(cur-prev)/seconds*10)) / 10
}

func interfaceRates(prev, cur map[string]nicCounters, seconds float64) []protocol.NetInterface {
	names := make([]string, 0, len(cur))
	for name := range cur {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]protocol.NetInterface, 0, len(names))
	for _, name := range names {
		current := cur[name]
		previous, ok := prev[name]
		nic := protocol.NetInterface{Name: name}
		if ok {
			nic.RxRate = Rate(previous.rx, current.rx, seconds)
			nic.TxRate = Rate(previous.tx, current.tx, seconds)
		}
		out = append(out, nic)
	}
	return out
}

func netCounters(ctx context.Context) (rx, tx uint64, perNIC map[string]nicCounters) {
	perNIC = map[string]nicCounters{}
	nics, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return 0, 0, perNIC
	}
	for _, n := range nics {
		if isLoopback(n.Name) {
			continue
		}
		rx += n.BytesRecv
		tx += n.BytesSent
		perNIC[n.Name] = nicCounters{rx: n.BytesRecv, tx: n.BytesSent}
	}
	return rx, tx, perNIC
}

func isLoopback(name string) bool {
	l := strings.ToLower(name)
	return l == "lo" || strings.HasPrefix(l, "loopback")
}

func diskCounters(ctx context.Context) (r, w uint64) {
	io, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return 0, 0
	}
	names := make([]string, 0, len(io))
	for name := range io {
		names = append(names, name)
	}
	for _, name := range PhysicalDisks(names) {
		r += io[name].ReadBytes
		w += io[name].WriteBytes
	}
	return r, w
}

// PhysicalDisks drops devices that would count the same bytes twice:
// partitions of a listed disk (sda1 when sda is listed), loop and ram
// devices, device-mapper and md arrays.
func PhysicalDisks(names []string) []string {
	var out []string
	for _, n := range names {
		if runtime.GOOS != "windows" && (strings.HasPrefix(n, "loop") || strings.HasPrefix(n, "ram") ||
			strings.HasPrefix(n, "zram") || strings.HasPrefix(n, "dm-") || strings.HasPrefix(n, "md") || strings.HasPrefix(n, "sr")) {
			continue
		}
		partition := false
		for _, other := range names {
			if other != n && strings.HasPrefix(n, other) {
				partition = true
				break
			}
		}
		if !partition {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// skipFS lists file systems that are not real storage.
var skipFS = map[string]bool{
	"squashfs": true, "tmpfs": true, "devtmpfs": true, "overlay": true, "proc": true, "sysfs": true,
	"cgroup": true, "cgroup2": true, "autofs": true, "devpts": true, "mqueue": true, "tracefs": true,
	"debugfs": true, "securityfs": true, "pstore": true, "bpf": true, "fusectl": true, "configfs": true,
	"hugetlbfs": true, "ramfs": true, "nsfs": true, "binfmt_misc": true, "efivarfs": true, "fuse.lxcfs": true,
}

// Disks lists the usage of real mounted file systems, one per device.
func Disks(ctx context.Context) []protocol.DiskUsage {
	parts, err := disk.PartitionsWithContext(ctx, false)
	out := []protocol.DiskUsage{}
	if err != nil {
		return out
	}
	seen := map[string]bool{}
	for _, p := range parts {
		if skipFS[p.Fstype] || seen[p.Device] || strings.HasPrefix(p.Mountpoint, "/snap/") {
			continue
		}
		u, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		seen[p.Device] = true
		out = append(out, protocol.DiskUsage{Mount: p.Mountpoint, FSType: p.Fstype, Used: u.Used, Total: u.Total})
		if len(out) >= maxDisks {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mount < out[j].Mount })
	return out
}

// SystemInfo fills protocol.SystemInfo with gopsutil. Assign it to
// sysinfo.Info in cmd/agent.
func SystemInfo() protocol.SystemInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hostname, _ := os.Hostname()
	info := protocol.SystemInfo{Hostname: hostname, OS: runtime.GOOS, Arch: runtime.GOARCH, CPUCores: runtime.NumCPU()}
	if h, err := host.InfoWithContext(ctx); err == nil {
		info.Platform = h.Platform
		info.PlatformVer = h.PlatformVersion
		info.KernelVersion = h.KernelVersion
		info.UptimeSeconds = h.Uptime
		if h.KernelArch != "" {
			info.Arch = h.KernelArch
		}
	}
	if c, err := cpu.InfoWithContext(ctx); err == nil && len(c) > 0 {
		info.CPUModel = strings.TrimSpace(c[0].ModelName)
	}
	if n, err := cpu.CountsWithContext(ctx, true); err == nil && n > 0 {
		info.CPUCores = n
	}
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		info.MemoryTotal = vm.Total
	}
	return info
}

func round1(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }
func round2(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }
