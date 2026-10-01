package brief

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// 降水、天气预警、地震的推送（B58）。

const (
	keyWeatherNotify     = "brief.weather_notify"
	keyWeatherAlertsSeen = "brief.weather_alerts_seen"
	keyQuakesSeen        = "brief.quakes_seen"
	keyRainSoonState     = "brief.weather_rain_state"

	// rainSoonQuiet is how long a rain notice keeps the next one away,
	// unless the rain stopped for rainSoonDry in between.
	rainSoonQuiet = 2 * time.Hour
	rainSoonDry   = 30 * time.Minute
	// quakeFresh is how old a quake may be and still be pushed. Older ones
	// (for example right after the switch is turned on) are only marked seen.
	quakeFresh = 6 * time.Hour
)

func defaultNotify() api.WeatherNotify {
	return api.WeatherNotify{RainLeadMinutes: 30, WarningMinLevel: api.Yellow, QuakeMinMagnitude: defaultQuakeMag, QuakeRadiusKm: defaultQuakeRadius}
}

func (m *Module) loadNotify(ctx context.Context) (api.WeatherNotify, error) {
	n := defaultNotify()
	err := m.get(ctx, keyWeatherNotify, &n)
	return n, err
}

func validNotify(n api.WeatherNotify) error {
	if n.RainLeadMinutes < 10 || n.RainLeadMinutes > 120 {
		return httpx.Invalid("提前的分钟数要在 10 到 120 之间")
	}
	if n.WarningMinLevel == api.Unknown || !n.WarningMinLevel.Valid() {
		return httpx.Invalid("预警级别要是蓝色、黄色、橙色或红色")
	}
	if n.QuakeMinMagnitude < 2 || n.QuakeMinMagnitude > 9 {
		return httpx.Invalid("震级要在 2 到 9 之间")
	}
	if n.QuakeRadiusKm < 50 || n.QuakeRadiusKm > 5000 {
		return httpx.Invalid("距离要在 50 到 5000 公里之间")
	}
	return nil
}

// GetWeatherNotify is GET /weather/notify.
func (m *Module) GetWeatherNotify(w http.ResponseWriter, r *http.Request) {
	n, err := m.loadNotify(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, n)
}

// PutWeatherNotify is PUT /weather/notify.
func (m *Module) PutWeatherNotify(w http.ResponseWriter, r *http.Request) {
	var body api.WeatherNotify
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := validNotify(body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := m.d.Settings.Set(r.Context(), keyWeatherNotify, body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, body)
}

func placeName(loc api.BriefLocation) string {
	if loc.Name != nil {
		return strings.TrimSpace(*loc.Name)
	}
	return ""
}

// ---- 快下雨 ----

type rainSoonState struct {
	LastSent time.Time `json:"lastSent"`
	LastWet  time.Time `json:"lastWet"`
}

// checkRainSoon pushes once when the 5-minute forecast shows rain or snow
// within the lead time.
func (m *Module) checkRainSoon(ctx context.Context, now time.Time) error {
	n, err := m.loadNotify(ctx)
	if err != nil || !n.RainSoon {
		return err
	}
	cfg, err := m.load(ctx)
	if err != nil || cfg.Location == nil {
		return err
	}
	qc, err := m.loadQWeather(ctx)
	if err != nil || !qc.ok() {
		return err
	}
	rain, err := m.fetchMinutely(ctx, qc, *cfg.Location)
	if err != nil {
		return err
	}
	var st rainSoonState
	if err := m.get(ctx, keyRainSoonState, &st); err != nil {
		return err
	}
	until := now.Add(time.Duration(n.RainLeadMinutes) * time.Minute)
	var first *api.MinutelyPoint
	for i, p := range rain.Points {
		// 每个点是它开始的 5 分钟
		if p.Precip > 0 && p.Time.Add(5*time.Minute).After(now) && !p.Time.After(until) {
			first = &rain.Points[i]
			break
		}
	}
	if first == nil {
		return nil
	}
	quiet := !st.LastSent.IsZero() && now.Sub(st.LastSent) < rainSoonQuiet
	stopped := st.LastWet.IsZero() || now.Sub(st.LastWet) >= rainSoonDry
	st.LastWet = now
	if quiet && !stopped {
		return m.d.Settings.Set(ctx, keyRainSoonState, st)
	}
	what := "雨"
	if first.Kind != nil && *first.Kind == api.Snow {
		what = "雪"
	}
	mins := int(math.Ceil(first.Time.Sub(now).Minutes()))
	title := "正在下" + what
	if mins > 0 {
		mins = (mins + 4) / 5 * 5
		title = fmt.Sprintf("%d 分钟后有%s", mins, what)
	}
	if p := placeName(*cfg.Location); p != "" {
		title += "（" + p + "）"
	}
	_, err = m.d.Notify.Send(ctx, notify.Notification{
		Kind: "weather.rain_soon", Title: title, Body: rain.Summary, Link: "/", Source: "brief",
		Data: map[string]any{"minutes": max(mins, 0), "kind": what},
	})
	if err != nil {
		return err
	}
	st.LastSent = now
	return m.d.Settings.Set(ctx, keyRainSoonState, st)
}

// ---- 天气预警 ----

type seenWarning struct {
	Level  api.WarningLevel `json:"level"`
	Title  string           `json:"title"`
	Expire *time.Time       `json:"expire,omitempty"`
}

// checkWarnings pushes new, upgraded and lifted alerts once each.
func (m *Module) checkWarnings(ctx context.Context, now time.Time) error {
	n, err := m.loadNotify(ctx)
	if err != nil || !n.Warnings {
		return err
	}
	cfg, err := m.load(ctx)
	if err != nil || cfg.Location == nil {
		return err
	}
	qc, err := m.loadQWeather(ctx)
	if err != nil || !qc.ok() {
		return err
	}
	items, err := m.fetchWarnings(ctx, qc, *cfg.Location)
	if err != nil {
		return err
	}
	seen := map[string]seenWarning{}
	if err := m.get(ctx, keyWeatherAlertsSeen, &seen); err != nil {
		return err
	}
	minRank := levelRank[n.WarningMinLevel]
	send := func(kind string, w api.WeatherWarning, level api.WarningLevel, title string) {
		if levelRank[level] < minRank {
			return
		}
		prio := notify.PriorityNormal
		if level == api.Red || level == api.Orange {
			prio = notify.PriorityUrgent
		}
		body := []rune(w.Text)
		if len(body) > 200 {
			body = append(body[:200], '…')
		}
		_, err := m.d.Notify.Send(ctx, notify.Notification{
			Kind: "weather.warning", Title: title, Body: string(body), Link: "/", Source: "brief", Priority: prio,
			Data: map[string]any{"id": w.Id, "level": string(level), "change": kind},
		})
		if err != nil {
			m.d.Log.Warn("brief: warning notify", "err", err)
		}
	}
	lift := func(id string, w api.WeatherWarning) {
		old, ok := seen[id]
		if !ok {
			return
		}
		delete(seen, id)
		title := w.Title
		if title == "" || !strings.Contains(title, "解除") {
			title = old.Title + "已解除"
		}
		send("cancel", w, old.Level, title)
	}

	current := map[string]bool{}
	for _, it := range items {
		if it.cancel {
			for _, id := range append([]string{it.w.Id}, it.supersedes...) {
				lift(id, it.w)
			}
			continue
		}
		w := it.w
		current[w.Id] = true
		// 更新的预警换了 id，旧的记录挪过来比较颜色
		prev, ok := seen[w.Id]
		for _, id := range it.supersedes {
			if old, found := seen[id]; found {
				if !ok || levelRank[old.Level] > levelRank[prev.Level] {
					prev, ok = old, true
				}
				delete(seen, id)
			}
		}
		switch {
		case !ok:
			send("new", w, w.Level, w.Title)
		case levelRank[w.Level] > levelRank[prev.Level]:
			send("upgrade", w, w.Level, w.Title)
		}
		seen[w.Id] = seenWarning{Level: w.Level, Title: w.Title, Expire: w.EndTime}
	}
	// 列表里没有了：过期的悄悄删掉，没到期就消失的当作解除
	for id, s := range seen {
		if current[id] {
			continue
		}
		if s.Expire != nil && !now.Before(*s.Expire) {
			delete(seen, id)
			continue
		}
		lift(id, api.WeatherWarning{Id: id})
	}
	if err := m.d.Settings.Set(ctx, keyWeatherAlertsSeen, seen); err != nil {
		return err
	}
	ids := make([]string, 0, len(seen))
	for id, s := range seen {
		ids = append(ids, id+"="+string(s.Level))
	}
	m.changed("warnings", ids)
	return nil
}

// changed publishes weather.updated when what kind holds differs from the
// last check, and drops the cached /weather/extra so pages see it.
func (m *Module) changed(kind string, ids []string) {
	slices.Sort(ids)
	sig := strings.Join(ids, ",")
	m.extraMu.Lock()
	old, had := m.lastSeen[kind]
	m.lastSeen[kind] = sig
	if had && old == sig {
		m.extraMu.Unlock()
		return
	}
	m.extraMu.Unlock()
	if kind == "warnings" {
		m.dropExtraCache()
	}
	if had || sig != "" {
		m.d.Bus.Publish("weather.updated", map[string]any{"kind": kind})
	}
}

// ---- 地震 ----

// checkQuakes pushes each new nearby earthquake once.
func (m *Module) checkQuakes(ctx context.Context, now time.Time) error {
	n, err := m.loadNotify(ctx)
	if err != nil || !n.Earthquakes {
		return err
	}
	cfg, err := m.load(ctx)
	if err != nil || cfg.Location == nil {
		return err
	}
	list, err := m.quakes(ctx)
	if err != nil {
		return err
	}
	seen := map[string]time.Time{}
	if err := m.get(ctx, keyQuakesSeen, &seen); err != nil {
		return err
	}
	near := nearbyQuakes(list, *cfg.Location, n.QuakeMinMagnitude, n.QuakeRadiusKm, now)
	ids := make([]string, 0, len(near))
	for _, q := range near {
		ids = append(ids, q.Id)
		if _, ok := seen[q.Id]; ok {
			continue
		}
		seen[q.Id] = q.Time
		if now.Sub(q.Time) > quakeFresh {
			continue
		}
		body := fmt.Sprintf("距离 %s 公里", num(q.DistanceKm))
		if q.DepthKm != nil {
			body += fmt.Sprintf("，深度 %s 公里", num(math.Round(*q.DepthKm)))
		}
		body += "，北京时间 " + q.Time.In(beijing).Format("15:04")
		_, err := m.d.Notify.Send(ctx, notify.Notification{
			Kind: "weather.quake", Title: fmt.Sprintf("M%.1f %s", q.Magnitude, q.Place), Body: body, Link: "/", Source: "brief",
			Data: map[string]any{"id": q.Id, "magnitude": q.Magnitude, "distanceKm": q.DistanceKm},
		})
		if err != nil {
			m.d.Log.Warn("brief: quake notify", "err", err)
		}
	}
	for id, t := range seen {
		if now.Sub(t) > quakeWindow {
			delete(seen, id)
		}
	}
	if err := m.d.Settings.Set(ctx, keyQuakesSeen, seen); err != nil {
		return err
	}
	m.changed("quakes", ids)
	return nil
}
