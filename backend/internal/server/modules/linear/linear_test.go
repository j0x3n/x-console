package linear_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/linear/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// The module is registered in app/modules.go, so testutil.New(t) includes it.

type localIssue struct {
	Key            string  `json:"key"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	Status         string  `json:"status"`
	Priority       int     `json:"priority"`
	DueDate        *string `json:"dueDate"`
	ExternalSource string  `json:"externalSource"`
	ExternalID     string  `json:"externalId"`
}

func errCode(raw []byte) string {
	var e struct{ Code string }
	_ = json.Unmarshal(raw, &e)
	return e.Code
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// setup starts a server and a fake Linear, creates project XC and maps the
// ENG team to it. ENG-1 is open, ENG-2 is done.
func setup(t *testing.T) (*testutil.Env, *fakeLinear, time.Time) {
	t.Helper()
	env := testutil.New(t)
	ln := newFakeLinear(t)
	base := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Millisecond)
	ln.set(func(f *fakeLinear) {
		f.issues["lin-1"] = &fakeIssue{ID: "lin-1", Identifier: "ENG-1", Title: "Ship sync", Description: "Both ways",
			Priority: 2, DueDate: "2026-10-01", StateID: "s-todo", UpdatedAt: base}
		f.issues["lin-2"] = &fakeIssue{ID: "lin-2", Identifier: "ENG-2", Title: "Old work", Priority: 0,
			StateID: "s-done", UpdatedAt: base}
	})
	var p struct{ ID int64 }
	env.MustDo(http.MethodPost, "/projects", map[string]any{"key": "XC", "name": "X Console"}, &p)
	env.Elevate()
	env.MustDo(http.MethodPut, "/linear/config", map[string]any{
		"apiKey": ln.key, "apiUrl": ln.URL(),
		"mappings": []any{map[string]any{"teamId": teamID, "teamKey": "ENG", "teamName": "Engineering", "projectId": p.ID}},
	}, nil)
	return env, ln, base
}

func syncNow(t *testing.T, env *testutil.Env) api.LinearSyncResult {
	t.Helper()
	var st api.LinearStatus
	env.MustDo(http.MethodPost, "/linear/sync", nil, &st)
	if st.LastSync == nil {
		t.Fatalf("no sync result: %+v", st)
	}
	return *st.LastSync
}

func getIssue(t *testing.T, env *testutil.Env, key string) localIssue {
	t.Helper()
	var is localIssue
	env.MustDo(http.MethodGet, "/issues/"+key, nil, &is)
	return is
}

func TestConfig(t *testing.T) {
	env := testutil.New(t)
	ln := newFakeLinear(t)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/linear/teams"}, {http.MethodPost, "/linear/sync"}, {http.MethodPost, "/linear/test"},
	} {
		code, raw := env.Do(c.method, c.path, nil, nil)
		if code != http.StatusPreconditionFailed || errCode(raw) != "integration_not_configured" {
			t.Fatalf("%s %s: %d %s", c.method, c.path, code, raw)
		}
	}
	code, raw := env.Do(http.MethodPut, "/linear/config", map[string]any{"apiKey": ln.key, "mappings": []any{}}, nil)
	if code != http.StatusForbidden || errCode(raw) != "elevation_required" {
		t.Fatalf("put without elevation: %d %s", code, raw)
	}
	env.Elevate()
	var cfg api.LinearConfig
	env.MustDo(http.MethodPut, "/linear/config", map[string]any{"apiKey": ln.key, "apiUrl": ln.URL(), "mappings": []any{}}, &cfg)
	if !cfg.HasKey || cfg.ApiKey != "••••••••5678" || cfg.ApiUrl != ln.URL() {
		t.Fatalf("config: %+v", cfg)
	}
	var res api.LinearTestResult
	env.MustDo(http.MethodPost, "/linear/test", nil, &res)
	if !res.Ok || res.User == nil || *res.User != "Jo" {
		t.Fatalf("test: %+v", res)
	}
	env.MustDo(http.MethodPost, "/linear/test", map[string]any{"apiKey": "lin_api_wrong_000000"}, &res)
	if res.Ok || res.Message == nil || !strings.Contains(*res.Message, "API key") {
		t.Fatalf("wrong key: %+v", res)
	}
	var teams []api.LinearTeam
	env.MustDo(http.MethodGet, "/linear/teams", nil, &teams)
	if len(teams) != 2 || teams[0].Key != "ENG" {
		t.Fatalf("teams: %+v", teams)
	}
	code, raw = env.Do(http.MethodPut, "/linear/config", map[string]any{"mappings": []any{
		map[string]any{"teamId": teamID, "projectId": 1}, map[string]any{"teamId": "team-ops", "projectId": 1},
	}}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("duplicate project: %d %s", code, raw)
	}
	var st api.LinearStatus
	env.MustDo(http.MethodGet, "/linear/status", nil, &st)
	if !st.Configured || st.MappingCount != 0 || st.LastSync != nil {
		t.Fatalf("status: %+v", st)
	}
}

func TestPullCreatesAndUpdates(t *testing.T) {
	env, ln, base := setup(t)
	res := syncNow(t, env)
	if !res.Ok || res.Created != 1 || res.Pulled != 0 || res.Pushed != 0 {
		t.Fatalf("first sync: %+v", res)
	}
	is := getIssue(t, env, "XC-1")
	if is.Title != "Ship sync" || is.Description != "Both ways" || is.Status != "todo" || is.Priority != 2 ||
		is.DueDate == nil || *is.DueDate != "2026-10-01" || is.ExternalSource != "linear" || is.ExternalID != "lin-1" {
		t.Fatalf("imported: %+v", is)
	}
	if code, _ := env.Do(http.MethodGet, "/issues/XC-2", nil, nil); code != http.StatusNotFound {
		t.Fatal("finished Linear issues should not be imported")
	}

	// Title and state changed in Linear.
	ln.set(func(f *fakeLinear) {
		i := f.issues["lin-1"]
		i.Title, i.StateID, i.DueDate, i.UpdatedAt = "Ship two-way sync", "s-prog", "", base.Add(time.Hour)
	})
	res = syncNow(t, env)
	if !res.Ok || res.Pulled != 1 || res.Created != 0 || res.Pushed != 0 || res.Conflicts != 0 {
		t.Fatalf("second sync: %+v", res)
	}
	is = getIssue(t, env, "XC-1")
	if is.Title != "Ship two-way sync" || is.Status != "in_progress" || is.DueDate != nil {
		t.Fatalf("updated: %+v", is)
	}
	// A started state named "In Review" becomes in_review.
	ln.set(func(f *fakeLinear) {
		f.issues["lin-1"].StateID, f.issues["lin-1"].UpdatedAt = "s-review", base.Add(2*time.Hour)
	})
	if res = syncNow(t, env); res.Pulled != 1 {
		t.Fatalf("review sync: %+v", res)
	}
	if is = getIssue(t, env, "XC-1"); is.Status != "in_review" {
		t.Fatalf("review status: %+v", is)
	}
	// Nothing changed: nothing happens, and nothing was echoed back.
	res = syncNow(t, env)
	if res.Pulled != 0 || res.Pushed != 0 || res.Created != 0 {
		t.Fatalf("idle sync: %+v", res)
	}
	time.Sleep(200 * time.Millisecond)
	if n := ln.mutationCount(); n != 0 {
		t.Fatalf("pulled changes were pushed back: %d mutations", n)
	}
}

func TestLocalChangePushes(t *testing.T) {
	env, ln, _ := setup(t)
	syncNow(t, env)

	env.MustDo(http.MethodPatch, "/issues/XC-1", map[string]any{"status": "in_progress"}, nil)
	waitFor(t, "status push", func() bool { return ln.issue("lin-1").StateID == "s-prog" })
	if m := ln.lastMutation(); len(m) != 1 || m["stateId"] != "s-prog" {
		t.Fatalf("mutation: %+v", m)
	}
	env.MustDo(http.MethodPatch, "/issues/XC-1", map[string]any{"status": "in_review", "title": "Ship it", "priority": 1}, nil)
	waitFor(t, "review push", func() bool { return ln.issue("lin-1").StateID == "s-review" })
	got := ln.issue("lin-1")
	if got.Title != "Ship it" || got.Priority != 1 {
		t.Fatalf("remote after push: %+v", got)
	}
	time.Sleep(200 * time.Millisecond)
	count := ln.mutationCount()
	if count != 2 {
		t.Fatalf("want 2 mutations (updated and status_changed pushed once), got %d", count)
	}
	// The next pull sees our own change and leaves the local issue alone:
	// in_review stays in_review although Linear only says "started".
	res := syncNow(t, env)
	if res.Pulled != 0 || res.Pushed != 0 || res.Conflicts != 0 {
		t.Fatalf("sync after push: %+v", res)
	}
	if is := getIssue(t, env, "XC-1"); is.Status != "in_review" || is.Title != "Ship it" {
		t.Fatalf("local after sync: %+v", is)
	}
	if ln.mutationCount() != count {
		t.Fatal("sync echoed the change")
	}

	// A local issue without a Linear id is never pushed.
	var p struct{ ID int64 }
	env.MustDo(http.MethodGet, "/projects/1", nil, &p)
	env.MustDo(http.MethodPost, "/projects/1/issues", map[string]any{"title": "Local only"}, nil)
	env.MustDo(http.MethodPatch, "/issues/XC-2", map[string]any{"status": "done"}, nil)
	time.Sleep(200 * time.Millisecond)
	if ln.mutationCount() != count {
		t.Fatal("local-only issue was pushed")
	}
}

// editWhileLinearDown changes the local issue while pushes fail, so both
// sides end up changed before the next sync.
func editWhileLinearDown(t *testing.T, env *testutil.Env, ln *fakeLinear, body map[string]any) {
	t.Helper()
	ln.set(func(f *fakeLinear) { f.failing = true; f.failures = 0 })
	env.MustDo(http.MethodPatch, "/issues/XC-1", body, nil)
	waitFor(t, "failed push", func() bool {
		ln.mu.Lock()
		defer ln.mu.Unlock()
		return ln.failures > 0
	})
	time.Sleep(50 * time.Millisecond)
	ln.set(func(f *fakeLinear) { f.failing = false })
	var st api.LinearStatus
	env.MustDo(http.MethodGet, "/linear/status", nil, &st)
	if st.LastSync == nil || st.LastSync.Ok || len(st.LastSync.Errors) == 0 {
		t.Fatalf("push error not recorded: %+v", st)
	}
}

func TestConflictNewerWins(t *testing.T) {
	t.Run("linear newer", func(t *testing.T) {
		env, ln, _ := setup(t)
		syncNow(t, env)
		editWhileLinearDown(t, env, ln, map[string]any{"title": "Local title"})
		ln.set(func(f *fakeLinear) {
			f.issues["lin-1"].Title = "Remote title"
			f.issues["lin-1"].UpdatedAt = time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
		})
		res := syncNow(t, env)
		if !res.Ok || res.Conflicts != 1 || res.Pulled != 1 || res.Pushed != 0 {
			t.Fatalf("sync: %+v", res)
		}
		if is := getIssue(t, env, "XC-1"); is.Title != "Remote title" {
			t.Fatalf("local: %+v", is)
		}
		if ln.mutationCount() != 0 {
			t.Fatal("older local change was pushed")
		}
	})
	t.Run("local newer", func(t *testing.T) {
		env, ln, base := setup(t)
		syncNow(t, env)
		editWhileLinearDown(t, env, ln, map[string]any{"title": "Local title", "priority": 4})
		ln.set(func(f *fakeLinear) {
			f.issues["lin-1"].Title = "Remote title"
			f.issues["lin-1"].UpdatedAt = base.Add(time.Hour) // after the last sync, before the local edit
		})
		res := syncNow(t, env)
		if !res.Ok || res.Conflicts != 1 || res.Pushed != 1 || res.Pulled != 0 {
			t.Fatalf("sync: %+v", res)
		}
		if got := ln.issue("lin-1"); got.Title != "Local title" || got.Priority != 4 {
			t.Fatalf("remote: %+v", got)
		}
		if is := getIssue(t, env, "XC-1"); is.Title != "Local title" {
			t.Fatalf("local: %+v", is)
		}
		// Settled: the next sync does nothing.
		if res := syncNow(t, env); res.Pulled+res.Pushed+res.Conflicts != 0 || !res.Ok {
			t.Fatalf("settled: %+v", res)
		}
	})
}

func TestMissedPushIsRetriedBySync(t *testing.T) {
	env, ln, _ := setup(t)
	syncNow(t, env)
	editWhileLinearDown(t, env, ln, map[string]any{"description": "Changed offline"})
	res := syncNow(t, env)
	if !res.Ok || res.Pushed != 1 {
		t.Fatalf("sync: %+v", res)
	}
	if got := ln.issue("lin-1"); got.Description != "Changed offline" {
		t.Fatalf("remote: %+v", got)
	}
}
