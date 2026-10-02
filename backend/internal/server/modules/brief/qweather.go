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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
)

// 和风天气（B58）。key 由用户在设置 → 早报里填，加密存在 brief.qweather。

const (
	keyQWeather         = "brief.qweather"
	keyQWeatherLocation = "brief.qweather_location"
)

// qwConfig is what brief.qweather holds.
type qwConfig struct {
	APIHost   string     `json:"apiHost"`
	APIKey    string     `json:"apiKey"`
	CheckedAt *time.Time `json:"checkedAt,omitempty"`
}

func (c qwConfig) ok() bool { return c.APIHost != "" && c.APIKey != "" }

func (m *Module) loadQWeather(ctx context.Context) (qwConfig, error) {
	var c qwConfig
	err := m.get(ctx, keyQWeather, &c)
	return c, err
}

var hostPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)

// cleanHost strips the scheme and the trailing slash the user may paste.
func cleanHost(s string) (string, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	s = strings.TrimRight(s, "/")
	if s == "" {
		return "", nil
	}
	if !hostPattern.MatchString(s) || !strings.Contains(s, ".") {
		return "", httpx.Invalid("API Host 只能是域名，例如 abc1234xyz.re.qweatherapi.com")
	}
	return s, nil
}

// qwError is a failed call to 和风天气. Code is its status code, for example
// 401, or the HTTP status when the body has none.
type qwError struct {
	Code string
	Msg  string
}

func (e *qwError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	switch e.Code {
	case "400":
		return "请求参数不对（400）"
	case "401":
		return "key 不对（401）"
	case "402":
		return "超过额度或欠费（402）"
	case "403":
		return "没有权限，或者 API Host 不对（403）"
	case "404":
		return "地点不对或接口不存在（404）"
	case "429":
		return "请求太频繁（429）"
	case "204":
		return "这个地点没有数据（204）"
	}
	return "和风天气返回错误 " + e.Code
}

// qwGet calls GET https://<host><path> with the key and decodes the JSON.
// Old v7 endpoints answer HTTP 200 with a "code" field; new ones answer
// with the HTTP status and an "error" object.
func (m *Module) qwGet(ctx context.Context, c qwConfig, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.qwBase(c.APIHost)+path, nil)
	if err != nil {
		return errors.New("和风天气地址不对")
	}
	req.Header.Set("X-QW-Api-Key", c.APIKey)
	resp, err := m.qwHTTP.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("连不上和风天气: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读和风天气返回失败: %w", err)
	}
	var head struct {
		Code  string `json:"code"`
		Error *struct {
			Status int    `json:"status"`
			Title  string `json:"title"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &head)
	if resp.StatusCode != http.StatusOK {
		code := strconv.Itoa(resp.StatusCode)
		if head.Error != nil && head.Error.Status != 0 {
			code = strconv.Itoa(head.Error.Status)
		} else if head.Code != "" {
			code = head.Code
		}
		return &qwError{Code: code}
	}
	if head.Code != "" && head.Code != "200" {
		return &qwError{Code: head.Code}
	}
	if err := json.Unmarshal(body, v); err != nil {
		return errors.New("和风天气返回的内容看不懂")
	}
	return nil
}

// qwLoc is "经度,纬度" with two decimals, as 和风天气 wants it.
func qwLoc(l api.BriefLocation) string {
	return strconv.FormatFloat(l.Lon, 'f', 2, 64) + "," + strconv.FormatFloat(l.Lat, 'f', 2, 64)
}

// qwPathLoc is "/纬度/经度" for the new endpoints.
func qwPathLoc(l api.BriefLocation) string {
	return "/" + strconv.FormatFloat(l.Lat, 'f', 2, 64) + "/" + strconv.FormatFloat(l.Lon, 'f', 2, 64)
}

type qwGeo struct {
	Location []qwCity `json:"location"`
}

type qwCity struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Lat     flexNum `json:"lat"`
	Lon     flexNum `json:"lon"`
	Adm1    string  `json:"adm1"`
	Adm2    string  `json:"adm2"`
	Country string  `json:"country"`
}

func (m *Module) requireQWeather(ctx context.Context) (qwConfig, error) {
	c, err := m.loadQWeather(ctx)
	if err == nil && !c.ok() {
		err = httpx.NewError(http.StatusPreconditionFailed, "integration_not_configured", "请先在设置中配置和风天气")
	}
	return c, err
}

func (m *Module) lookupPlaces(ctx context.Context, c qwConfig, query string) ([]qwCity, error) {
	var raw qwGeo
	q := url.Values{"location": {query}, "lang": {"zh"}, "number": {"8"}}
	err := m.qwGet(ctx, c, "/geo/v2/city/lookup?"+q.Encode(), &raw)
	var qe *qwError
	if errors.As(err, &qe) && qe.Code == "404" {
		return []qwCity{}, nil
	}
	return raw.Location, err
}

// lookupCity asks GeoAPI for the city ID of a place.
func (m *Module) lookupCity(ctx context.Context, c qwConfig, l api.BriefLocation) (string, error) {
	var raw qwGeo
	if err := m.qwGet(ctx, c, "/geo/v2/city/lookup?location="+qwLoc(l), &raw); err != nil {
		return "", err
	}
	if len(raw.Location) == 0 || raw.Location[0].ID == "" {
		return "", &qwError{Code: "404"}
	}
	return raw.Location[0].ID, nil
}

type cachedCity struct {
	Host string  `json:"host"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	ID   string  `json:"id"`
	City *qwCity `json:"city,omitempty"`
}

// cityID returns the city ID of the brief location, cached in settings
// until the location changes.
func (m *Module) cityID(ctx context.Context, c qwConfig, l api.BriefLocation) (string, error) {
	if id := deref(l.Id); id != "" {
		return id, nil
	}
	loc, err := m.resolveLocation(ctx, c, l)
	return deref(loc.Id), err
}

// resolveLocation converts old coordinates or a selected ID to GeoAPI's city.
func (m *Module) resolveLocation(ctx context.Context, c qwConfig, l api.BriefLocation) (api.BriefLocation, error) {
	m.cityMu.Lock()
	defer m.cityMu.Unlock()
	var cc cachedCity
	if err := m.get(ctx, keyQWeatherLocation, &cc); err != nil {
		return l, err
	}
	id := deref(l.Id)
	if cc.Host == c.APIHost && cc.City != nil && cc.City.Lat.Set && cc.City.Lon.Set && ((id != "" && id == cc.ID) || (id == "" && math.Abs(cc.Lat-l.Lat) < 1e-6 && math.Abs(cc.Lon-l.Lon) < 1e-6)) {
		city := cc.City
		return api.BriefLocation{Id: &city.ID, Name: &city.Name, Lat: city.Lat.V, Lon: city.Lon.V}, nil
	}
	query := id
	if query == "" {
		query = qwLoc(l)
	}
	cities, err := m.lookupPlaces(ctx, c, query)
	if err != nil {
		return l, err
	}
	if len(cities) == 0 {
		return l, errors.New("和风天气没有找到这个地区")
	}
	city := cities[0]
	if city.ID == "" || city.Name == "" || !city.Lat.Set || !city.Lon.Set || math.Abs(city.Lat.V) > 90 || math.Abs(city.Lon.V) > 180 || (id != "" && id != city.ID) {
		return l, errors.New("和风天气返回的地区信息不完整")
	}
	if err = m.d.Settings.Set(ctx, keyQWeatherLocation, cachedCity{Host: c.APIHost, Lat: l.Lat, Lon: l.Lon, ID: city.ID, City: &city}); err != nil {
		return l, err
	}
	return api.BriefLocation{Id: &city.ID, Name: &city.Name, Lat: city.Lat.V, Lon: city.Lon.V}, nil
}

// flexNum accepts 12, 12.5, "12.5" and "".
type flexNum struct {
	V   float64
	Set bool
}

func (f flexNum) MarshalJSON() ([]byte, error) {
	if !f.Set {
		return []byte("null"), nil
	}
	return json.Marshal(f.V)
}

func (f *flexNum) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" || s == "NA" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil // 看不懂的数字当没有
	}
	f.V, f.Set = v, true
	return nil
}

// qwTime parses 和风天气 times such as 2026-10-01T18:55+08:00 (no seconds).
func qwTime(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02T15:04Z07:00", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// hhmm takes the HH:MM out of 2026-10-01T06:39+08:00.
func hhmm(s string) *string {
	if t, ok := qwTime(s); ok {
		v := t.Format("15:04")
		return &v
	}
	return nil
}

// ---- 配置接口 ----

func qwView(c qwConfig) api.QWeatherConfig {
	return api.QWeatherConfig{ApiHost: c.APIHost, KeySet: c.APIKey != "", CheckedAt: c.CheckedAt}
}

// GetQWeatherConfig is GET /weather/qweather.
func (m *Module) GetQWeatherConfig(w http.ResponseWriter, r *http.Request) {
	c, err := m.loadQWeather(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, qwView(c))
}

// PutQWeatherConfig is PUT /weather/qweather. The new settings are tried
// against GeoAPI before they are saved.
func (m *Module) PutQWeatherConfig(w http.ResponseWriter, r *http.Request) {
	var body api.QWeatherConfigInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	err := m.saveQWeather(ctx, body)
	m.d.Audit.Record(ctx, "brief.qweather", "", map[string]any{"apiHost": strings.TrimSpace(body.ApiHost), "clearKey": deref(body.ClearKey)}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := m.loadQWeather(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, qwView(c))
}

func (m *Module) saveQWeather(ctx context.Context, in api.QWeatherConfigInput) error {
	host, err := cleanHost(in.ApiHost)
	if err != nil {
		return err
	}
	old, err := m.loadQWeather(ctx)
	if err != nil {
		return err
	}
	c := qwConfig{APIHost: host, APIKey: old.APIKey}
	if k := strings.TrimSpace(deref(in.ApiKey)); k != "" {
		c.APIKey = k
	}
	if deref(in.ClearKey) {
		c.APIKey = ""
	}
	if c.ok() {
		// 用北京试调一次，不依赖早报有没有设置位置
		probe := api.BriefLocation{Lat: 39.90, Lon: 116.40}
		if cfg, err := m.load(ctx); err == nil && cfg.Location != nil {
			probe = *cfg.Location
		}
		if _, err := m.lookupCity(ctx, c, probe); err != nil {
			return httpx.Invalid("和风天气验证失败：" + err.Error())
		}
		now := m.now().UTC()
		c.CheckedAt = &now
	} else if !deref(in.ClearKey) {
		if host == "" && c.APIKey != "" {
			return httpx.Invalid("请填 API Host")
		}
		if host != "" && c.APIKey == "" {
			return httpx.Invalid("请填 API KEY")
		}
	}
	if err := m.d.Settings.SetSecret(ctx, keyQWeather, c); err != nil {
		return err
	}
	_ = m.d.Settings.Delete(ctx, keyQWeatherLocation)
	m.dropExtraCache()
	return nil
}
