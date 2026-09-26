package focus

import (
	"context"
	"time"
)

// Hooks for the external test package.

func Sweep(m *Module, ctx context.Context, now time.Time) error { return m.sweep(ctx, now) }

// SetMinute shortens a planned minute so the real timer fires quickly.
func SetMinute(m *Module, d time.Duration) { m.minute = d }
