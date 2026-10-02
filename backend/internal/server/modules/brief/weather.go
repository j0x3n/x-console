package brief

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
)

// weatherTTL is how long a forecast is reused. The home page asks often.
const weatherTTL = 10 * time.Minute

// weatherForceGap is the shortest gap between forced refreshes of one place.
// Clicking refresh many times must not hammer QWeather.
const weatherForceGap = time.Minute

type cachedWeather struct {
	w  api.Weather
	at time.Time
}

// fetchWeather returns the current weather and today's forecast, cached per
// place and API address. force skips the cache unless it is under a minute old.
func (m *Module) fetchWeather(ctx context.Context, base string, loc api.BriefLocation, force bool) (api.Weather, error) {
	c, err := m.requireQWeather(ctx)
	if err != nil {
		return api.Weather{}, err
	}
	loc, err = m.resolveLocation(ctx, c, loc)
	if err != nil {
		return api.Weather{}, err
	}
	m.weatherFetchMu.Lock()
	defer m.weatherFetchMu.Unlock()
	key := fmt.Sprintf("qweather|%s|%x|%s|%.6f|%.6f", c.APIHost, sha256.Sum256([]byte(c.APIKey)), deref(loc.Id), loc.Lat, loc.Lon)
	ttl := weatherTTL
	if force {
		ttl = weatherForceGap
	}
	m.weatherMu.Lock()
	cached, ok := m.weatherCache[key]
	m.weatherMu.Unlock()
	if ok && m.now().Sub(cached.at) < ttl {
		w := cached.w
		w.Location = loc.Name
		return w, nil
	}
	w, err := m.fetchQWeather(ctx, c, loc)
	if err != nil {
		m.d.Log.Warn("brief: 和风实况查询失败", "err", err)
	}
	if err == nil {
		m.weatherMu.Lock()
		m.weatherCache[key] = cachedWeather{w: w, at: m.now()}
		m.weatherMu.Unlock()
	}
	return w, err
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// weatherLine is the one-line weather summary used in the brief.
func weatherLine(w api.Weather) string {
	place := ""
	if w.Location != nil && *w.Location != "" {
		place = *w.Location + "，"
	}
	s := fmt.Sprintf("%s%s，现在 %s°C。今天 %s 到 %s°C", place, w.Summary, num(w.Temperature), num(w.Low), num(w.High))
	if w.PrecipitationChance > 0 {
		s += fmt.Sprintf("，降水概率 %d%%", w.PrecipitationChance)
	}
	s += "。"
	if w.PrecipitationChance >= 50 {
		s += "出门记得带伞。"
	}
	return s
}

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
