package habits_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

var shanghai, _ = time.LoadLocation("Asia/Shanghai")

func at(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, shanghai) }

func setup(t *testing.T) (*testutil.Env, *habits.Module) {
	env := testutil.New(t)
	svc, ok := module.Lookup[contracts.Habits](env.App.Deps.Registry, contracts.HabitsKey)
	if !ok {
		t.Fatal("contracts.Habits not provided")
	}
	return env, svc.(*habits.Module)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func createWater(t *testing.T, env *testutil.Env) api.Habit {
	t.Helper()
	var h api.Habit
	env.MustDo(http.MethodPost, "/habits", map[string]any{
		"name": "喝水", "unit": "杯", "dailyTarget": 8, "remindMode": "interval",
		"remindIntervalMinutes": 60, "remindWindow": "09:00-21:00",
	}, &h)
	return h
}

func todayOf(t *testing.T, env *testutil.Env, id int64) api.HabitToday {
	t.Helper()
	var list []api.HabitToday
	env.MustDo(http.MethodGet, "/habits/today", nil, &list)
	for _, p := range list {
		if p.Habit.Id == id {
			return p
		}
	}
	t.Fatalf("habit %d not in today", id)
	return api.HabitToday{}
}

func TestHabitCRUDAndCheckin(t *testing.T) {
	env, _ := setup(t)
	if status, _ := env.Do(http.MethodPost, "/habits", map[string]any{"name": ""}, nil); status != http.StatusBadRequest {
		t.Fatalf("empty name: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/habits", map[string]any{"name": "x", "remindMode": "interval"}, nil); status != http.StatusBadRequest {
		t.Fatalf("interval without minutes: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/habits", map[string]any{"name": "x", "remindMode": "times", "remindTimes": []string{"8点"}}, nil); status != http.StatusBadRequest {
		t.Fatalf("bad time: %d", status)
	}
	if status, _ := env.Do(http.MethodGet, "/habits/999", nil, nil); status != http.StatusNotFound {
		t.Fatalf("missing: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/habits/999/checkin", nil, nil); status != http.StatusNotFound {
		t.Fatalf("checkin missing: %d", status)
	}
	h := createWater(t, env)
	if h.Unit != "杯" || h.DailyTarget != 8 || h.RemindMode != api.Interval {
		t.Fatalf("created: %+v", h)
	}

	goals, cancel := env.App.Deps.Bus.Subscribe("habit.goal_reached", 8)
	defer cancel()

	var res struct {
		Log   api.HabitLog
		Today api.HabitToday
	}
	env.MustDo(http.MethodPost, fmt.Sprintf("/habits/%d/checkin", h.Id), nil, &res)
	if res.Today.Done != 1 || res.Log.Source != "web" {
		t.Fatalf("checkin: %+v", res)
	}
	env.MustDo(http.MethodPost, fmt.Sprintf("/habits/%d/checkin", h.Id), map[string]any{"amount": 6, "note": "一大壶"}, &res)
	if res.Today.Done != 7 || res.Today.Reached {
		t.Fatalf("checkin 6: %+v", res.Today)
	}
	if status, _ := env.Do(http.MethodPost, fmt.Sprintf("/habits/%d/checkin", h.Id), map[string]any{"amount": -1}, nil); status != http.StatusBadRequest {
		t.Fatalf("negative amount: %d", status)
	}
	env.MustDo(http.MethodPost, fmt.Sprintf("/habits/%d/checkin", h.Id), nil, &res)
	if !res.Today.Reached || res.Today.Streak != 1 {
		t.Fatalf("reached: %+v", res.Today)
	}
	select {
	case ev := <-goals:
		if ev.Data.(map[string]any)["habitId"].(int64) != h.Id {
			t.Fatalf("goal event: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no habit.goal_reached event")
	}

	// Undo the last check-in.
	env.MustDo(http.MethodDelete, fmt.Sprintf("/habits/logs/%d", res.Log.Id), nil, nil)
	if p := todayOf(t, env, h.Id); p.Done != 7 || len(p.Logs) != 2 {
		t.Fatalf("after undo: %+v", p)
	}
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/habits/logs/%d", res.Log.Id), nil, nil); status != http.StatusNotFound {
		t.Fatalf("undo twice: %d", status)
	}

	var stats api.HabitStats
	env.MustDo(http.MethodGet, fmt.Sprintf("/habits/%d/stats?days=7", h.Id), nil, &stats)
	if len(stats.Days) != 7 || stats.Days[6].Amount != 7 || stats.Total != 7 {
		t.Fatalf("stats: %+v", stats)
	}
	if status, _ := env.Do(http.MethodGet, fmt.Sprintf("/habits/%d/stats?days=0", h.Id), nil, nil); status != http.StatusBadRequest {
		t.Fatalf("stats days=0: %d", status)
	}

	// Archive hides it from today; delete removes it.
	env.MustDo(http.MethodPatch, fmt.Sprintf("/habits/%d", h.Id), map[string]any{"archived": true, "dailyTarget": 10}, &h)
	if !h.Archived || h.DailyTarget != 10 {
		t.Fatalf("patch: %+v", h)
	}
	var list []api.HabitToday
	env.MustDo(http.MethodGet, "/habits/today", nil, &list)
	if len(list) != 0 {
		t.Fatalf("archived habit in today: %+v", list)
	}
	var all []api.Habit
	env.MustDo(http.MethodGet, "/habits?archived=true", nil, &all)
	if len(all) != 1 {
		t.Fatalf("archived list: %+v", all)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/habits/%d", h.Id), nil, nil)
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/habits/%d", h.Id), nil, nil); status != http.StatusNotFound {
		t.Fatalf("delete twice: %d", status)
	}
}

func TestProgressResetsAtLocalMidnight(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	h := createWater(t, env)
	if err := habits.CheckinAt(m, ctx, h.Id, 3, "web", at(1, 23, 59)); err != nil {
		t.Fatal(err)
	}
	before, _ := habits.TodayAt(m, ctx, at(1, 23, 59).Add(30*time.Second))
	after, _ := habits.TodayAt(m, ctx, at(2, 0, 1))
	if before[0].Done != 3 || after[0].Done != 0 || len(after[0].Logs) != 0 {
		t.Fatalf("before %v after %v", before[0].Done, after[0].Done)
	}
}

func TestReminderButtonChecksIn(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	h := createWater(t, env)
	now := time.Now().In(shanghai)
	// Pick a simulated time inside the window, today, with no check-in yet.
	sim := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, shanghai)
	if err := habits.Tick(m, ctx, sim); err != nil {
		t.Fatal(err)
	}
	var notes struct {
		Items []struct{ Kind, Title, Body string }
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) != 1 || notes.Items[0].Kind != "habit.reminder" || notes.Items[0].Title != "喝水" {
		t.Fatalf("reminder: %+v", notes.Items)
	}
	// Not again within the hour, nor outside the window.
	_ = habits.Tick(m, ctx, sim.Add(30*time.Minute))
	_ = habits.Tick(m, ctx, time.Date(now.Year(), now.Month(), now.Day(), 22, 0, 0, 0, shanghai))
	env.MustDo(http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) != 1 {
		t.Fatalf("extra reminders: %+v", notes.Items)
	}

	// "+1" from a Web Push button.
	env.MustDo(http.MethodPost, "/notify/actions", map[string]string{"actionId": fmt.Sprintf("habit.checkin:%d:1", h.Id)}, nil)
	p := todayOf(t, env, h.Id)
	if p.Done != 1 || p.Logs[0].Source != "webpush" {
		t.Fatalf("after +1: %+v", p)
	}
	// "Skip" silences the rest of the day.
	env.MustDo(http.MethodPost, "/notify/actions", map[string]string{"actionId": fmt.Sprintf("habit.skip:%d", h.Id)}, nil)
	_ = habits.Tick(m, ctx, sim.Add(3*time.Hour))
	env.MustDo(http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) != 1 {
		t.Fatalf("reminded after skip: %+v", notes.Items)
	}
	if status, _ := env.Do(http.MethodPost, "/notify/actions", map[string]string{"actionId": "habit.checkin:999:1"}, nil); status != http.StatusNotFound {
		t.Fatalf("missing habit: %d", status)
	}
}

// fakeHA records WatchEntity calls.
type fakeHA struct {
	mu      sync.Mutex
	watched []string
}

func (f *fakeHA) State(ctx context.Context, id string) (contracts.HAState, error) {
	return contracts.HAState{EntityID: id, State: "off"}, nil
}
func (f *fakeHA) CallService(context.Context, string, string, map[string]any) error { return nil }
func (f *fakeHA) WatchEntity(id string) {
	f.mu.Lock()
	f.watched = append(f.watched, id)
	f.mu.Unlock()
}
func (f *fakeHA) has(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.watched {
		if w == id {
			return true
		}
	}
	return false
}

func TestHomeAssistantCheckin(t *testing.T) {
	env, _ := setup(t)
	ha := &fakeHA{}
	module.Provide[contracts.HomeAssistant](env.App.Deps.Registry, contracts.HomeAssistantKey, ha)
	var h api.Habit
	env.MustDo(http.MethodPost, "/habits", map[string]any{"name": "刷牙", "haEntityId": "binary_sensor.toothbrush", "dailyTarget": 2}, &h)
	if !ha.has("binary_sensor.toothbrush") {
		t.Fatal("WatchEntity not called")
	}
	if status, _ := env.Do(http.MethodPatch, fmt.Sprintf("/habits/%d", h.Id), map[string]any{"haEntityId": "toothbrush"}, nil); status != http.StatusBadRequest {
		t.Fatalf("bad entity: %d", status)
	}
	bus := env.App.Deps.Bus
	publish := func(state string) {
		bus.Publish("ha.state_changed", contracts.HAState{EntityID: "binary_sensor.toothbrush", State: state, LastChanged: time.Now()})
	}
	// The fake reports "off" as the current state, so "on" is a change.
	publish("on")
	waitFor(t, "check-in from HA", func() bool { return todayOf(t, env, h.Id).Done == 1 })
	publish("on")          // same state: ignored
	publish("unavailable") // not a real change
	publish("on")          // back from unavailable, still "on": ignored
	bus.Publish("ha.state_changed", map[string]any{"entityId": "light.other", "state": "on"})
	publish("off")
	waitFor(t, "second check-in", func() bool { return todayOf(t, env, h.Id).Done == 2 })
	time.Sleep(50 * time.Millisecond)
	p := todayOf(t, env, h.Id)
	if p.Done != 2 || p.Logs[0].Source != "ha" || !p.Reached {
		t.Fatalf("after HA events: %+v", p)
	}
}

func TestWorkouts(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	bad := []map[string]any{{"weekday": 8, "title": "x", "items": []any{}}}
	if status, _ := env.Do(http.MethodPut, "/workouts/plans", bad, nil); status != http.StatusBadRequest {
		t.Fatalf("weekday 8: %d", status)
	}
	var plans []api.WorkoutPlan
	env.MustDo(http.MethodPut, "/workouts/plans", []map[string]any{
		{"weekday": 1, "title": "腿", "items": []map[string]any{{"name": "深蹲", "sets": 5, "reps": 5, "weight": 60}}},
		{"weekday": 4, "title": "胸", "items": []map[string]any{{"name": "卧推", "sets": 3, "reps": 8, "weight": 40}}},
	}, &plans)
	if len(plans) != 2 || plans[0].Title != "腿" || *plans[0].Items[0].Sets != 5 {
		t.Fatalf("plans: %+v", plans)
	}
	var log api.WorkoutLog
	env.MustDo(http.MethodPost, "/workouts/logs", map[string]any{"planId": *plans[0].Id, "durationMinutes": 45}, &log)
	if len(log.Items) != 1 || log.Items[0].Name != "深蹲" || log.DurationMinutes != 45 {
		t.Fatalf("log: %+v", log)
	}
	if status, _ := env.Do(http.MethodPost, "/workouts/logs", map[string]any{"date": "yesterday"}, nil); status != http.StatusBadRequest {
		t.Fatalf("bad date: %d", status)
	}
	var logs []api.WorkoutLog
	env.MustDo(http.MethodGet, "/workouts/logs", nil, &logs)
	if len(logs) != 1 {
		t.Fatalf("logs: %+v", logs)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/workouts/logs/%d", log.Id), nil, nil)
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/workouts/logs/%d", log.Id), nil, nil); status != http.StatusNotFound {
		t.Fatalf("delete twice: %d", status)
	}

	// Daily notice at the configured time on a day with a plan.
	env.MustDo(http.MethodPut, "/workouts/settings", map[string]any{"notifyEnabled": true, "notifyTime": "07:30"}, nil)
	if status, _ := env.Do(http.MethodPut, "/workouts/settings", map[string]any{"notifyEnabled": true, "notifyTime": "7:30"}, nil); status != http.StatusBadRequest {
		t.Fatalf("bad time: %d", status)
	}
	monday := time.Date(2026, 10, 5, 7, 30, 0, 0, shanghai)
	if err := habits.Tick(m, ctx, monday); err != nil {
		t.Fatal(err)
	}
	_ = habits.Tick(m, ctx, monday.Add(time.Minute)) // once per day
	tuesday := monday.AddDate(0, 0, 1)
	_ = habits.Tick(m, ctx, tuesday) // no plan on Tuesday
	var notes struct {
		Items []struct{ Kind, Title, Body string }
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) != 1 || notes.Items[0].Kind != "workout.plan" || notes.Items[0].Title != "今天的训练：腿" || notes.Items[0].Body != "深蹲 5×5 60kg" {
		t.Fatalf("workout notice: %+v", notes.Items)
	}
}

func TestActionsAndContract(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	h := createWater(t, env)
	run := func(name, input string) any {
		t.Helper()
		out, err := env.App.Deps.Actions.Run(ctx, name, json.RawMessage(input))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	p := run("habits.checkin", `{"name":"喝水","amount":2}`).(api.HabitToday)
	if p.Done != 2 || p.Logs[0].Source != "ai" {
		t.Fatalf("checkin action: %+v", p)
	}
	if _, err := env.App.Deps.Actions.Run(ctx, "habits.checkin", json.RawMessage(`{"name":"跑步"}`)); err == nil {
		t.Fatal("unknown habit accepted")
	}
	today := run("habits.today", `{}`).([]api.HabitToday)
	if len(today) != 1 || today[0].Done != 2 {
		t.Fatalf("today action: %+v", today)
	}
	log := run("workouts.log", `{"durationMinutes":30,"items":[{"name":"跑步"}]}`).(api.WorkoutLog)
	if log.DurationMinutes != 30 {
		t.Fatalf("workout action: %+v", log)
	}

	var svc contracts.Habits = m
	if err := svc.Checkin(ctx, h.Id, 1, "automation"); err != nil {
		t.Fatal(err)
	}
	progress, err := svc.Today(ctx)
	if err != nil || len(progress) != 1 || progress[0].Done != 3 || progress[0].Target != 8 || progress[0].Unit != "杯" {
		t.Fatalf("contract today: %+v %v", progress, err)
	}
}

// B96: the AI sets every reminder field and a template is used once.
func TestHabitActionsReminderFields(t *testing.T) {
	env, _ := setup(t)
	ctx := context.Background()
	run := func(name, input string) any {
		t.Helper()
		out, err := env.App.Deps.Actions.Run(ctx, name, json.RawMessage(input))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	h := run("habits.create", `{"name":"起来活动","remindMode":"interval","remindIntervalMinutes":45,"remindWhen":["awake"],"remindWindow":"09:00-21:00"}`).(api.Habit)
	if h.RemindMode != "interval" || h.RemindIntervalMinutes != 45 || len(h.RemindWhen) != 1 || h.RemindWhen[0] != "awake" {
		t.Fatalf("create with reminder: %+v", h)
	}
	water := run("habits.create", `{"name":"喝水","template":"water"}`).(api.Habit)
	if water.DailyTarget != 8 || water.RemindMode != "interval" {
		t.Fatalf("template defaults: %+v", water)
	}
	again := run("habits.create", `{"name":"多喝水","template":"water"}`).(map[string]any)
	if again["existing"] != true || again["habit"].(api.Habit).Id != water.Id {
		t.Fatalf("template used twice: %+v", again)
	}
	up := run("habits.update", fmt.Sprintf(`{"id":%d,"remindMode":"times","remindTimes":["09:00","21:00"]}`, h.Id)).(api.Habit)
	if up.RemindMode != "times" || len(up.RemindTimes) != 2 || up.Name != "起来活动" {
		t.Fatalf("update: %+v", up)
	}
	if _, err := env.App.Deps.Actions.Run(ctx, "habits.update", json.RawMessage(`{"remindMode":"times"}`)); err == nil {
		t.Fatal("update without id accepted")
	}
}
