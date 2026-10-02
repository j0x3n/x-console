package brief

import (
	"context"
	"errors"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
)

type qwCurrent struct {
	Now struct {
		Temp     flexNum `json:"temp"`
		Humidity flexNum `json:"humidity"`
		Text     string  `json:"text"`
		Icon     string  `json:"icon"`
		ObsTime  string  `json:"obsTime"`
	} `json:"now"`
}
type qwDailyWeather struct {
	Daily []struct {
		Date    string  `json:"fxDate"`
		Max     flexNum `json:"tempMax"`
		Min     flexNum `json:"tempMin"`
		Pop     flexNum `json:"pop"`
		Sunrise string  `json:"sunrise"`
		Sunset  string  `json:"sunset"`
	} `json:"daily"`
}

func (m *Module) fetchQWeather(ctx context.Context, c qwConfig, loc api.BriefLocation) (api.Weather, error) {
	id, err := m.cityID(ctx, c, loc)
	if err != nil {
		return api.Weather{}, err
	}
	query := "?location=" + url.QueryEscape(id) + "&lang=zh"
	var now qwCurrent
	if err = m.qwGet(ctx, c, "/v7/weather/now"+query, &now); err != nil {
		return api.Weather{}, err
	}
	var daily qwDailyWeather
	if err = m.qwGet(ctx, c, "/v7/weather/3d"+query, &daily); err != nil {
		return api.Weather{}, err
	}
	if !now.Now.Temp.Set || now.Now.Text == "" || len(daily.Daily) == 0 || !daily.Daily[0].Max.Set || !daily.Daily[0].Min.Set {
		return api.Weather{}, errors.New("和风天气没有返回实况或今天的预报")
	}
	d := daily.Daily[0]
	source := api.Qweather
	w := api.Weather{Temperature: round1(now.Now.Temp.V), Summary: now.Now.Text, WeatherCode: qwWMO(now.Now.Icon), Icon: &now.Now.Icon, Source: &source, High: round1(d.Max.V), Low: round1(d.Min.V), Latitude: loc.Lat, Longitude: loc.Lon, Location: loc.Name, FetchedAt: m.now().UTC()}
	if now.Now.Humidity.Set {
		h := int(math.Round(now.Now.Humidity.V))
		w.Humidity = &h
	}
	zone := m.d.Config.Location
	if obs, ok := qwTime(now.Now.ObsTime); ok {
		zone = obs.Location()
	}
	local := m.now().In(zone)
	sunrise, sunset := "07:00", "19:00"
	if d.Sunrise != "" && d.Sunset != "" {
		sunrise, sunset = d.Sunrise, d.Sunset
	}
	hhmm := local.Format("15:04")
	day := hhmm >= sunrise && hhmm < sunset
	w.IsDay = &day
	pop := d.Pop
	if !pop.Set {
		var hourly struct {
			Hourly []struct {
				Time string  `json:"fxTime"`
				Pop  flexNum `json:"pop"`
			} `json:"hourly"`
		}
		if err = m.qwGet(ctx, c, "/v7/weather/24h"+query, &hourly); err != nil {
			m.d.Log.Warn("brief: 和风降水概率查询失败", "err", err)
		} else {
			for _, h := range hourly.Hourly {
				if t, ok := qwTime(h.Time); ok && t.In(zone).Format("2006-01-02") != local.Format("2006-01-02") {
					continue
				}
				if h.Pop.Set && (!pop.Set || h.Pop.V > pop.V) {
					pop = h.Pop
				}
			}
		}
	}
	if pop.Set {
		w.PrecipitationChance = int(math.Round(math.Max(0, math.Min(100, pop.V))))
	}
	return w, nil
}

// v7 icon codes are mapped to the nearest WMO condition for old clients.
func qwWMO(icon string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(icon))
	switch n {
	case 100, 150:
		return 0
	case 102:
		return 1
	case 101, 103, 151, 152, 153:
		return 2
	case 104:
		return 3
	case 300, 350:
		return 80
	case 301, 351:
		return 81
	case 302:
		return 95
	case 303, 304:
		return 99
	case 305, 309:
		return 61
	case 306, 313, 314:
		return 63
	case 307, 308, 310, 311, 312, 315, 316:
		return 65
	case 317:
		return 66
	case 318:
		return 67
	case 399:
		return 61
	case 400:
		return 71
	case 401:
		return 73
	case 402, 403:
		return 75
	case 404, 405, 406, 456:
		return 77
	case 407, 457:
		return 85
	case 408:
		return 71
	case 409:
		return 73
	case 410:
		return 75
	case 499:
		return 71
	case 500, 501, 509, 510, 514, 515:
		return 45
	case 502, 503, 504, 507, 508, 511, 512, 513:
		return 48
	}
	return 3
}
