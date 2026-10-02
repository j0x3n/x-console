package github_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// The module is registered in app/modules.go, so testutil.New(t) includes it.

const repo = "acme/app"

func errCode(raw []byte) string {
	var e struct{ Code string }
	_ = json.Unmarshal(raw, &e)
	return e.Code
}

// setup starts a server and a fake GitHub and configures the token.
func setup(t *testing.T) (*testutil.Env, *fakeGitHub) {
	t.Helper()
	env := testutil.New(t)
	gh := newFakeGitHub(t)
	env.Elevate()
	env.MustDo(http.MethodPut, "/github/config", map[string]any{
		"token": gh.token, "repos": []string{repo}, "apiUrl": gh.URL() + "/",
	}, nil)
	return env, gh
}

func syncNow(t *testing.T, env *testutil.Env) api.GitHubStatus {
	t.Helper()
	var st api.GitHubStatus
	env.MustDo(http.MethodPost, "/github/sync", nil, &st)
	return st
}

func pulls(t *testing.T, env *testutil.Env) []api.GitHubPull {
	t.Helper()
	var out []api.GitHubPull
	env.MustDo(http.MethodGet, "/github/pulls", nil, &out)
	return out
}

func ciNotifications(t *testing.T, env *testutil.Env) []string {
	t.Helper()
	var out struct {
		Items []struct {
			Kind  string `json:"kind"`
			Title string `json:"title"`
		} `json:"items"`
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &out)
	var titles []string
	for _, n := range out.Items {
		if n.Kind == "github.ci_failed" {
			titles = append(titles, n.Title)
		}
	}
	return titles
}

func TestNotConfigured(t *testing.T) {
	env := testutil.New(t)
	var cfg api.GitHubConfig
	env.MustDo(http.MethodGet, "/github/config", nil, &cfg)
	if cfg.HasToken || len(cfg.Repos) != 0 || cfg.ApiUrl != "https://api.github.com" {
		t.Fatalf("config: %+v", cfg)
	}
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/github/pulls"}, {http.MethodGet, "/github/runs"}, {http.MethodGet, "/github/issues"},
		{http.MethodPost, "/github/sync"}, {http.MethodPost, "/github/test"},
	} {
		code, raw := env.Do(c.method, c.path, nil, nil)
		if code != http.StatusPreconditionFailed || errCode(raw) != "integration_not_configured" {
			t.Fatalf("%s %s: %d %s", c.method, c.path, code, raw)
		}
	}
	var st api.GitHubStatus
	env.MustDo(http.MethodGet, "/github/status", nil, &st)
	if st.Configured || st.LastSyncAt != nil {
		t.Fatalf("status: %+v", st)
	}
	gh, ok := module.Lookup[contracts.GitHub](env.App.Deps.Registry, contracts.GitHubKey)
	if !ok {
		t.Fatal("contracts.GitHub not provided")
	}
	if _, _, err := gh.CreatePR(context.Background(), contracts.CreatePR{Repo: repo, Head: "x", Base: "main", Title: "T"}); err == nil {
		t.Fatal("CreatePR without token should fail")
	}
}

func TestConfig(t *testing.T) {
	env := testutil.New(t)
	gh := newFakeGitHub(t)
	// A new token needs elevation.
	code, raw := env.Do(http.MethodPut, "/github/config", map[string]any{"token": gh.token, "repos": []string{}}, nil)
	if code != http.StatusForbidden || errCode(raw) != "elevation_required" {
		t.Fatalf("put without elevation: %d %s", code, raw)
	}
	env.Elevate()
	var cfg api.GitHubConfig
	env.MustDo(http.MethodPut, "/github/config", map[string]any{
		"token": gh.token, "repos": []string{" https://github.com/acme/app.git ", "acme/app", "acme/lib"}, "apiUrl": gh.URL(),
	}, &cfg)
	if !cfg.HasToken || cfg.Token != "••••••••1234" || strings.Contains(string(mustJSON(cfg)), gh.token) {
		t.Fatalf("token not masked: %+v", cfg)
	}
	if len(cfg.Repos) != 2 || cfg.Repos[0] != "acme/app" || cfg.Repos[1] != "acme/lib" || cfg.ApiUrl != gh.URL() {
		t.Fatalf("config: %+v", cfg)
	}
	code, raw = env.Do(http.MethodPut, "/github/config", map[string]any{"repos": []string{"not a repo"}}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("bad repo: %d %s", code, raw)
	}
	code, raw = env.Do(http.MethodPut, "/github/config", map[string]any{"repos": []string{repo}, "apiUrl": "ftp://x"}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("bad url: %d %s", code, raw)
	}

	var res api.GitHubTestResult
	env.MustDo(http.MethodPost, "/github/test", nil, &res)
	if !res.Ok || res.Login == nil || *res.Login != "jo" {
		t.Fatalf("test: %+v", res)
	}
	env.MustDo(http.MethodPost, "/github/test", map[string]any{"token": "wrong-token-000000"}, &res)
	if res.Ok || res.Message == nil || !strings.Contains(*res.Message, "令牌") {
		t.Fatalf("test with wrong token: %+v", res)
	}
	env.MustDo(http.MethodPut, "/github/config", map[string]any{"repos": []string{}, "clearToken": true}, &cfg)
	if cfg.HasToken {
		t.Fatalf("token not cleared: %+v", cfg)
	}
}

func mustJSON(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

func TestSync(t *testing.T) {
	env, gh := setup(t)
	gh.set(func(f *fakeGitHub) {
		approved := pull(1, "Add login", "feature/login", "sha1")
		draft := pull(2, "WIP: refactor", "refactor", "sha2")
		draft["draft"] = true
		draft["requested_reviewers"] = []any{map[string]any{"login": "amy"}}
		f.pulls[repo] = []map[string]any{approved, draft}
		f.reviews[repo+"#1"] = []map[string]any{
			{"user": map[string]any{"login": "amy"}, "state": "CHANGES_REQUESTED"},
			{"user": map[string]any{"login": "amy"}, "state": "APPROVED"},
		}
		f.statuses["sha1"] = map[string]any{"state": "success", "total_count": 1}
		f.checkRuns["sha1"] = []map[string]any{{"status": "completed", "conclusion": "success"}}
		f.checkRuns["sha2"] = []map[string]any{{"status": "in_progress", "conclusion": nil}}
		f.runs[repo] = []map[string]any{run(11, 1, "CI", "main", "completed", "success"), run(10, 1, "CI", "feature/login", "completed", "failure")}
		f.issues[repo] = []map[string]any{
			{"number": 5, "title": "Bug", "html_url": "https://github.com/acme/app/issues/5", "user": map[string]any{"login": "jo"},
				"assignees": []any{map[string]any{"login": "jo"}}, "labels": []any{map[string]any{"name": "bug"}},
				"created_at": "2026-09-20T10:00:00Z", "updated_at": "2026-09-21T10:00:00Z"},
			{"number": 6, "title": "Assigned only", "html_url": "https://github.com/acme/app/issues/6", "user": map[string]any{"login": "amy"},
				"assignees": []any{map[string]any{"login": "jo"}}, "labels": []any{},
				"created_at": "2026-09-20T10:00:00Z", "updated_at": "2026-09-22T10:00:00Z"},
			{"number": 1, "title": "Add login", "html_url": "https://github.com/acme/app/pull/1", "user": map[string]any{"login": "jo"},
				"assignees": []any{}, "labels": []any{}, "pull_request": map[string]any{"url": "x"},
				"created_at": "2026-09-20T10:00:00Z", "updated_at": "2026-09-22T10:00:00Z"},
		}
	})
	st := syncNow(t, env)
	if st.LastError != nil || st.LastSyncAt == nil || st.Login == nil || *st.Login != "jo" || st.RateLimitRemaining == nil {
		t.Fatalf("status: %+v", st)
	}
	list := pulls(t, env)
	if len(list) != 2 {
		t.Fatalf("pulls: %+v", list)
	}
	byNum := map[int]api.GitHubPull{}
	for _, p := range list {
		byNum[p.Number] = p
	}
	if p := byNum[1]; p.CheckState != "success" || p.ReviewState != "approved" || p.HeadRef != "feature/login" || p.BaseRef != "main" || p.Author != "jo" || p.Draft {
		t.Fatalf("pull 1: %+v", p)
	}
	if p := byNum[2]; p.CheckState != "pending" || p.ReviewState != "pending" || !p.Draft {
		t.Fatalf("pull 2: %+v", p)
	}

	var runs []api.GitHubRun
	env.MustDo(http.MethodGet, "/github/runs", nil, &runs)
	if len(runs) != 2 || runs[0].Id != 11 || !runs[0].DefaultBranch || runs[1].DefaultBranch || runs[1].Conclusion != "failure" {
		t.Fatalf("runs: %+v", runs)
	}
	var issues []api.GitHubIssue
	env.MustDo(http.MethodGet, "/github/issues", nil, &issues)
	if len(issues) != 2 {
		t.Fatalf("issues: %+v", issues)
	}
	rel := map[int]api.GitHubIssueRelation{}
	for _, is := range issues {
		rel[is.Number] = is.Relation
	}
	if rel[5] != "both" || rel[6] != "assigned" || issues[1].Labels[0] != "bug" {
		t.Fatalf("issues: %+v", issues)
	}

	// Deleted PRs leave the cache; unchanged data is revalidated with ETags.
	gh.set(func(f *fakeGitHub) { f.pulls[repo] = f.pulls[repo][:1] })
	_, before := gh.counts()
	syncNow(t, env)
	if list := pulls(t, env); len(list) != 1 || list[0].Number != 1 {
		t.Fatalf("after close: %+v", list)
	}
	if _, after := gh.counts(); after <= before {
		t.Fatal("expected 304 answers on the second sync")
	}

	// Unwatching a repository drops its cache.
	env.MustDo(http.MethodPut, "/github/config", map[string]any{"repos": []string{"acme/other"}}, nil)
	gh.set(func(f *fakeGitHub) {})
	syncNow(t, env)
	if list := pulls(t, env); len(list) != 0 {
		t.Fatalf("after unwatch: %+v", list)
	}
}

func TestSyncRecentClosedPulls(t *testing.T) {
	env, gh := setup(t)
	now := time.Now().UTC()
	gh.set(func(f *fakeGitHub) {
		open := make([]map[string]any, 150)
		for i := range open {
			open[i] = pull(i+1, "Open", "branch", "")
		}
		closed := pull(201, "Closed", "closed", "")
		closed["state"] = "closed"
		closed["closed_at"] = now.Add(-24 * time.Hour).Format(time.RFC3339)
		closed["updated_at"] = closed["closed_at"]
		merged := pull(202, "Merged", "merged", "")
		merged["state"] = "closed"
		merged["closed_at"] = now.Add(-2 * 24 * time.Hour).Format(time.RFC3339)
		merged["merged_at"] = now.Add(-2 * 24 * time.Hour).Format(time.RFC3339)
		merged["updated_at"] = merged["closed_at"]
		old := pull(203, "Old", "old", "")
		old["state"] = "closed"
		old["closed_at"] = now.Add(-8 * 24 * time.Hour).Format(time.RFC3339)
		old["updated_at"] = old["closed_at"]
		f.pulls[repo] = append(open, closed, merged, old)
	})
	syncNow(t, env)
	got := pulls(t, env)
	if len(got) != 152 {
		t.Fatalf("want 150 open and 2 recent closed PRs, got %d", len(got))
	}
	states := map[int]api.GitHubPullState{}
	for _, p := range got {
		states[p.Number] = p.State
	}
	if states[1] != "open" || states[150] != "open" || states[201] != "closed" || states[202] != "merged" {
		t.Fatalf("pull states: %+v", states)
	}
	if _, ok := states[203]; ok {
		t.Fatal("PR closed eight days ago should not be cached")
	}
}

func TestClosedPullsStopAtCutoff(t *testing.T) {
	env, gh := setup(t)
	now := time.Now().UTC()
	gh.set(func(f *fakeGitHub) {
		var list []map[string]any
		// Newest update first, like GitHub with sort=updated&direction=desc.
		for i := 0; i < 1000; i++ {
			p := pull(1000+i, "Closed", "b", "")
			at := now.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339)
			p["state"], p["closed_at"], p["updated_at"] = "closed", at, at
			list = append(list, p)
		}
		f.pulls[repo] = list
	})
	syncNow(t, env)
	if got := len(pulls(t, env)); got != 7*24 {
		t.Fatalf("want the %d PRs closed in the last week, got %d", 7*24, got)
	}
	if requests, _ := gh.counts(); requests > 10 {
		t.Fatalf("read old closed pages: %d requests", requests)
	}
}

func TestCIFailedNotifiesOnce(t *testing.T) {
	env, gh := setup(t)
	gh.set(func(f *fakeGitHub) {
		f.pulls[repo] = []map[string]any{pull(7, "Speed up", "speed", "s1")}
		f.checkRuns["s1"] = []map[string]any{{"status": "completed", "conclusion": "success"}}
		f.runs[repo] = []map[string]any{run(1, 9, "Build", "main", "completed", "success")}
	})
	syncNow(t, env)
	if n := ciNotifications(t, env); len(n) != 0 {
		t.Fatalf("green should not notify: %v", n)
	}
	// New commit: pending first, then red.
	gh.set(func(f *fakeGitHub) {
		f.pulls[repo] = []map[string]any{pull(7, "Speed up", "speed", "s2")}
		f.checkRuns["s2"] = []map[string]any{{"status": "queued"}}
	})
	syncNow(t, env)
	gh.set(func(f *fakeGitHub) {
		f.checkRuns["s2"] = []map[string]any{{"status": "completed", "conclusion": "success"}, {"status": "completed", "conclusion": "failure"}}
		f.runs[repo] = append([]map[string]any{run(2, 9, "Build", "main", "completed", "failure")}, f.runs[repo]...)
	})
	syncNow(t, env)
	syncNow(t, env) // still red: no second notification
	got := ciNotifications(t, env)
	if len(got) != 1 {
		t.Fatalf("want one default branch notification, got %v", got)
	}
	joined := strings.Join(got, "|")
	if !strings.Contains(joined, "acme/app") {
		t.Fatalf("notifications: %v", got)
	}
	if p := pulls(t, env); p[0].CheckState != "failure" {
		t.Fatalf("check state: %+v", p)
	}
	// Green again, then red again: notifies again.
	gh.set(func(f *fakeGitHub) {
		f.checkRuns["s2"] = []map[string]any{{"status": "completed", "conclusion": "success"}}
	})
	syncNow(t, env)
	gh.set(func(f *fakeGitHub) {
		f.checkRuns["s2"] = []map[string]any{{"status": "completed", "conclusion": "failure"}}
		f.runs[repo] = append([]map[string]any{run(3, 9, "Build", "main", "completed", "failure")}, f.runs[repo]...)
	})
	syncNow(t, env)
	if got := ciNotifications(t, env); len(got) != 2 {
		t.Fatalf("second transition: %v", got)
	}
}

func TestLinksToIssuesAndCodingTasks(t *testing.T) {
	env, gh := setup(t)
	var p struct{ ID int64 }
	env.MustDo(http.MethodPost, "/projects", map[string]any{"key": "XC", "name": "X Console"}, &p)
	for i := 0; i < 12; i++ {
		env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/issues", p.ID), map[string]any{"title": fmt.Sprintf("Issue %d", i+1)}, nil)
	}
	gh.set(func(f *fakeGitHub) {
		f.pulls[repo] = []map[string]any{
			pull(3, "XC-12: fix the login page", "feature/xc-3-login", "a"),
			pull(4, "Bump UTF-8 handling", "xc/42-bump", "b"),
		}
	})
	syncNow(t, env)
	syncNow(t, env) // links are attached once

	type link struct {
		Kind, Title, URL, Ref string
	}
	var links []link
	env.MustDo(http.MethodGet, "/issues/XC-12/links", nil, &links)
	if len(links) != 1 || links[0].Kind != "pull_request" || links[0].Ref != "acme/app#3" || links[0].URL != "https://github.com/acme/app/pull/3" {
		t.Fatalf("XC-12 links: %+v", links)
	}
	env.MustDo(http.MethodGet, "/issues/XC-3/links", nil, &links)
	if len(links) != 1 {
		t.Fatalf("XC-3 links (from branch): %+v", links)
	}
	var auditCount int
	var entries struct {
		Items []struct{ Action string } `json:"items"`
	}
	env.MustDo(http.MethodGet, "/audit?limit=200", nil, &entries)
	for _, e := range entries.Items {
		if e.Action == "issue_link.create" {
			auditCount++
		}
	}
	if auditCount != 2 {
		t.Fatalf("links should be created once each, audit shows %d", auditCount)
	}
	byNum := map[int]api.GitHubPull{}
	for _, pr := range pulls(t, env) {
		byNum[pr.Number] = pr
	}
	if keys := byNum[3].IssueKeys; len(keys) != 2 {
		t.Fatalf("issue keys: %+v", byNum[3])
	}
	if pr := byNum[4]; len(pr.IssueKeys) != 0 || pr.CodingTaskId == nil || *pr.CodingTaskId != 42 {
		t.Fatalf("coding task link: %+v", pr)
	}
}

func TestCreatePRAndActions(t *testing.T) {
	env, gh := setup(t)
	var p struct{ ID int64 }
	env.MustDo(http.MethodPost, "/projects", map[string]any{"key": "XC", "name": "X Console"}, &p)
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/issues", p.ID), map[string]any{"title": "One"}, nil)

	svc, ok := module.Lookup[contracts.GitHub](env.App.Deps.Registry, contracts.GitHubKey)
	if !ok {
		t.Fatal("contracts.GitHub not provided")
	}
	ctx := context.Background()
	u, n, err := svc.CreatePR(ctx, contracts.CreatePR{Repo: repo, Head: "xc/7-one", Base: "main", Title: "XC-1 do one", Body: "Done by the agent", Draft: true})
	if err != nil || n != 101 || u != "https://github.com/acme/app/pull/101" {
		t.Fatalf("CreatePR: %q %d %v", u, n, err)
	}
	gh.set(func(f *fakeGitHub) {
		if len(f.created) != 1 || f.created[0]["draft"] != true || f.created[0]["body"] != "Done by the agent" || f.created[0]["base"] != "main" {
			t.Fatalf("created: %+v", f.created)
		}
	})
	// It shows up right away and is linked to XC-1.
	list := pulls(t, env)
	if len(list) != 1 || list[0].Number != 101 || len(list[0].IssueKeys) != 1 || list[0].IssueKeys[0] != "XC-1" || list[0].CodingTaskId == nil {
		t.Fatalf("pulls after create: %+v", list)
	}
	if _, _, err := svc.CreatePR(ctx, contracts.CreatePR{Repo: repo, Head: "x", Base: "main"}); err == nil {
		t.Fatal("empty title should fail")
	}
	if _, _, err := svc.CreatePR(ctx, contracts.CreatePR{Repo: "bad", Head: "x", Base: "main", Title: "T"}); err == nil {
		t.Fatal("bad repo should fail")
	}

	out, err := env.App.Deps.Actions.Run(ctx, "github.create_pr", json.RawMessage(`{"repo":"acme/app","head":"feat","base":"main","title":"Via action"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m := out.(map[string]any); m["number"] != 102 {
		t.Fatalf("action create: %+v", out)
	}
	out, err = env.App.Deps.Actions.Run(ctx, "github.list_pulls", json.RawMessage(`{"repo":"acme/app"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := out.([]api.GitHubPull); len(got) != 2 {
		t.Fatalf("action list: %+v", got)
	}
}

func TestRateLimit(t *testing.T) {
	env, gh := setup(t)
	gh.set(func(f *fakeGitHub) { f.rateLimited = true })
	st := syncNow(t, env)
	if st.LastError == nil || !strings.Contains(*st.LastError, "次数用完") {
		t.Fatalf("status: %+v", st)
	}
	before, _ := gh.counts()
	// Until the reset time no request goes out.
	gh.set(func(f *fakeGitHub) { f.rateLimited = false })
	st = syncNow(t, env)
	if after, _ := gh.counts(); after != before {
		t.Fatalf("requests during the pause: %d → %d", before, after)
	}
	if st.LastError == nil || st.RateLimitRemaining == nil || *st.RateLimitRemaining != 0 {
		t.Fatalf("status: %+v", st)
	}
}

func githubModule(t *testing.T, env *testutil.Env) *github.Module {
	t.Helper()
	svc, ok := module.Lookup[contracts.GitHub](env.App.Deps.Registry, contracts.GitHubKey)
	if !ok {
		t.Fatal("contracts.GitHub not provided")
	}
	return svc.(*github.Module)
}

func TestAvailableRepos(t *testing.T) {
	// Without a token it is a 412 like the other endpoints.
	bare := testutil.New(t)
	if code, raw := bare.Do(http.MethodGet, "/github/available-repos", nil, nil); code != http.StatusPreconditionFailed || errCode(raw) != "integration_not_configured" {
		t.Fatalf("no token: %d %s", code, raw)
	}

	env, gh := setup(t)
	gh.set(func(f *fakeGitHub) {
		for i := 0; i < 250; i++ {
			r := map[string]any{"full_name": fmt.Sprintf("acme/repo-%03d", i), "private": i%2 == 0, "description": nil, "pushed_at": "2026-09-20T10:00:00Z"}
			if i == 0 {
				r["description"] = strings.Repeat("长", 300)
			}
			f.userRepos = append(f.userRepos, r)
		}
	})
	var list api.GitHubAvailableRepos
	env.MustDo(http.MethodGet, "/github/available-repos", nil, &list)
	if len(list.Repos) != 250 || list.Repos[0].FullName != "acme/repo-000" || !list.Repos[0].Private || list.Repos[1].Private {
		t.Fatalf("repos: %d %+v", len(list.Repos), list.Repos[:2])
	}
	if d := list.Repos[0].Description; d == nil || len([]rune(*d)) != 200 || list.Repos[1].Description != nil || list.Repos[0].PushedAt == nil {
		t.Fatalf("description or pushedAt: %+v", list.Repos[0])
	}
	requests, _ := gh.counts()
	if requests < 3 {
		t.Fatalf("expected three pages, got %d requests", requests)
	}

	// Within ten minutes the list comes from memory.
	env.MustDo(http.MethodGet, "/github/available-repos", nil, &list)
	if again, _ := gh.counts(); again != requests {
		t.Fatalf("cached list asked GitHub again: %d -> %d", requests, again)
	}
	// refresh=true asks again, and unchanged pages come back as 304.
	_, notModified := gh.counts()
	env.MustDo(http.MethodGet, "/github/available-repos?refresh=true", nil, &list)
	if again, nm := gh.counts(); again != requests+3 || nm != notModified+3 {
		t.Fatalf("refresh: requests %d -> %d, 304s %d -> %d", requests, again, notModified, nm)
	}

	// After ten minutes the cache expires by itself.
	m := githubModule(t, env)
	future := time.Now().Add(11 * time.Minute)
	m.SetNow(func() time.Time { return future })
	env.MustDo(http.MethodGet, "/github/available-repos", nil, &list)
	if again, _ := gh.counts(); again != requests+6 {
		t.Fatalf("expired list not refetched: %d", again)
	}
}

func TestPullsAllPages(t *testing.T) {
	env, gh := setup(t)
	gh.set(func(f *fakeGitHub) {
		for i := 1; i <= 150; i++ {
			f.pulls[repo] = append(f.pulls[repo], pull(i, fmt.Sprintf("PR %d", i), fmt.Sprintf("branch-%d", i), fmt.Sprintf("sha%d", i)))
		}
	})
	if st := syncNow(t, env); st.LastError != nil {
		t.Fatalf("sync: %+v", st)
	}
	if got := len(pulls(t, env)); got != 150 {
		t.Fatalf("pulls: %d", got)
	}
}

func TestSlowSyncWhenQuotaLow(t *testing.T) {
	env, gh := setup(t)
	m := githubModule(t, env)
	clock := time.Now().UTC()
	m.SetNow(func() time.Time { return clock })

	var st api.GitHubStatus
	env.MustDo(http.MethodPost, "/github/sync", nil, &st)
	if st.SyncIntervalSeconds == nil || *st.SyncIntervalSeconds != 60 || st.RateLimitLimit == nil || *st.RateLimitLimit != 5000 {
		t.Fatalf("healthy status: %+v", st)
	}
	requests, _ := gh.counts()
	clock = clock.Add(time.Minute)
	if err := m.ScheduledSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if again, _ := gh.counts(); again == requests {
		t.Fatal("with plenty of quota the scheduled sync should run every minute")
	}

	// 400 of 5000 left: below a tenth.
	gh.set(func(f *fakeGitHub) { f.remaining = 400 })
	env.MustDo(http.MethodPost, "/github/sync", nil, &st)
	if st.SyncIntervalSeconds == nil || *st.SyncIntervalSeconds != 300 || st.RateLimitRemaining == nil || *st.RateLimitRemaining != 400 {
		t.Fatalf("low quota status: %+v", st)
	}
	requests, _ = gh.counts()
	clock = clock.Add(time.Minute)
	if err := m.ScheduledSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if again, _ := gh.counts(); again != requests {
		t.Fatalf("low quota should skip after one minute: %d -> %d", requests, again)
	}
	clock = clock.Add(4*time.Minute + time.Second)
	if err := m.ScheduledSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if again, _ := gh.counts(); again == requests {
		t.Fatal("low quota should sync again after five minutes")
	}

	// The limit resets: back to every minute.
	clock = clock.Add(2 * time.Hour)
	env.MustDo(http.MethodGet, "/github/status", nil, &st)
	if st.SyncIntervalSeconds == nil || *st.SyncIntervalSeconds != 60 {
		t.Fatalf("after reset: %+v", st)
	}
}
