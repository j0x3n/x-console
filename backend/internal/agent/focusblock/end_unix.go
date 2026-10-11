//go:build !windows

package focusblock

import (
	"context"

	"github.com/shirou/gopsutil/v4/process"
)

// end sends SIGTERM first, and SIGKILL when the program ignored it.
func end(ctx context.Context, p *process.Process, force bool) error {
	if force {
		return p.KillWithContext(ctx)
	}
	return p.TerminateWithContext(ctx)
}
