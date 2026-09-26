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
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
)

// weatherTTL is how long a forecast is reused. The home page asks often.
const weatherTTL = 10 * time.Minute

type cachedWeather struct {
	w  api.Weather
	at time.Time
}

// openMeteo is the part of the Open-Meteo forecast response we read.
type openMeteo struct {
	Current struct {
		Temperature float64 `json:"temperature_2m"`
		WeatherCode int     `json:"weather_code"`
	} `json:"current"`
	Daily struct {
		WeatherCode []int      `json:"weather_code"`
		Max         []float64  `json:"temperature_2m_max"`
		Min         []float64  `json:"temperature_2m_min"`
		Precip      []*float64 `json:"precipitation_probability_max"`
	} `json:"daily"`
}

// fetchWeather returns the current weather and today's forecast, cached per
// place and API address.
func (m *Module) fetchWeather(ctx context.Context, base string, loc api.BriefLocation) (api.Weather, error) {
	key := fmt.Sprintf("%s|%.3f|%.3f", base, loc.Lat, loc.Lon)
	m.weatherMu.Lock()
	if c, ok := m.weatherCache[key]; ok && time.Since(c.at) < weatherTTL {
		m.weatherMu.Unlock()
		w := c.w
		w.Location = loc.Name
		return w, nil
	}
	m.weatherMu.Unlock()

	q := url.Values{
		"latitude":      {strconv.FormatFloat(loc.Lat, 'f', 4, 64)},
		"longitude":     {strconv.FormatFloat(loc.Lon, 'f', 4, 64)},
		"current":       {"temperature_2m,weather_code"},
		"daily":         {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max"},
		"timezone":      {m.d.Config.Location.String()},
		"forecast_days": {"1"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/forecast?"+q.Encode(), nil)
	if err != nil {
		return api.Weather{}, errors.New("天气接口地址不对")
	}
	resp, err := m.http.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return api.Weather{}, fmt.Errorf("天气接口连接失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return api.Weather{}, fmt.Errorf("天气接口返回 HTTP %d", resp.StatusCode)
	}
	var raw openMeteo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return api.Weather{}, errors.New("天气接口返回的内容看不懂")
	}
	if len(raw.Daily.Max) == 0 || len(raw.Daily.Min) == 0 {
		return api.Weather{}, errors.New("天气接口没有返回今天的预报")
	}
	w := api.Weather{
		Latitude: loc.Lat, Longitude: loc.Lon, Temperature: round1(raw.Current.Temperature), WeatherCode: raw.Current.WeatherCode,
		Summary: weatherText(raw.Current.WeatherCode), High: round1(raw.Daily.Max[0]), Low: round1(raw.Daily.Min[0]),
		FetchedAt: time.Now().UTC(), Location: loc.Name,
	}
	if len(raw.Daily.Precip) > 0 && raw.Daily.Precip[0] != nil {
		w.PrecipitationChance = int(math.Round(*raw.Daily.Precip[0]))
	}
	m.weatherMu.Lock()
	m.weatherCache[key] = cachedWeather{w: w, at: time.Now()}
	m.weatherMu.Unlock()
	return w, nil
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// weatherText describes a WMO weather code in Chinese.
func weatherText(code int) string {
	switch code {
	case 0:
		return "晴"
	case 1:
		return "晴间多云"
	case 2:
		return "多云"
	case 3:
		return "阴"
	case 45, 48:
		return "雾"
	case 51, 53, 55:
		return "毛毛雨"
	case 56, 57:
		return "冻毛毛雨"
	case 61:
		return "小雨"
	case 63:
		return "中雨"
	case 65:
		return "大雨"
	case 66, 67:
		return "冻雨"
	case 71:
		return "小雪"
	case 73:
		return "中雪"
	case 75:
		return "大雪"
	case 77:
		return "雪粒"
	case 80:
		return "阵雨"
	case 81, 82:
		return "强阵雨"
	case 85, 86:
		return "阵雪"
	case 95:
		return "雷阵雨"
	case 96, 99:
		return "雷阵雨伴有冰雹"
	}
	return "天气代码 " + strconv.Itoa(code)
}

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
