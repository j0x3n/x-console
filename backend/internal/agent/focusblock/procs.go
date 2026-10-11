package focusblock

import (
	"context"

	"github.com/shirou/gopsutil/v4/process"
)

// Proc is a running program: its id and the name of its executable, without
// the path.
type Proc struct {
	PID  int32
	Name string
}

// Procs is what the blocker needs from the system. Tests replace it.
type Procs interface {
	List(ctx context.Context) ([]Proc, error)
	// End asks a process to stop. force is set when it was asked before and
	// is still running. Windows ends it at once either way.
	End(ctx context.Context, pid int32, force bool) error
}

type systemProcs struct{}

func (systemProcs) List(ctx context.Context) ([]Proc, error) {
	all, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Proc, 0, len(all))
	for _, p := range all {
		name, err := p.NameWithContext(ctx)
		if err != nil || name == "" {
			continue // gone meanwhile, or not readable
		}
		out = append(out, Proc{PID: p.Pid, Name: name})
	}
	return out, nil
}

func (systemProcs) End(ctx context.Context, pid int32, force bool) error {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return nil // already gone
	}
	return end(ctx, p, force)
}
