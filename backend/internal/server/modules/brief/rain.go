package brief

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

const (
	keyRainAlert     = "brief.rain_alert"
	keyRainAlertLast = "brief.rain_alert_last"

	defaultGeoBase = "https://geocoding-api.open-meteo.com"
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

// rainChance is the highest hourly precipitation probability in the next
// hours hours.
func (m *Module) rainChance(ctx context.Context, base string, loc api.BriefLocation, hours int) (int, error) {
	q := url.Values{
		"latitude":       {strconv.FormatFloat(loc.Lat, 'f', 4, 64)},
		"longitude":      {strconv.FormatFloat(loc.Lon, 'f', 4, 64)},
		"hourly":         {"precipitation_probability"},
		"timezone":       {m.d.Config.Location.String()},
		"forecast_hours": {strconv.Itoa(hours)},
	}
	var raw struct {
		Hourly struct {
			Precip []*float64 `json:"precipitation_probability"`
		} `json:"hourly"`
	}
	if err := m.getJSON(ctx, base+"/v1/forecast?"+q.Encode(), &raw); err != nil {
		return 0, err
	}
	best := 0
	for _, p := range raw.Hourly.Precip {
		if p != nil {
			best = max(best, int(math.Round(*p)))
		}
	}
	return best, nil
}

func (m *Module) SearchWeatherPlaces(w http.ResponseWriter, r *http.Request, params api.SearchWeatherPlacesParams) {
	name := strings.TrimSpace(params.Q)
	if name == "" {
		httpx.Fail(w, r, httpx.Invalid("请输入地名"))
		return
	}
	q := url.Values{"name": {name}, "count": {"8"}, "language": {"zh"}, "format": {"json"}}
	var raw struct {
		Results []struct {
			Name    string  `json:"name"`
			Admin1  string  `json:"admin1"`
			Country string  `json:"country"`
			Lat     float64 `json:"latitude"`
			Lon     float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := m.getJSON(r.Context(), m.geoBase+"/v1/search?"+q.Encode(), &raw); err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "weather_unavailable", err.Error()))
		return
	}
	out := make([]api.WeatherPlace, 0, len(raw.Results))
	for _, p := range raw.Results {
		out = append(out, api.WeatherPlace{Name: p.Name, Region: p.Admin1, Country: p.Country, Lat: round4(p.Lat), Lon: round4(p.Lon)})
	}
	httpx.JSON(w, http.StatusOK, out)
}

func round4(f float64) float64 { return math.Round(f*1e4) / 1e4 }

// getJSON fetches a small JSON document from a weather service.
func (m *Module) getJSON(ctx context.Context, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return errors.New("天气接口地址不对")
	}
	resp, err := m.http.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("天气接口连接失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("天气接口返回 HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v); err != nil {
		return errors.New("天气接口返回的内容看不懂")
	}
	return nil
}
