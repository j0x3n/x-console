package brief

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
)

// Hooks for the external test package.

func CheckRain(m *Module, ctx context.Context, now time.Time) error { return m.checkRain(ctx, now) }

func SetGeoBase(m *Module, base string) { m.geoBase, m.osmBase = base, base }

func Tick(m *Module, ctx context.Context, now time.Time) error { return m.tick(ctx, now) }

func ShouldSend(enabled bool, at string, now time.Time, loc *time.Location, sentToday bool) bool {
	return shouldSend(enabled, at, now, loc, sentToday)
}

func NextRun(at string, now time.Time, loc *time.Location, sentToday bool) (time.Time, bool) {
	return nextRun(at, now, loc, sentToday)
}

// Generate builds a brief with the stored settings but the given registry,
// so tests choose which modules exist.
func Generate(m *Module, ctx context.Context, reg *module.Registry, now time.Time) (content string, keys []string, err error) {
	cfg, err := m.load(ctx)
	if err != nil {
		return "", nil, err
	}
	res := m.generate(ctx, cfg, reg, now)
	for _, s := range res.sections {
		keys = append(keys, string(s.Key))
	}
	return res.content, keys, nil
}
