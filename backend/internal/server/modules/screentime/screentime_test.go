package screentime_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/screentime"
	"github.com/j0x3n/x-console/backend/internal/server/modules/screentime/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// clockDay is the fixed "now" of the tests: Saturday 2026-10-10 09:00 in Asia/Shanghai.
var clockDay = time.Date(2026, 10, 10, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))

type rig struct {
	env    *testutil.Env
	m      *screentime.Module
	client *conn.Client
	host   string
}

func setup(t *testing.T) *rig {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*screentime.Module](env.App.Deps.Registry, screentime.ServiceKey)
	if !ok {
		t.Fatal("screentime module not registered")
	}
	screentime.SetNow(m, func() time.Time { return clockDay })
	r := &rig{env: env, m: m}
	r.host = env.Agent("desktop", []string{protocol.CapSystemInfo, protocol.CapScreenTime}, func(c *conn.Client) { r.client = c })
	return r
}

// minute is the Unix minute at hour:min on the test day plus dayOffset days.
func minute(dayOffset, hour, min int) int64 {
	return time.Date(2026, 10, 10+dayOffset, hour, min, 0, 0, clockDay.Location()).Unix() / 60
}

// send emits one sample from the agent and waits until the server has stored
// it (or, when wantStored is false, long enough to be sure it was not).
func (r *rig) send(t *testing.T, min int64, app, title string) {
	t.Helper()
	if err := r.client.Emit(context.Background(), protocol.EventScreenSample, protocol.ScreenSample{Minute: min, App: app, Title: title}); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) count(t *testing.T) int {
	t.Helper()
	var n int
	if err := r.env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM screen_minutes`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *rig) waitCount(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for r.count(t) != want {
		if time.Now().After(deadline) {
			t.Fatalf("stored minutes: got %d, want %d", r.count(t), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// settle gives a sample that should be dropped time to arrive.
func settle() { time.Sleep(150 * time.Millisecond) }

func (r *rig) summary(t *testing.T, query string) api.ScreenTimeSummary {
	t.Helper()
	var s api.ScreenTimeSummary
	r.env.MustDo(http.MethodGet, "/screentime/summary"+query, nil, &s)
	return s
}

func cats(s api.ScreenTimeSummary) map[string]int {
	out := map[string]int{}
	for _, c := range s.Categories {
		out[string(c.Category)] = c.Minutes
	}
	return out
}

func (r *rig) setSettings(t *testing.T, enabled, keepTitles bool, disabled []string) {
	t.Helper()
	if disabled == nil {
		disabled = []string{}
	}
	r.env.MustDo(http.MethodPut, "/screentime/settings", map[string]any{"enabled": enabled, "keepTitles": keepTitles, "disabledHosts": disabled}, nil)
}

func (r *rig) addRule(t *testing.T, field, pattern, category string) api.ScreenTimeRule {
	t.Helper()
	var rule api.ScreenTimeRule
	r.env.MustDo(http.MethodPost, "/screentime/rules", map[string]any{"field": field, "pattern": pattern, "category": category}, &rule)
	return rule
}

func (r *rig) insert(t *testing.T, min int64, app, category string) {
	t.Helper()
	if _, err := r.env.App.Deps.DB.Exec(`INSERT INTO screen_minutes (host_id, minute, app, category, title) VALUES (?, ?, ?, ?, '')`, r.host, min, app, category); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) titles(t *testing.T) []string {
	t.Helper()
	rows, err := r.env.App.Deps.DB.Query(`SELECT title FROM screen_minutes WHERE title <> '' ORDER BY minute`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		out = append(out, s)
	}
	return out
}

func TestSamplesAreClassifiedAndTitlesAreNotKept(t *testing.T) {
	r := setup(t)
	r.send(t, minute(0, 8, 0), "Code.exe", "main.go - x-console")
	r.send(t, minute(0, 8, 1), "Code.exe", "module.go - x-console")
	r.send(t, minute(0, 8, 2), "chrome.exe", "Lo-fi mix - YouTube")
	r.send(t, minute(0, 8, 3), "chrome.exe", "Claude")
	r.send(t, minute(0, 8, 4), "msedge.exe", "某个新闻站")
	r.send(t, minute(0, 8, 5), "WeChat.exe", "微信")
	r.send(t, minute(0, 8, 6), "explorer.exe", "")
	r.waitCount(t, 7)

	if titles := r.titles(t); len(titles) != 0 {
		t.Fatalf("titles must not be stored by default: %v", titles)
	}
	s := r.summary(t, "?range=day")
	if s.Minutes != 7 || s.State != api.Ok {
		t.Fatalf("summary: %+v", s)
	}
	want := map[string]int{"coding": 2, "entertainment": 1, "ai": 1, "web": 1, "chat": 1, "other": 1}
	got := cats(s)
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("category %s: got %d, want %d (%v)", k, got[k], v, got)
		}
	}
	if len(s.Days) != 0 {
		t.Fatalf("day view has no day list: %+v", s.Days)
	}
	if len(s.Apps) != 5 || s.Apps[0].Minutes != 2 || s.Apps[1].Minutes != 2 || s.Apps[2].Minutes != 1 {
		t.Fatalf("apps must be sorted by minutes: %+v", s.Apps)
	}
	for _, a := range s.Apps {
		if a.App == "Code.exe" && (a.Minutes != 2 || a.Category != api.Coding) {
			t.Fatalf("Code.exe: %+v", a)
		}
	}
}

func TestSamePlaceSameMinuteOverwrites(t *testing.T) {
	r := setup(t)
	r.send(t, minute(0, 8, 0), "Code.exe", "")
	r.waitCount(t, 1)
	r.send(t, minute(0, 8, 0), "Steam.exe", "")
	deadline := time.Now().Add(5 * time.Second)
	for cats(r.summary(t, ""))["entertainment"] != 1 {
		if time.Now().After(deadline) {
			t.Fatal("second report did not replace the first")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r.count(t) != 1 {
		t.Fatalf("count: %d", r.count(t))
	}
}

func TestOldAndFutureSamplesAreDropped(t *testing.T) {
	r := setup(t)
	r.send(t, minute(-2, 8, 0), "Code.exe", "") // two days ago
	r.send(t, minute(0, 9, 30), "Code.exe", "") // half an hour ahead
	r.send(t, minute(0, 8, 0), "", "no program")
	r.send(t, minute(0, 8, 1), "Code.exe", "") // the only good one
	r.waitCount(t, 1)
	settle()
	if r.count(t) != 1 {
		t.Fatalf("count: %d", r.count(t))
	}
}

func TestTitlesKeptOnlyWhenAsked(t *testing.T) {
	r := setup(t)
	r.setSettings(t, true, true, nil)
	r.send(t, minute(0, 8, 0), "chrome.exe", "Lo-fi mix - YouTube")
	r.waitCount(t, 1)
	if titles := r.titles(t); len(titles) != 1 || titles[0] != "Lo-fi mix - YouTube" {
		t.Fatalf("kept titles: %v", titles)
	}
	// 关掉以后已存的标题马上清掉，类别还在
	r.setSettings(t, true, false, nil)
	if titles := r.titles(t); len(titles) != 0 {
		t.Fatalf("titles after switching off: %v", titles)
	}
	if got := cats(r.summary(t, "")); got["entertainment"] != 1 {
		t.Fatalf("category must stay: %v", got)
	}
}

func TestSwitchesStopRecording(t *testing.T) {
	r := setup(t)
	r.setSettings(t, false, false, nil)
	r.send(t, minute(0, 8, 0), "Code.exe", "")
	settle()
	if r.count(t) != 0 {
		t.Fatal("master switch off must not record")
	}
	if s := r.summary(t, ""); s.State != api.Disabled {
		t.Fatalf("state: %s", s.State)
	}

	r.setSettings(t, true, false, []string{r.host})
	r.send(t, minute(0, 8, 1), "Code.exe", "")
	settle()
	if r.count(t) != 0 {
		t.Fatal("a computer switched off must not record")
	}
	var set api.ScreenTimeSettings
	r.env.MustDo(http.MethodGet, "/screentime/settings", nil, &set)
	if len(set.Hosts) != 1 || set.Hosts[0].Id != r.host || set.Hosts[0].Enabled || !set.Hosts[0].Online || !set.Hosts[0].Supported {
		t.Fatalf("settings: %+v", set)
	}

	r.setSettings(t, true, false, nil)
	r.send(t, minute(0, 8, 2), "Code.exe", "")
	r.waitCount(t, 1)
}

func TestStateWithoutDataExplainsWhy(t *testing.T) {
	r := setup(t)
	if s := r.summary(t, ""); s.State != api.Waiting || s.Minutes != 0 {
		t.Fatalf("agent online, no data: %+v", s)
	}
	// 没有能记录的代理
	env := testutil.New(t)
	var s api.ScreenTimeSummary
	env.MustDo(http.MethodGet, "/screentime/summary", nil, &s)
	if s.State != api.NoAgent {
		t.Fatalf("state: %s", s.State)
	}
}

func TestOwnRulesWinAndReclassifyOldMinutes(t *testing.T) {
	r := setup(t)
	r.send(t, minute(0, 8, 0), "Foo.exe", "")
	r.send(t, minute(0, 8, 1), "Code.exe", "")
	r.waitCount(t, 2)
	if got := cats(r.summary(t, "")); got["other"] != 1 || got["coding"] != 1 {
		t.Fatalf("before rule: %v", got)
	}

	foo := r.addRule(t, "app", "foo", "office") // 不写 .exe、不分大小写
	if got := cats(r.summary(t, "")); got["office"] != 1 || got["other"] != 0 {
		t.Fatalf("after rule: %v", got)
	}
	// 自己的规则排在内置规则前面
	r.addRule(t, "app", "Code.exe", "entertainment")
	if got := cats(r.summary(t, "")); got["entertainment"] != 1 || got["coding"] != 0 {
		t.Fatalf("own rule beats built-in: %v", got)
	}

	// 删掉规则，回到内置规则
	r.env.MustDo(http.MethodDelete, "/screentime/rules/"+strconv.FormatInt(foo.Id, 10), nil, nil)
	if got := cats(r.summary(t, "")); got["other"] != 1 || got["office"] != 0 {
		t.Fatalf("after delete: %v", got)
	}
	var list struct{ Items []api.ScreenTimeRule }
	r.env.MustDo(http.MethodGet, "/screentime/rules", nil, &list)
	if len(list.Items) != 1 || list.Items[0].Pattern != "Code.exe" {
		t.Fatalf("rules: %+v", list.Items)
	}
	if status, _ := r.env.Do(http.MethodDelete, "/screentime/rules/9999", nil, nil); status != 404 {
		t.Fatalf("delete missing: %d", status)
	}
}

func TestTitleRuleOnlyTouchesMinutesWithATitle(t *testing.T) {
	r := setup(t)
	// 先不存标题：这一分钟的类别是按标题定的，标题丢掉后不能被后来的规则改回去
	r.send(t, minute(0, 8, 0), "chrome.exe", "Lo-fi mix - YouTube")
	r.waitCount(t, 1)
	r.setSettings(t, true, true, nil)
	r.send(t, minute(0, 8, 1), "chrome.exe", "Quarterly report draft")
	r.waitCount(t, 2)
	if got := cats(r.summary(t, "")); got["entertainment"] != 1 || got["web"] != 1 {
		t.Fatalf("before rule: %v", got)
	}

	r.addRule(t, "title", "quarterly report", "office")
	got := cats(r.summary(t, ""))
	if got["office"] != 1 || got["entertainment"] != 1 || got["web"] != 0 {
		t.Fatalf("title rule: %v", got)
	}
	// 新来的记录也按这条规则分，哪怕标题不存
	r.setSettings(t, true, false, nil)
	r.send(t, minute(0, 8, 2), "chrome.exe", "Quarterly report v2")
	r.waitCount(t, 3)
	if got := cats(r.summary(t, "")); got["office"] != 2 {
		t.Fatalf("new minute: %v", got)
	}
}

func TestRuleValidation(t *testing.T) {
	r := setup(t)
	for _, body := range []map[string]any{
		{"field": "path", "pattern": "x", "category": "coding"},
		{"field": "app", "pattern": "  ", "category": "coding"},
		{"field": "app", "pattern": strings.Repeat("a", 101), "category": "coding"},
		{"field": "app", "pattern": "x", "category": "games"},
	} {
		if status, _ := r.env.Do(http.MethodPost, "/screentime/rules", body, nil); status != 400 {
			t.Fatalf("%v: status %d", body, status)
		}
	}
}

func TestWeekAndMonthSummaries(t *testing.T) {
	r := setup(t)
	// 样本只收一天以内的，更早的直接写进表里
	r.insert(t, minute(-5, 10, 0), "Code.exe", "coding")  // Mon 10-05
	r.insert(t, minute(-5, 10, 1), "Code.exe", "coding")  // Mon 10-05
	r.insert(t, minute(-9, 23, 59), "Code.exe", "coding") // Thu 10-01, in the month only
	r.insert(t, minute(-6, 23, 59), "Code.exe", "coding") // Sun 10-04 23:59, in last week
	r.send(t, minute(0, 8, 0), "Steam.exe", "")           // Sat 10-10
	r.waitCount(t, 5)

	week := r.summary(t, "?range=week")
	if week.From != "2026-10-05" || week.To != "2026-10-11" || len(week.Days) != 7 || week.Minutes != 3 {
		t.Fatalf("week: from %s to %s days %d minutes %d", week.From, week.To, len(week.Days), week.Minutes)
	}
	if week.Days[0].Date != "2026-10-05" || week.Days[0].Minutes != 2 || week.Days[0].Categories["coding"] != 2 {
		t.Fatalf("monday: %+v", week.Days[0])
	}
	if week.Days[5].Categories["entertainment"] != 1 || week.Days[6].Minutes != 0 {
		t.Fatalf("saturday/sunday: %+v %+v", week.Days[5], week.Days[6])
	}

	month := r.summary(t, "?range=month&date=2026-10-20")
	if month.From != "2026-10-01" || month.To != "2026-10-31" || len(month.Days) != 31 || month.Minutes != 5 {
		t.Fatalf("month: %s..%s days %d minutes %d", month.From, month.To, len(month.Days), month.Minutes)
	}

	// 上周日 23:59 属于上一周
	prev := r.summary(t, "?range=week&date=2026-10-04")
	if prev.From != "2026-09-28" || prev.Minutes != 2 || prev.Days[6].Minutes != 1 {
		t.Fatalf("previous week: %+v", prev)
	}

	if status, _ := r.env.Do(http.MethodGet, "/screentime/summary?range=year", nil, nil); status != 400 {
		t.Fatalf("bad range: %d", status)
	}
	if status, _ := r.env.Do(http.MethodGet, "/screentime/summary?date=tomorrow", nil, nil); status != 400 {
		t.Fatalf("bad date: %d", status)
	}
}

func TestSummaryCanLimitToOneComputer(t *testing.T) {
	r := setup(t)
	r.send(t, minute(0, 8, 0), "Code.exe", "")
	r.waitCount(t, 1)
	if s := r.summary(t, "?hostId="+r.host); s.Minutes != 1 {
		t.Fatalf("own host: %+v", s)
	}
	if s := r.summary(t, "?hostId=nope"); s.Minutes != 0 {
		t.Fatalf("other host: %+v", s)
	}
}

func TestCleanupKeepsRecordsLongerThanTitles(t *testing.T) {
	r := setup(t)
	r.setSettings(t, true, true, nil)
	r.send(t, minute(0, 8, 0), "chrome.exe", "today")
	r.waitCount(t, 1)
	db := r.env.App.Deps.DB
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO screen_minutes (host_id, minute, app, category, title) VALUES ('h', ?, 'chrome.exe', 'web', 'old title')`, minute(-40, 8, 0))
	exec(`INSERT INTO screen_minutes (host_id, minute, app, category, title) VALUES ('h', ?, 'chrome.exe', 'web', 'ancient')`, minute(-401, 8, 0))
	if err := screentime.Cleanup(r.m, context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.count(t) != 2 {
		t.Fatalf("the 401 day old minute must go: %d", r.count(t))
	}
	if titles := r.titles(t); len(titles) != 1 || titles[0] != "today" {
		t.Fatalf("titles older than 30 days must go: %v", titles)
	}
}

func TestClearNeedsElevation(t *testing.T) {
	env := testutil.New(t)
	if status, _ := env.Do(http.MethodDelete, "/screentime/data", nil, nil); status != 403 {
		t.Fatalf("without elevation: %d", status)
	}
	if _, err := env.App.Deps.DB.Exec(`INSERT INTO screen_minutes (host_id, minute, app, category, title) VALUES ('h', 1, 'Code.exe', 'coding', '')`); err != nil {
		t.Fatal(err)
	}
	env.Elevate()
	if status, _ := env.Do(http.MethodDelete, "/screentime/data", nil, nil); status != 204 {
		t.Fatalf("clear: %d", status)
	}
	var n int
	if err := env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM screen_minutes`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("records left: %d %v", n, err)
	}
}

func TestHiddenModuleGate(t *testing.T) {
	r := setup(t)
	env := r.env
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, nil)
	env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"screentime"}}, nil)
	if status, _ := env.Do(http.MethodGet, "/screentime/summary", nil, nil); status != 200 {
		t.Fatalf("unlocked: %d", status)
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	for _, path := range []string{"/screentime/summary", "/screentime/settings", "/screentime/rules"} {
		if status, _ := env.Do(http.MethodGet, path, nil, nil); status != 404 {
			t.Fatalf("locked %s: %d", path, status)
		}
	}
	// 上报不受影响
	r.send(t, minute(0, 8, 0), "Code.exe", "")
	r.waitCount(t, 1)
	var avail struct{ Modules []string }
	env.MustDo(http.MethodGet, "/app/modules", nil, &avail)
	for _, id := range avail.Modules {
		if id == "screentime" {
			t.Fatalf("screentime still available: %v", avail.Modules)
		}
	}
}

func TestAIAction(t *testing.T) {
	r := setup(t)
	r.send(t, minute(0, 8, 0), "Code.exe", "secret title")
	r.waitCount(t, 1)
	out, err := r.env.App.Deps.Actions.Run(context.Background(), "screentime.summary", json.RawMessage(`{"range":"day"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), "coding") || !strings.Contains(string(raw), "Code.exe") || strings.Contains(string(raw), "secret title") {
		t.Fatalf("action output: %s", raw)
	}
}
