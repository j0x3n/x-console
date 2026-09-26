//go:build !windows

package proc

import (
	"context"
	"syscall"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
)

// Signals accepted by proc.kill on Unix.
var signals = map[string]syscall.Signal{
	"": syscall.SIGTERM, "TERM": syscall.SIGTERM, "KILL": syscall.SIGKILL, "INT": syscall.SIGINT,
	"HUP": syscall.SIGHUP, "STOP": syscall.SIGSTOP, "CONT": syscall.SIGCONT,
}

func kill(ctx context.Context, p *process.Process, signal string) error {
	sig, ok := signals[signal]
	if !ok {
		return rpcutil.BadParams("unknown signal %q", signal)
	}
	return rpcutil.FromOS(p.SendSignalWithContext(ctx, sig))
}
