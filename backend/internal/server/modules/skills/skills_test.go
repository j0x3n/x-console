package skills_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/skills"
	"github.com/j0x3n/x-console/backend/internal/server/modules/skills/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type fixture struct {
	env *testutil.Env
	m   *skills.Module
	now time.Time
}

// setup fixes the time at Wednesday 2026-11-11 10:00.
func setup(t *testing.T) *fixture {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*skills.Module](env.App.Deps.Registry, skills.ServiceKey)
	if !ok {
		t.Fatal("skills module not registered")
	}
	f := &fixture{env: env, m: m, now: time.Date(2026, 11, 11, 10, 0, 0, 0, skills.Loc(m))}
	skills.SetNow(m, func() time.Time { return f.now })
	return f
}

func (f *fixture) day(offset int) string { return f.now.AddDate(0, 0, offset).Format("2006-01-02") }

func (f *fixture) skill(t *testing.T, name string, target int) api.Skill {
	t.Helper()
	body := map[string]any{"name": name}
	if target > 0 {
		body["weeklyTarget"] = target
	}
	var s api.Skill
	f.env.MustDo(http.MethodPost, "/skills", body, &s)
	return s
}

func (f *fixture) log(t *testing.T, id int64, minutes int, day, content, stuck string) api.SkillSession {
	t.Helper()
	var s api.SkillSession
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/skills/%d/sessions", id),
		map[string]any{"minutes": minutes, "practicedOn": day, "content": content, "stuck": stuck}, &s)
	return s
}

func (f *fixture) list(t *testing.T, query string) []api.Skill {
	t.Helper()
	var out struct {
		Items     []api.Skill
		WeekStart string
		Today     string
	}
	f.env.MustDo(http.MethodGet, "/skills"+query, nil, &out)
	if out.Today != "2026-11-11" || out.WeekStart != "2026-11-09" {
		t.Fatalf("dates: %+v", out)
	}
	return out.Items
}

func (f *fixture) bad(t *testing.T, method, path string, body any) {
	t.Helper()
	if code, _ := f.env.Do(method, path, body, nil); code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
		t.Errorf("%s %s %v: status %d", method, path, body, code)
	}
}

func TestSkillValidation(t *testing.T) {
	f := setup(t)
	f.bad(t, http.MethodPost, "/skills", map[string]any{"name": "  "})
	f.bad(t, http.MethodPost, "/skills", map[string]any{"name": strings.Repeat("字", 51)})
	f.bad(t, http.MethodPost, "/skills", map[string]any{"name": "x", "weeklyTarget": 10081})
	f.skill(t, "英语口语", 0)
	f.bad(t, http.MethodPost, "/skills", map[string]any{"name": " 英语口语 "})
	f.skill(t, "Rust", 0)
	f.bad(t, http.MethodPost, "/skills", map[string]any{"name": "rust"})
	other := f.skill(t, "算法题", 0)
	f.bad(t, http.MethodPatch, fmt.Sprintf("/skills/%d", other.Id), map[string]any{"name": "RUST"})
	if code, _ := f.env.Do(http.MethodPatch, "/skills/999", map[string]any{"name": "x"}, nil); code != http.StatusNotFound {
		t.Errorf("patch missing: %d", code)
	}
}

func TestSessionValidation(t *testing.T) {
	f := setup(t)
	s := f.skill(t, "英语口语", 0)
	path := fmt.Sprintf("/skills/%d/sessions", s.Id)
	f.bad(t, http.MethodPost, path, map[string]any{"minutes": 0})
	f.bad(t, http.MethodPost, path, map[string]any{"minutes": 1441})
	f.bad(t, http.MethodPost, path, map[string]any{"minutes": 30, "practicedOn": f.day(1)})
	f.bad(t, http.MethodPost, path, map[string]any{"minutes": 30, "practicedOn": f.now.AddDate(-1, 0, -1).Format("2006-01-02")})
	f.bad(t, http.MethodPost, path, map[string]any{"minutes": 30, "practicedOn": "11/11"})
	f.bad(t, http.MethodPost, path, map[string]any{"minutes": 30, "content": strings.Repeat("字", 1001)})
	f.bad(t, http.MethodPost, path, map[string]any{"minutes": 30, "stuck": strings.Repeat("字", 1001)})
	if code, _ := f.env.Do(http.MethodPost, "/skills/999/sessions", map[string]any{"minutes": 30}, nil); code != http.StatusNotFound {
		t.Errorf("missing skill: %d", code)
	}
	// Today (the default) and exactly one year back are fine.
	var one api.SkillSession
	f.env.MustDo(http.MethodPost, path, map[string]any{"minutes": 30}, &one)
	if one.PracticedOn != "2026-11-11" {
		t.Errorf("default day: %+v", one)
	}
	f.log(t, s.Id, 30, f.now.AddDate(-1, 0, 0).Format("2006-01-02"), "", "")
}

func TestWeeklyNumbers(t *testing.T) {
	f := setup(t)
	s := f.skill(t, "英语口语", 120)
	empty := f.list(t, "")[0]
	if empty.WeekMinutes != 0 || empty.TotalMinutes != 0 || empty.LastPracticedOn != nil || len(empty.Weeks) != 8 || *empty.WeeklyTarget != 120 {
		t.Fatalf("empty: %+v", empty)
	}
	// Wed 11-11 and Mon 11-09 are this week. Sun 11-08 is last week.
	f.log(t, s.Id, 30, f.day(0), "对话", "")
	f.log(t, s.Id, 30, f.day(-2), "听力", "连读")
	f.log(t, s.Id, 45, f.day(-3), "单词", "")
	f.log(t, s.Id, 20, f.day(-60), "很早以前", "")
	got := f.list(t, "")[0]
	if got.WeekMinutes != 60 || got.TotalMinutes != 125 || got.LastPracticedOn == nil || *got.LastPracticedOn != "2026-11-11" {
		t.Fatalf("numbers: %+v", got)
	}
	if got.Weeks[7] != 60 || got.Weeks[6] != 45 || got.Weeks[0] != 0 {
		t.Fatalf("weeks: %v", got.Weeks)
	}
	// The next Monday starts a new week.
	f.now = f.now.AddDate(0, 0, 5)
	f.env.MustDo(http.MethodGet, "/skills", nil, nil)
	next := f.list2(t)[0]
	if next.WeekMinutes != 0 || next.Weeks[6] != 60 || next.Weeks[5] != 45 {
		t.Fatalf("next week: %+v", next)
	}
}

// list2 reads the list without the date check of list, for tests that move the clock.
func (f *fixture) list2(t *testing.T) []api.Skill {
	t.Helper()
	var out struct{ Items []api.Skill }
	f.env.MustDo(http.MethodGet, "/skills", nil, &out)
	return out.Items
}

func TestSessionsListEditDelete(t *testing.T) {
	f := setup(t)
	s := f.skill(t, "算法题", 0)
	a := f.log(t, s.Id, 30, f.day(-2), "二分", "")
	f.log(t, s.Id, 45, f.day(-1), "动态规划", "状态怎么定义")
	c := f.log(t, s.Id, 25, f.day(0), "图", "")

	var out struct{ Items []api.SkillSession }
	f.env.MustDo(http.MethodGet, fmt.Sprintf("/skills/%d/sessions", s.Id), nil, &out)
	if len(out.Items) != 3 || out.Items[0].Id != c.Id || out.Items[2].Id != a.Id {
		t.Fatalf("order: %+v", out.Items)
	}
	f.env.MustDo(http.MethodGet, fmt.Sprintf("/skills/%d/sessions?stuck=true", s.Id), nil, &out)
	if len(out.Items) != 1 || out.Items[0].Stuck != "状态怎么定义" {
		t.Fatalf("stuck: %+v", out.Items)
	}
	f.env.MustDo(http.MethodGet, fmt.Sprintf("/skills/%d/sessions?limit=1", s.Id), nil, &out)
	if len(out.Items) != 1 {
		t.Fatalf("limit: %+v", out.Items)
	}
	if code, _ := f.env.Do(http.MethodGet, fmt.Sprintf("/skills/%d/sessions?limit=501", s.Id), nil, nil); code == http.StatusOK {
		t.Error("limit 501 accepted")
	}

	var up api.SkillSession
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/skill-sessions/%d", a.Id), map[string]any{"minutes": 50, "stuck": "边界"}, &up)
	if up.Minutes != 50 || up.Stuck != "边界" || up.Content != "二分" || up.PracticedOn != a.PracticedOn {
		t.Fatalf("patched: %+v", up)
	}
	f.bad(t, http.MethodPatch, fmt.Sprintf("/skill-sessions/%d", a.Id), map[string]any{"minutes": 0})
	f.bad(t, http.MethodPatch, fmt.Sprintf("/skill-sessions/%d", a.Id), map[string]any{"practicedOn": f.day(3)})
	f.bad(t, http.MethodPatch, fmt.Sprintf("/skill-sessions/%d", a.Id), map[string]any{"practicedOn": ""})
	if code, _ := f.env.Do(http.MethodPatch, "/skill-sessions/9999", map[string]any{"minutes": 5}, nil); code != http.StatusNotFound {
		t.Errorf("patch missing: %d", code)
	}
	if code, _ := f.env.Do(http.MethodDelete, fmt.Sprintf("/skill-sessions/%d", a.Id), nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := f.env.Do(http.MethodDelete, fmt.Sprintf("/skill-sessions/%d", a.Id), nil, nil); code != http.StatusNotFound {
		t.Fatalf("delete twice: %d", code)
	}
	if got := f.list(t, "")[0]; got.TotalMinutes != 70 {
		t.Fatalf("total after delete: %+v", got)
	}
}

func TestArchiveAndDelete(t *testing.T) {
	f := setup(t)
	s := f.skill(t, "弹琴", 60)
	keep := f.skill(t, "绘画", 0)
	f.log(t, s.Id, 30, f.day(0), "", "")

	var up api.Skill
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/skills/%d", s.Id), map[string]any{"archived": true, "weeklyTarget": 0}, &up)
	if !up.Archived || up.WeeklyTarget != nil || up.TotalMinutes != 30 {
		t.Fatalf("archived: %+v", up)
	}
	if items := f.list(t, ""); len(items) != 1 || items[0].Id != keep.Id {
		t.Fatalf("default list: %+v", items)
	}
	if items := f.list(t, "?archived=true"); len(items) != 2 || !items[1].Archived {
		t.Fatalf("with archived: %+v", items)
	}
	f.bad(t, http.MethodPost, fmt.Sprintf("/skills/%d/sessions", s.Id), map[string]any{"minutes": 5})
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/skills/%d", s.Id), map[string]any{"archived": false, "name": "钢琴"}, &up)
	if up.Archived || up.Name != "钢琴" {
		t.Fatalf("restored: %+v", up)
	}

	// Deleting the skill takes its sessions along.
	if code, _ := f.env.Do(http.MethodDelete, fmt.Sprintf("/skills/%d", s.Id), nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	var left int
	if err := f.env.App.Deps.DB.QueryRow(`SELECT count(*) FROM skill_sessions`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("sessions left: %d %v", left, err)
	}
	if code, _ := f.env.Do(http.MethodDelete, fmt.Sprintf("/skills/%d", s.Id), nil, nil); code != http.StatusNotFound {
		t.Fatalf("delete twice: %d", code)
	}
}

func TestLimits(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	db := f.env.App.Deps.DB
	s := f.skill(t, "第一项", 0)
	for i := 0; i < 5000; i++ {
		if _, err := db.ExecContext(ctx, `INSERT INTO skill_sessions (skill_id, practiced_on, minutes, created_at) VALUES (?, '2026-11-01', 1, ?)`, s.Id, f.now.UTC()); err != nil {
			t.Fatal(err)
		}
	}
	f.bad(t, http.MethodPost, fmt.Sprintf("/skills/%d/sessions", s.Id), map[string]any{"minutes": 5})
	for i := 1; i < 100; i++ {
		if _, err := db.ExecContext(ctx, `INSERT INTO skills (name, created_at) VALUES (?, ?)`, fmt.Sprintf("技能%d", i), f.now.UTC()); err != nil {
			t.Fatal(err)
		}
	}
	f.bad(t, http.MethodPost, "/skills", map[string]any{"name": "第一百零一项"})
}

func TestActions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	acts := f.env.App.Deps.Actions
	s := f.skill(t, "英语口语", 0)
	if _, err := acts.Run(ctx, "skills.log", []byte(`{"skill":"英语口语","minutes":40,"content":"对话","stuck":"时态"}`)); err != nil {
		t.Fatal(err)
	}
	// By id and with a past date.
	if _, err := acts.Run(ctx, "skills.log", []byte(fmt.Sprintf(`{"skill":"%d","minutes":20,"date":%q}`, s.Id, f.day(-1)))); err != nil {
		t.Fatal(err)
	}
	if _, err := acts.Run(ctx, "skills.log", []byte(`{"skill":"不存在","minutes":20}`)); err == nil {
		t.Fatal("unknown skill should fail")
	}
	if _, err := acts.Run(ctx, "skills.log", []byte(`{"skill":"英语口语","minutes":0}`)); err == nil {
		t.Fatal("zero minutes should fail")
	}
	list, err := acts.Run(ctx, "skills.list", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	items := list.(map[string]any)["items"].([]api.Skill)
	if len(items) != 1 || items[0].TotalMinutes != 60 || items[0].WeekMinutes != 60 {
		t.Fatalf("list: %+v", items)
	}
}

func TestGoalSource(t *testing.T) {
	f := setup(t)
	// The goals module reads the real clock, so this test works in the
	// quarter that is running now and logs today's practice.
	f.now = time.Now().In(skills.Loc(f.m))
	year, quarter := f.now.Year(), (int(f.now.Month())-1)/3+1
	qStart := time.Date(year, time.Month((quarter-1)*3+1), 1, 0, 0, 0, 0, f.now.Location())
	s := f.skill(t, "英语口语", 0)
	archived := f.skill(t, "旧技能", 0)
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/skills/%d", archived.Id), map[string]any{"archived": true}, nil)
	f.log(t, s.Id, 30, f.day(0), "", "")
	f.log(t, s.Id, 45, f.day(0), "", "")
	f.log(t, s.Id, 99, qStart.AddDate(0, 0, -1).Format("2006-01-02"), "", "") // last quarter

	var metrics struct {
		Items []struct {
			Source  string
			RefKind string
			Options []struct{ Value, Label string }
		}
	}
	f.env.MustDo(http.MethodGet, "/goals/metrics", nil, &metrics)
	found := false
	for _, it := range metrics.Items {
		if it.Source == "skill_minutes" {
			found = true
			if it.RefKind != "skill" || len(it.Options) != 1 || it.Options[0].Label != "英语口语" {
				t.Fatalf("metric: %+v", it)
			}
		}
	}
	if !found {
		t.Fatalf("skill_minutes not offered: %+v", metrics)
	}

	var g struct {
		Id      int64
		Results []struct {
			Value       *float64
			Progress    *float64
			Unavailable string
		}
	}
	f.env.MustDo(http.MethodPost, "/goals", map[string]any{"title": "口语", "year": year, "quarter": quarter, "results": []map[string]any{
		{"name": "练习时长", "source": "skill_minutes", "ref": fmt.Sprint(s.Id), "target": 150, "unit": "分钟"},
	}}, &g)
	r := g.Results[0]
	if r.Unavailable != "" || r.Value == nil || *r.Value != 75 || r.Progress == nil || *r.Progress != 50 {
		t.Fatalf("result: %+v", r)
	}
	// An archived skill cannot be chosen.
	if code, _ := f.env.Do(http.MethodPost, "/goals", map[string]any{"title": "x", "year": year, "quarter": quarter, "results": []map[string]any{
		{"name": "旧", "source": "skill_minutes", "ref": fmt.Sprint(archived.Id), "target": 10, "unit": "分钟"},
	}}, nil); code == http.StatusCreated {
		t.Error("archived skill accepted")
	}
}
