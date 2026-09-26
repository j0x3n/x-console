package focus_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/focus"
	"github.com/j0x3n/x-console/backend/internal/server/modules/focus/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func setup(t *testing.T) (*testutil.Env, *focus.Module) {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*focus.Module](env.App.Deps.Registry, focus.ServiceKey)
	if !ok {
		t.Fatal("focus module not registered")
	}
	return env, m
}

type notification struct{ Kind, Title, Body, Link string }

func notifications(t *testing.T, env *testutil.Env) []notification {
	t.Helper()
	var out struct{ Items []notification }
	env.MustDo(http.MethodGet, "/notifications", nil, &out)
	return out.Items
}

func current(t *testing.T, env *testutil.Env) *api.FocusSession {
	t.Helper()
	var out struct {
		Session *api.FocusSession `json:"session"`
	}
	env.MustDo(http.MethodGet, "/focus/current", nil, &out)
	return out.Session
}

func TestStartStopAndValidation(t *testing.T) {
	env, _ := setup(t)
	if cur := current(t, env); cur != nil {
		t.Fatalf("current before start: %+v", cur)
	}
	var s api.FocusSession
	env.MustDo(http.MethodPost, "/focus/start", nil, &s)
	if s.PlannedMinutes != 25 || s.EndsAt.Sub(s.StartedAt) != 25*time.Minute || s.EndedAt != nil {
		t.Fatalf("started: %+v", s)
	}
	if status, _ := env.Do(http.MethodPost, "/focus/start", map[string]any{"minutes": 10}, nil); status != http.StatusConflict {
		t.Fatalf("second start: %d", status)
	}
	if cur := current(t, env); cur == nil || cur.Id != s.Id {
		t.Fatalf("current: %+v", cur)
	}

	var stopped api.FocusSession
	env.MustDo(http.MethodPost, fmt.Sprintf("/focus/%d/stop", s.Id), map[string]any{"note": "interrupted"}, &stopped)
	if stopped.EndedAt == nil || stopped.Completed || stopped.Note != "interrupted" || stopped.ActualSeconds > 5 {
		t.Fatalf("stopped early: %+v", stopped)
	}
	if status, _ := env.Do(http.MethodPost, fmt.Sprintf("/focus/%d/stop", s.Id), nil, nil); status != http.StatusConflict {
		t.Fatalf("stop twice: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/focus/999/stop", nil, nil); status != http.StatusNotFound {
		t.Fatalf("stop missing: %d", status)
	}
	for _, body := range []map[string]any{{"minutes": 0}, {"minutes": 181}, {"issueKey": "NOPE-1"}} {
		if status, _ := env.Do(http.MethodPost, "/focus/start", body, nil); status != http.StatusBadRequest {
			t.Fatalf("%v: %d", body, status)
		}
	}
	if status, _ := env.Do(http.MethodGet, "/focus/stats?days=0", nil, nil); status != http.StatusBadRequest {
		t.Fatalf("stats days=0: %d", status)
	}

	// A linked issue shows its title.
	issues, ok := module.Lookup[contracts.Issues](env.App.Deps.Registry, contracts.IssuesKey)
	if !ok {
		t.Fatal("no issues provider")
	}
	var project struct{ Id int64 }
	env.MustDo(http.MethodPost, "/projects", map[string]any{"key": "XC", "name": "X Console"}, &project)
	ref, err := issues.Create(context.Background(), contracts.CreateIssue{ProjectID: project.Id, Title: "Fix login"})
	if err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodPost, "/focus/start", map[string]any{"minutes": 50, "issueKey": ref.Key}, &s)
	if s.IssueKey != ref.Key || s.IssueTitle == nil || *s.IssueTitle != "Fix login" || s.PlannedMinutes != 50 {
		t.Fatalf("linked: %+v", s)
	}
}

func TestNotifiesWhenTimeIsUpEvenWithoutThePage(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	var s api.FocusSession
	env.MustDo(http.MethodPost, "/focus/start", map[string]any{"minutes": 25}, &s)

	// Not yet.
	if err := focus.Sweep(m, ctx, s.StartedAt.Add(24*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if n := notifications(t, env); len(n) != 0 {
		t.Fatalf("early notification: %+v", n)
	}
	// Time is up: exactly one notification, even if the sweep runs again.
	for range 2 {
		if err := focus.Sweep(m, ctx, s.StartedAt.Add(25*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	n := notifications(t, env)
	if len(n) != 1 || n[0].Kind != "focus.done" || n[0].Link != "/calendar/focus" {
		t.Fatalf("notifications: %+v", n)
	}
	if cur := current(t, env); cur == nil || cur.NotifiedAt == nil {
		t.Fatalf("still running and marked notified: %+v", cur)
	}

	// Nobody pressed stop: an hour later it is closed as completed, and the
	// time after the planned end does not count.
	if err := focus.Sweep(m, ctx, s.StartedAt.Add(25*time.Minute+time.Hour)); err != nil {
		t.Fatal(err)
	}
	if cur := current(t, env); cur != nil {
		t.Fatalf("stale session still open: %+v", cur)
	}
	var stats api.FocusStats
	env.MustDo(http.MethodGet, "/focus/stats?days=7", nil, &stats)
	if len(stats.Days) != 7 || stats.Completed != 1 || stats.TotalSeconds != 25*60 || len(stats.Recent) != 1 || !stats.Recent[0].Completed {
		t.Fatalf("stats: %+v", stats)
	}
	if last := stats.Days[6]; last.Date != time.Now().In(env.App.Deps.Config.Location).Format(time.DateOnly) || last.Sessions != 1 {
		t.Fatalf("today: %+v", last)
	}

	// "再来一个" from the notification starts the same session again.
	env.MustDo(http.MethodPost, "/notify/actions", map[string]string{"actionId": fmt.Sprintf("focus.again:%d", s.Id)}, nil)
	if cur := current(t, env); cur == nil || cur.PlannedMinutes != 25 || cur.Id == s.Id {
		t.Fatalf("again: %+v", cur)
	}
}

func TestRealTimerFires(t *testing.T) {
	env, m := setup(t)
	focus.SetMinute(m, 50*time.Millisecond)
	ch, cancel := env.App.Deps.Bus.Subscribe("notification.created", 4)
	defer cancel()
	var s api.FocusSession
	env.MustDo(http.MethodPost, "/focus/start", map[string]any{"minutes": 1}, &s)
	select {
	case ev := <-ch:
		raw, _ := json.Marshal(ev.Data)
		var n notification
		_ = json.Unmarshal(raw, &n)
		if n.Kind != "focus.done" {
			t.Fatalf("notification: %s", raw)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timer did not fire")
	}

	// A session whose time is up does not block the next one.
	var next api.FocusSession
	env.MustDo(http.MethodPost, "/focus/start", map[string]any{"minutes": 5}, &next)
	var stats api.FocusStats
	env.MustDo(http.MethodGet, "/focus/stats?days=1", nil, &stats)
	if stats.Completed != 1 {
		t.Fatalf("auto-closed session not completed: %+v", stats)
	}
}

func TestActions(t *testing.T) {
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
	if cur := run("focus.current", `{}`).(*api.FocusSession); cur != nil {
		t.Fatalf("current: %+v", cur)
	}
	started := run("focus.start", `{"minutes":15}`).(api.FocusSession)
	if cur := run("focus.current", `{}`).(*api.FocusSession); cur == nil || cur.Id != started.Id {
		t.Fatalf("current after start: %+v", cur)
	}
	stopped := run("focus.stop", `{"completed":true}`).(api.FocusSession)
	if stopped.Id != started.Id || !stopped.Completed {
		t.Fatalf("stop: %+v", stopped)
	}
	if _, err := env.App.Deps.Actions.Run(ctx, "focus.stop", json.RawMessage(`{}`)); err == nil {
		t.Fatal("stop without a running session")
	}
	if _, err := env.App.Deps.Actions.Run(ctx, "focus.start", json.RawMessage(`{"minutes":"x"}`)); err == nil {
		t.Fatal("bad input accepted")
	}
}
