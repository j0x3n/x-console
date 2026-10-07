package brief_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/brief"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

const goodKey = "good-key"

// qwFake is a fake 和风天气 plus fake earthquake feeds.
type qwFake struct {
	*httptest.Server
	mu        sync.Mutex
	alertsV1  string // JSON of alerts[]; "" answers 404
	warningV7 string // JSON of warning[]
	minutely  string // JSON of minutely[]
	nowIcon   string // 实况图标；"" answers 404
	hourly    string // JSON of hourly[]
	fail      map[string]bool
	cenc      string // "" answers 500
	usgs      string
}

func (f *qwFake) set(fn func(f *qwFake)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func newQWFake(t *testing.T) *qwFake {
	t.Helper()
	f := &qwFake{fail: map[string]bool{}, alertsV1: "[]", warningV7: "[]", minutely: "[]", usgs: `{"features":[]}`}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		p := r.URL.Path
		switch {
		case p == "/cenc":
			if f.cenc == "" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(f.cenc))
			return
		case p == "/usgs":
			_, _ = w.Write([]byte(f.usgs))
			return
		}
		if r.Header.Get("X-QW-Api-Key") != goodKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"status":401,"type":"x","title":"Unauthorized"}}`))
			return
		}
		for prefix, bad := range f.fail {
			if bad && strings.HasPrefix(p, prefix) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		switch {
		case p == "/geo/v2/city/lookup":
			_, _ = w.Write([]byte(`{"code":"200","location":[{"name":"上海","id":"101020100","lat":"31.23","lon":"121.47"}]}`))
		case strings.HasPrefix(p, "/weatheralert/v1/current/"):
			if f.alertsV1 == "" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"status":404}}`))
				return
			}
			_, _ = fmt.Fprintf(w, `{"metadata":{},"alerts":%s}`, f.alertsV1)
		case p == "/v7/warning/now":
			_, _ = fmt.Fprintf(w, `{"code":"200","warning":%s}`, f.warningV7)
		case p == "/v7/weather/now" && f.nowIcon != "":
			_, _ = fmt.Fprintf(w, `{"code":"200","now":{"temp":"19","text":"x","icon":%q}}`, f.nowIcon)
		case p == "/v7/weather/3d" && f.nowIcon != "":
			_, _ = w.Write([]byte(`{"code":"200","daily":[{"tempMax":"26","tempMin":"9"}]}`))
		case p == "/v7/weather/24h" && f.hourly != "":
			_, _ = fmt.Fprintf(w, `{"code":"200","hourly":%s}`, f.hourly)
		case p == "/v7/minutely/5m":
			_, _ = fmt.Fprintf(w, `{"code":"200","summary":"20分钟后有雨","minutely":%s}`, f.minutely)
		case strings.HasPrefix(p, "/airquality/v1/current/"):
			_, _ = w.Write([]byte(`{"indexes":[{"code":"us-epa","aqi":80,"level":"2","category":"Moderate"},
				{"code":"cn-mee","aqi":62,"level":"2","category":"良","primaryPollutant":{"code":"pm10","name":"PM10"}}],
				"pollutants":[{"code":"pm2p5","concentration":{"value":30.5}},{"code":"pm10","concentration":{"value":73}}]}`))
		case p == "/v7/air/now":
			_, _ = w.Write([]byte(`{"code":"200","now":{"aqi":"40","level":"1","category":"优","primary":"NA","pm10":"35","pm2p5":"20"}}`))
		case p == "/v7/indices/1d":
			_, _ = w.Write([]byte(`{"code":"200","daily":[{"date":"2026-10-01","type":"3","name":"穿衣指数","level":"5","category":"较冷","text":"多穿点"}]}`))
		case p == "/v7/astronomy/sun":
			_, _ = w.Write([]byte(`{"code":"200","sunrise":"2026-10-01T05:52+08:00","sunset":"2026-10-01T17:41+08:00"}`))
		case p == "/v7/astronomy/moon":
			_, _ = w.Write([]byte(`{"code":"200","moonrise":"2026-10-01T15:10+08:00","moonset":"","moonPhase":[{"name":"上弦月"}]}`))
		case p == "/v7/historical/weather":
			_, _ = w.Write([]byte(`{"code":"200","weatherDaily":{"tempMax":"26","tempMin":"19"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func setupQW(t *testing.T) (*testutil.Env, *brief.Module, *qwFake) {
	t.Helper()
	env, m, ws := setup(t)
	f := newQWFake(t)
	brief.SetQWeatherBase(m, f.URL)
	ctx := context.Background()
	_ = env.App.Deps.Settings.Set(ctx, "brief.quake_cenc_url", f.URL+"/cenc")
	_ = env.App.Deps.Settings.Set(ctx, "brief.quake_usgs_url", f.URL+"/usgs")
	putSettings(t, env, baseSettings(ws))
	_ = env.App.Deps.Settings.Delete(ctx, "brief.qweather")
	_ = env.App.Deps.Settings.Delete(ctx, "brief.qweather_location")
	return env, m, f
}

func configureQW(t *testing.T, env *testutil.Env) {
	t.Helper()
	env.MustDo(http.MethodPut, "/weather/qweather", map[string]any{"apiHost": "abc.re.qweatherapi.com", "apiKey": goodKey}, nil)
}

type noteList struct {
	Items []struct{ Kind, Title, Body, Priority string }
}

func notes(t *testing.T, env *testutil.Env, kind string) []struct{ Kind, Title, Body, Priority string } {
	t.Helper()
	var out noteList
	env.MustDo(http.MethodGet, "/notifications?limit=100", nil, &out)
	var keep []struct{ Kind, Title, Body, Priority string }
	for _, n := range out.Items {
		if n.Kind == kind {
			keep = append(keep, n)
		}
	}
	return keep
}

func TestQWeatherConfig(t *testing.T) {
	env, _, _ := setupQW(t)
	var c api.QWeatherConfig
	env.MustDo(http.MethodGet, "/weather/qweather", nil, &c)
	if c.KeySet || c.ApiHost != "" {
		t.Fatalf("default: %+v", c)
	}
	if status, body := env.Do(http.MethodPut, "/weather/qweather", map[string]any{"apiHost": "abc.re.qweatherapi.com", "apiKey": "bad"}, nil); status != http.StatusBadRequest || !strings.Contains(string(body), "key 不对") {
		t.Fatalf("bad key: %d %s", status, body)
	}
	env.MustDo(http.MethodGet, "/weather/qweather", nil, &c)
	if c.KeySet {
		t.Fatal("bad key was saved")
	}
	if status, _ := env.Do(http.MethodPut, "/weather/qweather", map[string]any{"apiHost": "a b/c", "apiKey": goodKey}, nil); status != http.StatusBadRequest {
		t.Fatalf("bad host: %d", status)
	}
	env.MustDo(http.MethodPut, "/weather/qweather", map[string]any{"apiHost": "https://abc.re.qweatherapi.com/", "apiKey": goodKey}, &c)
	if !c.KeySet || c.ApiHost != "abc.re.qweatherapi.com" || c.CheckedAt == nil {
		t.Fatalf("saved: %+v", c)
	}
	// key 留空时保留原来的
	env.MustDo(http.MethodPut, "/weather/qweather", map[string]any{"apiHost": "xyz.re.qweatherapi.com", "apiKey": ""}, &c)
	if !c.KeySet || c.ApiHost != "xyz.re.qweatherapi.com" {
		t.Fatalf("kept key: %+v", c)
	}
	_, raw := env.Do(http.MethodGet, "/weather/qweather", nil, nil)
	if strings.Contains(string(raw), goodKey) {
		t.Fatalf("key leaked: %s", raw)
	}
	env.MustDo(http.MethodPut, "/weather/qweather", map[string]any{"apiHost": "xyz.re.qweatherapi.com", "clearKey": true}, &c)
	if c.KeySet {
		t.Fatalf("cleared: %+v", c)
	}
}

func cencQuakes(now time.Time) string {
	bj := time.FixedZone("CST", 8*3600)
	ts := func(d time.Duration) string { return now.Add(-d).In(bj).Format(time.DateTime) }
	return fmt.Sprintf(`[
		{"CATA_ID":"near","M":"5.0","O_TIME":%q,"EPI_LAT":"30.27","EPI_LON":"120.15","EPI_DEPTH":10,"LOCATION_C":"浙江杭州市"},
		{"CATA_ID":"far","M":"6.1","O_TIME":%q,"EPI_LAT":"30.30","EPI_LON":"102.90","EPI_DEPTH":"12","LOCATION_C":"四川雅安市芦山县"},
		{"CATA_ID":"old","M":"5.5","O_TIME":%q,"EPI_LAT":"30.27","EPI_LON":"120.15","EPI_DEPTH":8,"LOCATION_C":"浙江杭州市"},
		{"CATA_ID":"small","M":3.1,"O_TIME":%q,"EPI_LAT":"31.00","EPI_LON":"121.00","EPI_DEPTH":5,"LOCATION_C":"上海松江区"}
	]`, ts(time.Hour), ts(2*time.Hour), ts(96*time.Hour), ts(30*time.Minute))
}

func TestWeatherExtra(t *testing.T) {
	env, m, f := setupQW(t)
	f.set(func(f *qwFake) { f.cenc = cencQuakes(time.Now()) })

	var ex api.WeatherExtra
	env.MustDo(http.MethodGet, "/weather/extra", nil, &ex)
	if ex.Configured || len(ex.Warnings) != 0 || ex.Indices == nil || ex.Air != nil {
		t.Fatalf("not configured: %+v", ex)
	}
	// 默认 M4.5、500 km：只有杭州那条
	if len(ex.Earthquakes) != 1 || ex.Earthquakes[0].Id != "cenc:near" || ex.Earthquakes[0].DistanceKm < 140 || ex.Earthquakes[0].DistanceKm > 190 {
		t.Fatalf("earthquakes: %+v", ex.Earthquakes)
	}

	configureQW(t, env)
	pt := time.Now().Add(20 * time.Minute).In(time.FixedZone("", 8*3600)).Format("2006-01-02T15:04-07:00")
	f.set(func(f *qwFake) {
		f.alertsV1 = "" // 新版 404，退回旧版
		f.warningV7 = `[
			{"id":"y1","title":"上海市气象台发布大风黄色预警","typeName":"大风","severityColor":"Yellow","sender":"上海市气象台","pubTime":"2026-10-01T08:00+08:00","text":"注意防风"},
			{"id":"r1","title":"上海市气象台发布暴雨红色预警","typeName":"暴雨","severityColor":"","sender":"上海市气象台","pubTime":"2026-10-01T07:00+08:00","text":"注意防雨"}]`
		f.minutely = fmt.Sprintf(`[{"fxTime":%q,"precip":"0.12","type":"rain"}]`, pt)
		f.fail["/v7/indices"] = true
		f.fail["/airquality/v1"] = true
	})
	brief.DropCaches(m)
	env.MustDo(http.MethodGet, "/weather/extra?refresh=true", nil, &ex)
	if !ex.Configured || len(ex.Warnings) != 2 || ex.Warnings[0].Id != "r1" || ex.Warnings[0].Level != api.Red || ex.Warnings[1].Level != api.Yellow {
		t.Fatalf("warnings: %+v", ex.Warnings)
	}
	if len(ex.Indices) != 0 {
		t.Fatalf("failed indices: %+v", ex.Indices)
	}
	if ex.Air != nil {
		t.Fatalf("failed air should be absent: %+v", ex.Air)
	}
	if ex.Minutely == nil || len(ex.Minutely.Points) != 1 || ex.Minutely.Points[0].Precip != 0.12 || *ex.Minutely.Points[0].Kind != api.Rain {
		t.Fatalf("minutely: %+v", ex.Minutely)
	}
	if ex.Astronomy == nil || *ex.Astronomy.Sunrise != "05:52" || *ex.Astronomy.MoonPhase != "上弦月" || ex.Astronomy.Moonset != nil {
		t.Fatalf("astronomy: %+v", ex.Astronomy)
	}
	if ex.Yesterday == nil || ex.Yesterday.High != 26 || ex.Yesterday.Low != 19 {
		t.Fatalf("yesterday: %+v", ex.Yesterday)
	}

	// 新版接口恢复，空气用 cn-mee 那条
	f.set(func(f *qwFake) {
		f.alertsV1 = `[{"id":"o1","headline":"橙色","eventType":{"name":"高温"},"color":{"code":"orange"},"senderName":"x",
			"issuedTime":"2026-10-01T09:00+08:00","messageType":{"code":"alert"}},
			{"id":"c1","headline":"解除","eventType":{"name":"大风"},"color":{"code":"yellow"},"senderName":"x",
			"issuedTime":"2026-10-01T09:00+08:00","messageType":{"code":"cancel","supersedes":["y1"]}}]`
		f.fail = map[string]bool{}
	})
	brief.DropCaches(m)
	env.MustDo(http.MethodGet, "/weather/extra?refresh=true", nil, &ex)
	if len(ex.Warnings) != 1 || ex.Warnings[0].Level != api.Orange || ex.Warnings[0].TypeName != "高温" {
		t.Fatalf("v1 warnings: %+v", ex.Warnings)
	}
	if ex.Air == nil || ex.Air.Aqi != 62 || ex.Air.Category != "良" || *ex.Air.Primary != "PM10" || *ex.Air.Pm10 != 73 {
		t.Fatalf("air v1: %+v", ex.Air)
	}
	if len(ex.Indices) != 1 || ex.Indices[0].Name != "穿衣指数" {
		t.Fatalf("indices: %+v", ex.Indices)
	}

	// 全部失败时回 502
	f.set(func(f *qwFake) {
		for _, p := range []string{"/weatheralert", "/v7", "/airquality", "/geo"} {
			f.fail[p] = true
		}
	})
	brief.DropCaches(m)
	_ = env.App.Deps.Settings.Delete(context.Background(), "brief.qweather_location")
	if status, _ := env.Do(http.MethodGet, "/weather/extra?refresh=true", nil, nil); status != http.StatusBadGateway {
		t.Fatalf("all failed: %d", status)
	}
}

func TestQuakesFallBackToUSGS(t *testing.T) {
	env, _, f := setupQW(t)
	now := time.Now()
	f.set(func(f *qwFake) {
		f.cenc = ""
		f.usgs = fmt.Sprintf(`{"features":[
			{"id":"us1","properties":{"mag":4.8,"place":"near Shanghai","time":%d},"geometry":{"coordinates":[121.9,31.5,12]}},
			{"id":"us2","properties":{"mag":4.9,"place":"far","time":%d},"geometry":{"coordinates":[139.7,35.7,30]}},
			{"id":"us3","properties":{"mag":null,"place":"x","time":%d},"geometry":{"coordinates":[121.5,31.2]}}]}`,
			now.Add(-time.Hour).UnixMilli(), now.Add(-time.Hour).UnixMilli(), now.UnixMilli())
	})
	var ex api.WeatherExtra
	env.MustDo(http.MethodGet, "/weather/extra", nil, &ex)
	if len(ex.Earthquakes) != 1 || ex.Earthquakes[0].Id != "usgs:us1" || *ex.Earthquakes[0].DepthKm != 12 {
		t.Fatalf("usgs: %+v", ex.Earthquakes)
	}
	// 上海到北京约 1070 公里
	if d := brief.DistanceKm(31.23, 121.47, 39.90, 116.40); math.Abs(d-1067) > 15 {
		t.Fatalf("distance: %v", d)
	}
}

func TestWeatherNotifySettings(t *testing.T) {
	env, _, _ := setupQW(t)
	var n api.WeatherNotify
	env.MustDo(http.MethodGet, "/weather/notify", nil, &n)
	if n.RainSoon || n.Warnings || n.Earthquakes || n.RainLeadMinutes != 30 || n.WarningMinLevel != api.Yellow || n.QuakeMinMagnitude != 4.5 || n.QuakeRadiusKm != 500 {
		t.Fatalf("defaults: %+v", n)
	}
	bad := n
	bad.QuakeRadiusKm = 10
	if status, _ := env.Do(http.MethodPut, "/weather/notify", bad, nil); status != http.StatusBadRequest {
		t.Fatalf("bad radius: %d", status)
	}
	bad = n
	bad.WarningMinLevel = api.Unknown
	if status, _ := env.Do(http.MethodPut, "/weather/notify", bad, nil); status != http.StatusBadRequest {
		t.Fatalf("bad level: %d", status)
	}
}

func TestWarningNotify(t *testing.T) {
	env, m, f := setupQW(t)
	configureQW(t, env)
	n := api.WeatherNotify{Warnings: true, RainLeadMinutes: 30, WarningMinLevel: api.Yellow, QuakeMinMagnitude: 4.5, QuakeRadiusKm: 500}
	env.MustDo(http.MethodPut, "/weather/notify", n, nil)
	ctx := context.Background()
	alert := func(id, color, code string, supersedes ...string) string {
		s, _ := json.Marshal(supersedes)
		return fmt.Sprintf(`{"id":%q,"headline":"上海市气象台发布暴雨%s预警","eventType":{"name":"暴雨"},"color":{"code":%q},
			"senderName":"上海市气象台","issuedTime":"2026-10-01T09:00+08:00","expireTime":"2099-01-01T00:00+08:00",
			"description":"雨很大","messageType":{"code":%q,"supersedes":%s}}`, id, color, color, code, s)
	}
	step := func(alerts ...string) {
		t.Helper()
		f.set(func(f *qwFake) { f.alertsV1 = "[" + strings.Join(alerts, ",") + "]" })
		if err := brief.CheckWarnings(m, ctx, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	step(alert("a", "yellow", "alert"), alert("b", "blue", "alert"))
	step(alert("a", "yellow", "alert"), alert("b", "blue", "alert")) // 同一条不重复推
	if got := notes(t, env, "weather.warning"); len(got) != 1 || !strings.Contains(got[0].Title, "yellow") || got[0].Priority == "urgent" {
		t.Fatalf("new: %+v", got)
	}
	step(alert("a2", "orange", "update", "a"), alert("b", "blue", "alert")) // 升级换了 id
	got := notes(t, env, "weather.warning")
	if len(got) != 2 || !strings.Contains(got[0].Title, "orange") || got[0].Priority != "urgent" {
		t.Fatalf("upgrade: %+v", got)
	}
	step(alert("a2", "orange", "update", "a"), alert("b", "blue", "alert"))
	step(alert("c", "orange", "cancel", "a2"), alert("b", "blue", "alert")) // 解除
	got = notes(t, env, "weather.warning")
	if len(got) != 3 || !strings.Contains(got[0].Title, "解除") {
		t.Fatalf("cancel: %+v", got)
	}
	step(alert("b", "blue", "alert"))
	if got := notes(t, env, "weather.warning"); len(got) != 3 {
		t.Fatalf("blue below minimum: %+v", got)
	}
}

func TestRainSoonNotify(t *testing.T) {
	env, m, f := setupQW(t)
	configureQW(t, env)
	n := api.WeatherNotify{RainSoon: true, RainLeadMinutes: 30, WarningMinLevel: api.Yellow, QuakeMinMagnitude: 4.5, QuakeRadiusKm: 500}
	env.MustDo(http.MethodPut, "/weather/notify", n, nil)
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, shanghai)
	// rainAt makes the 2-hour forecast seen at now, raining from start for dur.
	rainAt := func(now time.Time, start, dur time.Duration, kind string) {
		var pts []string
		for i := 0; i < 24; i++ {
			t0 := now.Add(time.Duration(i) * 5 * time.Minute)
			p := "0.0"
			if d := t0.Sub(now); d >= start && d < start+dur {
				p = "0.3"
			}
			pts = append(pts, fmt.Sprintf(`{"fxTime":%q,"precip":%q,"type":%q}`, t0.Format("2006-01-02T15:04-07:00"), p, kind))
		}
		f.set(func(f *qwFake) { f.minutely = "[" + strings.Join(pts, ",") + "]" })
	}
	check := func(now time.Time) {
		t.Helper()
		if err := brief.CheckRainSoon(m, ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	rainAt(base, 20*time.Minute, time.Hour, "rain")
	check(base)
	got := notes(t, env, "weather.rain_soon")
	if len(got) != 1 || got[0].Title != "20 分钟后有雨（上海）" || got[0].Body != "20分钟后有雨" {
		t.Fatalf("first: %+v", got)
	}
	// 冷却期内一直在下，不重复推
	for i := 1; i <= 6; i++ {
		now := base.Add(time.Duration(i) * 5 * time.Minute)
		rainAt(now, 0, time.Hour, "rain")
		check(now)
	}
	if got := notes(t, env, "weather.rain_soon"); len(got) != 1 {
		t.Fatalf("cooldown: %+v", got)
	}
	// 停了 30 分钟以后又要下雪，再推
	for i := 7; i <= 13; i++ {
		now := base.Add(time.Duration(i) * 5 * time.Minute)
		rainAt(now, 0, 0, "rain")
		check(now)
	}
	now := base.Add(70 * time.Minute)
	rainAt(now, 10*time.Minute, time.Hour, "snow")
	check(now)
	got = notes(t, env, "weather.rain_soon")
	if len(got) != 2 || got[0].Title != "10 分钟后有雪（上海）" {
		t.Fatalf("after stop: %+v", got)
	}
}

// 2026-10-07 东海：实况晴、逐小时降水概率 0，分钟降水却说一直在下小雨。
func TestDoubtfulMinutelyRain(t *testing.T) {
	env, m, f := setupQW(t)
	configureQW(t, env)
	n := api.WeatherNotify{RainSoon: true, RainLeadMinutes: 30, WarningMinLevel: api.Yellow, QuakeMinMagnitude: 4.5, QuakeRadiusKm: 500}
	env.MustDo(http.MethodPut, "/weather/notify", n, nil)
	ctx := context.Background()
	now := time.Now()
	var pts, hours []string
	for i := 0; i < 24; i++ {
		pts = append(pts, fmt.Sprintf(`{"fxTime":%q,"precip":"0.05","type":"rain"}`, now.Add(time.Duration(i)*5*time.Minute).Format("2006-01-02T15:04-07:00")))
	}
	hour := now.Truncate(time.Hour)
	setPop := func(pop string) {
		hours = hours[:0]
		for i := 0; i < 4; i++ {
			hours = append(hours, fmt.Sprintf(`{"fxTime":%q,"pop":%q}`, hour.Add(time.Duration(i)*time.Hour).Format("2006-01-02T15:04-07:00"), pop))
		}
		f.set(func(f *qwFake) { f.hourly = "[" + strings.Join(hours, ",") + "]" })
	}
	f.set(func(f *qwFake) {
		f.minutely = "[" + strings.Join(pts, ",") + "]"
		f.nowIcon = "150"
	})
	setPop("0")
	extra := func() api.WeatherExtra {
		t.Helper()
		brief.DropCaches(m)
		var ex api.WeatherExtra
		env.MustDo(http.MethodGet, "/weather/extra?refresh=true", nil, &ex)
		if ex.Minutely == nil || len(ex.Minutely.Points) != 24 {
			t.Fatalf("minutely: %+v", ex.Minutely)
		}
		return ex
	}

	ex := extra()
	if ex.Minutely.Points[0].Precip != 0 || ex.Minutely.Summary != "分钟预报和实况对不上，按没有降水处理" {
		t.Fatalf("doubtful rain kept: %+v", ex.Minutely)
	}
	if err := brief.CheckRainSoon(m, ctx, now); err != nil {
		t.Fatal(err)
	}
	if got := notes(t, env, "weather.rain_soon"); len(got) != 0 {
		t.Fatalf("doubtful rain pushed: %+v", got)
	}

	// 逐小时也说可能下，信分钟降水
	setPop("60")
	if ex := extra(); ex.Minutely.Points[0].Precip != 0.05 {
		t.Fatalf("pop 60: %+v", ex.Minutely)
	}
	// 实况在下雨，也信
	setPop("0")
	f.set(func(f *qwFake) { f.nowIcon = "305" })
	brief.DropCaches(m)
	if err := brief.CheckRainSoon(m, ctx, now); err != nil {
		t.Fatal(err)
	}
	if got := notes(t, env, "weather.rain_soon"); len(got) != 1 {
		t.Fatalf("raining now: %+v", got)
	}
}

func TestQuakeNotify(t *testing.T) {
	env, m, f := setupQW(t)
	n := api.WeatherNotify{Earthquakes: true, RainLeadMinutes: 30, WarningMinLevel: api.Yellow, QuakeMinMagnitude: 3, QuakeRadiusKm: 2000}
	env.MustDo(http.MethodPut, "/weather/notify", n, nil)
	f.set(func(f *qwFake) { f.cenc = cencQuakes(time.Now()) })
	ctx := context.Background()
	for range 2 {
		if err := brief.CheckQuakes(m, ctx, time.Now()); err != nil {
			t.Fatal(err)
		}
		brief.DropCaches(m)
	}
	got := notes(t, env, "weather.quake")
	// near、far、small 各一次；old 超过 3 天
	if len(got) != 3 {
		t.Fatalf("quakes: %+v", got)
	}
	titles := got[0].Title + got[1].Title + got[2].Title
	if !strings.Contains(titles, "M6.1 四川雅安市芦山县") || !strings.Contains(titles, "M3.1 上海松江区") {
		t.Fatalf("titles: %s", titles)
	}
	for _, q := range got {
		if !strings.HasPrefix(q.Body, "距离 ") || !strings.Contains(q.Body, "北京时间") {
			t.Fatalf("body: %s", q.Body)
		}
	}
}
