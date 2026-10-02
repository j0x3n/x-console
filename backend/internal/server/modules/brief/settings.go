package brief

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Settings keys. Each value is stored on its own key, as in docs/specs/M11.md.
const (
	keyEnabled     = "brief.enabled"
	keyTime        = "brief.time"
	keyChannels    = "brief.channels"
	keyLocation    = "brief.location"
	keySections    = "brief.sections"
	keyAIPolish    = "brief.ai_polish"
	keyWeatherBase = "brief.weather_api_base"
)

const (
	defaultTime        = "08:00"
	defaultWeatherBase = "https://api.open-meteo.com"
)

// allSections is every section in display order.
var allSections = []string{"weather", "calendar", "issues", "reminders", "alerts", "habits", "renewals"}

// config is the brief configuration with defaults applied.
type config struct {
	Enabled     bool
	Time        string
	Channels    []string
	Location    *api.BriefLocation
	Sections    []string
	AIPolish    bool
	WeatherBase string
}

func (c config) has(section string) bool { return slices.Contains(c.Sections, section) }

func (m *Module) get(ctx context.Context, key string, v any) error {
	if err := m.d.Settings.Get(ctx, key, v); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return err
	}
	return nil
}

// load reads the configuration. Missing keys take their defaults.
func (m *Module) load(ctx context.Context) (config, error) {
	c := config{Enabled: true, Time: defaultTime, Channels: []string{}, Sections: slices.Clone(allSections), WeatherBase: defaultWeatherBase}
	var loc api.BriefLocation
	var hasLoc bool
	for key, v := range map[string]any{
		keyEnabled: &c.Enabled, keyTime: &c.Time, keyChannels: &c.Channels, keySections: &c.Sections,
		keyAIPolish: &c.AIPolish, keyWeatherBase: &c.WeatherBase,
	} {
		if err := m.get(ctx, key, v); err != nil {
			return c, err
		}
	}
	if hasLoc, _ = m.d.Settings.Has(ctx, keyLocation); hasLoc {
		if err := m.get(ctx, keyLocation, &loc); err != nil {
			return c, err
		}
		c.Location = &loc
	}
	if c.WeatherBase == "" {
		c.WeatherBase = defaultWeatherBase
	}
	if c.Channels == nil {
		c.Channels = []string{}
	}
	return c, nil
}

// validate checks a settings update and returns it as a config.
func (m *Module) validate(in api.BriefSettings) (config, error) {
	c := config{Enabled: in.Enabled, Time: strings.TrimSpace(in.Time), Location: in.Location, AIPolish: in.AiPolish != nil && *in.AiPolish}
	if c.Time == "" {
		c.Time = defaultTime
	}
	if _, err := time.Parse("15:04", c.Time); err != nil {
		return c, httpx.Invalid("时间要写成 HH:MM，例如 08:00")
	}
	known := m.d.Notify.ChannelNames()
	c.Channels = []string{}
	for _, ch := range in.Channels {
		if !slices.Contains(known, ch) {
			return c, httpx.Invalid("没有这个渠道: " + ch)
		}
		if !slices.Contains(c.Channels, ch) {
			c.Channels = append(c.Channels, ch)
		}
	}
	c.Sections = []string{}
	for _, s := range allSections {
		if slices.Contains(in.Sections, api.BriefSectionKey(s)) {
			c.Sections = append(c.Sections, s)
		}
	}
	for _, s := range in.Sections {
		if !slices.Contains(allSections, string(s)) {
			return c, httpx.Invalid("没有这个部分: " + string(s))
		}
	}
	if l := c.Location; l != nil {
		if l.Lat < -90 || l.Lat > 90 || l.Lon < -180 || l.Lon > 180 {
			return c, httpx.Invalid("经纬度超出范围")
		}
		name := strings.TrimSpace(deref(l.Name))
		if len([]rune(name)) > 60 {
			return c, httpx.Invalid("地名太长了")
		}
		id := strings.TrimSpace(deref(l.Id))
		if len(id) > 64 || strings.ContainsAny(id, " /?&#\r\n") {
			return c, httpx.Invalid("和风城市 ID 不对")
		}
		c.Location = &api.BriefLocation{Lat: l.Lat, Lon: l.Lon, Name: &name, Id: &id}
	}
	c.WeatherBase = strings.TrimRight(strings.TrimSpace(deref(in.WeatherApiBase)), "/")
	if c.WeatherBase != "" {
		u, err := url.Parse(c.WeatherBase)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return c, httpx.Invalid("天气接口地址要以 http(s):// 开头")
		}
	} else {
		c.WeatherBase = defaultWeatherBase
	}
	return c, nil
}

func (m *Module) save(ctx context.Context, c config) error {
	for key, v := range map[string]any{
		keyEnabled: c.Enabled, keyTime: c.Time, keyChannels: c.Channels, keySections: c.Sections,
		keyAIPolish: c.AIPolish, keyWeatherBase: c.WeatherBase,
	} {
		if err := m.d.Settings.Set(ctx, key, v); err != nil {
			return err
		}
	}
	if c.Location == nil {
		return m.d.Settings.Delete(ctx, keyLocation)
	}
	return m.d.Settings.Set(ctx, keyLocation, c.Location)
}

func (m *Module) view(ctx context.Context, c config, now time.Time) api.BriefSettingsView {
	sections := make([]api.BriefSectionKey, 0, len(c.Sections))
	for _, s := range c.Sections {
		sections = append(sections, api.BriefSectionKey(s))
	}
	channels := m.d.Notify.ChannelNames()
	slices.Sort(channels)
	_, aiOK := m.polisher()
	base := c.WeatherBase
	out := api.BriefSettingsView{
		Enabled: c.Enabled, Time: c.Time, Channels: c.Channels, Location: c.Location, Sections: sections,
		AiPolish: &c.AIPolish, WeatherApiBase: &base, AvailableChannels: channels, AiAvailable: aiOK,
	}
	if c.Enabled {
		if next, ok := nextRun(c.Time, now, m.d.Config.Location, m.sentOn(ctx, now)); ok {
			out.NextRunAt = &next
		}
	}
	return out
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
