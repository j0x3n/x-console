package checkins_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/checkins"
	"github.com/j0x3n/x-console/backend/internal/server/modules/checkins/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func setup(t *testing.T) (*testutil.Env, *checkins.Module, func(h, m int)) {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*checkins.Module](env.App.Deps.Registry, checkins.ServiceKey)
	if !ok {
		t.Fatal("checkins module not registered")
	}
	loc := checkins.Loc(m)
	at := func(h, mi int) {
		checkins.SetNow(m, func() time.Time { return time.Date(2026, 11, 11, h, mi, 0, 0, loc) })
	}
	at(9, 0)
	return env, m, at
}

func questions(t *testing.T, env *testutil.Env) []api.CheckinQuestion {
	t.Helper()
	var out struct{ Items []api.CheckinQuestion }
	env.MustDo(http.MethodGet, "/checkins/questions", nil, &out)
	return out.Items
}

func today(t *testing.T, env *testutil.Env) []api.CheckinSlot {
	t.Helper()
	var out struct{ Slots []api.CheckinSlot }
	env.MustDo(http.MethodGet, "/checkins/today", nil, &out)
	return out.Slots
}

func asks(t *testing.T, env *testutil.Env) []string {
	t.Helper()
	var out struct {
		Items []struct{ Kind, Title string }
	}
	env.MustDo(http.MethodGet, "/notifications?limit=100", nil, &out)
	var titles []string
	for _, n := range out.Items {
		if n.Kind == "checkin.ask" {
			titles = append(titles, n.Title)
		}
	}
	return titles
}

func enable(t *testing.T, env *testutil.Env) {
	t.Helper()
	env.MustDo(http.MethodPut, "/checkins/settings", api.CheckinSettings{Enabled: true}, nil)
}

func TestDefaultsAndSettings(t *testing.T) {
	env, _, _ := setup(t)
	qs := questions(t, env)
	if len(qs) != 2 || qs[0].Label != "现在精力怎么样" || len(qs[0].Times) != 2 || qs[1].Times[0] != "20:00" {
		t.Fatalf("defaults: %+v", qs)
	}
	var s api.CheckinSettings
	env.MustDo(http.MethodGet, "/checkins/settings", nil, &s)
	if s.Enabled {
		t.Fatal("the switch should start off")
	}
	enable(t, env)
	env.MustDo(http.MethodGet, "/checkins/settings", nil, &s)
	if !s.Enabled {
		t.Fatal("the switch did not stick")
	}
}

func TestQuestionCrudAndValidation(t *testing.T) {
	env, _, _ := setup(t)
	var q api.CheckinQuestion
	env.MustDo(http.MethodPost, "/checkins/questions", api.CheckinQuestionInput{Label: " 心情 ", LowLabel: "差", HighLabel: "好",
		Times: []string{"21:30", "08:00", "21:30"}, Enabled: true}, &q)
	if q.Label != "心情" || strings.Join(q.Times, ",") != "08:00,21:30" || q.SortOrder != 3 {
		t.Fatalf("created: %+v", q)
	}
	env.MustDo(http.MethodPut, "/checkins/questions/"+itoa(q.Id), api.CheckinQuestionInput{Label: "心情好不好", Times: []string{}, Enabled: false}, &q)
	if q.Label != "心情好不好" || q.Enabled || len(q.Times) != 0 {
		t.Fatalf("updated: %+v", q)
	}
	bad := []api.CheckinQuestionInput{
		{Label: "", Times: []string{}},
		{Label: "x", Times: []string{"25:00"}},
		{Label: "x", Times: []string{"1:00"}},
		{Label: "x", Times: []string{"01:00", "02:00", "03:00", "04:00", "05:00", "06:00", "07:00"}},
		{Label: strings.Repeat("字", 61), Times: []string{}},
	}
	for _, in := range bad {
		if status, _ := env.Do(http.MethodPost, "/checkins/questions", in, nil); status != http.StatusBadRequest {
			t.Errorf("%+v: status %d", in, status)
		}
	}
	if status, _ := env.Do(http.MethodPut, "/checkins/questions/9999", api.CheckinQuestionInput{Label: "x", Times: []string{}}, nil); status != http.StatusNotFound {
		t.Errorf("update unknown: %d", status)
	}
	env.MustDo(http.MethodDelete, "/checkins/questions/"+itoa(q.Id), nil, nil)
	if status, _ := env.Do(http.MethodDelete, "/checkins/questions/"+itoa(q.Id), nil, nil); status != http.StatusNotFound {
		t.Errorf("delete twice: %d", status)
	}
	for i := 0; i < 8; i++ {
		env.MustDo(http.MethodPost, "/checkins/questions", api.CheckinQuestionInput{Label: "问题", Times: []string{}}, nil)
	}
	if status, _ := env.Do(http.MethodPost, "/checkins/questions", api.CheckinQuestionInput{Label: "多了", Times: []string{}}, nil); status != http.StatusBadRequest {
		t.Errorf("11th question: %d", status)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestTodaySlotStatuses(t *testing.T) {
	env, _, at := setup(t)
	status := func() map[string]api.CheckinSlotStatus {
		out := map[string]api.CheckinSlotStatus{}
		for _, s := range today(t, env) {
			out[s.Slot] = s.Status
		}
		return out
	}
	at(9, 0)
	if got := status(); got["10:00"] != api.Upcoming || got["15:00"] != api.Upcoming || got["20:00"] != api.Upcoming {
		t.Fatalf("morning: %v", got)
	}
	at(10, 30)
	if got := status(); got["10:00"] != api.Pending || got["15:00"] != api.Upcoming {
		t.Fatalf("10:30: %v", got)
	}
	at(11, 0)
	if got := status(); got["10:00"] != api.Missed {
		t.Fatalf("11:00: %v", got)
	}
	qs := questions(t, env)
	slot := "10:00"
	env.MustDo(http.MethodPut, "/checkins/answers", api.CheckinAnswerInput{QuestionId: qs[0].Id, Value: 4, Slot: &slot}, nil)
	var found *api.CheckinSlot
	for _, s := range today(t, env) {
		if s.Slot == "10:00" {
			s := s
			found = &s
		}
	}
	if found == nil || found.Status != api.Answered || found.Value == nil || *found.Value != 4 {
		t.Fatalf("answered slot: %+v", found)
	}
}

func TestAnswerRules(t *testing.T) {
	env, _, _ := setup(t)
	q := questions(t, env)[0]
	var a api.CheckinAnswer
	env.MustDo(http.MethodPut, "/checkins/answers", api.CheckinAnswerInput{QuestionId: q.Id, Value: 3}, &a)
	if a.Slot != "manual" || a.Date != "2026-11-11" || a.Source != api.Web || a.Value != 3 {
		t.Fatalf("free answer: %+v", a)
	}
	var again api.CheckinAnswer
	env.MustDo(http.MethodPut, "/checkins/answers", api.CheckinAnswerInput{QuestionId: q.Id, Value: 5}, &again)
	if again.Id != a.Id || again.Value != 5 {
		t.Fatalf("overwrite: %+v then %+v", a, again)
	}
	future, old, wrongSlot := "2026-11-12", "2024-01-01", "11:11"
	cases := []api.CheckinAnswerInput{
		{QuestionId: q.Id, Value: 0}, {QuestionId: q.Id, Value: 6}, {QuestionId: 9999, Value: 3},
		{QuestionId: q.Id, Value: 3, Date: &future}, {QuestionId: q.Id, Value: 3, Date: &old}, {QuestionId: q.Id, Value: 3, Slot: &wrongSlot},
	}
	wants := []int{400, 400, 404, 400, 400, 400}
	for i, in := range cases {
		if status, _ := env.Do(http.MethodPut, "/checkins/answers", in, nil); status != wants[i] {
			t.Errorf("case %d: status %d, want %d", i, status, wants[i])
		}
	}
	yesterday := "2026-11-10"
	env.MustDo(http.MethodPut, "/checkins/answers", api.CheckinAnswerInput{QuestionId: q.Id, Value: 2, Date: &yesterday}, nil)
}

func TestAskOncePerSlot(t *testing.T) {
	env, m, at := setup(t)
	ctx := context.Background()
	// Switch off: nothing is asked.
	at(10, 1)
	if err := checkins.AskTick(m, ctx); err != nil {
		t.Fatal(err)
	}
	if got := asks(t, env); len(got) != 0 {
		t.Fatalf("asked while off: %v", got)
	}
	enable(t, env)
	// Before the time: nothing. After: once, however often the tick runs.
	at(9, 59)
	_ = checkins.AskTick(m, ctx)
	if got := asks(t, env); len(got) != 0 {
		t.Fatalf("asked early: %v", got)
	}
	at(10, 1)
	for i := 0; i < 3; i++ {
		if err := checkins.AskTick(m, ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := asks(t, env); len(got) != 1 || got[0] != "现在精力怎么样" {
		t.Fatalf("asked: %v", got)
	}
	// 70 minutes late is too late for the 15:00 slot.
	at(16, 10)
	_ = checkins.AskTick(m, ctx)
	if got := asks(t, env); len(got) != 1 {
		t.Fatalf("asked after the window: %v", got)
	}
	// A slot answered on the page before the question comes is not asked.
	q := questions(t, env)[1]
	slot := "20:00"
	env.MustDo(http.MethodPut, "/checkins/answers", api.CheckinAnswerInput{QuestionId: q.Id, Value: 2, Slot: &slot}, nil)
	at(20, 5)
	_ = checkins.AskTick(m, ctx)
	if got := asks(t, env); len(got) != 1 {
		t.Fatalf("asked an answered slot: %v", got)
	}
}

func TestButtonRecordsAnswer(t *testing.T) {
	env, m, at := setup(t)
	ctx := context.Background()
	enable(t, env)
	at(10, 1)
	_ = checkins.AskTick(m, ctx)
	q := questions(t, env)[0]
	id := "checkin.answer:" + itoa(q.Id) + ":2026-11-11:1000:4"
	if err := env.App.Deps.Notify.HandleAction(ctx, id); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, s := range today(t, env) {
		if s.Slot == "10:00" && s.Status == api.Answered && s.Value != nil && *s.Value == 4 {
			found = true
		}
	}
	if !found {
		t.Fatalf("slots: %+v", today(t, env))
	}
	// Pressing another button overwrites.
	if err := env.App.Deps.Notify.HandleAction(ctx, "checkin.answer:"+itoa(q.Id)+":2026-11-11:1000:2"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"checkin.answer:1:2026-11-11:1000", "checkin.answer:x:2026-11-11:1000:3", "checkin.answer:1:nope:1000:3", "checkin.other:1:2026-11-11:1000:3", "checkin.answer:1:2026-11-11:100:3", "checkin.answer:1:2026-11-11:1000:9"} {
		if err := env.App.Deps.Notify.HandleAction(ctx, bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}

func TestSeriesAndContract(t *testing.T) {
	env, m, at := setup(t)
	q := questions(t, env)[0]
	put := func(date string, v int, slot string) {
		in := api.CheckinAnswerInput{QuestionId: q.Id, Value: v, Date: &date}
		if slot != "" {
			in.Slot = &slot
		}
		env.MustDo(http.MethodPut, "/checkins/answers", in, nil)
	}
	put("2026-11-09", 2, "")
	put("2026-11-10", 4, "10:00")
	put("2026-11-10", 2, "15:00")
	put("2026-11-11", 5, "")
	at(12, 0)
	var out struct {
		From, To string
		Items    []api.CheckinSeries
	}
	env.MustDo(http.MethodGet, "/checkins/series?days=3", nil, &out)
	if out.From != "2026-11-09" || out.To != "2026-11-11" || len(out.Items) != 2 {
		t.Fatalf("series: %+v", out)
	}
	s := out.Items[0]
	if s.Count != 4 || s.Average != 3.25 || len(s.Points) != 3 || s.Points[1].Average != 3 || s.Points[1].Count != 2 {
		t.Fatalf("points: %+v", s)
	}
	if out.Items[1].Count != 0 || len(out.Items[1].Points) != 0 {
		t.Fatalf("empty question: %+v", out.Items[1])
	}
	env.MustDo(http.MethodGet, "/checkins/series?days=1", nil, &out)
	if out.Items[0].Count != 1 {
		t.Fatalf("one day: %+v", out.Items[0])
	}
	svc, ok := module.Lookup[contracts.CheckinSeries](env.App.Deps.Registry, contracts.CheckinSeriesKey)
	if !ok {
		t.Fatal("contract missing")
	}
	loc := checkins.Loc(m)
	days, err := svc.Daily(context.Background(), time.Date(2026, 11, 10, 0, 0, 0, 0, loc), time.Date(2026, 11, 11, 23, 0, 0, 0, loc))
	if err != nil || len(days) != 2 || days[0].Date != "2026-11-10" || days[0].Average != 3 || days[1].Average != 5 {
		t.Fatalf("daily: %+v %v", days, err)
	}
}

func TestActions(t *testing.T) {
	env, _, _ := setup(t)
	q := questions(t, env)[1]
	if _, err := env.App.Deps.Actions.Run(context.Background(), "checkins.answer", []byte(`{"questionId":`+itoa(q.Id)+`,"value":2}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.App.Deps.Actions.Run(context.Background(), "checkins.answer", []byte(`{"questionId":`+itoa(q.Id)+`,"value":9}`)); err == nil {
		t.Fatal("score 9 should fail")
	}
	res, err := env.App.Deps.Actions.Run(context.Background(), "checkins.series", []byte(`{"days":7}`))
	if err != nil || res == nil {
		t.Fatalf("series action: %v %v", res, err)
	}
}
