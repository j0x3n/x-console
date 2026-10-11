package goals_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

var cst = time.FixedZone("CST", 8*3600)

// wed is a Wednesday inside the fourth quarter of 2026. The quarter has 92
// days and 41.5 of them have passed.
var wed = time.Date(2026, 11, 11, 12, 0, 0, 0, cst)

// fakeSource stands in for the modules that measure data. A kind with a per
// day rate gives rate * days between from and until, so a week adds 7 * rate.
type fakeSource struct {
	mu      sync.Mutex
	rate    map[string]float64 // "kind:ref"
	missing map[string]bool    // kind
	broken  map[string]bool    // kind
	calls   int
}

var allKinds = []contracts.GoalMetricKind{
	{Kind: contracts.GoalMetricHabitCheckins, Module: "habits", RefKind: contracts.GoalRefHabit},
	{Kind: contracts.GoalMetricWorkouts, Module: "habits", RefKind: contracts.GoalRefNone},
	{Kind: contracts.GoalMetricFocusMinutes, Module: "calendar", RefKind: contracts.GoalRefNone},
	{Kind: contracts.GoalMetricScreenMinutes, Module: "screentime", RefKind: contracts.GoalRefCategory},
	{Kind: contracts.GoalMetricGitHubCommits, Module: "github", RefKind: contracts.GoalRefNone},
	{Kind: contracts.GoalMetricGitHubMergedPRs, Module: "github", RefKind: contracts.GoalRefNone},
}

func (f *fakeSource) Kinds() []contracts.GoalMetricKind { return allKinds }

func (f *fakeSource) Options(_ context.Context, kind string) ([]contracts.GoalMetricOption, error) {
	switch kind {
	case contracts.GoalMetricHabitCheckins:
		return []contracts.GoalMetricOption{{Value: "1", Label: "读书"}, {Value: "2", Label: "跑步"}}, nil
	case contracts.GoalMetricScreenMinutes:
		return []contracts.GoalMetricOption{{Value: "", Label: "全部分类"}, {Value: "coding", Label: "编码"}}, nil
	}
	return nil, nil
}

func (f *fakeSource) Measure(_ context.Context, kind, ref string, from, until time.Time) (float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.missing[kind] {
		return 0, contracts.ErrGoalMetricMissing
	}
	if f.broken[kind] {
		return 0, errors.New("boom")
	}
	return f.rate[kind+":"+ref] * until.Sub(from).Hours() / 24, nil
}

type noSource struct{}

func (noSource) Kinds() []contracts.GoalMetricKind { return nil }
func (noSource) Options(context.Context, string) ([]contracts.GoalMetricOption, error) {
	return nil, nil
}
func (noSource) Measure(context.Context, string, string, time.Time, time.Time) (float64, error) {
	return 0, errors.New("none")
}

type fakeHidden map[string]bool

// Hidden follows the real vault: work marked with IgnoreHidden sees everything.
func (h fakeHidden) Hidden(ctx context.Context, name string) bool {
	return h[name] && !contracts.HidingIgnored(ctx)
}

type fixture struct {
	env  *testutil.Env
	m    *goals.Module
	src  *fakeSource
	now  time.Time
	nowM sync.Mutex
}

func (f *fixture) setNow(t time.Time) {
	f.nowM.Lock()
	f.now = t
	f.nowM.Unlock()
}

// setup starts the server with the data modules replaced by one fake, and the
// clock set to clock.
func setup(t *testing.T, clock time.Time) *fixture {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*goals.Module](env.App.Deps.Registry, goals.ServiceKey)
	if !ok {
		t.Fatal("goals module not registered")
	}
	f := &fixture{env: env, m: m, now: clock, src: &fakeSource{
		rate:    map[string]float64{"habit_checkins:1": 1, "workouts:": 0.5, "focus_minutes:": 60, "screen_minutes:coding": 120, "screen_minutes:": 300, "github_commits:": 2, "github_merged_prs:": 0.25},
		missing: map[string]bool{}, broken: map[string]bool{},
	}}
	reg := env.App.Deps.Registry
	module.Provide[contracts.GoalMetricSource](reg, contracts.GoalMetricPrefix+"habits", f.src)
	for _, name := range []string{"focus", "screentime", "github"} {
		module.Provide[contracts.GoalMetricSource](reg, contracts.GoalMetricPrefix+name, noSource{})
	}
	goals.SetNow(m, func() time.Time {
		f.nowM.Lock()
		defer f.nowM.Unlock()
		return f.now
	})
	return f
}

func (f *fixture) hide(modules ...string) {
	h := fakeHidden{}
	for _, name := range modules {
		h[name] = true
	}
	module.Provide[contracts.HiddenModules](f.env.App.Deps.Registry, contracts.HiddenModulesKey, h)
}

func (f *fixture) create(body map[string]any) api.Goal {
	f.env.T.Helper()
	var g api.Goal
	f.env.MustDo(http.MethodPost, "/goals", body, &g)
	return g
}

func (f *fixture) expect(status int, method, path string, body any) string {
	f.env.T.Helper()
	got, raw := f.env.Do(method, path, body, nil)
	if got != status {
		f.env.T.Fatalf("%s %s: status %d, want %d: %s", method, path, got, status, raw)
	}
	return string(raw)
}

func manual(name string, start, target float64) map[string]any {
	return map[string]any{"name": name, "source": "manual", "startValue": start, "target": target, "unit": "本"}
}

func auto(name, source, ref string, target float64) map[string]any {
	return map[string]any{"name": name, "source": source, "ref": ref, "target": target, "unit": "次"}
}

func q4(title string, results ...map[string]any) map[string]any {
	return map[string]any{"title": title, "year": 2026, "quarter": 4, "results": results}
}

func near(t *testing.T, label string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: nil, want %v", label, want)
	}
	if d := *got - want; d > 0.06 || d < -0.06 {
		t.Fatalf("%s: %v, want %v", label, *got, want)
	}
}

func resultByName(g api.Goal, name string) api.GoalResult {
	for _, r := range g.Results {
		if r.Name == name {
			return r
		}
	}
	panic("no result " + name)
}

// ---------- pure functions ----------

func TestResultPercent(t *testing.T) {
	cases := []struct{ value, start, target, want float64 }{
		{5, 0, 10, 50},
		{0, 0, 10, 0},
		{10, 0, 10, 100},
		{25, 0, 10, 100}, // beyond the target counts as done
		{-3, 0, 10, 0},   // below the start counts as nothing
		{77.5, 80, 75, 50},
		{75, 80, 75, 100},
		{82, 80, 75, 0}, // got worse than the start
		{3, 3, 3, 100},  // no distance to go
		{2, 3, 3, 0},
	}
	for _, c := range cases {
		if got := goals.ResultPercent(c.value, c.start, c.target); got != c.want {
			t.Errorf("ResultPercent(%v,%v,%v) = %v, want %v", c.value, c.start, c.target, got, c.want)
		}
	}
}

func TestQuarterBoundsAndExpected(t *testing.T) {
	for _, c := range []struct {
		year, q  int
		from, to string
	}{
		{2026, 1, "2026-01-01", "2026-04-01"},
		{2026, 2, "2026-04-01", "2026-07-01"},
		{2026, 3, "2026-07-01", "2026-10-01"},
		{2026, 4, "2026-10-01", "2027-01-01"},
	} {
		from, to := goals.QuarterBounds(c.year, c.q, cst)
		if from.Format("2006-01-02") != c.from || to.Format("2006-01-02") != c.to {
			t.Errorf("Q%d: %s to %s", c.q, from.Format("2006-01-02"), to.Format("2006-01-02"))
		}
	}
	from, to := goals.QuarterBounds(2026, 4, cst)
	if got := goals.ExpectedPercent(from, to, from.Add(-time.Hour)); got != 0 {
		t.Errorf("before the quarter: %v", got)
	}
	if got := goals.ExpectedPercent(from, to, from); got != 0 {
		t.Errorf("first moment: %v", got)
	}
	if got := goals.ExpectedPercent(from, to, from.AddDate(0, 0, 46)); got != 50 {
		t.Errorf("half way: %v", got)
	}
	if got := goals.ExpectedPercent(from, to, to.Add(-time.Minute)); got < 99.9 || got >= 100 {
		t.Errorf("last minute: %v", got)
	}
	if got := goals.ExpectedPercent(from, to, to); got != 100 {
		t.Errorf("after the quarter: %v", got)
	}
}

func TestStateOf(t *testing.T) {
	from, to := goals.QuarterBounds(2026, 4, cst)
	mid := from.AddDate(0, 0, 40)
	p := func(v float64) *float64 { return &v }
	for _, c := range []struct {
		name     string
		progress *float64
		expected float64
		now      time.Time
		want     api.GoalState
	}{
		{"done", p(100), 40, mid, api.GoalStateDone},
		{"done also after the quarter", p(100), 100, to.AddDate(0, 0, 3), api.GoalStateDone},
		{"on track", p(45), 43, mid, api.GoalStateOnTrack},
		{"exactly at the margin is still on track", p(30), 40, mid, api.GoalStateOnTrack},
		{"just below the margin is behind", p(29.9), 40, mid, api.GoalStateBehind},
		{"nothing measured is not behind", nil, 40, mid, api.GoalStateOnTrack},
		{"not started", p(0), 0, from.Add(-24 * time.Hour), api.GoalStateUpcoming},
		{"ended", p(60), 100, to, api.GoalStateMissed},
	} {
		if got := goals.StateOf(c.progress, c.expected, from, to, c.now); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestWeekStartIsMonday(t *testing.T) {
	for day, want := range map[int]string{9: "2026-11-09", 10: "2026-11-09", 15: "2026-11-09", 16: "2026-11-16"} {
		got := goals.WeekStart(time.Date(2026, 11, day, 23, 59, 0, 0, cst), cst)
		if got.Format("2006-01-02") != want || got.Hour() != 0 {
			t.Errorf("11-%d: %v, want %s", day, got, want)
		}
	}
}

// ---------- progress ----------

func TestProgressMixOfSources(t *testing.T) {
	f := setup(t, wed)
	g := f.create(q4("读书和锻炼",
		manual("读完几本书", 0, 4),
		auto("读书打卡", "habit_checkins", "1", 80), // 41.5 days at 1 a day
		auto("专注", "focus_minutes", "", 6000),   // 41.5 * 60 = 2490
	))
	if g.Year != 2026 || g.Quarter != 4 || g.From != "2026-10-01" || g.Until != "2026-12-31" || g.Status != api.GoalStatusActive {
		t.Fatalf("goal: %+v", g)
	}
	near(t, "expected", &g.Expected, 45.1)
	read := resultByName(g, "读完几本书")
	near(t, "manual progress", read.Progress, 0)
	habit := resultByName(g, "读书打卡")
	near(t, "habit value", habit.Value, 41.5)
	near(t, "habit progress", habit.Progress, 51.9)
	if habit.RefLabel != "读书" {
		t.Fatalf("ref label: %q", habit.RefLabel)
	}
	focus := resultByName(g, "专注")
	near(t, "focus progress", focus.Progress, 41.5)
	near(t, "goal progress is the average", g.Progress, (0+51.9+41.5)/3)
	if g.State != api.GoalStateBehind {
		t.Fatalf("state: %s", g.State)
	}

	// the manual result catches up
	var res api.GoalResult
	f.env.MustDo(http.MethodPost, "/goals/results/"+strconv.FormatInt(read.Id, 10)+"/value", map[string]any{"value": 3}, &res)
	near(t, "manual after update", res.Progress, 75)
	var again api.Goal
	f.env.MustDo(http.MethodGet, "/goals/"+strconv.FormatInt(g.Id, 10), nil, &again)
	near(t, "average after update", again.Progress, (75+51.9+41.5)/3)
	if again.State != api.GoalStateOnTrack {
		t.Fatalf("state: %s", again.State)
	}
}

func TestEverySourceKind(t *testing.T) {
	f := setup(t, wed)
	g := f.create(q4("全部来源",
		auto("a", "habit_checkins", "2", 100),
		auto("b", "workouts", "", 100),
		auto("c", "focus_minutes", "", 10000),
		auto("d", "screen_minutes", "coding", 10000),
		auto("e", "screen_minutes", "", 20000),
		auto("f", "github_commits", "", 200),
		auto("g", "github_merged_prs", "", 20),
	))
	// rate * 41.5 days
	want := map[string]float64{"a": 0, "b": 20.75, "c": 2490, "d": 4980, "e": 12450, "f": 83, "g": 10.375}
	for name, v := range want {
		r := resultByName(g, name)
		if r.Value == nil {
			t.Fatalf("%s: no value", name)
		}
		near(t, name, r.Value, v)
	}
	if got := resultByName(g, "e").RefLabel; got != "全部分类" {
		t.Fatalf("label for all categories: %q", got)
	}
}

func TestProgressEdgeCases(t *testing.T) {
	f := setup(t, wed)
	g := f.create(q4("边界",
		manual("减重", 80, 75),
		manual("已超过", 0, 10),
		auto("超额", "habit_checkins", "1", 10),
	))
	over := resultByName(g, "超额")
	near(t, "value keeps the real number", over.Value, 41.5)
	near(t, "progress stops at 100", over.Progress, 100)
	if over.State != api.GoalStateDone {
		t.Fatalf("state: %s", over.State)
	}
	var res api.GoalResult
	f.env.MustDo(http.MethodPost, "/goals/results/"+strconv.FormatInt(resultByName(g, "减重").Id, 10)+"/value", map[string]any{"value": 77.5}, &res)
	near(t, "weight loss", res.Progress, 50)
	f.env.MustDo(http.MethodPost, "/goals/results/"+strconv.FormatInt(resultByName(g, "已超过").Id, 10)+"/value", map[string]any{"value": 25}, &res)
	near(t, "beyond the target", res.Progress, 100)
	near(t, "value is kept", res.Value, 25)
}

func TestQuarterTiming(t *testing.T) {
	f := setup(t, wed)
	old := f.create(map[string]any{"title": "上季度", "year": 2026, "quarter": 3, "results": []map[string]any{auto("x", "habit_checkins", "1", 200)}})
	next := f.create(map[string]any{"title": "明年", "year": 2027, "quarter": 1, "results": []map[string]any{auto("x", "habit_checkins", "1", 200)}})
	if old.State != api.GoalStateMissed || old.Expected != 100 {
		t.Fatalf("old quarter: %s %v", old.State, old.Expected)
	}
	// the auto number stops at the end of the quarter: 92 days
	near(t, "old quarter value", old.Results[0].Value, 92)
	if next.State != api.GoalStateUpcoming || next.Expected != 0 {
		t.Fatalf("future quarter: %s %v", next.State, next.Expected)
	}
	near(t, "future quarter value", next.Results[0].Value, 0)

	// first and last day of the quarter
	f.setNow(time.Date(2026, 10, 1, 0, 0, 0, 0, cst))
	var g api.Goal
	f.env.MustDo(http.MethodGet, "/goals/"+strconv.FormatInt(f.create(q4("季初")).Id, 10), nil, &g)
	if g.Expected != 0 || g.State != api.GoalStateOnTrack {
		t.Fatalf("first day: %v %s", g.Expected, g.State)
	}
	f.setNow(time.Date(2026, 12, 31, 23, 0, 0, 0, cst))
	f.env.MustDo(http.MethodGet, "/goals/"+strconv.FormatInt(g.Id, 10), nil, &g)
	if g.Expected < 99 || g.State != api.GoalStateOnTrack {
		t.Fatalf("last day: %v %s", g.Expected, g.State)
	}
	f.setNow(time.Date(2027, 1, 1, 0, 0, 0, 0, cst))
	f.env.MustDo(http.MethodGet, "/goals/"+strconv.FormatInt(g.Id, 10), nil, &g)
	if g.Expected != 100 || g.State != api.GoalStateMissed {
		t.Fatalf("after: %v %s", g.Expected, g.State)
	}
}

func TestUnavailableSourcesLeaveTheAverage(t *testing.T) {
	f := setup(t, wed)
	g := f.create(q4("有的算不出",
		manual("手动", 0, 10),
		auto("打卡", "habit_checkins", "1", 83),
		auto("提交", "github_commits", "", 100),
		auto("专注", "focus_minutes", "", 4980),
	))
	only := f.create(q4("全隐藏", auto("a", "workouts", "", 10)))
	f.src.missing["github_commits"] = true
	f.src.broken["focus_minutes"] = true
	f.hide("habits")
	var got api.Goal
	f.env.MustDo(http.MethodGet, "/goals/"+strconv.FormatInt(g.Id, 10), nil, &got)
	for name, want := range map[string]string{"打卡": "hidden", "提交": "missing", "专注": "error", "手动": ""} {
		r := resultByName(got, name)
		if r.Unavailable != want {
			t.Errorf("%s: unavailable %q, want %q", name, r.Unavailable, want)
		}
		if want != "" && (r.Value != nil || r.Progress != nil) {
			t.Errorf("%s: has numbers while unavailable", name)
		}
	}
	near(t, "only the manual one counts", got.Progress, 0)

	// nothing can be measured at all
	f.env.MustDo(http.MethodGet, "/goals/"+strconv.FormatInt(only.Id, 10), nil, &only)
	if only.Progress != nil || only.State != api.GoalStateOnTrack {
		t.Fatalf("progress %v state %s", only.Progress, only.State)
	}
}

// ---------- goals and key results ----------

func TestCreateValidation(t *testing.T) {
	f := setup(t, wed)
	many := make([]map[string]any, 21)
	for i := range many {
		many[i] = manual("x", 0, 1)
	}
	for name, body := range map[string]map[string]any{
		"no title":          {"title": " ", "year": 2026, "quarter": 4},
		"long title":        {"title": strings.Repeat("长", 201), "year": 2026, "quarter": 4},
		"quarter 5":         {"title": "t", "year": 2026, "quarter": 5},
		"quarter 0":         {"title": "t", "year": 2026, "quarter": 0},
		"year":              {"title": "t", "year": 1999, "quarter": 1},
		"too many results":  {"title": "t", "year": 2026, "quarter": 4, "results": many},
		"no result name":    q4("t", map[string]any{"name": "", "source": "manual", "target": 1}),
		"unknown source":    q4("t", map[string]any{"name": "n", "source": "weight", "target": 1}),
		"manual start=goal": q4("t", manual("n", 5, 5)),
		"auto target 0":     q4("t", auto("n", "workouts", "", 0)),
		"auto negative":     q4("t", auto("n", "workouts", "", -1)),
		"habit no ref":      q4("t", auto("n", "habit_checkins", "", 5)),
		"habit bad ref":     q4("t", auto("n", "habit_checkins", "99", 5)),
		"category bad ref":  q4("t", auto("n", "screen_minutes", "dance", 5)),
		"ref on no-ref":     q4("t", auto("n", "workouts", "1", 5)),
		"ref on manual":     q4("t", map[string]any{"name": "n", "source": "manual", "ref": "1", "target": 5}),
	} {
		t.Run(name, func(t *testing.T) { f.expect(http.StatusBadRequest, http.MethodPost, "/goals", body) })
	}
	f.expect(http.StatusBadRequest, http.MethodGet, "/goals?year=2026", nil)
	f.expect(http.StatusBadRequest, http.MethodGet, "/goals?status=zzz", nil)
	f.expect(http.StatusNotFound, http.MethodGet, "/goals/999", nil)
	f.expect(http.StatusNotFound, http.MethodPut, "/goals/999", q4("t"))
	f.expect(http.StatusNotFound, http.MethodPatch, "/goals/999", map[string]any{"status": "done"})
	f.expect(http.StatusNotFound, http.MethodDelete, "/goals/999", nil)
	f.expect(http.StatusNotFound, http.MethodPost, "/goals/results/999/value", map[string]any{"value": 1})
}

func TestSourceThatIsHiddenOrOffCannotBeChosen(t *testing.T) {
	f := setup(t, wed)
	f.hide("github")
	f.expect(http.StatusBadRequest, http.MethodPost, "/goals", q4("t", auto("n", "github_commits", "", 5)))
	// a kind that no provider serves cannot be chosen
	module.Provide[contracts.GoalMetricSource](f.env.App.Deps.Registry, contracts.GoalMetricPrefix+"habits", noSource{})
	f.expect(http.StatusBadRequest, http.MethodPost, "/goals", q4("t", auto("n", "workouts", "", 5)))
}

func TestUpdateReplacesResults(t *testing.T) {
	f := setup(t, wed)
	g := f.create(q4("目标", manual("甲", 0, 10), manual("乙", 0, 10), auto("丙", "workouts", "", 100)))
	jia, bing := resultByName(g, "甲"), resultByName(g, "丙")
	var res api.GoalResult
	f.env.MustDo(http.MethodPost, "/goals/results/"+strconv.FormatInt(jia.Id, 10)+"/value", map[string]any{"value": 4}, &res)

	body := map[string]any{"title": "改了名", "year": 2026, "quarter": 4, "note": "备注", "results": []map[string]any{
		{"id": bing.Id, "name": "丙改", "source": "workouts", "target": 50, "unit": "次"},
		{"id": jia.Id, "name": "甲改", "source": "manual", "target": 20, "startValue": 0, "unit": "本"},
		manual("丁", 0, 5),
	}}
	var updated api.Goal
	f.env.MustDo(http.MethodPut, "/goals/"+strconv.FormatInt(g.Id, 10), body, &updated)
	if updated.Title != "改了名" || updated.Note != "备注" || len(updated.Results) != 3 {
		t.Fatalf("updated: %+v", updated)
	}
	if updated.Results[0].Name != "丙改" || updated.Results[1].Name != "甲改" || updated.Results[2].Name != "丁" {
		t.Fatalf("order follows the input: %v %v %v", updated.Results[0].Name, updated.Results[1].Name, updated.Results[2].Name)
	}
	if updated.Results[1].Id != jia.Id {
		t.Fatal("an edited key result keeps its id")
	}
	near(t, "value survives the edit", updated.Results[1].Value, 4)
	near(t, "progress follows the new target", updated.Results[1].Progress, 20)
	for _, r := range updated.Results {
		if r.Name == "乙" {
			t.Fatal("a key result that was left out is deleted")
		}
	}

	// ids of another goal, and the same id twice, are refused
	other := f.create(q4("别的", manual("戊", 0, 1)))
	f.expect(http.StatusBadRequest, http.MethodPut, "/goals/"+strconv.FormatInt(g.Id, 10), map[string]any{"title": "t", "year": 2026, "quarter": 4, "results": []map[string]any{
		{"id": other.Results[0].Id, "name": "偷", "source": "manual", "target": 1},
	}})
	f.expect(http.StatusBadRequest, http.MethodPut, "/goals/"+strconv.FormatInt(g.Id, 10), map[string]any{"title": "t", "year": 2026, "quarter": 4, "results": []map[string]any{
		{"id": jia.Id, "name": "a", "source": "manual", "target": 1},
		{"id": jia.Id, "name": "b", "source": "manual", "target": 1},
	}})
	// nothing changed by the failed saves
	var kept api.Goal
	f.env.MustDo(http.MethodGet, "/goals/"+strconv.FormatInt(g.Id, 10), nil, &kept)
	if len(kept.Results) != 3 {
		t.Fatalf("failed save changed the goal: %d results", len(kept.Results))
	}
}

func TestEditingKeepsAHiddenSource(t *testing.T) {
	f := setup(t, wed)
	g := f.create(q4("目标", auto("提交", "github_commits", "", 100)))
	f.hide("github")
	var got api.Goal
	f.env.MustDo(http.MethodPut, "/goals/"+strconv.FormatInt(g.Id, 10), map[string]any{"title": "新名", "year": 2026, "quarter": 4, "results": []map[string]any{
		{"id": g.Results[0].Id, "name": "提交", "source": "github_commits", "target": 120},
	}}, &got)
	if got.Title != "新名" || got.Results[0].Target != 120 || got.Results[0].Unavailable != "hidden" {
		t.Fatalf("got %+v", got)
	}
}

func TestUntouchedManualResultFollowsItsStart(t *testing.T) {
	f := setup(t, wed)
	g := f.create(q4("体重", manual("体重", 80, 70)))
	var got api.Goal
	f.env.MustDo(http.MethodPut, "/goals/"+strconv.FormatInt(g.Id, 10), map[string]any{"title": "体重", "year": 2026, "quarter": 4, "results": []map[string]any{
		{"id": g.Results[0].Id, "name": "体重", "source": "manual", "startValue": 90, "target": 70},
	}}, &got)
	near(t, "value", got.Results[0].Value, 90)
	near(t, "progress", got.Results[0].Progress, 0)
	// once a value was entered, a new start does not move it
	f.env.MustDo(http.MethodPost, "/goals/results/"+strconv.FormatInt(g.Results[0].Id, 10)+"/value", map[string]any{"value": 85}, nil)
	f.env.MustDo(http.MethodPut, "/goals/"+strconv.FormatInt(g.Id, 10), map[string]any{"title": "体重", "year": 2026, "quarter": 4, "results": []map[string]any{
		{"id": g.Results[0].Id, "name": "体重", "source": "manual", "startValue": 95, "target": 70},
	}}, &got)
	near(t, "value after a new start", got.Results[0].Value, 85)
}

func TestManualValueRules(t *testing.T) {
	f := setup(t, wed)
	g := f.create(q4("目标", manual("手动", 0, 10), auto("自动", "workouts", "", 100)))
	id := func(name string) string { return strconv.FormatInt(resultByName(g, name).Id, 10) }
	f.expect(http.StatusBadRequest, http.MethodPost, "/goals/results/"+id("自动")+"/value", map[string]any{"value": 5})
	f.expect(http.StatusBadRequest, http.MethodPost, "/goals/results/"+id("手动")+"/value", map[string]any{"value": 1e15})
	f.expect(http.StatusOK, http.MethodPost, "/goals/results/"+id("手动")+"/value", map[string]any{"value": 2})
}

func TestStatusFilterListAndDelete(t *testing.T) {
	f := setup(t, wed)
	a := f.create(q4("甲", manual("x", 0, 1)))
	b := f.create(map[string]any{"title": "乙", "year": 2026, "quarter": 3})
	c := f.create(q4("丙"))
	var done api.Goal
	f.env.MustDo(http.MethodPatch, "/goals/"+strconv.FormatInt(c.Id, 10), map[string]any{"status": "done"}, &done)
	f.expect(http.StatusBadRequest, http.MethodPatch, "/goals/"+strconv.FormatInt(c.Id, 10), map[string]any{"status": "paused"})

	type list struct {
		Items   []api.Goal
		Current api.GoalPeriod
	}
	var all, q4Only, active list
	f.env.MustDo(http.MethodGet, "/goals", nil, &all)
	f.env.MustDo(http.MethodGet, "/goals?year=2026&quarter=4", nil, &q4Only)
	f.env.MustDo(http.MethodGet, "/goals?year=2026&quarter=4&status=active", nil, &active)
	if len(all.Items) != 3 || all.Items[0].Id != a.Id || all.Items[2].Id != b.Id {
		t.Fatalf("all: newest quarter first, then creation order: %+v", titles(all.Items))
	}
	if all.Current.Year != 2026 || all.Current.Quarter != 4 {
		t.Fatalf("current: %+v", all.Current)
	}
	if len(q4Only.Items) != 2 || len(active.Items) != 1 || active.Items[0].Id != a.Id {
		t.Fatalf("filters: %v / %v", titles(q4Only.Items), titles(active.Items))
	}

	f.expect(http.StatusNoContent, http.MethodDelete, "/goals/"+strconv.FormatInt(a.Id, 10), nil)
	f.expect(http.StatusNotFound, http.MethodGet, "/goals/"+strconv.FormatInt(a.Id, 10), nil)
	var left int
	if err := f.env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM goal_results`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("key results of a deleted goal stay: %d %v", left, err)
	}
}

func titles(items []api.Goal) []string {
	var out []string
	for _, g := range items {
		out = append(out, g.Title)
	}
	return out
}

func TestMetricsListFollowsProvidersAndHiding(t *testing.T) {
	f := setup(t, wed)
	var out struct{ Items []api.GoalMetric }
	f.env.MustDo(http.MethodGet, "/goals/metrics", nil, &out)
	if len(out.Items) != 7 || out.Items[0].Source != api.HabitCheckins || out.Items[0].RefKind != api.Habit || len(out.Items[0].Options) != 2 {
		t.Fatalf("metrics: %+v", out.Items)
	}
	f.hide("habits", "screentime")
	f.env.MustDo(http.MethodGet, "/goals/metrics", nil, &out)
	var got []string
	for _, m := range out.Items {
		got = append(got, string(m.Source))
	}
	if strings.Join(got, ",") != "focus_minutes,github_commits,github_merged_prs,skill_minutes" {
		t.Fatalf("with habits and screentime hidden: %v", got)
	}
}

// ---------- weekly review ----------

func TestReviewShowsTheWeeksMovement(t *testing.T) {
	f := setup(t, wed) // the week of 2026-11-09, three and a half days in
	g := f.create(q4("目标",
		manual("手动", 0, 10),
		auto("打卡", "habit_checkins", "1", 92),
	))
	manualID := strconv.FormatInt(resultByName(g, "手动").Id, 10)
	// 3 on the Friday before the week, 5 on Tuesday of the week
	f.setNow(time.Date(2026, 11, 6, 10, 0, 0, 0, cst))
	f.env.MustDo(http.MethodPost, "/goals/results/"+manualID+"/value", map[string]any{"value": 3}, nil)
	f.setNow(time.Date(2026, 11, 10, 10, 0, 0, 0, cst))
	f.env.MustDo(http.MethodPost, "/goals/results/"+manualID+"/value", map[string]any{"value": 5}, nil)
	f.setNow(wed)

	var rv api.GoalReview
	f.env.MustDo(http.MethodGet, "/goals/reviews/current", nil, &rv)
	if rv.WeekStart != "2026-11-09" || rv.WeekEnd != "2026-11-15" || !rv.Current || rv.Saved {
		t.Fatalf("review: %+v", rv)
	}
	if len(rv.Goals) != 1 {
		t.Fatalf("goals: %d", len(rv.Goals))
	}
	wg := rv.Goals[0]
	hand, habit := wg.Results[0], wg.Results[1]
	near(t, "manual now", hand.Value, 5)
	near(t, "manual change", hand.Change, 2)
	near(t, "manual progress", hand.Progress, 50)
	near(t, "auto change is this week's increase", habit.Change, 2.5)
	// the week is measured up to now: 2026-10-01 to 11-11 12:00 is 41.5 days
	near(t, "auto value", habit.Value, 41.5)
	near(t, "goal progress", wg.Progress, (50+45.1)/2)
	near(t, "goal progress before", wg.ProgressBefore, (30+42.4)/2)
	near(t, "goal change", wg.Change, ((50+45.1)-(30+42.4))/2)
	near(t, "expected at the time of the review", &wg.Expected, 45.1)

	// any date of the week gives the same week
	var same api.GoalReview
	f.env.MustDo(http.MethodGet, "/goals/reviews/2026-11-13", nil, &same)
	if same.WeekStart != "2026-11-09" {
		t.Fatalf("week from a date: %s", same.WeekStart)
	}
	f.expect(http.StatusBadRequest, http.MethodGet, "/goals/reviews/2026-11-16", nil)
	f.expect(http.StatusBadRequest, http.MethodGet, "/goals/reviews/next", nil)
	f.expect(http.StatusBadRequest, http.MethodPut, "/goals/reviews/2027-01-04", map[string]any{"wentWell": "x"})
}

func TestReviewLeavesOutGoalsOfOtherQuarters(t *testing.T) {
	f := setup(t, wed)
	f.create(q4("这季度"))
	f.create(map[string]any{"title": "上季度", "year": 2026, "quarter": 3})
	f.create(map[string]any{"title": "明年", "year": 2027, "quarter": 1})
	done := f.create(q4("做完了"))
	f.env.MustDo(http.MethodPatch, "/goals/"+strconv.FormatInt(done.Id, 10), map[string]any{"status": "dropped"}, nil)
	var rv api.GoalReview
	f.env.MustDo(http.MethodGet, "/goals/reviews/current", nil, &rv)
	if len(rv.Goals) != 1 || rv.Goals[0].Title != "这季度" {
		t.Fatalf("goals in the review: %+v", rv.Goals)
	}
	// the week that holds the first day of a quarter touches both quarters
	f.setNow(time.Date(2026, 10, 1, 9, 0, 0, 0, cst)) // Thursday, in the week of 09-28
	f.env.MustDo(http.MethodGet, "/goals/reviews/current", nil, &rv)
	titlesInWeek := ""
	for _, g := range rv.Goals {
		titlesInWeek += g.Title + ","
	}
	if titlesInWeek != "上季度,这季度," {
		t.Fatalf("the week that spans two quarters has both: %s", titlesInWeek)
	}
}

func TestReviewSaveEditAndHistory(t *testing.T) {
	f := setup(t, wed)
	g := f.create(q4("目标", auto("打卡", "habit_checkins", "1", 92)))

	var rv api.GoalReview
	f.env.MustDo(http.MethodPut, "/goals/reviews/current", map[string]any{"wentWell": "跑了三次", "fellShort": "没读书", "nextWeek": "周二晚上八点读书一小时"}, &rv)
	if !rv.Saved || rv.WentWell != "跑了三次" || rv.NextWeek != "周二晚上八点读书一小时" || rv.UpdatedAt == nil {
		t.Fatalf("saved: %+v", rv)
	}
	near(t, "snapshot progress", rv.Goals[0].Progress, 45.1)

	// one record a week: saving again edits it, and a missing field is kept
	f.env.MustDo(http.MethodPut, "/goals/reviews/2026-11-12", map[string]any{"fellShort": "没读书，也没睡够"}, &rv)
	if rv.WeekStart != "2026-11-09" || rv.FellShort != "没读书，也没睡够" || rv.WentWell != "跑了三次" {
		t.Fatalf("edited: %+v", rv)
	}
	var rows int
	if err := f.env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM goal_reviews`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("rows: %d %v", rows, err)
	}

	// the snapshot follows later saves in the week and the week after
	f.setNow(time.Date(2026, 11, 14, 20, 0, 0, 0, cst))
	f.env.MustDo(http.MethodPut, "/goals/reviews/current", map[string]any{}, &rv)
	near(t, "refreshed on Saturday", rv.Goals[0].Progress, 48.7) // 44.83 days of 92 at one a day
	_ = g

	// two weeks on it no longer changes, whatever the goal looks like now
	f.setNow(time.Date(2026, 12, 7, 9, 0, 0, 0, cst)) // the week of 12-07; 11-09 ended 21 days ago
	f.env.MustDo(http.MethodGet, "/goals/reviews/2026-11-09", nil, &rv)
	before := *rv.Goals[0].Progress
	f.src.rate["habit_checkins:1"] = 0
	f.env.MustDo(http.MethodPut, "/goals/reviews/2026-11-09", map[string]any{"wentWell": "补写一句"}, &rv)
	if rv.WentWell != "补写一句" || *rv.Goals[0].Progress != before {
		t.Fatalf("old week: text edits, numbers stay: %v vs %v", *rv.Goals[0].Progress, before)
	}
	f.src.rate["habit_checkins:1"] = 1

	// deleting the goal does not change the history
	f.env.MustDo(http.MethodDelete, "/goals/"+strconv.FormatInt(g.Id, 10), nil, nil)
	f.env.MustDo(http.MethodGet, "/goals/reviews/2026-11-09", nil, &rv)
	if len(rv.Goals) != 1 || rv.Goals[0].Title != "目标" {
		t.Fatalf("snapshot after the goal is gone: %+v", rv.Goals)
	}

	// last week's plan is shown on the next week's page
	f.env.MustDo(http.MethodGet, "/goals/reviews/2026-11-16", nil, &rv)
	if rv.LastPlan != "周二晚上八点读书一小时" || rv.Saved {
		t.Fatalf("next week: %+v", rv)
	}

	var list struct{ Items []api.GoalReviewSummary }
	f.env.MustDo(http.MethodGet, "/goals/reviews", nil, &list)
	if len(list.Items) != 1 || list.Items[0].WeekStart != "2026-11-09" || !list.Items[0].Answered || list.Items[0].WeekEnd != "2026-11-15" {
		t.Fatalf("history: %+v", list.Items)
	}
}

func TestReviewValidationAndHiddenSnapshot(t *testing.T) {
	f := setup(t, wed)
	f.create(q4("目标", auto("打卡", "habit_checkins", "1", 92), manual("手动", 0, 10)))
	f.expect(http.StatusBadRequest, http.MethodPut, "/goals/reviews/current", map[string]any{"wentWell": strings.Repeat("长", 5001)})

	// saved while hidden content is locked: the real numbers are kept
	f.hide("habits")
	var rv api.GoalReview
	f.env.MustDo(http.MethodPut, "/goals/reviews/current", map[string]any{"wentWell": "好"}, &rv)
	hab := rv.Goals[0].Results[0]
	if hab.Unavailable != "hidden" || hab.Value != nil || hab.Change != nil {
		t.Fatalf("hidden numbers are masked: %+v", hab)
	}
	near(t, "the goal's own number leaves the hidden one out", rv.Goals[0].Progress, 0)
	if rv.Goals[0].Change != nil {
		t.Fatal("change is unknown while a result is hidden")
	}
	f.hide() // unlocked
	f.env.MustDo(http.MethodGet, "/goals/reviews/current", nil, &rv)
	near(t, "after unlocking", rv.Goals[0].Results[0].Value, 41.5)
}

// ---------- reminder ----------

type notice struct{ Kind, Title, Body, Link string }

func notices(t *testing.T, f *fixture) []notice {
	t.Helper()
	var out struct{ Items []notice }
	f.env.MustDo(http.MethodGet, "/notifications", nil, &out)
	var got []notice
	for _, n := range out.Items {
		if n.Kind == "goals.review" {
			got = append(got, n)
		}
	}
	return got
}

func TestReviewReminder(t *testing.T) {
	sun := func(h, m int) time.Time { return time.Date(2026, 11, 15, h, m, 0, 0, cst) }
	tick := func(f *fixture, at time.Time) {
		t.Helper()
		f.setNow(at)
		if err := goals.RemindTick(f.m, context.Background(), at); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("sent once on Sunday evening", func(t *testing.T) {
		f := setup(t, sun(9, 0))
		f.create(q4("目标", manual("x", 0, 10)))
		tick(f, sun(19, 59))
		if n := notices(t, f); len(n) != 0 {
			t.Fatal("before the set time")
		}
		tick(f, sun(20, 0))
		n := notices(t, f)
		if len(n) != 1 || n[0].Link != "/goals/review" || !strings.Contains(n[0].Body, "1 个目标") {
			t.Fatalf("notices: %+v", n)
		}
		tick(f, sun(20, 1))
		tick(f, sun(23, 59))
		if n := notices(t, f); len(n) != 1 {
			t.Fatalf("the same week is reminded once: %d", len(n))
		}
		// next Sunday again
		next := sun(20, 5).AddDate(0, 0, 7)
		tick(f, next)
		if n := notices(t, f); len(n) != 2 {
			t.Fatalf("a new week: %d", len(n))
		}
	})
	t.Run("late start still sends on the same Sunday", func(t *testing.T) {
		f := setup(t, sun(9, 0))
		f.create(q4("目标"))
		tick(f, sun(22, 30))
		if n := notices(t, f); len(n) != 1 {
			t.Fatalf("notices: %d", len(n))
		}
	})
	t.Run("not on other days", func(t *testing.T) {
		f := setup(t, sun(9, 0))
		f.create(q4("目标"))
		tick(f, sun(21, 0).AddDate(0, 0, 1)) // Monday evening
		tick(f, sun(21, 0).AddDate(0, 0, -1))
		if n := notices(t, f); len(n) != 0 {
			t.Fatalf("notices: %d", len(n))
		}
	})
	t.Run("not without an active goal", func(t *testing.T) {
		f := setup(t, sun(9, 0))
		g := f.create(q4("目标"))
		f.env.MustDo(http.MethodPatch, "/goals/"+strconv.FormatInt(g.Id, 10), map[string]any{"status": "done"}, nil)
		tick(f, sun(21, 0))
		if n := notices(t, f); len(n) != 0 {
			t.Fatalf("notices: %d", len(n))
		}
	})
	t.Run("not when the review is written", func(t *testing.T) {
		f := setup(t, sun(9, 0))
		f.create(q4("目标"))
		f.env.MustDo(http.MethodPut, "/goals/reviews/current", map[string]any{"nextWeek": "周一早上跑步"}, nil)
		tick(f, sun(21, 0))
		if n := notices(t, f); len(n) != 0 {
			t.Fatalf("notices: %d", len(n))
		}
	})
	t.Run("an empty saved review does not count as written", func(t *testing.T) {
		f := setup(t, sun(9, 0))
		f.create(q4("目标"))
		f.env.MustDo(http.MethodPut, "/goals/reviews/current", map[string]any{}, nil)
		tick(f, sun(21, 0))
		if n := notices(t, f); len(n) != 1 {
			t.Fatalf("notices: %d", len(n))
		}
	})
	t.Run("settings: switch off and change the time", func(t *testing.T) {
		f := setup(t, sun(9, 0))
		f.create(q4("目标"))
		var s api.GoalSettings
		f.env.MustDo(http.MethodGet, "/goals/settings", nil, &s)
		if !s.ReviewReminder || s.ReviewTime != "20:00" {
			t.Fatalf("defaults: %+v", s)
		}
		f.expect(http.StatusBadRequest, http.MethodPut, "/goals/settings", map[string]any{"reviewReminder": true, "reviewTime": "8pm"})
		f.expect(http.StatusBadRequest, http.MethodPut, "/goals/settings", map[string]any{"reviewReminder": true, "reviewTime": "24:00"})
		f.env.MustDo(http.MethodPut, "/goals/settings", map[string]any{"reviewReminder": true, "reviewTime": "18:30"}, &s)
		tick(f, sun(18, 29))
		if n := notices(t, f); len(n) != 0 {
			t.Fatal("before 18:30")
		}
		tick(f, sun(18, 30))
		if n := notices(t, f); len(n) != 1 {
			t.Fatal("at 18:30")
		}
		f.env.MustDo(http.MethodPut, "/goals/settings", map[string]any{"reviewReminder": false, "reviewTime": "18:30"}, &s)
		tick(f, sun(18, 30).AddDate(0, 0, 7))
		if n := notices(t, f); len(n) != 1 {
			t.Fatal("switched off")
		}
	})
	t.Run("the body says when a goal is behind", func(t *testing.T) {
		f := setup(t, sun(9, 0))
		f.create(q4("目标", auto("打卡", "habit_checkins", "1", 5000)))
		tick(f, sun(20, 0))
		n := notices(t, f)
		if len(n) != 1 || !strings.Contains(n[0].Body, "落后") {
			t.Fatalf("notices: %+v", n)
		}
	})
}

// ---------- actions ----------

func TestActions(t *testing.T) {
	f := setup(t, wed)
	g := f.create(q4("读书", manual("本数", 0, 4), auto("打卡", "habit_checkins", "1", 83)))
	f.create(map[string]any{"title": "上季度", "year": 2026, "quarter": 3})
	ctx := context.Background()
	run := func(name, input string) (map[string]json.RawMessage, error) {
		out, err := f.env.App.Deps.Actions.Run(ctx, name, json.RawMessage(input))
		if err != nil {
			return nil, err
		}
		raw, _ := json.Marshal(out)
		var m map[string]json.RawMessage
		_ = json.Unmarshal(raw, &m)
		return m, nil
	}

	out, err := run("goals.list", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	var listed []struct {
		ID      int64
		Title   string
		Results []struct {
			ID     int64
			Source string
		}
	}
	_ = json.Unmarshal(out["goals"], &listed)
	if len(listed) != 2 || listed[0].Title != "读书" || len(listed[0].Results) != 2 {
		t.Fatalf("list: %s", out["goals"])
	}
	out, err = run("goals.list", `{"year":2026,"quarter":3}`)
	if err != nil || !strings.Contains(string(out["goals"]), "上季度") || strings.Contains(string(out["goals"]), "读书") {
		t.Fatalf("list by quarter: %v %s", err, out["goals"])
	}
	if _, err := run("goals.list", `{"year":2026}`); err == nil {
		t.Fatal("year without quarter")
	}
	if _, err := run("goals.list", `{"status":"x"}`); err == nil {
		t.Fatal("bad status")
	}

	out, err = run("goals.progress", `{"goalId":`+strconv.FormatInt(g.Id, 10)+`}`)
	if err != nil {
		t.Fatal(err)
	}
	var prog []api.Goal
	_ = json.Unmarshal(out["goals"], &prog)
	if len(prog) != 1 || prog[0].Progress == nil || prog[0].State == "" {
		t.Fatalf("progress: %s", out["goals"])
	}
	if _, err := run("goals.progress", `{"goalId":999}`); err == nil {
		t.Fatal("unknown goal")
	}

	manualID := strconv.FormatInt(resultByName(g, "本数").Id, 10)
	autoID := strconv.FormatInt(resultByName(g, "打卡").Id, 10)
	if _, err := run("goals.update_progress", `{"resultId":`+manualID+`,"value":2}`); err != nil {
		t.Fatal(err)
	}
	var again api.Goal
	f.env.MustDo(http.MethodGet, "/goals/"+strconv.FormatInt(g.Id, 10), nil, &again)
	near(t, "set by the action", resultByName(again, "本数").Progress, 50)
	if _, err := run("goals.update_progress", `{"resultId":`+autoID+`,"value":2}`); err == nil {
		t.Fatal("an automatic key result cannot be set")
	}
	if _, err := run("goals.update_progress", `{"resultId":`+manualID+`}`); err == nil {
		t.Fatal("value is required")
	}
}
