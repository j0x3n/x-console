package habits_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type fakePresence struct {
	mu    sync.Mutex
	state map[string]contracts.HostPresence
}

func (f *fakePresence) State(id string) contracts.HostPresence {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state[id]
}

func (f *fakePresence) Subscribe() (<-chan contracts.HostPresence, func()) {
	ch := make(chan contracts.HostPresence)
	return ch, func() { close(ch) }
}

func (f *fakePresence) set(id string, p contracts.HostPresence) {
	f.mu.Lock()
	f.state[id] = p
	f.mu.Unlock()
}

func putSchedule(env *testutil.Env) {
	env.MustDo(http.MethodPut, "/habits/schedule", map[string]any{
		"workDays": []int{1, 2, 3, 4, 5}, "wakeTime": "12:00", "sleepTime": "03:30",
		"workStart": "14:00", "workEnd": "23:00", "timezone": "Asia/Shanghai", "idleMinutes": 5,
	}, nil)
}

func reminderCount(env *testutil.Env) int {
	var out struct{ Items []struct{ Kind string } }
	env.MustDo(http.MethodGet, "/notifications", nil, &out)
	n := 0
	for _, item := range out.Items {
		if item.Kind == "habit.reminder" {
			n++
		}
	}
	return n
}

func TestScheduleAPIAndHabitDefaults(t *testing.T) {
	env, _ := setup(t)
	putSchedule(env)
	var schedule map[string]any
	env.MustDo(http.MethodGet, "/habits/schedule", nil, &schedule)
	if schedule["timezone"] != "Asia/Shanghai" || schedule["wakeTime"] != "12:00" || schedule["idleMinutes"] != float64(5) {
		t.Fatalf("schedule: %+v", schedule)
	}
	for _, change := range []map[string]any{{"timezone": "Invalid/Zone"}, {"idleMinutes": 0}, {"workDays": []int{8}}, {"workEnd": ""}, {"sleepTime": "12:00"}} {
		body := map[string]any{"workDays": []int{1}, "wakeTime": "12:00", "sleepTime": "03:30", "workStart": "14:00", "workEnd": "23:00", "timezone": "UTC", "idleMinutes": 5}
		for key, value := range change {
			body[key] = value
		}
		if status, _ := env.Do(http.MethodPut, "/habits/schedule", body, nil); status != http.StatusBadRequest {
			t.Fatalf("invalid schedule %v: %d", change, status)
		}
	}
	var h map[string]any
	env.MustDo(http.MethodPost, "/habits", map[string]any{"name": "旧习惯", "remindMode": "interval", "remindIntervalMinutes": 60, "remindWindow": "09:00-21:00"}, &h)
	if !reflect.DeepEqual(h["remindWhen"], []any{"window"}) || h["remindOnHost"] != false || h["nextRemindAt"] == nil {
		t.Fatalf("legacy defaults: %+v", h)
	}
	for _, body := range []map[string]any{
		{"name": "x", "remindWhen": []string{"invalid"}},
		{"name": "x", "remindMode": "interval", "remindIntervalMinutes": 5, "remindWhen": []string{"active"}},
		{"name": "x", "activeHostIds": []string{"missing-host"}},
		{"name": "x", "template": "invalid"},
	} {
		if status, _ := env.Do(http.MethodPost, "/habits", body, nil); status != http.StatusBadRequest {
			t.Fatalf("invalid habit %v: %d", body, status)
		}
	}
}

func TestScheduleTimezoneMatchesProgressAndWorkouts(t *testing.T) {
	env, m := setup(t)
	env.MustDo(http.MethodPut, "/habits/schedule", map[string]any{"workDays": []int{1, 2, 3, 4, 5}, "wakeTime": "12:00", "sleepTime": "03:30", "timezone": "America/Los_Angeles", "idleMinutes": 5}, nil)
	var h struct{ Id int64 }
	env.MustDo(http.MethodPost, "/habits", map[string]any{"name": "时区习惯", "dailyTarget": 8}, &h)
	ctx := context.Background()
	now := time.Date(2026, 10, 6, 2, 0, 0, 0, shanghai)
	if err := habits.CheckinAt(m, ctx, h.Id, 1, "web", now.Add(-4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	list, err := habits.TodayAt(m, ctx, now)
	if err != nil || len(list) != 1 || list[0].Done != 1 {
		t.Fatalf("timezone progress %+v %v", list, err)
	}
}

func TestHiddenHostCannotBeSelectedOrExposed(t *testing.T) {
	env, _ := setup(t)
	id := env.Agent("desktop", nil, nil)
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "health-vault-pass"}, nil)
	env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"pc"}}, nil)
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	var presence []contracts.HostPresence
	env.MustDo(http.MethodGet, "/habits/presence", nil, &presence)
	for _, p := range presence {
		if p.HostID == id {
			t.Fatal("hidden desktop exposed")
		}
	}
	if status, _ := env.Do(http.MethodPost, "/habits", map[string]any{"name": "护眼", "template": "eyes", "activeHostIds": []string{id}}, nil); status != http.StatusBadRequest {
		t.Fatalf("hidden desktop selected: %d", status)
	}
}

func TestScheduledAwakeCrossMidnight(t *testing.T) {
	env, m := setup(t)
	putSchedule(env)
	env.MustDo(http.MethodPost, "/habits", map[string]any{"name": "喝水", "dailyTarget": 8, "remindMode": "interval", "remindIntervalMinutes": 60, "remindWhen": []string{"awake"}}, nil)
	ctx := context.Background()
	if err := habits.Tick(m, ctx, at(6, 2, 0)); err != nil {
		t.Fatal(err)
	}
	if n := reminderCount(env); n != 1 {
		t.Fatalf("awake reminder: %d", n)
	}
	if err := habits.Tick(m, ctx, at(6, 4, 0)); err != nil {
		t.Fatal(err)
	}
	if err := habits.Tick(m, ctx, at(6, 8, 0)); err != nil {
		t.Fatal(err)
	}
	if n := reminderCount(env); n != 1 {
		t.Fatalf("sleep reminders: %d", n)
	}
}

func TestStructuredHostReminderRoutesOnlySelectedAgent(t *testing.T) {
	env, m := setup(t)
	shown := make(chan string, 4)
	id := env.Agent("desktop", []string{"notify.show"}, func(c *conn.Client) {
		c.Handle("notify.show", func(_ context.Context, raw json.RawMessage) (any, error) {
			var input struct{ Title, Body string }
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			shown <- input.Title + "\n" + input.Body
			return map[string]bool{"shown": true}, nil
		})
	})
	legacy := env.Agent("desktop", nil, nil)
	title, body := "护眼 $(test)", "文本 `test` <>&"
	habits.SendHostReminder(m, context.Background(), []string{id, id, legacy, "missing"}, title, body)
	select {
	case got := <-shown:
		if got != title+"\n"+body {
			t.Fatalf("notification text: %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no host notification")
	}
	select {
	case <-shown:
		t.Fatal("duplicate notification")
	default:
	}
}

type presenceTestModule struct{}

func (presenceTestModule) Name() string     { return "health-test-presence" }
func (presenceTestModule) Mount(chi.Router) {}

func setupWithPresence(t *testing.T) (*testutil.Env, *habits.Module, *fakePresence) {
	t.Helper()
	presence := &fakePresence{state: map[string]contracts.HostPresence{}}
	env := testutil.New(t, func(d *module.Deps) (module.Module, error) {
		module.Provide[contracts.Presence](d.Registry, contracts.PresenceKey, presence)
		return presenceTestModule{}, nil
	})
	service, _ := module.Lookup[contracts.Habits](env.App.Deps.Registry, contracts.HabitsKey)
	return env, service.(*habits.Module), presence
}

func TestHealthReminderSystemNotificationAndSnooze(t *testing.T) {
	env, m, presence := setupWithPresence(t)
	putSchedule(env)
	shown := make(chan struct{ Title, Body string }, 4)
	id := env.Agent("desktop", []string{"presence", "notify.show"}, func(c *conn.Client) {
		c.Handle("notify.show", func(_ context.Context, raw json.RawMessage) (any, error) {
			var input struct{ Title, Body string }
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			shown <- input
			return map[string]bool{"shown": true}, nil
		})
	})
	presence.set(id, contracts.HostPresence{HostID: id, Online: true, Known: false})
	var h struct{ Id int64 }
	env.MustDo(http.MethodPost, "/habits", map[string]any{"name": "护眼", "dailyTarget": 10, "remindMode": "interval", "remindIntervalMinutes": 5, "remindWhen": []string{"active"}, "activeHostIds": []string{id}, "remindOnHost": true, "template": "eyes"}, &h)
	now := time.Now().Truncate(time.Minute)
	ctx := context.Background()
	if err := habits.Tick(m, ctx, now.Add(-5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := habits.Tick(m, ctx, now); err != nil {
		t.Fatal(err)
	}
	select {
	case input := <-shown:
		if input.Title != "护眼" || input.Body == "" {
			t.Fatalf("system notification: %+v", input)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no system notification")
	}
	env.MustDo(http.MethodPost, "/notify/actions", map[string]string{"actionId": fmt.Sprintf("habit.snooze:%d", h.Id)}, nil)
	if err := habits.Tick(m, ctx, now.Add(9*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if n := reminderCount(env); n != 1 {
		t.Fatalf("snooze too early: %d", n)
	}
	if err := habits.Tick(m, ctx, now.Add(11*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if n := reminderCount(env); n != 2 {
		t.Fatalf("snooze did not remind: %d", n)
	}
}

func TestFixedHealthReminderDoesNotCatchUp(t *testing.T) {
	env, m := setup(t)
	putSchedule(env)
	env.MustDo(http.MethodPost, "/habits", map[string]any{"name": "吃药", "template": "medicine"}, nil)
	ctx := context.Background()
	if err := habits.Tick(m, ctx, at(6, 9, 2)); err != nil {
		t.Fatal(err)
	}
	if n := reminderCount(env); n != 0 {
		t.Fatalf("fixed reminder caught up: %d", n)
	}
	if err := habits.Tick(m, ctx, at(6, 21, 0)); err != nil {
		t.Fatal(err)
	}
	if n := reminderCount(env); n != 1 {
		t.Fatalf("fixed reminder missing: %d", n)
	}
}

func TestActiveUnknownResetAndCheckin(t *testing.T) {
	env, m, presence := setupWithPresence(t)
	putSchedule(env)
	id := env.Agent("desktop", []string{"presence"}, nil)
	presence.set(id, contracts.HostPresence{HostID: id, Online: true, State: "unknown", Known: false})
	var h struct{ Id int64 }
	env.MustDo(http.MethodPost, "/habits", map[string]any{"name": "护眼", "dailyTarget": 10, "remindMode": "interval", "remindIntervalMinutes": 5, "remindWhen": []string{"active"}, "activeHostIds": []string{id}, "template": "eyes"}, &h)
	ctx := context.Background()
	start := at(6, 15, 0)
	for _, minute := range []int{0, 4, 5} {
		if err := habits.Tick(m, ctx, start.Add(time.Duration(minute)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if n := reminderCount(env); n != 1 {
		t.Fatalf("unknown fallback: %d", n)
	}
	presence.set(id, contracts.HostPresence{HostID: id, Online: true, Known: true, State: "idle", IdleSeconds: new(int64(600))})
	if err := habits.Tick(m, ctx, start.Add(8*time.Minute)); err != nil {
		t.Fatal(err)
	}
	presence.set(id, contracts.HostPresence{HostID: id, Online: true, Known: true, State: "active", IdleSeconds: new(int64(0))})
	if err := habits.Tick(m, ctx, start.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if n := reminderCount(env); n != 1 {
		t.Fatalf("caught up after absence: %d", n)
	}
	if err := habits.CheckinAt(m, ctx, h.Id, 1, "web", start.Add(12*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := habits.Tick(m, ctx, start.Add(15*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if n := reminderCount(env); n != 1 {
		t.Fatalf("checkin did not reset: %d", n)
	}
	if err := habits.Tick(m, ctx, start.Add(17*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if n := reminderCount(env); n != 2 {
		t.Fatalf("checkin next reminder: %d", n)
	}
	var got map[string]any
	env.MustDo(http.MethodGet, fmt.Sprintf("/habits/%d", h.Id), nil, &got)
	if got["nextRemindAt"] == nil {
		t.Fatalf("missing next reminder: %+v", got)
	}
}
