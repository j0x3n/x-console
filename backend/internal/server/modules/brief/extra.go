package brief

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
)

// GET /weather/extra（B58）：预警、分钟降水、空气、生活指数、天文、昨天、地震。

const (
	extraTTL      = 5 * time.Minute
	extraForceGap = time.Minute
)

type cachedExtra struct {
	key string
	v   api.WeatherExtra
	at  time.Time
}

func (m *Module) dropExtraCache() {
	m.extraMu.Lock()
	m.extra = cachedExtra{}
	m.extraMu.Unlock()
}

// GetWeatherExtra is GET /weather/extra.
func (m *Module) GetWeatherExtra(w http.ResponseWriter, r *http.Request, params api.GetWeatherExtraParams) {
	ctx := r.Context()
	cfg, err := m.load(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if cfg.Location == nil {
		httpx.Fail(w, r, httpx.ErrIntegrationMissing)
		return
	}
	qc, err := m.loadQWeather(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.weatherExtra(ctx, qc, *cfg.Location, params.Refresh != nil && *params.Refresh)
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "weather_unavailable", err.Error()))
		return
	}
	n, err := m.loadNotify(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	minMag, radius := defaultQuakeMag, defaultQuakeRadius
	if n.Earthquakes {
		minMag, radius = n.QuakeMinMagnitude, n.QuakeRadiusKm
	}
	out.Earthquakes = []api.Earthquake{}
	if list, err := m.quakes(ctx); err != nil {
		m.d.Log.Warn("brief: earthquakes", "err", err)
	} else {
		out.Earthquakes = nearbyQuakes(list, *cfg.Location, minMag, radius, m.now())
	}
	httpx.JSON(w, http.StatusOK, out)
}

// weatherExtra is the 和风天气 part of /weather/extra, cached five minutes.
func (m *Module) weatherExtra(ctx context.Context, qc qwConfig, loc api.BriefLocation, force bool) (api.WeatherExtra, error) {
	empty := api.WeatherExtra{Warnings: []api.WeatherWarning{}, Indices: []api.WeatherIndex{}, FetchedAt: m.now().UTC()}
	if !qc.ok() {
		return empty, nil
	}
	var err error
	loc, err = m.resolveLocation(ctx, qc, loc)
	if err != nil {
		return empty, err
	}
	key := fmt.Sprintf("%s|%.4f|%.4f", qc.APIHost, loc.Lat, loc.Lon)
	ttl := extraTTL
	if force {
		ttl = extraForceGap
	}
	m.extraMu.Lock()
	if c := m.extra; c.key == key && m.now().Sub(c.at) < ttl {
		m.extraMu.Unlock()
		return c.v, nil
	}
	m.extraMu.Unlock()

	out := empty
	out.Configured = true
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		errs []error
		oks  int
	)
	run := func(name string, f func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := f()
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				m.d.Log.Warn("brief: qweather", "item", name, "err", err)
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
				return
			}
			oks++
		}()
	}
	run("预警", func() error {
		items, err := m.fetchWarnings(ctx, qc, loc)
		if err == nil {
			ws := activeWarnings(items)
			mu.Lock()
			out.Warnings = ws
			mu.Unlock()
		}
		return err
	})
	run("分钟降水", func() error {
		v, err := m.fetchMinutely(ctx, qc, loc)
		if err == nil {
			mu.Lock()
			out.Minutely = &v
			mu.Unlock()
		}
		return err
	})
	run("空气质量", func() error {
		v, err := m.fetchAir(ctx, qc, loc)
		if err == nil {
			mu.Lock()
			out.Air = &v
			mu.Unlock()
		}
		return err
	})
	run("生活指数", func() error {
		v, err := m.fetchIndices(ctx, qc, loc)
		if err == nil {
			mu.Lock()
			out.Indices = v
			mu.Unlock()
		}
		return err
	})
	run("天文", func() error {
		v, err := m.fetchAstronomy(ctx, qc, loc)
		if err == nil {
			mu.Lock()
			out.Astronomy = &v
			mu.Unlock()
		}
		return err
	})
	run("昨天", func() error {
		v, err := m.fetchYesterday(ctx, qc, loc)
		if err == nil {
			mu.Lock()
			out.Yesterday = &v
			mu.Unlock()
		}
		return err
	})
	wg.Wait()
	if oks == 0 && len(errs) > 0 {
		return empty, errs[0]
	}
	m.extraMu.Lock()
	m.extra = cachedExtra{key: key, v: out, at: m.now()}
	m.extraMu.Unlock()
	return out, nil
}

// ---- 一天只变一次的，缓存到当天结束 ----

// daily runs f once per key and local day. When f says keep is false the
// value is used once and asked again next time.
func daily[T any](m *Module, key string, f func() (v T, keep bool, err error)) (T, error) {
	day := m.now().In(m.d.Config.Location).Format("20060102")
	full := day + "|" + key
	m.extraMu.Lock()
	if v, ok := m.daily[full]; ok {
		m.extraMu.Unlock()
		return v.(T), nil
	}
	m.extraMu.Unlock()
	v, keep, err := f()
	if err != nil || !keep {
		return v, err
	}
	m.extraMu.Lock()
	for k := range m.daily {
		if !strings.HasPrefix(k, day+"|") {
			delete(m.daily, k)
		}
	}
	m.daily[full] = v
	m.extraMu.Unlock()
	return v, nil
}

// ---- 预警 ----

// warningItem is one alert as 和风天气 sends it, including cancel notices.
type warningItem struct {
	w          api.WeatherWarning
	cancel     bool
	supersedes []string
}

var levelRank = map[api.WarningLevel]int{api.Unknown: 0, api.Blue: 1, api.Yellow: 2, api.Orange: 3, api.Red: 4}

// warningLevel turns 和风天气's color into a WarningLevel. When the color
// is missing it looks at the title, which always names the color.
func warningLevel(color, title string) api.WarningLevel {
	switch strings.ToLower(strings.TrimSpace(color)) {
	case "red":
		return api.Red
	case "orange":
		return api.Orange
	case "yellow":
		return api.Yellow
	case "blue":
		return api.Blue
	}
	for _, c := range []struct {
		word  string
		level api.WarningLevel
	}{{"红色", api.Red}, {"橙色", api.Orange}, {"黄色", api.Yellow}, {"蓝色", api.Blue}} {
		if strings.Contains(title, c.word) {
			return c.level
		}
	}
	return api.Unknown
}

// fetchWarnings asks the new alert endpoint and falls back to the v7 one.
func (m *Module) fetchWarnings(ctx context.Context, qc qwConfig, loc api.BriefLocation) ([]warningItem, error) {
	var err error
	loc, err = m.resolveLocation(ctx, qc, loc)
	if err != nil {
		return nil, err
	}
	items, err := m.fetchWarningsV1(ctx, qc, loc)
	if err == nil {
		return items, nil
	}
	old, oldErr := m.fetchWarningsV7(ctx, qc, loc)
	if oldErr != nil {
		return nil, err
	}
	return old, nil
}

func (m *Module) fetchWarningsV1(ctx context.Context, qc qwConfig, loc api.BriefLocation) ([]warningItem, error) {
	var raw struct {
		Alerts []struct {
			ID            string `json:"id"`
			Headline      string `json:"headline"`
			SenderName    string `json:"senderName"`
			IssuedTime    string `json:"issuedTime"`
			EffectiveTime string `json:"effectiveTime"`
			OnsetTime     string `json:"onsetTime"`
			ExpireTime    string `json:"expireTime"`
			Description   string `json:"description"`
			Instruction   string `json:"instruction"`
			EventType     struct {
				Name string `json:"name"`
			} `json:"eventType"`
			Color struct {
				Code string `json:"code"`
			} `json:"color"`
			MessageType struct {
				Code       string   `json:"code"`
				Supersedes []string `json:"supersedes"`
			} `json:"messageType"`
		} `json:"alerts"`
	}
	if err := m.qwGet(ctx, qc, "/weatheralert/v1/current"+qwPathLoc(loc), &raw); err != nil {
		return nil, err
	}
	out := make([]warningItem, 0, len(raw.Alerts))
	for _, a := range raw.Alerts {
		w := api.WeatherWarning{
			Id: a.ID, Title: a.Headline, TypeName: a.EventType.Name, Level: warningLevel(a.Color.Code, a.Headline),
			Sender: a.SenderName, Text: joinText(a.Description, a.Instruction),
		}
		w.IssuedAt, _ = qwTime(a.IssuedTime)
		start := a.EffectiveTime
		if start == "" {
			start = a.OnsetTime
		}
		w.StartTime = timePtr(start)
		w.EndTime = timePtr(a.ExpireTime)
		out = append(out, warningItem{w: w, cancel: strings.EqualFold(a.MessageType.Code, "cancel"), supersedes: a.MessageType.Supersedes})
	}
	return out, nil
}

func (m *Module) fetchWarningsV7(ctx context.Context, qc qwConfig, loc api.BriefLocation) ([]warningItem, error) {
	var raw struct {
		Warning []struct {
			ID            string `json:"id"`
			Title         string `json:"title"`
			TypeName      string `json:"typeName"`
			SeverityColor string `json:"severityColor"`
			Sender        string `json:"sender"`
			PubTime       string `json:"pubTime"`
			StartTime     string `json:"startTime"`
			EndTime       string `json:"endTime"`
			Status        string `json:"status"`
			Text          string `json:"text"`
		} `json:"warning"`
	}
	if err := m.qwGet(ctx, qc, "/v7/warning/now?location="+qwLoc(loc), &raw); err != nil {
		return nil, err
	}
	out := make([]warningItem, 0, len(raw.Warning))
	for _, a := range raw.Warning {
		w := api.WeatherWarning{
			Id: a.ID, Title: a.Title, TypeName: a.TypeName, Level: warningLevel(a.SeverityColor, a.Title),
			Sender: a.Sender, Text: a.Text, StartTime: timePtr(a.StartTime), EndTime: timePtr(a.EndTime),
		}
		w.IssuedAt, _ = qwTime(a.PubTime)
		out = append(out, warningItem{w: w, cancel: strings.EqualFold(a.Status, "cancel")})
	}
	return out, nil
}

// activeWarnings drops cancel notices and sorts the rest, heaviest first.
func activeWarnings(items []warningItem) []api.WeatherWarning {
	out := []api.WeatherWarning{}
	for _, it := range items {
		if !it.cancel {
			out = append(out, it.w)
		}
	}
	slices.SortStableFunc(out, func(a, b api.WeatherWarning) int {
		if d := levelRank[b.Level] - levelRank[a.Level]; d != 0 {
			return d
		}
		return b.IssuedAt.Compare(a.IssuedAt)
	})
	return out
}

func joinText(parts ...string) string {
	var keep []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, "\n\n")
}

func timePtr(s string) *time.Time {
	if t, ok := qwTime(s); ok {
		return &t
	}
	return nil
}

// ---- 分钟降水 ----

func (m *Module) fetchMinutely(ctx context.Context, qc qwConfig, loc api.BriefLocation) (api.MinutelyRain, error) {
	var err error
	loc, err = m.resolveLocation(ctx, qc, loc)
	if err != nil {
		return api.MinutelyRain{}, err
	}
	var raw struct {
		Summary  string `json:"summary"`
		Minutely []struct {
			FxTime string  `json:"fxTime"`
			Precip flexNum `json:"precip"`
			Type   string  `json:"type"`
		} `json:"minutely"`
	}
	if err := m.qwGet(ctx, qc, "/v7/minutely/5m?location="+qwLoc(loc), &raw); err != nil {
		return api.MinutelyRain{}, err
	}
	out := api.MinutelyRain{Summary: raw.Summary, Points: []api.MinutelyPoint{}}
	for _, p := range raw.Minutely {
		t, ok := qwTime(p.FxTime)
		if !ok {
			continue
		}
		pt := api.MinutelyPoint{Time: t, Precip: p.Precip.V}
		if k := api.MinutelyPointKind(p.Type); k.Valid() {
			pt.Kind = &k
		}
		out.Points = append(out.Points, pt)
		if len(out.Points) == 24 {
			break
		}
	}
	return out, nil
}

// ---- 空气质量 ----

func (m *Module) fetchAir(ctx context.Context, qc qwConfig, loc api.BriefLocation) (api.AirQuality, error) {
	return m.fetchAirV1(ctx, qc, loc)
}

func (m *Module) fetchAirV1(ctx context.Context, qc qwConfig, loc api.BriefLocation) (api.AirQuality, error) {
	var raw struct {
		Indexes []struct {
			Code             string  `json:"code"`
			Aqi              flexNum `json:"aqi"`
			Level            flexNum `json:"level"`
			Category         string  `json:"category"`
			PrimaryPollutant *struct {
				Name string `json:"name"`
			} `json:"primaryPollutant"`
		} `json:"indexes"`
		Pollutants []struct {
			Code          string `json:"code"`
			Concentration struct {
				Value flexNum `json:"value"`
			} `json:"concentration"`
		} `json:"pollutants"`
	}
	if err := m.qwGet(ctx, qc, "/airquality/v1/current"+qwPathLoc(loc)+"?lang=zh", &raw); err != nil {
		return api.AirQuality{}, err
	}
	if len(raw.Indexes) == 0 {
		return api.AirQuality{}, errors.New("没有空气质量数据")
	}
	idx := raw.Indexes[0]
	for _, x := range raw.Indexes {
		if x.Code == "cn-mee" {
			idx = x
			break
		}
	}
	out := api.AirQuality{Aqi: int(idx.Aqi.V + 0.5), Level: int(idx.Level.V), Category: idx.Category}
	if idx.PrimaryPollutant != nil && idx.PrimaryPollutant.Name != "" {
		out.Primary = &idx.PrimaryPollutant.Name
	}
	for _, p := range raw.Pollutants {
		if !p.Concentration.Value.Set {
			continue
		}
		v := p.Concentration.Value.V
		switch p.Code {
		case "pm2p5":
			out.Pm2p5 = &v
		case "pm10":
			out.Pm10 = &v
		}
	}
	return out, nil
}

func (m *Module) fetchIndices(ctx context.Context, qc qwConfig, loc api.BriefLocation) ([]api.WeatherIndex, error) {
	id, err := m.cityID(ctx, qc, loc)
	if err != nil {
		return nil, err
	}
	return daily(m, "indices|"+qc.APIHost+"|"+qwLoc(loc), func() ([]api.WeatherIndex, bool, error) {
		var raw struct {
			Daily []api.WeatherIndex `json:"daily"`
		}
		if err := m.qwGet(ctx, qc, "/v7/indices/1d?type=1,2,3,5,8,9&location="+id+"&lang=zh", &raw); err != nil {
			return nil, false, err
		}
		if raw.Daily == nil {
			raw.Daily = []api.WeatherIndex{}
		}
		return raw.Daily, true, nil
	})
}

func (m *Module) fetchAstronomy(ctx context.Context, qc qwConfig, loc api.BriefLocation) (api.Astronomy, error) {
	id, err := m.cityID(ctx, qc, loc)
	if err != nil {
		return api.Astronomy{}, err
	}
	date := m.now().In(m.d.Config.Location).Format("20060102")
	return daily(m, "astronomy|"+qc.APIHost+"|"+id, func() (api.Astronomy, bool, error) {
		var sun struct {
			Sunrise string `json:"sunrise"`
			Sunset  string `json:"sunset"`
		}
		var moon struct {
			Moonrise  string `json:"moonrise"`
			Moonset   string `json:"moonset"`
			MoonPhase []struct {
				Name string `json:"name"`
			} `json:"moonPhase"`
		}
		sunErr := m.qwGet(ctx, qc, "/v7/astronomy/sun?location="+id+"&date="+date, &sun)
		moonErr := m.qwGet(ctx, qc, "/v7/astronomy/moon?location="+id+"&date="+date, &moon)
		if sunErr != nil && moonErr != nil {
			return api.Astronomy{}, false, sunErr
		}
		out := api.Astronomy{Sunrise: hhmm(sun.Sunrise), Sunset: hhmm(sun.Sunset), Moonrise: hhmm(moon.Moonrise), Moonset: hhmm(moon.Moonset)}
		if len(moon.MoonPhase) > 0 && moon.MoonPhase[0].Name != "" {
			out.MoonPhase = &moon.MoonPhase[0].Name
		}
		return out, sunErr == nil && moonErr == nil, nil // 有一半失败时不缓存，下次再试
	})
}

func (m *Module) fetchYesterday(ctx context.Context, qc qwConfig, loc api.BriefLocation) (api.DayRange, error) {
	id, err := m.cityID(ctx, qc, loc)
	if err != nil {
		return api.DayRange{}, err
	}
	date := m.now().In(m.d.Config.Location).AddDate(0, 0, -1).Format("20060102")
	return daily(m, "yesterday|"+qc.APIHost+"|"+id, func() (api.DayRange, bool, error) {
		var raw struct {
			WeatherDaily struct {
				TempMax flexNum `json:"tempMax"`
				TempMin flexNum `json:"tempMin"`
			} `json:"weatherDaily"`
		}
		if err := m.qwGet(ctx, qc, "/v7/historical/weather?location="+id+"&date="+date, &raw); err != nil {
			return api.DayRange{}, false, err
		}
		if !raw.WeatherDaily.TempMax.Set || !raw.WeatherDaily.TempMin.Set {
			return api.DayRange{}, false, errors.New("没有昨天的数据")
		}
		return api.DayRange{High: raw.WeatherDaily.TempMax.V, Low: raw.WeatherDaily.TempMin.V}, true, nil
	})
}
