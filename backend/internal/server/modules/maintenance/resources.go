package maintenance

import (
	"context"
	"errors"
	"net/http"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/core"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

type Point struct {
	At                 time.Time `json:"at"`
	CPU                float64   `json:"cpu"`
	RSS                uint64    `json:"rss"`
	Heap               uint64    `json:"heap"`
	Goroutines         int       `json:"goroutines"`
	BrowserConnections int       `json:"browserConnections"`
	AgentConnections   int       `json:"agentConnections"`
}

func (m *Module) sample(ctx context.Context) {
	m.sampleMu.Lock()
	defer m.sampleMu.Unlock()
	p := Point{At: time.Now().UTC(), Goroutines: runtime.NumGoroutine()}
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	p.Heap = stats.HeapAlloc
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err == nil {
		if memory, e := proc.MemoryInfoWithContext(ctx); e == nil {
			p.RSS = memory.RSS
		}
		if times, e := proc.TimesWithContext(ctx); e == nil {
			seconds := times.User + times.System
			if !m.cpuAt.IsZero() {
				p.CPU = max(0, (seconds-m.cpuSeconds)/p.At.Sub(m.cpuAt).Seconds()*100)
			}
			m.cpuAt = p.At
			m.cpuSeconds = seconds
		}
	}
	if conns, ok := module.Lookup[contracts.MaintenanceConnections](m.d.Registry, contracts.MaintenanceConnectionsKey); ok {
		p.BrowserConnections = conns.BrowserConnections()
		p.AgentConnections = conns.AgentConnections()
	}
	m.mu.Lock()
	m.points = append(m.points, p)
	if len(m.points) > 60 {
		m.points = append([]Point(nil), m.points[len(m.points)-60:]...)
	}
	m.mu.Unlock()
}

func (m *Module) metrics() []Point {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Point{}, m.points...)
}

func databaseUsage(path string) ([]contracts.StorageUsage, error) {
	out := []contracts.StorageUsage{}
	for _, entry := range []struct{ suffix, key, label string }{{"", "database", "数据库"}, {"-wal", "database_wal", "数据库 WAL"}, {"-shm", "database_shm", "数据库 SHM"}} {
		u := contracts.StorageUsage{Key: entry.key, Label: entry.label, Location: "local", Available: true}
		info, err := os.Stat(path + entry.suffix)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err == nil {
			u.Bytes = info.Size()
			u.Files = 1
		}
		out = append(out, u)
	}
	return out, nil
}

func countDirectory(ctx context.Context, root, key, label string) (contracts.StorageUsage, error) {
	u := contracts.StorageUsage{Key: key, Label: label, Location: "local", Available: true}
	for info, err := range (files.Local{Root: root}).List(ctx, "") {
		if err != nil {
			return u, err
		}
		u.Files++
		u.Bytes += info.Size
	}
	return u, nil
}

func (m *Module) storageUsage(ctx context.Context, refresh bool) ([]contracts.StorageUsage, error) {
	m.usageMu.Lock()
	defer m.usageMu.Unlock()
	m.mu.Lock()
	at := m.usageAt
	cached := append([]contracts.StorageUsage(nil), m.usage...)
	m.mu.Unlock()
	if !refresh && !at.IsZero() && time.Since(at) < 10*time.Minute {
		return cached, nil
	}
	out, err := databaseUsage(m.d.Config.DBPath())
	if err != nil {
		return nil, err
	}
	reporters := module.All[contracts.StorageReporter](m.d.Registry)
	keys := make([]string, 0, len(reporters))
	for k := range reporters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		usage, e := reporters[k].Usage(ctx)
		if e != nil {
			return nil, e
		}
		out = append(out, usage...)
	}
	for _, dir := range []struct{ path, key, label string }{{m.d.Config.FilesCacheDir(), "cache", "S3 本地缓存"}, {m.d.Config.TmpDir(), "tmp", "临时目录"}} {
		u, e := countDirectory(ctx, dir.path, dir.key, dir.label)
		if e != nil {
			return nil, e
		}
		out = append(out, u)
	}
	out = append(out, contracts.StorageUsage{Key: "logs", Label: "日志", Location: "external", Available: false, Note: "进程日志由外部日志系统保存"})
	m.mu.Lock()
	m.usage = append([]contracts.StorageUsage(nil), out...)
	m.usageAt = time.Now()
	m.mu.Unlock()
	return out, nil
}

func (m *Module) overviewHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	usage, err := m.storageUsage(ctx, r.URL.Query().Get("refresh") == "true")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	visible := []contracts.StorageUsage{}
	for _, u := range usage {
		if m.hidden(ctx, u.Module) {
			continue
		}
		if !auth.VaultUnlocked(ctx) && (u.Module == "notes" || u.Module == "drive") {
			table := "notes"
			if u.Module == "drive" {
				table = "drive_items"
			}
			var hidden int64
			if e := m.d.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE hidden=1").Scan(&hidden); e != nil || hidden > 0 {
				continue
			}
		}
		visible = append(visible, u)
	}
	points := m.metrics()
	var current Point
	if len(points) > 0 {
		current = points[len(points)-1]
	}
	var memoryTotal, memoryUsed, diskTotal, diskFree uint64
	var machineCPU float64
	if v, e := mem.VirtualMemoryWithContext(ctx); e == nil {
		memoryTotal = v.Total
		memoryUsed = v.Used
	}
	if v, e := disk.UsageWithContext(ctx, m.d.Config.DataDir); e == nil {
		diskTotal = v.Total
		diskFree = v.Free
	}
	if v, e := cpu.PercentWithContext(ctx, 0, false); e == nil && len(v) > 0 {
		machineCPU = v[0]
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"version": core.Version, "builtAt": core.BuiltAt, "startedAt": m.started, "goVersion": runtime.Version(), "dataDir": m.d.Config.DataDir, "process": current, "machine": map[string]any{"cpu": machineCPU, "memoryTotal": memoryTotal, "memoryUsed": memoryUsed, "diskTotal": diskTotal, "diskFree": diskFree}, "storage": visible})
}

func (m *Module) vacuumHandler(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	m.busy = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.busy = false; m.usageAt = time.Time{}; m.mu.Unlock() }()
	before, err := databaseUsage(m.d.Config.DBPath())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	conn, err := m.d.DB.Conn(r.Context())
	if err == nil {
		defer conn.Close()
		_, err = conn.ExecContext(r.Context(), "PRAGMA wal_checkpoint(TRUNCATE)")
		if err == nil {
			_, err = conn.ExecContext(r.Context(), "VACUUM")
		}
		if err == nil {
			_, err = conn.ExecContext(r.Context(), "PRAGMA optimize")
		}
		if err == nil {
			_, err = conn.ExecContext(r.Context(), "PRAGMA wal_checkpoint(TRUNCATE)")
		}
	}
	if conn != nil {
		_ = conn.Close()
	}
	after, e := databaseUsage(m.d.Config.DBPath())
	if err == nil {
		err = e
	}
	var b, a int64
	for _, u := range before {
		b += u.Bytes
	}
	for _, u := range after {
		a += u.Bytes
	}
	if m.d.Audit != nil {
		m.d.Audit.Record(r.Context(), "maintenance.vacuum", "database", map[string]any{"beforeBytes": b, "afterBytes": a}, err)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"beforeBytes": b, "afterBytes": a, "freedBytes": max(int64(0), b-a)})
}
