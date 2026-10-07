package brief

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

const (
	keyRainAlert     = "brief.rain_alert"
	keyRainAlertLast = "brief.rain_alert_last"

	// rainQuiet is the shortest gap between two rain notifications.
	rainQuiet = 6 * time.Hour
)

func defaultRainAlert() api.RainAlert {
	return api.RainAlert{Enabled: false, Threshold: 60, LeadHours: 2}
}

func (m *Module) loadRainAlert(ctx context.Context) (api.RainAlert, error) {
	a := defaultRainAlert()
	if err := m.get(ctx, keyRainAlert, &a); err != nil {
		return a, err
	}
	return a, nil
}

func validRainAlert(a api.RainAlert) error {
	if a.Threshold < 10 || a.Threshold > 100 {
		return httpx.Invalid("降雨概率要在 10 到 100 之间")
	}
	if a.LeadHours < 1 || a.LeadHours > 6 {
		return httpx.Invalid("提前的小时数要在 1 到 6 之间")
	}
	return nil
}

func (m *Module) GetRainAlert(w http.ResponseWriter, r *http.Request) {
	a, err := m.loadRainAlert(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, a)
}

func (m *Module) PutRainAlert(w http.ResponseWriter, r *http.Request) {
	var body api.RainAlert
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := validRainAlert(body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := m.d.Settings.Set(r.Context(), keyRainAlert, body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, body)
}

// checkRain sends one notification when rain is likely in the next few hours.
// It runs every 30 minutes and stays quiet for rainQuiet after a notice.
func (m *Module) checkRain(ctx context.Context, now time.Time) error {
	a, err := m.loadRainAlert(ctx)
	if err != nil || !a.Enabled {
		return err
	}
	cfg, err := m.load(ctx)
	if err != nil || cfg.Location == nil {
		return err
	}
	var last time.Time
	if err := m.get(ctx, keyRainAlertLast, &last); err != nil {
		return err
	}
	if !last.IsZero() && now.Sub(last) < rainQuiet {
		return nil
	}
	chance, err := m.rainChance(ctx, cfg.WeatherBase, *cfg.Location, a.LeadHours)
	if err != nil || chance < a.Threshold {
		return err
	}
	place := "你设置的位置"
	if cfg.Location.Name != nil && *cfg.Location.Name != "" {
		place = *cfg.Location.Name
	}
	n := notify.Notification{
		Kind: "weather.rain", Title: "可能要下雨了",
		Body:   fmt.Sprintf("%s未来 %d 小时降雨概率 %d%%，出门记得带伞。", place, a.LeadHours, chance),
		Link:   "/",
		Source: "brief",
		Data:   map[string]any{"chance": chance, "hours": a.LeadHours},
	}
	if _, err := m.d.Notify.Send(ctx, n); err != nil {
		return err
	}
	return m.d.Settings.Set(ctx, keyRainAlertLast, now.UTC())
}

func (m *Module) SearchWeatherPlaces(w http.ResponseWriter, r *http.Request, params api.SearchWeatherPlacesParams) {
	name := strings.TrimSpace(params.Q)
	if name == "" {
		httpx.Fail(w, r, httpx.Invalid("请输入地名"))
		return
	}
	if len([]rune(name)) > 60 {
		httpx.Fail(w, r, httpx.Invalid("地名太长了"))
		return
	}
	if _, err := m.requireQWeather(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.searchPlaces(r.Context(), name)
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "weather_unavailable", err.Error()))
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) searchPlaces(ctx context.Context, query string) ([]api.WeatherPlace, error) {
	c, err := m.requireQWeather(ctx)
	if err != nil {
		return nil, err
	}
	cities, err := m.lookupPlaces(ctx, c, query)
	if err != nil {
		return nil, err
	}
	out := make([]api.WeatherPlace, 0, len(cities))
	for _, city := range cities {
		if city.ID == "" || city.Name == "" || !city.Lat.Set || !city.Lon.Set {
			continue
		}
		region := city.Adm1
		if city.Adm2 != "" && city.Adm2 != city.Adm1 && city.Adm2 != city.Name {
			region = strings.TrimSpace(city.Adm2 + " · " + city.Adm1)
		}
		id := city.ID
		out = append(out, api.WeatherPlace{Id: &id, Name: city.Name, Region: region, Country: city.Country, Lat: city.Lat.V, Lon: city.Lon.V})
	}
	return out, nil
}

// rainChance uses the selected city's QWeather hourly forecast.
func (m *Module) rainChance(ctx context.Context, base string, loc api.BriefLocation, hours int) (int, error) {
	best, _, err := m.popAhead(ctx, loc, hours)
	return best, err
}

// popAhead is the highest hourly rain chance in the next hours. known is
// false when no hour in that range has a chance.
func (m *Module) popAhead(ctx context.Context, loc api.BriefLocation, hours int) (best int, known bool, err error) {
	c, err := m.requireQWeather(ctx)
	if err != nil {
		return 0, false, err
	}
	loc, err = m.resolveLocation(ctx, c, loc)
	if err != nil {
		return 0, false, err
	}
	var raw struct {
		Hourly []struct {
			Time string  `json:"fxTime"`
			Pop  flexNum `json:"pop"`
		} `json:"hourly"`
	}
	if err = m.qwGet(ctx, c, "/v7/weather/24h?location="+deref(loc.Id)+"&lang=zh", &raw); err != nil {
		return 0, false, err
	}
	// 逐小时的时间是这一小时开始的时候，正在过的这一小时也算
	from := m.now().Add(-time.Hour)
	end := m.now().Add(time.Duration(hours) * time.Hour)
	for _, h := range raw.Hourly {
		t, ok := qwTime(h.Time)
		if !ok || !t.After(from) || t.After(end) || !h.Pop.Set {
			continue
		}
		known = true
		if chance := int(h.Pop.V + 0.5); chance > best {
			best = chance
		}
	}
	return best, known, nil
}
