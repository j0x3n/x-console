package github_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func notifications(t *testing.T, env *testutil.Env) []string {
	t.Helper()
	var out struct {
		Items []struct{ Kind string } `json:"items"`
	}
	env.MustDo(http.MethodGet, "/notifications?limit=200", nil, &out)
	kinds := []string{}
	for _, n := range out.Items {
		if strings.HasPrefix(n.Kind, "github.") {
			kinds = append(kinds, n.Kind)
		}
	}
	return kinds
}
func allNotify() api.RepoNotify {
	return api.RepoNotify{Events: []string{"ci_started", "ci_succeeded", "ci_failed", "ci_cancelled", "ci_recovered", "push", "pr_opened", "pr_merged", "pr_closed", "pr_review", "issue_opened", "issue_assigned", "release"}, CiBranches: api.All}
}
func TestRepositoryNotificationsBaselineTransitionsAndOverrides(t *testing.T) {
	env, gh := setup(t)
	env.MustDo(http.MethodPut, "/github/notify", api.GitHubNotifySettings{Defaults: allNotify(), Repos: []api.RepoNotifyOverride{}}, nil)
	gh.set(func(f *fakeGitHub) {
		f.runs[repo] = []map[string]any{run(1, 1, "CI", "main", "completed", "failure")}
		f.commits[repo] = []map[string]any{commit("abc", "old")}
	})
	syncNow(t, env)
	if got := notifications(t, env); len(got) != 0 {
		t.Fatalf("baseline notifies: %v", got)
	}
	gh.set(func(f *fakeGitHub) {
		f.runs[repo] = append([]map[string]any{run(2, 1, "CI", "main", "in_progress", "")}, f.runs[repo]...)
		f.commits[repo] = append([]map[string]any{commit("def", "new"), commit("ghi", "also new")}, f.commits[repo]...)
		f.pulls[repo] = []map[string]any{pull(1, "PR", "feat", "")}
	})
	syncNow(t, env)
	gh.set(func(f *fakeGitHub) {
		f.runs[repo][0] = run(2, 1, "CI", "main", "completed", "success")
		f.reviews[repo+"#1"] = []map[string]any{{"user": map[string]any{"login": "amy"}, "state": "APPROVED"}}
	})
	syncNow(t, env)
	syncNow(t, env)
	got := notifications(t, env)
	for _, kind := range []string{"github.ci_started", "github.ci_succeeded", "github.ci_recovered", "github.push", "github.pr_opened", "github.pr_review"} {
		if countKind(got, kind) != 1 {
			t.Fatalf("want one %s: %v", kind, got)
		}
	}
	env.MustDo(http.MethodPut, "/github/notify", api.GitHubNotifySettings{Defaults: allNotify(), Repos: []api.RepoNotifyOverride{{ConnectionId: 0, Repo: repo, Notify: api.RepoNotify{Events: []string{}, CiBranches: api.Default}}}}, nil)
	var repos []api.WatchedRepo
	env.MustDo(http.MethodGet, "/github/repos", nil, &repos)
	if !repos[0].NotifyCustom || !repos[0].NotifyOff {
		t.Fatalf("override flags: %+v", repos)
	}
	gh.set(func(f *fakeGitHub) {
		f.runs[repo] = append([]map[string]any{run(3, 1, "CI", "main", "completed", "failure")}, f.runs[repo]...)
	})
	syncNow(t, env)
	if next := notifications(t, env); len(next) != len(got) {
		t.Fatalf("disabled override: %v", next)
	}
	if s, _ := env.Do(http.MethodPut, "/github/notify", map[string]any{"defaults": map[string]any{"events": []string{"unknown"}, "ciBranches": "default"}, "repos": []any{}}, nil); s != 400 {
		t.Fatalf("invalid event: %d", s)
	}
}
func countKind(kinds []string, want string) int {
	n := 0
	for _, k := range kinds {
		if k == want {
			n++
		}
	}
	return n
}
func TestEmptyBaselineAndNotificationSummary(t *testing.T) {
	env, gh := setup(t)
	env.MustDo(http.MethodPut, "/github/notify", api.GitHubNotifySettings{Defaults: allNotify(), Repos: []api.RepoNotifyOverride{}}, nil)
	clock := time.Now().UTC()
	m := githubModule(t, env)
	m.SetNow(func() time.Time { return clock })
	syncNow(t, env)
	gh.set(func(f *fakeGitHub) {
		for i := 1; i <= 7; i++ {
			f.pulls[repo] = append(f.pulls[repo], pull(i, "new", "feat", ""))
		}
	})
	syncNow(t, env)
	syncNow(t, env)
	if got := notifications(t, env); countKind(got, "github.pr_opened") != 3 {
		t.Fatalf("limit: %v", got)
	}
	clock = clock.Add(61 * time.Second)
	if err := m.FlushNotify(context.Background()); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Items []struct{ Kind, Body string } `json:"items"`
	}
	env.MustDo(http.MethodGet, "/notifications?limit=200", nil, &out)
	found := false
	for _, n := range out.Items {
		if n.Kind == "github.pr_opened" && strings.Contains(n.Body, "还有 4 条") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing summary: %+v", out)
	}
	if err := m.FlushNotify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := notifications(t, env); countKind(got, "github.pr_opened") != 4 {
		t.Fatalf("duplicate summary: %v", got)
	}
}
func TestWebhookAndSyncDeduplicateBothOrders(t *testing.T) {
	for _, hookFirst := range []bool{true, false} {
		t.Run(fmt.Sprint(hookFirst), func(t *testing.T) {
			env := testutil.New(t)
			gh := newFakeGitHub(t)
			env.Elevate()
			var created struct {
				Connection    gitConnection
				WebhookSecret string
			}
			env.MustDo(http.MethodPost, "/git-connections", map[string]any{"kind": "github", "name": "hook", "baseUrl": gh.URL(), "token": gh.token}, &created)
			env.MustDo(http.MethodPut, "/github/config", map[string]any{"watches": []api.RepoWatch{{ConnectionId: created.Connection.ID, Repo: repo}}}, nil)
			env.MustDo(http.MethodPut, "/github/notify", api.GitHubNotifySettings{Defaults: allNotify(), Repos: []api.RepoNotifyOverride{}}, nil)
			syncNow(t, env)
			payload := map[string]any{"action": "opened", "repository": map[string]any{"full_name": repo, "default_branch": "main"}, "pull_request": pull(1, "new", "feat", "")}
			raw, _ := json.Marshal(payload)
			post := func(secret, delivery string) int {
				mac := hmac.New(sha256.New, []byte(secret))
				mac.Write(raw)
				req, _ := http.NewRequest(http.MethodPost, env.URL(fmt.Sprintf("/hooks/git/%d", created.Connection.ID)), bytes.NewReader(raw))
				req.Header.Set("X-GitHub-Event", "pull_request")
				req.Header.Set("X-GitHub-Delivery", delivery)
				req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				res.Body.Close()
				return res.StatusCode
			}
			if s := post("wrong", "wrong"); s != 401 {
				t.Fatalf("signature: %d", s)
			}
			if hookFirst {
				if s := post(created.WebhookSecret, "one"); s != 204 {
					t.Fatalf("hook: %d", s)
				}
			}
			gh.set(func(f *fakeGitHub) { f.pulls[repo] = []map[string]any{pull(1, "new", "feat", "")} })
			syncNow(t, env)
			if s := post(created.WebhookSecret, "one"); s != 204 {
				t.Fatalf("hook replay: %d", s)
			}
			if s := post(created.WebhookSecret, "two"); s != 204 {
				t.Fatalf("semantic replay: %d", s)
			}
			if got := notifications(t, env); countKind(got, "github.pr_opened") != 1 {
				t.Fatalf("duplicates: %v", got)
			}
		})
	}
}

func TestNotificationsPersistAndRollback(t *testing.T) {
	env, gh := setup(t)
	env.MustDo(http.MethodPut, "/github/notify", api.GitHubNotifySettings{Defaults: allNotify(), Repos: []api.RepoNotifyOverride{}}, nil)
	syncNow(t, env)
	if _, err := env.App.Deps.DB.Exec(`CREATE TRIGGER fail_github_notice BEFORE INSERT ON notifications WHEN NEW.source='github' BEGIN SELECT RAISE(ABORT,'notice failed'); END`); err != nil {
		t.Fatal(err)
	}
	gh.set(func(f *fakeGitHub) { f.pulls[repo] = []map[string]any{pull(1, "retry", "feat", "")} })
	if st := syncNow(t, env); st.LastError == nil {
		t.Fatal("expected notification write failure")
	}
	if _, err := env.App.Deps.DB.Exec(`DROP TRIGGER fail_github_notice`); err != nil {
		t.Fatal(err)
	}
	syncNow(t, env)
	replacement, err := restartGitHub(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := replacement.(*github.Module).ScheduledSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := notifications(t, env); countKind(got, "github.pr_opened") != 1 {
		t.Fatalf("retry or restart duplicates: %v", got)
	}
}

func TestNotificationBranchAttemptsAndOtherEvents(t *testing.T) {
	env, gh := setup(t)
	policy := allNotify()
	policy.CiBranches = api.Default
	env.MustDo(http.MethodPut, "/github/notify", api.GitHubNotifySettings{Defaults: policy, Repos: []api.RepoNotifyOverride{}}, nil)
	gh.set(func(f *fakeGitHub) {
		f.pulls[repo] = []map[string]any{pull(1, "merge", "feat", ""), pull(2, "close", "feat", "")}
		f.issues[repo] = []map[string]any{{"number": 1, "title": "assign", "state": "open", "user": map[string]any{"login": "other"}, "assignees": []any{}, "labels": []any{}}}
	})
	syncNow(t, env)
	gh.set(func(f *fakeGitHub) {
		f.pulls[repo][0]["state"] = "closed"
		f.pulls[repo][0]["closed_at"] = time.Now().UTC().Format(time.RFC3339)
		f.pulls[repo][0]["updated_at"] = time.Now().UTC().Format(time.RFC3339)
		f.pulls[repo][0]["merged"] = true
		f.pulls[repo][1]["state"] = "closed"
		f.pulls[repo][1]["closed_at"] = time.Now().UTC().Format(time.RFC3339)
		f.pulls[repo][1]["updated_at"] = time.Now().UTC().Format(time.RFC3339)
		f.issues[repo][0]["assignees"] = []any{map[string]any{"login": "JO"}}
		f.issues[repo] = append(f.issues[repo], map[string]any{"number": 2, "title": "new", "state": "open", "user": map[string]any{"login": "other"}, "assignees": []any{}, "labels": []any{}})
		f.releases[repo] = map[string]any{"tag_name": "v1", "name": "version"}
		f.runs[repo] = []map[string]any{run(1, 1, "CI", "feat", "completed", "failure"), run(2, 1, "CI", "main", "completed", "cancelled")}
	})
	syncNow(t, env)
	got := notifications(t, env)
	for _, kind := range []string{"pr_merged", "pr_closed", "issue_opened", "issue_assigned", "release", "ci_cancelled"} {
		if countKind(got, "github."+kind) != 1 {
			t.Fatalf("missing %s: %v", kind, got)
		}
	}
	if countKind(got, "github.ci_failed") != 0 {
		t.Fatalf("nondefault CI: %v", got)
	}
	policy.CiBranches = api.All
	env.MustDo(http.MethodPut, "/github/notify", api.GitHubNotifySettings{Defaults: policy, Repos: []api.RepoNotifyOverride{}}, nil)
	gh.set(func(f *fakeGitHub) { f.runs[repo][0]["run_attempt"] = 2 })
	syncNow(t, env)
	syncNow(t, env)
	if got := notifications(t, env); countKind(got, "github.ci_failed") != 1 {
		t.Fatalf("retry attempt: %v", got)
	}
}

func restartGitHub(env *testutil.Env) (module.Module, error) {
	deps := *env.App.Deps
	deps.Actions = actions.NewRegistry()
	return github.New(&deps)
}

func TestWebhookAllEventsAndAccountIsolation(t *testing.T) {
	for _, hookFirst := range []bool{true, false} {
		t.Run(fmt.Sprint(hookFirst), func(t *testing.T) {
			env := testutil.New(t)
			gh := newFakeGitHub(t)
			env.Elevate()
			var a, b struct {
				Connection    gitConnection
				WebhookSecret string
			}
			body := map[string]any{"kind": "github", "name": "A", "baseUrl": gh.URL(), "token": gh.token}
			env.MustDo(http.MethodPost, "/git-connections", body, &a)
			body["name"] = "B"
			env.MustDo(http.MethodPost, "/git-connections", body, &b)
			env.MustDo(http.MethodPut, "/github/config", map[string]any{"watches": []api.RepoWatch{{ConnectionId: a.Connection.ID, Repo: repo}, {ConnectionId: b.Connection.ID, Repo: repo}}}, nil)
			env.MustDo(http.MethodPut, "/github/notify", api.GitHubNotifySettings{Defaults: allNotify(), Repos: []api.RepoNotifyOverride{}}, nil)
			gh.set(func(f *fakeGitHub) {
				f.pulls[repo] = []map[string]any{pull(1, "baseline", "feat", "")}
				f.runs[repo] = []map[string]any{run(1, 1, "CI", "main", "completed", "failure")}
			})
			syncNow(t, env)
			gh.set(func(f *fakeGitHub) {
				f.pulls[repo][0]["state"] = "closed"
				f.pulls[repo][0]["closed_at"] = time.Now().UTC().Format(time.RFC3339)
				f.pulls[repo][0]["updated_at"] = time.Now().UTC().Format(time.RFC3339)
				f.pulls[repo][0]["merged"] = true
				f.commits[repo] = []map[string]any{commit("new2", "latest"), commit("new1", "first")}
				f.runs[repo] = append([]map[string]any{run(2, 1, "CI", "main", "completed", "success")}, f.runs[repo]...)
				f.issues[repo] = []map[string]any{{"number": 1, "title": "issue", "state": "open", "user": map[string]any{"login": "other"}, "assignees": []any{map[string]any{"login": "jo"}}, "labels": []any{}}}
				f.releases[repo] = map[string]any{"tag_name": "v1", "name": "version"}
			})
			hooks := []struct {
				event   string
				payload map[string]any
			}{
				{"push", map[string]any{"ref": "refs/heads/main", "commits": []any{map[string]any{"id": "new1", "message": "first"}, map[string]any{"id": "new2", "message": "latest"}}}},
				{"workflow_run", map[string]any{"action": "completed", "workflow_run": run(2, 1, "CI", "main", "completed", "success")}},
				{"pull_request", map[string]any{"action": "closed", "pull_request": map[string]any{"number": 1, "title": "baseline", "state": "closed", "merged": true}}},
				{"issues", map[string]any{"action": "opened", "issue": map[string]any{"number": 1, "title": "issue", "assignees": []any{map[string]any{"login": "jo"}}}}},
				{"release", map[string]any{"action": "published", "release": map[string]any{"tag_name": "v1", "name": "version"}}},
			}
			postHooks := func() {
				for i, h := range hooks {
					h.payload["repository"] = map[string]any{"full_name": repo, "default_branch": "main"}
					raw, _ := json.Marshal(h.payload)
					receiver, ok := module.Lookup[contracts.GitWebhookReceiver](env.App.Deps.Registry, contracts.GitWebhookKey)
					if !ok {
						t.Fatal("missing receiver")
					}
					for j := 0; j < 2; j++ {
						if err := receiver.ReceiveGitWebhook(context.Background(), contracts.GitWebhook{ConnectionID: a.Connection.ID, Event: h.event, DeliveryID: fmt.Sprintf("%d-%d", i, j), Body: raw}); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			if hookFirst {
				postHooks()
			}
			syncNow(t, env)
			postHooks()
			syncNow(t, env)
			got := notifications(t, env)
			for _, kind := range []string{"push", "ci_succeeded", "ci_recovered", "pr_merged", "issue_opened", "issue_assigned", "release"} {
				if countKind(got, "github."+kind) != 2 {
					t.Fatalf("account or hook dedup %s: %v", kind, got)
				}
			}
			replacement, err := restartGitHub(env)
			if err != nil {
				t.Fatal(err)
			}
			_ = replacement
			postHooks()
			if next := notifications(t, env); len(next) != len(got) {
				t.Fatalf("restart hook duplicate: %v", next)
			}
		})
	}
}

func TestConcurrentWebhookSummarySurvivesRestart(t *testing.T) {
	env, gh := setup(t)
	env.MustDo(http.MethodPut, "/github/notify", api.GitHubNotifySettings{Defaults: allNotify(), Repos: []api.RepoNotifyOverride{}}, nil)
	syncNow(t, env)
	gh.set(func(f *fakeGitHub) { f.pulls[repo] = []map[string]any{} })
	m := githubModule(t, env)
	clock := time.Now().UTC()
	m.SetNow(func() time.Time { return clock })
	receiver := contracts.GitWebhookReceiver(m)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payload := map[string]any{"action": "opened", "repository": map[string]any{"full_name": repo}, "pull_request": pull(i%7+1, "concurrent", "feat", "")}
			raw, _ := json.Marshal(payload)
			errs <- receiver.ReceiveGitWebhook(context.Background(), contracts.GitWebhook{ConnectionID: 0, Event: "pull_request", DeliveryID: fmt.Sprint(i), Body: raw})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := notifications(t, env); countKind(got, "github.pr_opened") != 3 {
		t.Fatalf("concurrent duplicates: %v", got)
	}
	replacement, err := restartGitHub(env)
	if err != nil {
		t.Fatal(err)
	}
	next := replacement.(*github.Module)
	clock = clock.Add(61 * time.Second)
	next.SetNow(func() time.Time { return clock })
	if err := next.FlushNotify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := next.FlushNotify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := notifications(t, env); countKind(got, "github.pr_opened") != 4 {
		t.Fatalf("persistent summary: %v", got)
	}
}

func TestVerifiedWebhookErrorAllowsRetry(t *testing.T) {
	env := testutil.New(t)
	gh := newFakeGitHub(t)
	env.Elevate()
	var c struct {
		Connection    gitConnection
		WebhookSecret string
	}
	env.MustDo(http.MethodPost, "/git-connections", map[string]any{"kind": "github", "name": "retry", "baseUrl": gh.URL(), "token": gh.token}, &c)
	env.MustDo(http.MethodPut, "/github/config", map[string]any{"watches": []api.RepoWatch{{ConnectionId: c.Connection.ID, Repo: repo}}}, nil)
	syncNow(t, env)
	raw, _ := json.Marshal(map[string]any{"action": "opened", "repository": map[string]any{"full_name": repo}, "pull_request": pull(1, "retry hook", "feat", "")})
	post := func() int {
		mac := hmac.New(sha256.New, []byte(c.WebhookSecret))
		mac.Write(raw)
		req, _ := http.NewRequest(http.MethodPost, env.URL(fmt.Sprintf("/hooks/git/%d", c.Connection.ID)), bytes.NewReader(raw))
		req.Header.Set("X-GitHub-Event", "pull_request")
		req.Header.Set("X-GitHub-Delivery", "retry")
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if _, err := env.App.Deps.DB.Exec(`CREATE TRIGGER fail_github_hook BEFORE INSERT ON notifications WHEN NEW.source='github' BEGIN SELECT RAISE(ABORT,'notice failed'); END`); err != nil {
		t.Fatal(err)
	}
	if code := post(); code != 500 {
		t.Fatalf("must propagate delivery error: %d", code)
	}
	if _, err := env.App.Deps.DB.Exec(`DROP TRIGGER fail_github_hook`); err != nil {
		t.Fatal(err)
	}
	if code := post(); code != 204 {
		t.Fatalf("retry: %d", code)
	}
	if code := post(); code != 204 {
		t.Fatalf("replay: %d", code)
	}
	if got := notifications(t, env); countKind(got, "github.pr_opened") != 1 {
		t.Fatalf("write failure dedup: %v", got)
	}
}
