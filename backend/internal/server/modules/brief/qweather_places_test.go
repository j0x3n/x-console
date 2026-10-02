package brief_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/brief"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
)

func TestQWeatherPlaceSelectionAndCoordinates(t *testing.T) {
	env, m, old := setup(t)
	var geoHits atomic.Int32
	var failed atomic.Bool
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/quakes" {
			fmt.Fprint(w, `[]`)
			return
		}
		if r.Header.Get("X-QW-Api-Key") != goodKey {
			t.Error("missing QWeather key")
		}
		if failed.Load() {
			w.WriteHeader(500)
			return
		}
		switch r.URL.Path {
		case "/geo/v2/city/lookup":
			geoHits.Add(1)
			q := r.URL.Query().Get("location")
			if q == "没有这个地区" {
				fmt.Fprint(w, `{"code":"404","location":[]}`)
				return
			}
			if q != "东海" && q != "101191002" && q != "118.75,34.54" && q != "121.47,31.23" {
				t.Errorf("unexpected GeoAPI query: %s", q)
			}
			fmt.Fprint(w, `{"code":"200","location":[{"id":"101191002","name":"东海","lat":"34.5225","lon":"118.7666","adm1":"江苏省","adm2":"连云港市","country":"中国"}]}`)
		case "/v7/weather/now":
			if r.URL.Query().Get("location") != "101191002" {
				t.Error("weather did not use selected ID")
			}
			fmt.Fprint(w, `{"code":"200","now":{"temp":"19","humidity":"83","text":"小雨","icon":"305"}}`)
		case "/v7/weather/3d":
			fmt.Fprint(w, `{"code":"200","daily":[{"tempMax":"21","tempMin":"13","pop":"80"}]}`)
		case "/airquality/v1/current/34.52/118.77":
			fmt.Fprint(w, `{"indexes":[{"code":"cn-mee-1h","aqi":85,"level":"2","category":"良"},{"code":"cn-mee","aqi":40,"level":"1","category":"优"}]}`)
		default:
			w.WriteHeader(500)
		}
	}))
	defer fake.Close()
	brief.SetQWeatherBase(m, fake.URL)
	_ = env.App.Deps.Settings.Set(context.Background(), "brief.quake_cenc_url", fake.URL+"/quakes")
	_ = env.App.Deps.Settings.Set(context.Background(), "brief.quake_usgs_url", fake.URL+"/quakes")
	if err := env.App.Deps.Settings.SetSecret(context.Background(), "brief.qweather", map[string]any{"apiHost": "test.qweather.com", "apiKey": goodKey}); err != nil {
		t.Fatal(err)
	}
	var places []api.WeatherPlace
	env.MustDo("GET", "/weather/places?q=%E4%B8%9C%E6%B5%B7", nil, &places)
	if len(places) != 1 || places[0].Id == nil || *places[0].Id != "101191002" || places[0].Region != "连云港市 · 江苏省" || places[0].Lat != 34.5225 {
		t.Fatalf("places: %+v", places)
	}
	env.MustDo("GET", "/weather/places?q=118.75,34.54", nil, &places)
	settings := baseSettings(old)
	settings["location"] = map[string]any{"id": *places[0].Id, "lat": 34.54, "lon": 118.75, "name": "旧地名"}
	view := putSettings(t, env, settings)
	if view.Location.Id == nil || *view.Location.Id != "101191002" || view.Location.Lat != 34.5225 || *view.Location.Name != "东海" {
		t.Fatalf("saved location not canonical: %+v", view.Location)
	}
	before := geoHits.Load()
	var weather api.Weather
	env.MustDo("GET", "/weather", nil, &weather)
	env.MustDo("GET", "/weather", nil, &weather)
	if geoHits.Load() != before || weather.Source == nil || *weather.Source != api.Qweather || weather.Latitude != 34.5225 {
		t.Fatalf("weather or ID reuse: %+v", weather)
	}
	var extra api.WeatherExtra
	env.MustDo("GET", "/weather/extra", nil, &extra)
	if extra.Air == nil || extra.Air.Aqi != 40 || extra.Air.Category != "优" {
		t.Fatalf("air coordinates or standard: %+v", extra.Air)
	}
	// Old saved coordinates are resolved by GeoAPI before any weather request.
	_ = env.App.Deps.Settings.Delete(context.Background(), "brief.qweather_location")
	_ = env.App.Deps.Settings.Set(context.Background(), "brief.location", map[string]any{"lat": 34.54, "lon": 118.75, "name": "旧东海"})
	before = geoHits.Load()
	env.MustDo("GET", "/weather", nil, &weather)
	if geoHits.Load() != before+1 || weather.Latitude != 34.5225 || *weather.Location != "东海" {
		t.Fatalf("old location: %+v", weather)
	}
	env.MustDo("GET", "/weather/places?q=%E6%B2%A1%E6%9C%89%E8%BF%99%E4%B8%AA%E5%9C%B0%E5%8C%BA", nil, &places)
	if len(places) != 0 {
		t.Fatal("no match should return empty list")
	}
	failed.Store(true)
	if status, _ := env.Do("GET", "/weather/places?q=东海", nil, nil); status != 502 {
		t.Fatalf("search failure: %d", status)
	}
	if old.hits.Load() != 0 {
		t.Fatal("contacted another weather source")
	}
}

func TestQWeatherRequiredForWeatherAndPlaces(t *testing.T) {
	env, _, old := setup(t)
	_ = env.App.Deps.Settings.Delete(context.Background(), "brief.qweather")
	_ = env.App.Deps.Settings.Set(context.Background(), "brief.location", map[string]any{"lat": 34.54, "lon": 118.75})
	for _, path := range []string{"/weather", "/weather/places?q=东海"} {
		status, body := env.Do("GET", path, nil, nil)
		if status != 412 || !strings.Contains(string(body), "和风") {
			t.Fatalf("%s: %d %s", path, status, body)
		}
	}
	if old.hits.Load() != 0 {
		t.Fatal("contacted another weather source without QWeather")
	}
}
