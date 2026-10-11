package forecasts_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/forecasts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/forecasts/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type fixture struct {
	env *testutil.Env
	m   *forecasts.Module
	now time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*forecasts.Module](env.App.Deps.Registry, forecasts.ServiceKey)
	if !ok {
		t.Fatal("forecasts module not registered")
	}
	f := &fixture{env: env, m: m, now: time.Date(2026, 11, 11, 10, 0, 0, 0, forecasts.Loc(m))}
	forecasts.SetNow(m, func() time.Time { return f.now })
	return f
}

func (f *fixture) day(offset int) string { return f.now.AddDate(0, 0, offset).Format("2006-01-02") }

func (f *fixture) add(t *testing.T, kind, statement string, prob int, due string) api.Forecast {
	t.Helper()
	body := map[string]any{"kind": kind, "statement": statement, "dueDate": due}
	if prob > 0 {
		body["probability"] = prob
	}
	var out api.Forecast
	f.env.MustDo(http.MethodPost, "/forecasts", body, &out)
	return out
}

func (f *fixture) resolve(t *testing.T, id int64, outcome string) api.Forecast {
	t.Helper()
	var out api.Forecast
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/forecasts/%d/resolve", id), map[string]any{"outcome": outcome, "note": "备注"}, &out)
	return out
}

func (f *fixture) notifications(t *testing.T) int {
	t.Helper()
	var out struct{ Items []struct{ Kind string } }
	f.env.MustDo(http.MethodGet, "/notifications?limit=100", nil, &out)
	n := 0
	for _, it := range out.Items {
		if it.Kind == "forecast.due" {
			n++
		}
	}
	return n
}

func (f *fixture) stats(t *testing.T) api.ForecastStats {
	t.Helper()
	var s api.ForecastStats
	f.env.MustDo(http.MethodGet, "/forecasts/stats", nil, &s)
	return s
}

func TestCreateValidation(t *testing.T) {
	f := setup(t)
	bad := []map[string]any{
		{"kind": "guess", "statement": "x", "dueDate": f.day(1)},
		{"kind": "prediction", "statement": "  ", "dueDate": f.day(1)},
		{"kind": "prediction", "statement": strings.Repeat("字", 501), "dueDate": f.day(1)},
		{"kind": "prediction", "statement": "x", "dueDate": f.day(-1)},
		{"kind": "prediction", "statement": "x", "dueDate": "2026/12/01"},
		{"kind": "prediction", "statement": "x", "dueDate": f.now.AddDate(5, 0, 2).Format("2006-01-02")},
		{"kind": "prediction", "statement": "x", "dueDate": f.day(1), "probability": 0},
		{"kind": "prediction", "statement": "x", "dueDate": f.day(1), "probability": 100},
		{"kind": "prediction", "statement": "x", "dueDate": f.day(1), "reason": strings.Repeat("字", 2001)},
	}
	for i, b := range bad {
		if code, _ := f.env.Do(http.MethodPost, "/forecasts", b, nil); code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
			t.Errorf("case %d: status %d", i, code)
		}
	}
	// Today and exactly five years ahead are fine, and so is a decision without confidence.
	f.add(t, "prediction", "今天", 50, f.day(0))
	f.add(t, "decision", "选了 A", 0, f.now.AddDate(5, 0, 0).Format("2006-01-02"))
}

func TestLifecycleAndStats(t *testing.T) {
	f := setup(t)
	a := f.add(t, "prediction", "这个功能两天做完", 70, f.day(2))
	if a.Outcome != nil || a.Overdue || a.Probability == nil || *a.Probability != 70 {
		t.Fatalf("created: %+v", a)
	}
	if s := f.stats(t); s.Count != 0 || !s.Small {
		t.Fatalf("empty stats: %+v", s)
	}

	// Edit while open: probability, statement and removing the probability.
	var up api.Forecast
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/forecasts/%d", a.Id), map[string]any{"statement": "三天做完", "probability": 60, "reason": "有风险"}, &up)
	if up.Statement != "三天做完" || *up.Probability != 60 || up.Reason != "有风险" {
		t.Fatalf("patched: %+v", up)
	}
	up = api.Forecast{}
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/forecasts/%d", a.Id), map[string]any{"probability": 0}, &up)
	if up.Probability != nil {
		t.Fatalf("probability should be gone: %+v", up)
	}
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/forecasts/%d", a.Id), map[string]any{"probability": 70}, &up)

	r := f.resolve(t, a.Id, "yes")
	if r.Outcome == nil || *r.Outcome != api.Yes || r.Note != "备注" || r.ResolvedAt == nil {
		t.Fatalf("resolved: %+v", r)
	}
	if s := f.stats(t); s.Count != 1 || s.Brier != 0.09 {
		t.Fatalf("stats: %+v", s)
	}

	// A resolved record only takes a note.
	if code, _ := f.env.Do(http.MethodPatch, fmt.Sprintf("/forecasts/%d", a.Id), map[string]any{"statement": "改"}, nil); code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
		t.Errorf("edit resolved: %d", code)
	}
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/forecasts/%d", a.Id), map[string]any{"note": "新备注"}, &up)
	if up.Note != "新备注" || up.Statement != "三天做完" {
		t.Fatalf("note: %+v", up)
	}

	// Void and decisions without confidence stay out of the stats.
	v := f.add(t, "prediction", "说不清", 90, f.day(1))
	f.resolve(t, v.Id, "void")
	d := f.add(t, "decision", "换了工具", 0, f.day(1))
	f.resolve(t, d.Id, "no")
	if s := f.stats(t); s.Count != 1 {
		t.Fatalf("void and no-confidence counted: %+v", s)
	}

	// Reopen takes it out of the stats again.
	up = api.Forecast{}
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/forecasts/%d/reopen", a.Id), nil, &up)
	if up.Outcome != nil {
		t.Fatalf("reopened: %+v", up)
	}
	if s := f.stats(t); s.Count != 0 {
		t.Fatalf("after reopen: %+v", s)
	}
	if code, _ := f.env.Do(http.MethodPost, "/forecasts/9999/reopen", nil, nil); code != http.StatusNotFound {
		t.Errorf("reopen missing: %d", code)
	}
	if code, _ := f.env.Do(http.MethodPost, fmt.Sprintf("/forecasts/%d/resolve", a.Id), map[string]any{"outcome": "maybe"}, nil); code == http.StatusOK {
		t.Error("bad outcome accepted")
	}
}

func TestListOrderAndOverdue(t *testing.T) {
	f := setup(t)
	late := f.add(t, "prediction", "晚的", 50, f.day(10))
	soon := f.add(t, "prediction", "早的", 50, f.day(1))
	done := f.add(t, "prediction", "做完的", 50, f.day(2))
	f.resolve(t, done.Id, "no")

	var out struct {
		Items []api.Forecast
		Today string
	}
	f.env.MustDo(http.MethodGet, "/forecasts", nil, &out)
	if out.Today != "2026-11-11" || len(out.Items) != 3 || out.Items[0].Id != soon.Id || out.Items[1].Id != late.Id || out.Items[2].Id != done.Id {
		t.Fatalf("all: %+v", out)
	}
	f.env.MustDo(http.MethodGet, "/forecasts?status=resolved", nil, &out)
	if len(out.Items) != 1 || out.Items[0].Id != done.Id {
		t.Fatalf("resolved: %+v", out)
	}
	f.env.MustDo(http.MethodGet, "/forecasts?status=open", nil, &out)
	if len(out.Items) != 2 || out.Items[0].Overdue {
		t.Fatalf("open: %+v", out)
	}
	// Two days later the first one is overdue.
	f.now = f.now.AddDate(0, 0, 2)
	f.env.MustDo(http.MethodGet, "/forecasts?status=open", nil, &out)
	if !out.Items[0].Overdue || out.Items[1].Overdue {
		t.Fatalf("overdue: %+v", out.Items)
	}
}

func TestDueReminder(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tick := func(offset, h, m int) {
		f.now = time.Date(2026, 11, 11+offset, h, m, 0, 0, forecasts.Loc(f.m))
		if err := forecasts.DueTick(f.m, ctx); err != nil {
			t.Fatal(err)
		}
	}
	a := f.add(t, "prediction", "后天见分晓", 70, f.day(2))
	tick(0, 9, 0)
	tick(1, 9, 0)
	if n := f.notifications(t); n != 0 {
		t.Fatalf("before due: %d", n)
	}
	tick(2, 8, 59) // before the reminder time
	if n := f.notifications(t); n != 0 {
		t.Fatalf("early: %d", n)
	}
	tick(2, 9, 0)
	tick(2, 12, 0)
	tick(3, 9, 0) // once only
	if n := f.notifications(t); n != 1 {
		t.Fatalf("reminders: %d", n)
	}

	// With an outcome there is no reminder.
	b := f.add(t, "prediction", "已经有结果", 50, f.day(1))
	f.resolve(t, b.Id, "yes")
	tick(1, 9, 0)
	if n := f.notifications(t); n != 1 {
		t.Fatalf("resolved: %d", n)
	}

	// A new due date gets a new reminder.
	var up api.Forecast
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/forecasts/%d", a.Id), map[string]any{"dueDate": f.day(2)}, &up)
	tick(3, 10, 0)
	if n := f.notifications(t); n != 2 {
		t.Fatalf("new date: %d", n)
	}

	// Written after the reminder time on the due day: no reminder right away.
	f.now = time.Date(2026, 11, 20, 15, 0, 0, 0, forecasts.Loc(f.m))
	f.add(t, "prediction", "今天写今天到期", 50, f.day(0))
	_ = forecasts.DueTick(f.m, ctx)
	if n := f.notifications(t); n != 2 {
		t.Fatalf("same day: %d", n)
	}

	// Switched off.
	f.env.MustDo(http.MethodPut, "/forecasts/settings", api.ForecastSettings{Remind: false, RemindTime: "09:00"}, nil)
	f.add(t, "prediction", "不提醒", 50, f.day(1))
	tick(1, 10, 0)
	if n := f.notifications(t); n != 2 {
		t.Fatalf("off: %d", n)
	}
	if code, _ := f.env.Do(http.MethodPut, "/forecasts/settings", api.ForecastSettings{Remind: true, RemindTime: "25:00"}, nil); code == http.StatusOK {
		t.Error("bad time accepted")
	}
}

func TestDeleteAndLimitAndActions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	a := f.add(t, "decision", "删掉", 0, f.day(1))
	if code, _ := f.env.Do(http.MethodDelete, fmt.Sprintf("/forecasts/%d", a.Id), nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := f.env.Do(http.MethodDelete, fmt.Sprintf("/forecasts/%d", a.Id), nil, nil); code != http.StatusNotFound {
		t.Fatalf("delete twice: %d", code)
	}

	acts := f.env.App.Deps.Actions
	res, err := acts.Run(ctx, "forecasts.add", []byte(fmt.Sprintf(`{"kind":"prediction","statement":"AI 记的","probability":80,"dueDate":%q}`, f.day(1))))
	if err != nil || res == nil {
		t.Fatalf("forecasts.add: %v %v", res, err)
	}
	if _, err := acts.Run(ctx, "forecasts.add", []byte(`{"kind":"prediction","statement":"","dueDate":"2030-01-01"}`)); err == nil {
		t.Fatal("empty statement should fail")
	}
	list, err := acts.Run(ctx, "forecasts.list", []byte(`{"status":"open"}`))
	if err != nil {
		t.Fatal(err)
	}
	if s := fmt.Sprintf("%+v", list); !strings.Contains(s, "AI 记的") {
		t.Errorf("list: %s", s)
	}
	var id int64
	for _, it := range list.(map[string]any)["items"].([]api.Forecast) {
		id = it.Id
	}
	if _, err := acts.Run(ctx, "forecasts.resolve", []byte(fmt.Sprintf(`{"id":%d,"outcome":"yes"}`, id))); err != nil {
		t.Fatal(err)
	}
	if _, err := acts.Run(ctx, "forecasts.resolve", []byte(`{"id":99999,"outcome":"yes"}`)); err == nil {
		t.Fatal("missing id should fail")
	}
	if _, err := acts.Run(ctx, "forecasts.list", []byte(`{"status":"x"}`)); err == nil {
		t.Fatal("bad status should fail")
	}

	// 2000 records is the limit.
	for i := 0; i < 2000-1; i++ {
		if _, err := f.env.App.Deps.DB.ExecContext(ctx, `INSERT INTO forecasts (kind, statement, due_date, created_at) VALUES ('decision', 'x', '2030-01-01', ?)`, f.now.UTC()); err != nil {
			t.Fatal(err)
		}
	}
	if code, _ := f.env.Do(http.MethodPost, "/forecasts", map[string]any{"kind": "decision", "statement": "多了", "dueDate": f.day(1)}, nil); code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
		t.Errorf("over the limit: %d", code)
	}
}
