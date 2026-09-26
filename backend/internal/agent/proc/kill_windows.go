//go:build windows

package proc

import (
	"context"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
)

// Windows has no signals: every request terminates the process.
func kill(ctx context.Context, p *process.Process, _ string) error {
	return rpcutil.FromOS(p.KillWithContext(ctx))
}
