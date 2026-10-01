package brief

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
)

// 地震速报（B58）。先用中国地震台网，取不到时用美国地质调查局。

const (
	keyQuakeCENC = "brief.quake_cenc_url"
	keyQuakeUSGS = "brief.quake_usgs_url"

	defaultCENCURL = "https://news.ceic.ac.cn/ajax/google?rand=1"
	defaultUSGSURL = "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/2.5_day.geojson"

	quakeTTL    = 2 * time.Minute
	quakeWindow = 3 * 24 * time.Hour
	quakeMax    = 10

	defaultQuakeMag    = 4.5
	defaultQuakeRadius = 500
)

var beijing = time.FixedZone("CST", 8*3600)

type quake struct {
	ID    string
	Time  time.Time
	Mag   float64
	Place string
	Depth *float64
	Lat   float64
	Lon   float64
}

type cachedQuakes struct {
	list []quake
	at   time.Time
}

// quakes is the recent earthquake list, cached two minutes.
func (m *Module) quakes(ctx context.Context) ([]quake, error) {
	m.quakeMu.Lock()
	defer m.quakeMu.Unlock()
	if c := m.quakeCache; c.list != nil && m.now().Sub(c.at) < quakeTTL {
		return c.list, nil
	}
	cenc, usgs := defaultCENCURL, defaultUSGSURL
	if err := m.get(ctx, keyQuakeCENC, &cenc); err != nil {
		return nil, err
	}
	if err := m.get(ctx, keyQuakeUSGS, &usgs); err != nil {
		return nil, err
	}
	list, err := m.fetchCENC(ctx, cenc)
	if err != nil {
		m.d.Log.Warn("brief: cenc earthquakes", "err", err)
		var usgsErr error
		if list, usgsErr = m.fetchUSGS(ctx, usgs); usgsErr != nil {
			return nil, fmt.Errorf("地震数据取不到: %w", errors.Join(err, usgsErr))
		}
	}
	if list == nil {
		list = []quake{}
	}
	m.quakeCache = cachedQuakes{list: list, at: m.now()}
	return list, nil
}

func (m *Module) fetchRaw(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errors.New("地址不对")
	}
	req.Header.Set("User-Agent", "x-console (self-hosted personal console)")
	resp, err := m.http.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

// fetchCENC reads the list behind the 中国地震台网 web page. It has no
// documentation; numbers come as strings or numbers.
func (m *Module) fetchCENC(ctx context.Context, u string) ([]quake, error) {
	body, err := m.fetchRaw(ctx, u)
	if err != nil {
		return nil, err
	}
	body = bytes.TrimSpace(body)
	// 有时会包一层 JSONP 的括号
	if len(body) > 1 && body[0] == '(' && body[len(body)-1] == ')' {
		body = body[1 : len(body)-1]
	}
	var raw []struct {
		ID       json.RawMessage `json:"CATA_ID"`
		M        flexNum         `json:"M"`
		OTime    string          `json:"O_TIME"`
		Lat      flexNum         `json:"EPI_LAT"`
		Lon      flexNum         `json:"EPI_LON"`
		Depth    flexNum         `json:"EPI_DEPTH"`
		Location string          `json:"LOCATION_C"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, errors.New("返回的内容看不懂")
	}
	out := make([]quake, 0, len(raw))
	for _, r := range raw {
		id := strings.Trim(string(r.ID), `"`)
		t, err := time.ParseInLocation(time.DateTime, strings.TrimSpace(r.OTime), beijing)
		if id == "" || err != nil || !r.M.Set || !r.Lat.Set || !r.Lon.Set {
			continue
		}
		q := quake{ID: "cenc:" + id, Time: t, Mag: r.M.V, Place: r.Location, Lat: r.Lat.V, Lon: r.Lon.V}
		if r.Depth.Set {
			d := r.Depth.V
			q.Depth = &d
		}
		out = append(out, q)
	}
	return out, nil
}

func (m *Module) fetchUSGS(ctx context.Context, u string) ([]quake, error) {
	body, err := m.fetchRaw(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Features []struct {
			ID         string `json:"id"`
			Properties struct {
				Mag   *float64 `json:"mag"`
				Place string   `json:"place"`
				Time  int64    `json:"time"`
			} `json:"properties"`
			Geometry struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, errors.New("返回的内容看不懂")
	}
	out := make([]quake, 0, len(raw.Features))
	for _, f := range raw.Features {
		c := f.Geometry.Coordinates
		if f.ID == "" || f.Properties.Mag == nil || len(c) < 2 {
			continue
		}
		q := quake{ID: "usgs:" + f.ID, Time: time.UnixMilli(f.Properties.Time), Mag: *f.Properties.Mag, Place: f.Properties.Place, Lat: c[1], Lon: c[0]}
		if len(c) > 2 {
			d := c[2]
			q.Depth = &d
		}
		out = append(out, q)
	}
	return out, nil
}

// distanceKm is the great-circle distance (haversine).
func distanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(a)))
}

// nearbyQuakes keeps the last three days within radius and at least minMag,
// newest first, at most ten.
func nearbyQuakes(list []quake, loc api.BriefLocation, minMag float64, radiusKm int, now time.Time) []api.Earthquake {
	out := []api.Earthquake{}
	for _, q := range list {
		if q.Mag < minMag || now.Sub(q.Time) > quakeWindow || q.Time.After(now.Add(time.Hour)) {
			continue
		}
		d := distanceKm(loc.Lat, loc.Lon, q.Lat, q.Lon)
		if d > float64(radiusKm) {
			continue
		}
		out = append(out, api.Earthquake{
			Id: q.ID, Time: q.Time.UTC(), Magnitude: q.Mag, Place: q.Place, DepthKm: q.Depth,
			Lat: q.Lat, Lon: q.Lon, DistanceKm: math.Round(d),
		})
	}
	slices.SortStableFunc(out, func(a, b api.Earthquake) int { return b.Time.Compare(a.Time) })
	if len(out) > quakeMax {
		out = out[:quakeMax]
	}
	return out
}
