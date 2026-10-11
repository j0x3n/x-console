package goals_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// The numbers come from the real habits, focus and screentime modules here,
// not from the fake source of the other tests.
func TestRealSources(t *testing.T) {
	env := testutil.New(t)
	m, _ := module.Lookup[*goals.Module](env.App.Deps.Registry, goals.ServiceKey)
	goals.SetNow(m, func() time.Time { return wed })
	db := env.App.Deps.DB
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	day := func(month time.Month, d, h int) time.Time { return time.Date(2026, month, d, h, 0, 0, 0, cst).UTC() }

	// habits: two habits, check-ins in and outside the quarter
	exec(`INSERT INTO habits (id, name, created_at) VALUES (1, '读书', ?), (2, '喝水', ?)`, wed.UTC(), wed.UTC())
	for _, at := range []time.Time{day(9, 30, 23), day(10, 5, 8), day(10, 20, 8), day(10, 20, 20), day(11, 2, 8)} {
		exec(`INSERT INTO habit_logs (habit_id, at) VALUES (1, ?)`, at)
	}
	exec(`INSERT INTO habit_logs (habit_id, at) VALUES (2, ?)`, day(10, 6, 9))
	exec(`INSERT INTO workout_logs (date, created_at) VALUES ('2026-09-01', ?), ('2026-10-08', ?), ('2026-11-03', ?)`, day(9, 1, 8), day(10, 8, 8), day(11, 3, 8))

	// focus: only finished sessions of a minute or more count
	ins := `INSERT INTO focus_sessions (started_at, ended_at, planned_minutes, actual_seconds, completed) VALUES (?, ?, 25, ?, 1)`
	exec(ins, day(10, 3, 9), day(10, 3, 10), 1500)
	exec(ins, day(10, 4, 9), day(10, 4, 10), 3000)
	exec(ins, day(10, 5, 9), day(10, 5, 10), 30)
	exec(ins, day(9, 20, 9), day(9, 20, 10), 1500)
	exec(`INSERT INTO focus_sessions (started_at, planned_minutes, actual_seconds) VALUES (?, 25, 0)`, day(11, 11, 9))

	// screen time: minutes of the day, by category
	minute := func(d time.Time) int64 { return d.Unix() / 60 }
	start := day(10, 3, 9)
	for i := 0; i < 100; i++ {
		exec(`INSERT INTO screen_minutes (host_id, minute, app, category) VALUES ('pc', ?, 'code', 'coding')`, minute(start)+int64(i))
	}
	for i := 0; i < 50; i++ {
		exec(`INSERT INTO screen_minutes (host_id, minute, app, category) VALUES ('pc', ?, 'chrome', 'web')`, minute(start)+1000+int64(i))
	}
	exec(`INSERT INTO screen_minutes (host_id, minute, app, category) VALUES ('pc', ?, 'code', 'coding')`, minute(day(9, 29, 9)))

	var metrics struct{ Items []api.GoalMetric }
	env.MustDo(http.MethodGet, "/goals/metrics", nil, &metrics)
	if len(metrics.Items) != 7 {
		t.Fatalf("metrics: %+v", metrics.Items)
	}
	for _, mt := range metrics.Items {
		switch mt.Source {
		case api.HabitCheckins:
			if len(mt.Options) != 2 || mt.Options[0].Label != "读书" || mt.Options[1].Value != "2" {
				t.Fatalf("habit options: %+v", mt.Options)
			}
		case api.ScreenMinutes:
			if len(mt.Options) != 8 || mt.Options[0].Value != "" || mt.Options[1].Value != "coding" {
				t.Fatalf("category options: %+v", mt.Options)
			}
		}
	}

	var g api.Goal
	env.MustDo(http.MethodPost, "/goals", q4("全部",
		auto("读书", "habit_checkins", "1", 10),
		auto("喝水", "habit_checkins", "2", 10),
		auto("训练", "workouts", "", 10),
		auto("专注", "focus_minutes", "", 100),
		auto("编码", "screen_minutes", "coding", 200),
		auto("全部时间", "screen_minutes", "", 300),
		auto("提交", "github_commits", "", 10),
		auto("合并", "github_merged_prs", "", 10),
	), &g)
	want := map[string]float64{"读书": 4, "喝水": 1, "训练": 2, "专注": 75, "编码": 100, "全部时间": 150}
	for name, v := range want {
		r := resultByName(g, name)
		if r.Value == nil {
			t.Fatalf("%s: no value (%q)", name, r.Unavailable)
		}
		near(t, name, r.Value, v)
	}
	if r := resultByName(g, "读书"); r.RefLabel != "读书" {
		t.Fatalf("label: %q", r.RefLabel)
	}
	// no GitHub account is connected: no data, not zero
	for _, name := range []string{"提交", "合并"} {
		if r := resultByName(g, name); r.Unavailable != "missing" || r.Value != nil {
			t.Fatalf("%s: %+v", name, r)
		}
	}
	near(t, "average of the six that can be read", g.Progress, (40+10+20+75+50+50)/6.0)

	// an unknown category or habit is refused at save time
	if status, _ := env.Do(http.MethodPost, "/goals", q4("x", auto("n", "habit_checkins", "3", 1)), nil); status != http.StatusBadRequest {
		t.Fatalf("unknown habit: %d", status)
	}

	// and the week view reads the same sources
	var rv api.GoalReview
	env.MustDo(http.MethodGet, "/goals/reviews/current", nil, &rv)
	if len(rv.Goals) != 1 || len(rv.Goals[0].Results) != 8 {
		t.Fatalf("review: %+v", rv.Goals)
	}
	read := rv.Goals[0].Results[0]
	near(t, "check-ins this week", read.Change, 0) // the last one was on 11-02, the week starts on 11-09
}
