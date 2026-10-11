package dayplan_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dayplan"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dayplan/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type fakeCalendar struct{ events []contracts.CalendarEvent }

func (f *fakeCalendar) Events(_ context.Context, from, to time.Time) ([]contracts.CalendarEvent, error) {
	var out []contracts.CalendarEvent
	for _, e := range f.events {
		if e.Start.Before(to) && e.End.After(from) {
			out = append(out, e)
		}
	}
	return out, nil
}

type fakeIssues struct{ issues []contracts.IssueRef }

func (f *fakeIssues) Get(context.Context, string) (contracts.IssueRef, error) {
	return contracts.IssueRef{}, errors.New("unused")
}
func (f *fakeIssues) Create(context.Context, contracts.CreateIssue) (contracts.IssueRef, error) {
	return contracts.IssueRef{}, errors.New("unused")
}
func (f *fakeIssues) SetStatus(context.Context, string, string) error { return nil }
func (f *fakeIssues) AttachLink(context.Context, string, contracts.IssueLink) error {
	return nil
}
func (f *fakeIssues) ListDue(_ context.Context, until time.Time) ([]contracts.IssueRef, error) {
	var out []contracts.IssueRef
	for _, i := range f.issues {
		if i.DueDate != nil && i.DueDate.Before(until) {
			out = append(out, i)
		}
	}
	return out, nil
}

type fakeReminders struct{ refs []contracts.ReminderRef }

func (f *fakeReminders) Create(context.Context, contracts.CreateReminder) (int64, error) {
	return 0, errors.New("unused")
}
func (f *fakeReminders) Upcoming(_ context.Context, until time.Time) ([]contracts.ReminderRef, error) {
	var out []contracts.ReminderRef
	for _, r := range f.refs {
		if r.At.Before(until) {
			out = append(out, r)
		}
	}
	return out, nil
}

type fakeAI struct {
	mu        sync.Mutex
	available bool
	reply     string
	err       error
	prompts   []string
}

func (f *fakeAI) Available(context.Context) bool { return f.available }
func (f *fakeAI) CompleteText(context.Context, string, string, string) (string, error) {
	return "", errors.New("unused")
}
func (f *fakeAI) CompleteJSON(_ context.Context, _, _, user string, _ json.RawMessage, out any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompts = append(f.prompts, user)
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.reply), out)
}

type fixture struct {
	env  *testutil.Env
	m    *dayplan.Module
	loc  *time.Location
	now  time.Time
	cal  *fakeCalendar
	iss  *fakeIssues
	rem  *fakeReminders
	ai   *fakeAI
	date string
}

// at gives a time on the test day (2026-11-11, a Wednesday).
func (f *fixture) at(h, m int) time.Time {
	return time.Date(2026, 11, 11, h, m, 0, 0, f.loc)
}

func (f *fixture) setNow(t time.Time) { f.now = t }

func setup(t *testing.T) *fixture {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*dayplan.Module](env.App.Deps.Registry, dayplan.ServiceKey)
	if !ok {
		t.Fatal("dayplan module not registered")
	}
	f := &fixture{env: env, m: m, loc: dayplan.Loc(m), date: "2026-11-11", cal: &fakeCalendar{}, iss: &fakeIssues{}, rem: &fakeReminders{}, ai: &fakeAI{}}
	f.now = f.at(8, 10)
	dayplan.SetNow(m, func() time.Time { return f.now })
	reg := env.App.Deps.Registry
	module.Provide[contracts.Calendar](reg, contracts.CalendarKey, f.cal)
	module.Provide[contracts.Issues](reg, contracts.IssuesKey, f.iss)
	module.Provide[contracts.Reminders](reg, contracts.RemindersKey, f.rem)
	module.Provide[contracts.LLM](reg, contracts.LLMKey, f.ai)
	return f
}

func (f *fixture) dueIssue(key, title string, daysFromToday int, priority int, status string) {
	due := f.at(0, 0).AddDate(0, 0, daysFromToday)
	f.iss.issues = append(f.iss.issues, contracts.IssueRef{Key: key, Title: title, DueDate: &due, Priority: priority, Status: status})
}

func (f *fixture) generate(t *testing.T, body map[string]any) (int, api.DayPlan) {
	t.Helper()
	var plan api.DayPlan
	status, _ := f.env.Do(http.MethodPost, "/dayplan/generate", body, &plan)
	return status, plan
}

func (f *fixture) get(t *testing.T, date string) (*api.DayPlan, []api.DayPlanSlot) {
	t.Helper()
	var out struct {
		Plan  *api.DayPlan
		Slots []api.DayPlanSlot
	}
	f.env.MustDo(http.MethodGet, "/dayplan?date="+date, nil, &out)
	return out.Plan, out.Slots
}

func titles(p api.DayPlan) string {
	var out []string
	for _, it := range p.Items {
		out = append(out, it.Title+"@"+it.Start+"-"+it.End)
	}
	return strings.Join(out, " | ")
}

func (f *fixture) notifications(t *testing.T, kind string) int {
	t.Helper()
	var out struct{ Items []struct{ Kind string } }
	f.env.MustDo(http.MethodGet, "/notifications?limit=100", nil, &out)
	n := 0
	for _, it := range out.Items {
		if it.Kind == kind {
			n++
		}
	}
	return n
}

func (f *fixture) busyMorning() {
	f.cal.events = append(f.cal.events,
		contracts.CalendarEvent{Title: "周会", Start: f.at(10, 0), End: f.at(11, 30)},
		contracts.CalendarEvent{Title: "生日", AllDay: true, Start: f.at(0, 0), End: f.at(0, 0).AddDate(0, 0, 1)},
	)
}

func TestRulePlanWorksAroundTheCalendar(t *testing.T) {
	f := setup(t)
	f.busyMorning()
	f.dueIssue("XC-1", "写周报", -2, 2, "todo")        // overdue, high: 100
	f.dueIssue("XC-2", "改登录页", 0, 0, "in_progress") // due today, started: 85
	f.dueIssue("XC-3", "已经做完", 0, 0, "done")        // left out
	f.dueIssue("XC-4", "明天才到期", 1, 0, "todo")       // left out
	f.rem.refs = []contracts.ReminderRef{
		{ID: 7, Title: "给医生打电话", At: f.at(15, 0)},
		{ID: 8, Title: "明天的提醒", At: f.at(15, 0).AddDate(0, 0, 1)},
	}
	f.ai.available = false
	status, plan := f.generate(t, nil)
	if status != http.StatusOK || plan.Status != api.Draft || plan.Source != api.DayPlanSourceRule || plan.Date != f.date {
		t.Fatalf("plan: %d %+v", status, plan)
	}
	// Best first into the earliest free stretch; the meeting 10:00-11:30 stays free of work.
	want := "写周报@09:00-10:00 | 改登录页@11:30-12:30 | 给医生打电话@15:00-15:30"
	if got := titles(plan); got != want {
		t.Fatalf("plan:\n got %s\nwant %s", got, want)
	}
	if plan.Items[2].Kind != api.DayPlanItemKindReminder || plan.Items[2].Ref != "7" || plan.Items[0].Ref != "XC-1" || !strings.Contains(plan.Items[0].Reason, "逾期") {
		t.Errorf("items: %+v", plan.Items)
	}
	// The page sees the plan and the free stretches around the meeting.
	got, slots := f.get(t, f.date)
	if got == nil || len(got.Items) != 3 {
		t.Fatalf("get: %+v", got)
	}
	if len(slots) != 2 || slots[0].Start != "09:00" || slots[0].End != "10:00" || slots[1].Start != "11:30" || slots[1].End != "18:00" {
		t.Errorf("slots: %+v", slots)
	}
	// No plan, no AI call.
	if len(f.ai.prompts) != 0 {
		t.Errorf("the AI was called: %v", f.ai.prompts)
	}
}

func TestSlotsStartAtNowToday(t *testing.T) {
	f := setup(t)
	f.setNow(f.at(13, 7))
	_, slots := f.get(t, f.date)
	if len(slots) != 1 || slots[0].Start != "13:15" {
		t.Fatalf("slots from 13:07: %+v", slots)
	}
	// Tomorrow has the whole work day.
	_, slots = f.get(t, "2026-11-12")
	if len(slots) != 1 || slots[0].Start != "09:00" || slots[0].End != "18:00" {
		t.Fatalf("tomorrow: %+v", slots)
	}
}

func TestAIPlanAndRejection(t *testing.T) {
	f := setup(t)
	f.busyMorning()
	f.dueIssue("XC-1", "写周报", -1, 0, "todo")
	f.dueIssue("XC-2", "改登录页", 0, 0, "todo")
	f.ai.available = true
	// The model picks the second candidate for the afternoon.
	f.ai.reply = `{"items":[{"candidate":1,"start":"13:00","end":"14:30","reason":"下午整块时间适合改页面"},{"candidate":0,"start":"09:00","end":"09:45","reason":"逾期的先清掉"}]}`
	_, plan := f.generate(t, nil)
	if plan.Source != api.DayPlanSourceAi || titles(plan) != "写周报@09:00-09:45 | 改登录页@13:00-14:30" || plan.Items[1].Reason != "下午整块时间适合改页面" {
		t.Fatalf("ai plan: %+v", plan)
	}
	if len(f.ai.prompts) != 1 || !strings.Contains(f.ai.prompts[0], "09:00-10:00") || !strings.Contains(f.ai.prompts[0], "11:30-18:00") || !strings.Contains(f.ai.prompts[0], "写周报") {
		t.Errorf("prompt: %q", f.ai.prompts)
	}
	// A time inside the meeting is not trusted: the rule plan is used.
	f.ai.reply = `{"items":[{"candidate":0,"start":"10:00","end":"11:00","reason":"x"}]}`
	_, plan = f.generate(t, nil)
	if plan.Source != api.DayPlanSourceRule || !strings.HasPrefix(titles(plan), "写周报@09:00-10:00") {
		t.Fatalf("rejected: %+v", plan)
	}
	// An unknown candidate, and an AI error: the same.
	f.ai.reply = `{"items":[{"candidate":9,"start":"","end":"","reason":"x"}]}`
	if _, plan = f.generate(t, nil); plan.Source != api.DayPlanSourceRule {
		t.Errorf("unknown candidate: %+v", plan)
	}
	f.ai.err = errors.New("boom")
	if _, plan = f.generate(t, nil); plan.Source != api.DayPlanSourceRule || len(plan.Items) != 2 {
		t.Errorf("ai error: %+v", plan)
	}
	// Switched off: no call at all.
	f.ai.err = nil
	n := len(f.ai.prompts)
	f.env.MustDo(http.MethodPut, "/dayplan/settings", settings(func(s *api.DayPlanSettings) { s.UseAi = false }), nil)
	if _, plan = f.generate(t, nil); plan.Source != api.DayPlanSourceRule || len(f.ai.prompts) != n {
		t.Errorf("ai off: %+v (%d prompts)", plan, len(f.ai.prompts))
	}
}

func settings(mutate func(*api.DayPlanSettings)) api.DayPlanSettings {
	s := api.DayPlanSettings{Auto: true, GenerateTime: "07:30", UseAi: true, WorkStart: "09:00", WorkEnd: "18:00", Review: true, ReviewTime: "20:30"}
	mutate(&s)
	return s
}

func TestSettingsValidation(t *testing.T) {
	f := setup(t)
	var s api.DayPlanSettings
	f.env.MustDo(http.MethodGet, "/dayplan/settings", nil, &s)
	if !s.Auto || !s.UseAi || s.GenerateTime != "07:30" || s.WorkStart != "09:00" || s.ReviewTime != "20:30" {
		t.Fatalf("defaults: %+v", s)
	}
	f.env.MustDo(http.MethodPut, "/dayplan/settings", settings(func(s *api.DayPlanSettings) { s.WorkStart, s.WorkEnd = "08:30", "17:00" }), &s)
	if s.WorkStart != "08:30" {
		t.Fatalf("saved: %+v", s)
	}
	for name, mutate := range map[string]func(*api.DayPlanSettings){
		"clock":      func(s *api.DayPlanSettings) { s.GenerateTime = "7:30" },
		"review":     func(s *api.DayPlanSettings) { s.ReviewTime = "25:00" },
		"short work": func(s *api.DayPlanSettings) { s.WorkStart, s.WorkEnd = "09:00", "09:20" },
		"reversed":   func(s *api.DayPlanSettings) { s.WorkStart, s.WorkEnd = "18:00", "09:00" },
	} {
		if status, _ := f.env.Do(http.MethodPut, "/dayplan/settings", settings(mutate), nil); status != http.StatusBadRequest {
			t.Errorf("%s: %d", name, status)
		}
	}
}

func TestEditConfirmAndReplace(t *testing.T) {
	f := setup(t)
	f.ai.available = false
	f.dueIssue("XC-1", "写周报", 0, 0, "todo")
	// Confirming nothing is refused.
	if status, _ := f.env.Do(http.MethodPost, "/dayplan/"+f.date+"/confirm", nil, nil); status != http.StatusNotFound {
		t.Errorf("confirm without plan: %d", status)
	}
	_, plan := f.generate(t, nil)
	str := func(s string) *string { return &s }
	put := func(items []api.DayPlanItemInput) (int, api.DayPlan) {
		var out api.DayPlan
		status, _ := f.env.Do(http.MethodPut, "/dayplan/"+f.date, map[string]any{"items": items}, &out)
		return status, out
	}
	status, edited := put([]api.DayPlanItemInput{
		{Title: "  写周报  ", Start: str("09:30"), End: str("10:30")},
		{Title: "去银行", Start: str("14:00"), End: str("14:45")},
		{Title: "不定时间的事"},
	})
	if status != http.StatusOK || edited.Source != api.DayPlanSourceManual || titles(edited) != "写周报@09:30-10:30 | 去银行@14:00-14:45 | 不定时间的事@-" || edited.Items[0].Kind != api.DayPlanItemKindManual {
		t.Fatalf("edit: %d %+v", status, edited)
	}
	for name, items := range map[string][]api.DayPlanItemInput{
		"empty title": {{Title: " "}},
		"overlap":     {{Title: "a", Start: str("09:00"), End: str("10:00")}, {Title: "b", Start: str("09:30"), End: str("10:30")}},
		"only start":  {{Title: "a", Start: str("09:00")}},
		"bad time":    {{Title: "a", Start: str("9点"), End: str("10点")}},
		"end before":  {{Title: "a", Start: str("10:00"), End: str("09:00")}},
		"nine items":  make([]api.DayPlanItemInput, 9),
	} {
		for i := range items {
			if items[i].Title == "" && name == "nine items" {
				items[i].Title = fmt.Sprint("事", i)
			}
		}
		if status, _ := put(items); status != http.StatusBadRequest {
			t.Errorf("%s: %d", name, status)
		}
	}
	// Confirm.
	var confirmed api.DayPlan
	f.env.MustDo(http.MethodPost, "/dayplan/"+f.date+"/confirm", nil, &confirmed)
	if confirmed.Status != api.Confirmed || confirmed.ConfirmedAt == nil || len(confirmed.Items) != 3 {
		t.Fatalf("confirmed: %+v", confirmed)
	}
	if status, _ := f.env.Do(http.MethodPost, "/dayplan/"+f.date+"/confirm", nil, nil); status != http.StatusConflict {
		t.Errorf("confirm twice: %d", status)
	}
	// A confirmed plan can still be edited and stays confirmed.
	if status, out := put([]api.DayPlanItemInput{{Title: "只剩一件", Start: str("09:00"), End: str("10:00")}}); status != http.StatusOK || out.Status != api.Confirmed || len(out.Items) != 1 {
		t.Errorf("edit confirmed: %d %+v", status, out)
	}
	// Generating over it needs replace.
	if status, _ := f.generate(t, nil); status != http.StatusConflict {
		t.Errorf("generate over confirmed: %d", status)
	}
	if status, again := f.generate(t, map[string]any{"replace": true}); status != http.StatusOK || again.Status != api.Draft || len(again.Items) != 1 || again.Items[0].Title != "写周报" {
		t.Errorf("replace: %d %+v", status, again)
	}
	// Empty plan cannot be confirmed; the past cannot be planned.
	put(nil)
	if status, _ := f.env.Do(http.MethodPost, "/dayplan/"+f.date+"/confirm", nil, nil); status != http.StatusBadRequest {
		t.Errorf("confirm empty: %d", status)
	}
	if status, _ := f.env.Do(http.MethodPost, "/dayplan/generate", map[string]any{"date": "2026-11-10"}, nil); status != http.StatusBadRequest {
		t.Errorf("past: %d", status)
	}
	if status, _ := f.env.Do(http.MethodPost, "/dayplan/generate", map[string]any{"date": "nope"}, nil); status != http.StatusBadRequest {
		t.Errorf("bad date: %d", status)
	}
	_ = plan
}

func TestDoneAndCarryOver(t *testing.T) {
	f := setup(t)
	f.ai.available = false
	f.dueIssue("XC-1", "写周报", 0, 0, "todo")
	f.dueIssue("XC-2", "改登录页", 0, 0, "todo")
	_, plan := f.generate(t, nil)
	f.env.MustDo(http.MethodPost, "/dayplan/"+f.date+"/confirm", nil, nil)
	var out api.DayPlan
	f.env.MustDo(http.MethodPut, fmt.Sprintf("/dayplan/items/%d/done", plan.Items[0].Id), map[string]any{"done": true}, &out)
	if !out.Items[0].Done || out.Items[1].Done {
		t.Fatalf("done: %+v", out.Items)
	}
	if status, _ := f.env.Do(http.MethodPut, "/dayplan/items/9999/done", map[string]any{"done": true}, nil); status != http.StatusNotFound {
		t.Errorf("unknown item: %d", status)
	}
	// Tomorrow: the issue that was not finished comes back as a leftover.
	f.setNow(f.at(8, 10).AddDate(0, 0, 1))
	f.iss.issues = nil
	_, next := f.generate(t, nil)
	if len(next.Items) != 1 || next.Items[0].Kind != api.DayPlanItemKindCarry || next.Items[0].Title != "改登录页" || next.Items[0].Ref != "XC-2" {
		t.Fatalf("carry over: %+v", next.Items)
	}
}

func TestTickDraftStartAndReview(t *testing.T) {
	f := setup(t)
	f.ai.available = false
	ctx := context.Background()
	f.dueIssue("XC-1", "写周报", 0, 0, "todo")
	tick := func(h, m int) {
		f.setNow(f.at(h, m))
		if err := dayplan.Tick(f.m, ctx); err != nil {
			t.Fatal(err)
		}
	}
	tick(7, 0) // before the draft time
	if p, _ := f.get(t, f.date); p != nil {
		t.Fatalf("early draft: %+v", p)
	}
	tick(7, 31)
	p, _ := f.get(t, f.date)
	if p == nil || p.Status != api.Draft || f.notifications(t, "dayplan.draft") != 1 {
		t.Fatalf("draft: %+v (%d notices)", p, f.notifications(t, "dayplan.draft"))
	}
	tick(7, 40) // not made twice
	if f.notifications(t, "dayplan.draft") != 1 {
		t.Fatal("the draft was announced twice")
	}
	// Nothing is reminded while it is a draft.
	tick(9, 1)
	if f.notifications(t, "dayplan.start") != 0 {
		t.Fatal("a draft must not remind")
	}
	// Confirmed: the item starts at 09:00 (time 7:31 made it from 7:45 on => from now).
	items, _ := f.get(t, f.date)
	start := items.Items[0].Start
	f.env.MustDo(http.MethodPost, "/dayplan/"+f.date+"/confirm", nil, nil)
	h, m := atoi(start[:2]), atoi(start[3:])
	tick(h, m-1)
	if f.notifications(t, "dayplan.start") != 0 {
		t.Fatal("reminded early")
	}
	tick(h, m+1)
	tick(h, m+2)
	if f.notifications(t, "dayplan.start") != 1 {
		t.Fatalf("start reminders: %d", f.notifications(t, "dayplan.start"))
	}
	// The button marks it done.
	if err := f.env.App.Deps.Notify.HandleAction(ctx, fmt.Sprintf("dayplan.done:%d", items.Items[0].Id)); err != nil {
		t.Fatal(err)
	}
	if err := f.env.App.Deps.Notify.HandleAction(ctx, "dayplan.done:x"); err == nil {
		t.Error("bad button should fail")
	}
	// Everything is done: no evening question.
	tick(20, 45)
	if f.notifications(t, "dayplan.review") != 0 {
		t.Fatal("review although all done")
	}
	// Another day with an open item gets one.
	f.env.MustDo(http.MethodPut, "/dayplan/"+f.date, map[string]any{"items": []map[string]any{{"title": "没做完的事"}}}, nil)
	tick(20, 29)
	tick(20, 30)
	tick(20, 50)
	if f.notifications(t, "dayplan.review") != 1 {
		t.Fatalf("review notices: %d", f.notifications(t, "dayplan.review"))
	}
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func TestActionsAndBriefSource(t *testing.T) {
	f := setup(t)
	f.ai.available = false
	ctx := context.Background()
	f.dueIssue("XC-1", "写周报", 0, 0, "todo")
	res, err := f.env.App.Deps.Actions.Run(ctx, "dayplan.today", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res)
	if !strings.Contains(string(b), `"plan":null`) {
		t.Errorf("no plan yet: %s", b)
	}
	if _, err := f.env.App.Deps.Actions.Run(ctx, "dayplan.generate", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	res, _ = f.env.App.Deps.Actions.Run(ctx, "dayplan.today", []byte(`{}`))
	b, _ = json.Marshal(res)
	if !strings.Contains(string(b), "写周报") {
		t.Errorf("plan: %s", b)
	}
	if _, err := f.env.App.Deps.Actions.Run(ctx, "dayplan.today", []byte(`{"date":"nope"}`)); err == nil {
		t.Error("bad date should fail")
	}
	src, ok := module.Lookup[contracts.DayPlanSource](f.env.App.Deps.Registry, contracts.DayPlanSourceKey)
	if !ok {
		t.Fatal("brief source not registered")
	}
	status, lines, err := src.Lines(ctx, f.date)
	if err != nil || status != "draft" || len(lines) != 1 || !strings.HasSuffix(lines[0], "写周报") || !strings.Contains(lines[0], "–") {
		t.Errorf("lines: %q %v %v", status, lines, err)
	}
	if status, lines, _ := src.Lines(ctx, "2030-01-01"); status != "" || len(lines) != 0 {
		t.Errorf("empty day: %q %v", status, lines)
	}
}
