package brief_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

var shanghai, _ = time.LoadLocation("Asia/Shanghai")

// 2026-10-27 is a Tuesday.
func at(day, h, m int) time.Time { return time.Date(2026, 10, day, h, m, 0, 0, shanghai) }

type weatherServer struct {
	*httptest.Server
	hits  atomic.Int32
	query atomic.Value // last query string
}

func newWeatherServer(t *testing.T) *weatherServer {
	t.Helper()
	s := &weatherServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.query.Store(r.URL.RawQuery)
		if r.URL.Path != "/v1/forecast" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"current":{"temperature_2m":18.24,"weather_code":2},
			"daily":{"weather_code":[61],"temperature_2m_max":[22.1],"temperature_2m_min":[14],"precipitation_probability_max":[60]}}`))
	}))
	t.Cleanup(s.Close)
	return s
}

func setup(t *testing.T) (*testutil.Env, *brief.Module, *weatherServer) {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*brief.Module](env.App.Deps.Registry, brief.ServiceKey)
	if !ok {
		t.Fatal("brief module not registered")
	}
	ws := newWeatherServer(t)
	return env, m, ws
}

func putSettings(t *testing.T, env *testutil.Env, body map[string]any) api.BriefSettingsView {
	t.Helper()
	var out api.BriefSettingsView
	env.MustDo(http.MethodPut, "/briefs/settings", body, &out)
	return out
}

func baseSettings(ws *weatherServer) map[string]any {
	return map[string]any{
		"enabled": true, "time": "08:00", "channels": []string{},
		"sections":       []string{"weather", "calendar", "issues", "reminders", "alerts", "habits", "renewals"},
		"location":       map[string]any{"lat": 31.23, "lon": 121.47, "name": "上海"},
		"weatherApiBase": ws.URL,
	}
}

func TestShouldSend(t *testing.T) {
	cases := []struct {
		now     time.Time
		enabled bool
		sent    bool
		want    bool
	}{
		{at(27, 7, 59), true, false, false},
		{at(27, 8, 0), true, false, true},
		{at(27, 10, 59), true, false, true}, // late start after downtime
		{at(27, 11, 0), true, false, false}, // too late, wait for tomorrow
		{at(27, 8, 1), true, true, false},   // already sent
		{at(27, 8, 0), false, false, false}, // switched off
	}
	for _, c := range cases {
		if got := brief.ShouldSend(c.enabled, "08:00", c.now, shanghai, c.sent); got != c.want {
			t.Fatalf("%v enabled=%v sent=%v: %v", c.now, c.enabled, c.sent, got)
		}
	}
	if brief.ShouldSend(true, "8点", at(27, 9, 0), shanghai, false) {
		t.Fatal("bad time accepted")
	}
	if next, _ := brief.NextRun("08:00", at(27, 7, 0), shanghai, false); !next.Equal(at(27, 8, 0)) {
		t.Fatalf("next before: %v", next)
	}
	if next, _ := brief.NextRun("08:00", at(27, 9, 0), shanghai, true); !next.Equal(at(28, 8, 0)) {
		t.Fatalf("next after send: %v", next)
	}
}

func TestBriefWithAllProvidersMissing(t *testing.T) {
	env, m, ws := setup(t)
	putSettings(t, env, baseSettings(ws))
	content, keys, err := brief.Generate(m, context.Background(), module.NewRegistry(), at(27, 8, 0))
	if err != nil {
		t.Fatal(err)
	}
	want := "# 早报 · 10月27日 周二\n\n## 天气\n\n上海，多云，现在 18.2°C。今天 14 到 22.1°C，降水概率 60%。出门记得带伞。\n"
	if content != want || strings.Join(keys, ",") != "weather" {
		t.Fatalf("content:\n%q\nkeys %v", content, keys)
	}
	q, _ := ws.query.Load().(string)
	if !strings.Contains(q, "latitude=31.2300") || !strings.Contains(q, "timezone=Asia%2FShanghai") {
		t.Fatalf("weather query: %s", q)
	}

	// Without a location the brief is just the date.
	s := baseSettings(ws)
	delete(s, "location")
	putSettings(t, env, s)
	content, keys, _ = brief.Generate(m, context.Background(), module.NewRegistry(), at(27, 8, 0))
	if content != "# 早报 · 10月27日 周二\n" || len(keys) != 0 {
		t.Fatalf("date only: %q %v", content, keys)
	}
}

// ---- fake providers ----

type fakeCalendar struct{}

func (fakeCalendar) Events(_ context.Context, from, to time.Time) ([]contracts.CalendarEvent, error) {
	return []contracts.CalendarEvent{
		{Title: "出差", Start: from, End: to, AllDay: true},
		{Title: "周会", Start: from.Add(9 * time.Hour), End: from.Add(10 * time.Hour), Location: "3 楼"},
	}, nil
}

type fakeIssues struct{ contracts.Issues }

func (fakeIssues) ListDue(_ context.Context, until time.Time) ([]contracts.IssueRef, error) {
	today := time.Date(until.Year(), until.Month(), until.Day(), 0, 0, 0, 0, until.Location())
	old := today.AddDate(0, 0, -2)
	return []contracts.IssueRef{
		{Key: "XC-1", Title: "修登录", DueDate: &old},
		{Key: "XC-2", Title: "写文档", DueDate: &today},
	}, nil
}

type fakeReminders struct{ contracts.Reminders }

func (fakeReminders) Upcoming(_ context.Context, until time.Time) ([]contracts.ReminderRef, error) {
	return []contracts.ReminderRef{
		{Title: "交房租", At: at(27, 14, 0)},
		{Title: "明天的事", At: until.Add(time.Hour)},
	}, nil
}

type fakeHosts struct {
	contracts.Hosts
	fail bool
}

func (h fakeHosts) Alerts(_ context.Context, since time.Time) ([]contracts.HostAlert, error) {
	if h.fail {
		return nil, errors.New("boom")
	}
	resolved := at(27, 1, 0)
	return []contracts.HostAlert{
		{HostName: "web-1", Message: "CPU 超过 90%", FiredAt: at(26, 23, 30), Resolved: &resolved},
		{HostName: "old", Message: "too old", FiredAt: since.Add(-time.Hour)},
	}, nil
}

type fakeHabits struct{ contracts.Habits }

func (fakeHabits) Today(context.Context) ([]contracts.HabitProgress, error) {
	return []contracts.HabitProgress{{Name: "喝水", Unit: "杯", Target: 8, Done: 2, Streak: 3}}, nil
}

type fakeRenewals struct{}

func (fakeRenewals) UpcomingRenewals(context.Context, time.Time) ([]brief.Renewal, error) {
	return []brief.Renewal{{Name: "域名 example.com", Date: at(30, 0, 0), Amount: 68, Currency: "CNY"}}, nil
}

type fakePolisher struct{}

func (fakePolisher) Polish(_ context.Context, md string) (string, error) {
	if !strings.Contains(md, "周会") {
		return "", errors.New("missing content")
	}
	return "今天有一个会，两个 Issue 要处理。", nil
}

func fakeRegistry(failHosts bool) *module.Registry {
	reg := module.NewRegistry()
	module.Provide[contracts.Calendar](reg, contracts.CalendarKey, fakeCalendar{})
	module.Provide[contracts.Issues](reg, contracts.IssuesKey, fakeIssues{})
	module.Provide[contracts.Reminders](reg, contracts.RemindersKey, fakeReminders{})
	module.Provide[contracts.Hosts](reg, contracts.HostsKey, fakeHosts{fail: failHosts})
	module.Provide[contracts.Habits](reg, contracts.HabitsKey, fakeHabits{})
	module.Provide[brief.RenewalSource](reg, brief.RenewalsKey, fakeRenewals{})
	module.Provide[brief.Polisher](reg, brief.PolisherKey, fakePolisher{})
	return reg
}

func TestBriefWithFakeProviders(t *testing.T) {
	env, m, ws := setup(t)
	s := baseSettings(ws)
	s["aiPolish"] = true
	putSettings(t, env, s)

	content, keys, err := brief.Generate(m, context.Background(), fakeRegistry(false), at(27, 8, 0))
	if err != nil {
		t.Fatal(err)
	}
	want := `# 早报 · 10月27日 周二

## 总结

今天有一个会，两个 Issue 要处理。

## 天气

上海，多云，现在 18.2°C。今天 14 到 22.1°C，降水概率 60%。出门记得带伞。

## 今天的日程

- 全天 出差
- 09:00–10:00 周会（3 楼）

## 到期的 Issue

- XC-1 修登录（逾期 2 天）
- XC-2 写文档（今天到期）

## 今天的提醒

- 14:00 交房租

## 服务器告警

- 10-26 23:30 web-1 CPU 超过 90%（已恢复）

## 习惯

- 喝水：今天目标 8 杯，已完成 2，已连续 3 天

## 续费

- 10-30 域名 example.com 68 CNY
`
	if content != want {
		t.Fatalf("content:\n%s\nwant:\n%s", content, want)
	}
	if strings.Join(keys, ",") != "summary,weather,calendar,issues,reminders,alerts,habits,renewals" {
		t.Fatalf("keys: %v", keys)
	}

	// A failing provider only drops its own section; disabled sections and
	// the AI summary switch are respected.
	s["aiPolish"] = false
	s["sections"] = []string{"calendar", "alerts", "habits"}
	putSettings(t, env, s)
	_, keys, _ = brief.Generate(m, context.Background(), fakeRegistry(true), at(27, 8, 0))
	if strings.Join(keys, ",") != "calendar,habits" {
		t.Fatalf("keys with failing hosts: %v", keys)
	}
}

type fakeChannel struct {
	mu   sync.Mutex
	got  []notify.Stored
	name string
}

func (c *fakeChannel) Name() string { return c.name }
func (c *fakeChannel) Send(_ context.Context, n notify.Stored) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, n)
	return nil
}
func (c *fakeChannel) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.got)
}

func TestScheduledBriefIsSentOncePerDay(t *testing.T) {
	env, m, ws := setup(t)
	ch := &fakeChannel{name: "fake"}
	env.App.Deps.Notify.RegisterChannel(ch)
	s := baseSettings(ws)
	s["channels"] = []string{"fake"}
	view := putSettings(t, env, s)
	if len(view.Channels) != 1 || view.NextRunAt == nil {
		t.Fatalf("settings: %+v", view)
	}
	ctx := context.Background()

	for _, now := range []time.Time{at(27, 7, 59), at(27, 8, 0), at(27, 8, 1), at(27, 9, 30)} {
		if err := brief.Tick(m, ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	var notes struct {
		Items []struct{ Kind, Title, Body, Link, Priority string }
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) != 1 || notes.Items[0].Kind != "brief.daily" || notes.Items[0].Title != "早报 · 10月27日 周二" ||
		notes.Items[0].Link != "/calendar/briefs?date=2026-10-27" || !strings.Contains(notes.Items[0].Body, "【天气】") {
		t.Fatalf("notifications: %+v", notes.Items)
	}
	if ch.count() != 1 {
		t.Fatalf("fake channel got %d", ch.count())
	}

	// Next day: too late after the window, then on time.
	_ = brief.Tick(m, ctx, at(28, 11, 30))
	_ = brief.Tick(m, ctx, at(29, 8, 5))
	if ch.count() != 2 {
		t.Fatalf("fake channel got %d", ch.count())
	}

	var list struct {
		Items      []api.Brief
		NextCursor *string
	}
	env.MustDo(http.MethodGet, "/briefs?limit=1", nil, &list)
	if len(list.Items) != 1 || list.Items[0].Date != "2026-10-29" || list.NextCursor == nil {
		t.Fatalf("list: %+v", list)
	}
	env.MustDo(http.MethodGet, "/briefs?limit=1&cursor="+*list.NextCursor, nil, &list)
	if len(list.Items) != 1 || list.Items[0].Date != "2026-10-27" || list.Items[0].SentAt == nil {
		t.Fatalf("page 2: %+v", list)
	}
	var one api.Brief
	env.MustDo(http.MethodGet, "/briefs/2026-10-27", nil, &one)
	if !strings.HasPrefix(one.Content, "# 早报 · 10月27日 周二") || len(one.Sections) == 0 || one.Sections[0].Key != "weather" {
		t.Fatalf("brief: %+v", one)
	}
	if status, _ := env.Do(http.MethodGet, "/briefs/2020-01-01", nil, nil); status != http.StatusNotFound {
		t.Fatalf("missing brief: %d", status)
	}
	if status, _ := env.Do(http.MethodGet, "/briefs/yesterday", nil, nil); status != http.StatusBadRequest {
		t.Fatalf("bad date: %d", status)
	}
}

func TestPreviewSendSettingsAndWeather(t *testing.T) {
	env, _, ws := setup(t)

	var view api.BriefSettingsView
	env.MustDo(http.MethodGet, "/briefs/settings", nil, &view)
	if !view.Enabled || view.Time != "08:00" || len(view.Sections) != 7 || view.Location != nil || !view.AiAvailable || len(view.Channels) != 0 {
		t.Fatalf("defaults: %+v", view)
	}
	if status, _ := env.Do(http.MethodGet, "/weather", nil, nil); status != http.StatusPreconditionFailed {
		t.Fatalf("weather without location: %d", status)
	}
	bad := []map[string]any{
		{"enabled": true, "time": "8点", "channels": []string{}, "sections": []string{}},
		{"enabled": true, "time": "08:00", "channels": []string{"pigeon"}, "sections": []string{}},
		{"enabled": true, "time": "08:00", "channels": []string{}, "sections": []string{"horoscope"}},
		{"enabled": true, "time": "08:00", "channels": []string{}, "sections": []string{"summary"}},
		{"enabled": true, "time": "08:00", "channels": []string{}, "sections": []string{}, "location": map[string]any{"lat": 91, "lon": 0}},
		{"enabled": true, "time": "08:00", "channels": []string{}, "sections": []string{}, "weatherApiBase": "ftp://x"},
	}
	for _, b := range bad {
		if status, raw := env.Do(http.MethodPut, "/briefs/settings", b, nil); status != http.StatusBadRequest {
			t.Fatalf("%v: %d %s", b, status, raw)
		}
	}
	putSettings(t, env, baseSettings(ws))

	var w api.Weather
	env.MustDo(http.MethodGet, "/weather", nil, &w)
	if w.Summary != "多云" || w.Temperature != 18.2 || w.High != 22.1 || w.Low != 14 || w.PrecipitationChance != 60 || w.Location == nil || *w.Location != "上海" {
		t.Fatalf("weather: %+v", w)
	}
	hits := ws.hits.Load()
	env.MustDo(http.MethodGet, "/weather", nil, &w)
	if ws.hits.Load() != hits {
		t.Fatal("weather not cached")
	}
	env.MustDo(http.MethodGet, "/weather?lat=39.9&lon=116.4", nil, &w)
	if w.Latitude != 39.9 || ws.hits.Load() != hits+1 {
		t.Fatalf("weather by coordinates: %+v", w)
	}
	if status, _ := env.Do(http.MethodGet, "/weather?lat=39.9", nil, nil); status != http.StatusBadRequest {
		t.Fatalf("lat only: %d", status)
	}

	// Preview is not stored; send stores and sends today's brief.
	var preview api.Brief
	env.MustDo(http.MethodPost, "/briefs/generate", nil, &preview)
	if preview.Id != nil || !strings.Contains(preview.Content, "## 天气") {
		t.Fatalf("preview: %+v", preview)
	}
	var list struct{ Items []api.Brief }
	env.MustDo(http.MethodGet, "/briefs", nil, &list)
	if len(list.Items) != 0 {
		t.Fatalf("preview stored: %+v", list)
	}
	var sent api.Brief
	env.MustDo(http.MethodPost, "/briefs/generate", map[string]any{"send": true}, &sent)
	if sent.Id == nil || sent.SentAt == nil {
		t.Fatalf("sent: %+v", sent)
	}
	env.MustDo(http.MethodGet, "/briefs/settings", nil, &view)
	if view.NextRunAt == nil || view.NextRunAt.In(shanghai).Hour() != 8 || !view.NextRunAt.After(time.Now()) {
		t.Fatalf("next run after sending today: %+v", view.NextRunAt)
	}

	out, err := env.App.Deps.Actions.Run(context.Background(), "brief.preview", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if res := out.(map[string]any); !strings.Contains(res["content"].(string), "早报") {
		t.Fatalf("action: %+v", res)
	}
	if _, err := env.App.Deps.Actions.Run(context.Background(), "brief.preview", json.RawMessage(`{"x":1}`)); err == nil {
		t.Fatal("unexpected input accepted")
	}
}
