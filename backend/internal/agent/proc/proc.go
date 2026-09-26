// Package proc lists and ends processes (proc.list, proc.kill).
package proc

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// sampleWindow is how long CPU usage is measured for proc.list.
var sampleWindow = 500 * time.Millisecond

const (
	defaultLimit = 200
	maxLimit     = 2000
	maxCmdline   = 1024
)

// Register adds proc.list and proc.kill.
func Register(c *conn.Client) {
	c.Handle(protocol.MethodProcList, handleList)
	c.Handle(protocol.MethodProcKill, handleKill)
}

func handleList(ctx context.Context, raw json.RawMessage) (any, error) {
	var p protocol.ProcListParams
	if err := rpcutil.Decode(raw, &p); err != nil {
		return nil, err
	}
	return List(ctx, p)
}

func handleKill(ctx context.Context, raw json.RawMessage) (any, error) {
	var p protocol.ProcKillParams
	if err := rpcutil.Decode(raw, &p); err != nil {
		return nil, err
	}
	return nil, Kill(ctx, p)
}

// List returns processes sorted by p.Sort. CPU is measured over a short
// window so the numbers reflect current load, not the lifetime average.
func List(ctx context.Context, p protocol.ProcListParams) (protocol.ProcessList, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	limit = min(limit, maxLimit)
	switch p.Sort {
	case "", "cpu", "mem", "pid", "name":
	default:
		return protocol.ProcessList{}, rpcutil.BadParams("sort must be cpu, mem, pid or name")
	}

	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return protocol.ProcessList{}, rpcutil.Failed("list processes: %v", err)
	}
	before := map[int32]float64{}
	for _, pr := range procs {
		if t, err := pr.TimesWithContext(ctx); err == nil {
			before[pr.Pid] = cpuSeconds(t)
		}
	}
	start := time.Now()
	select {
	case <-time.After(sampleWindow):
	case <-ctx.Done():
		return protocol.ProcessList{}, ctx.Err()
	}
	elapsed := time.Since(start).Seconds()
	var memTotal uint64
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		memTotal = vm.Total
	}

	items := make([]protocol.ProcessInfo, 0, len(procs))
	for _, pr := range procs {
		name, err := pr.NameWithContext(ctx)
		if err != nil {
			continue // exited meanwhile
		}
		info := protocol.ProcessInfo{PID: pr.Pid, Name: name}
		if t, err := pr.TimesWithContext(ctx); err == nil {
			if b, ok := before[pr.Pid]; ok {
				info.CPU = CPUPercent(b, cpuSeconds(t), elapsed)
			}
		}
		info.PPID, _ = pr.PpidWithContext(ctx)
		info.User, _ = pr.UsernameWithContext(ctx)
		if m, err := pr.MemoryInfoWithContext(ctx); err == nil {
			info.MemRSS = m.RSS
			if memTotal > 0 {
				info.MemPercent = float64(int64(float64(m.RSS)/float64(memTotal)*1000)) / 10
			}
		}
		if cmd, err := pr.CmdlineWithContext(ctx); err == nil {
			info.Cmdline = truncate(cmd, maxCmdline)
		}
		if ms, err := pr.CreateTimeWithContext(ctx); err == nil && ms > 0 {
			info.StartedAt = time.UnixMilli(ms).UTC()
		}
		if st, err := pr.StatusWithContext(ctx); err == nil && len(st) > 0 {
			info.Status = st[0]
		}
		items = append(items, info)
	}
	Sort(items, p.Sort)
	total := len(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return protocol.ProcessList{Items: items, Total: total}, nil
}

// Sort orders processes: cpu and mem descending, pid ascending, name A-Z.
func Sort(items []protocol.ProcessInfo, by string) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch by {
		case "mem":
			if a.MemRSS != b.MemRSS {
				return a.MemRSS > b.MemRSS
			}
		case "pid":
			return a.PID < b.PID
		case "name":
			if !strings.EqualFold(a.Name, b.Name) {
				return strings.ToLower(a.Name) < strings.ToLower(b.Name)
			}
		default:
			if a.CPU != b.CPU {
				return a.CPU > b.CPU
			}
			if a.MemRSS != b.MemRSS {
				return a.MemRSS > b.MemRSS
			}
		}
		return a.PID < b.PID
	})
}

// CPUPercent is the share of one core used between two CPU time readings.
func CPUPercent(before, after, elapsed float64) float64 {
	if elapsed <= 0 || after < before {
		return 0
	}
	return float64(int64((after-before)/elapsed*1000)) / 10
}

func cpuSeconds(t *cpu.TimesStat) float64 { return t.User + t.System }

// Kill ends a process. The agent refuses to kill itself.
func Kill(ctx context.Context, p protocol.ProcKillParams) error {
	if p.PID <= 0 {
		return rpcutil.BadParams("pid is required")
	}
	if int(p.PID) == os.Getpid() {
		return rpcutil.BadParams("refusing to kill the agent itself")
	}
	pr, err := process.NewProcessWithContext(ctx, p.PID)
	if err != nil {
		return &protocol.Error{Code: protocol.CodeNotFound, Message: "no such process"}
	}
	return kill(ctx, pr, strings.ToUpper(strings.TrimPrefix(strings.ToUpper(p.Signal), "SIG")))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
