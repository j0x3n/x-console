package brief

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/db"
)

// Hooks for the external test package.

func CheckRain(m *Module, ctx context.Context, now time.Time) error { return m.checkRain(ctx, now) }

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

// SetNow replaces the clock the weather cache uses.
func SetNow(m *Module, now func() time.Time) { m.now = now }

// SetQWeatherBase points every 和风天气 host at base.
func SetQWeatherBase(m *Module, base string) { m.qwBase = func(string) string { return base } }

func CheckRainSoon(m *Module, ctx context.Context, now time.Time) error {
	return m.checkRainSoon(ctx, now)
}

func CheckWarnings(m *Module, ctx context.Context, now time.Time) error {
	return m.checkWarnings(ctx, now)
}

func CheckQuakes(m *Module, ctx context.Context, now time.Time) error { return m.checkQuakes(ctx, now) }

func DistanceKm(lat1, lon1, lat2, lon2 float64) float64 { return distanceKm(lat1, lon1, lat2, lon2) }

// DropCaches forgets cached 和风天气 and earthquake data.
func DropCaches(m *Module) {
	m.dropExtraCache()
	m.extraMu.Lock()
	m.daily = map[string]any{}
	m.extraMu.Unlock()
	m.quakeMu.Lock()
	m.quakeCache = cachedQuakes{}
	m.quakeMu.Unlock()
}

// NotifyBody exposes the push text of a brief (B92).
func NotifyBody(content, sections string) string {
	return notifyBody(db.Brief{Content: content, Sections: sections})
}
