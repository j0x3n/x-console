package brief_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/brief"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
)

func TestQWeatherCurrentCacheAndFailure(t *testing.T) {
	env, m, ws := setup(t)
	putSettings(t, env, baseSettings(ws))
	_ = env.App.Deps.Settings.Delete(context.Background(), "brief.qweather_location")
	var geo, nowHits atomic.Int32
	var fail atomic.Bool
	now := at(2, 10, 0)
	brief.SetNow(m, func() time.Time { return now })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-QW-Api-Key") != goodKey {
			t.Error("missing API key")
		}
		if fail.Load() && r.URL.Path == "/v7/weather/now" {
			w.WriteHeader(500)
			return
		}
		switch r.URL.Path {
		case "/geo/v2/city/lookup":
			geo.Add(1)
			fmt.Fprint(w, `{"code":"200","location":[{"id":"101020100","name":"上海","lat":"31.23","lon":"121.47"}]}`)
		case "/v7/weather/now":
			nowHits.Add(1)
			if r.URL.Query().Get("location") != "101020100" {
				t.Error("not using city ID")
			}
			fmt.Fprint(w, `{"code":"200","now":{"temp":"23.4","text":"小雨","icon":"305","humidity":"88","obsTime":"2026-10-02T10:00+08:00"}}`)
		case "/v7/weather/3d":
			fmt.Fprint(w, `{"code":"200","daily":[{"tempMax":"25","tempMin":"17","sunrise":"06:00","sunset":"18:00"}]}`)
		case "/v7/weather/24h":
			fmt.Fprint(w, `{"code":"200","hourly":[{"pop":"40"},{"pop":"75"}]}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	brief.SetQWeatherBase(m, srv.URL)
	if err := env.App.Deps.Settings.SetSecret(context.Background(), "brief.qweather", map[string]any{"apiHost": "example.qweather.com", "apiKey": goodKey}); err != nil {
		t.Fatal(err)
	}
	var weather api.Weather
	env.MustDo("GET", "/weather", nil, &weather)
	if weather.Source == nil || *weather.Source != api.Qweather || weather.Temperature != 23.4 || weather.Summary != "小雨" || *weather.Icon != "305" || weather.WeatherCode != 61 || *weather.Humidity != 88 || !*weather.IsDay || weather.PrecipitationChance != 75 {
		t.Fatal(weather)
	}
	env.MustDo("GET", "/weather?refresh=true", nil, &weather)
	if nowHits.Load() != 1 {
		t.Fatal("force gap ignored")
	}
	now = now.Add(2 * time.Minute)
	env.MustDo("GET", "/weather?refresh=true", nil, &weather)
	if geo.Load() != 1 || nowHits.Load() != 2 {
		t.Fatalf("geo=%d now=%d", geo.Load(), nowHits.Load())
	}
	fail.Store(true)
	now = now.Add(11 * time.Minute)
	before := ws.hits.Load()
	if status, _ := env.Do("GET", "/weather", nil, nil); status != 502 {
		t.Fatalf("failed QWeather: %d", status)
	}
	if status, _ := env.Do("GET", "/weather?refresh=true", nil, nil); status != 502 {
		t.Fatalf("failed refresh: %d", status)
	}
	if ws.hits.Load() != before {
		t.Fatal("contacted fallback weather source")
	}
}
