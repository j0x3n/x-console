//go:build windows

package focusblock

import (
	"context"

	"github.com/shirou/gopsutil/v4/process"
)

// end terminates the process. Windows has no signals.
func end(ctx context.Context, p *process.Process, _ bool) error {
	return p.KillWithContext(ctx)
}
