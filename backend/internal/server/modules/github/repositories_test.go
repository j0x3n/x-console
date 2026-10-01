package github_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestWatchesIsolateAccountsAndEmpty(t *testing.T) {
	env := testutil.New(t)
	a, b := newFakeGitHub(t), newFakeGitHub(t)
	env.Elevate()
	ca := createConnection(t, env, map[string]any{"kind": "github", "name": "A", "baseUrl": a.URL(), "token": a.token})
	cb := createConnection(t, env, map[string]any{"kind": "github", "name": "B", "baseUrl": b.URL(), "token": b.token})
	for i, f := range []*fakeGitHub{a, b} {
		f.set(func(f *fakeGitHub) {
			f.pulls[repo] = []map[string]any{pull(1, fmt.Sprintf("account %d", i), "feat", "abc")}
			f.runs[repo] = []map[string]any{run(1, 1, "CI", "main", "completed", "success")}
			f.commits[repo] = []map[string]any{commit("abc", fmt.Sprintf("commit %d", i))}
		})
	}
	var cfg api.GitHubConfig
	env.MustDo(http.MethodPut, "/github/config", map[string]any{"watches": []api.RepoWatch{{ConnectionId: ca.ID, Repo: repo}, {ConnectionId: cb.ID, Repo: repo}}}, &cfg)
	if cfg.Watches == nil || len(*cfg.Watches) != 2 {
		t.Fatalf("watches: %+v", cfg)
	}
	if st := syncNow(t, env); st.LastError != nil {
		t.Fatalf("sync: %+v", st)
	}
	ps := pulls(t, env)
	if len(ps) != 2 || ps[0].ConnectionId == nil || ps[1].ConnectionId == nil || *ps[0].ConnectionId == *ps[1].ConnectionId {
		t.Fatalf("account isolation: %+v", ps)
	}
	var rs []api.GitHubRun
	env.MustDo(http.MethodGet, fmt.Sprintf("/github/runs?connectionId=%d&repo=%s", cb.ID, repo), nil, &rs)
	if len(rs) != 1 || rs[0].Id != 1 || *rs[0].ConnectionId != cb.ID {
		t.Fatalf("runs: %+v", rs)
	}
	var cs []api.GitHubCommit
	env.MustDo(http.MethodGet, "/github/commits", nil, &cs)
	if len(cs) != 2 || cs[0].CheckState != "success" || cs[1].CheckState != "success" {
		t.Fatalf("commits: %+v", cs)
	}
	var ws []api.WatchedRepo
	env.MustDo(http.MethodGet, "/github/repos", nil, &ws)
	if len(ws) != 2 || ws[0].OpenPulls != 1 || ws[0].LastCommit == nil {
		t.Fatalf("repos: %+v", ws)
	}
	if s, _ := env.Do(http.MethodGet, "/github/runs/1/jobs?repo="+repo, nil, nil); s != 400 {
		t.Fatalf("ambiguous jobs: %d", s)
	}
	env.MustDo(http.MethodPut, "/github/config", map[string]any{"watches": []api.RepoWatch{{ConnectionId: cb.ID, Repo: repo}}}, nil)
	if len(pulls(t, env)) != 1 {
		t.Fatal("unwatched account remains visible")
	}
	env.MustDo(http.MethodPut, "/github/config", map[string]any{"watches": []api.RepoWatch{}}, nil)
	env.MustDo(http.MethodGet, "/github/config", nil, &cfg)
	if cfg.Watches == nil || len(*cfg.Watches) != 0 {
		t.Fatalf("empty watches: %+v", cfg)
	}
	if len(pulls(t, env)) != 0 {
		t.Fatal("explicit empty watches restored old repos")
	}
}

func commit(sha, message string) map[string]any {
	return map[string]any{"sha": sha, "html_url": "https://github.com/acme/app/commit/" + sha, "author": map[string]any{"login": "jo", "avatar_url": "https://example.test/a.png"}, "commit": map[string]any{"message": message, "author": map[string]any{"name": "Jo", "date": "2026-10-01T10:00:00Z"}, "committer": map[string]any{"date": "2026-10-01T10:00:00Z"}}}
}

func TestJobsCacheProgressAndQuotaIsolation(t *testing.T) {
	env, gh := setup(t)
	gh.set(func(f *fakeGitHub) {
		f.runs[repo] = []map[string]any{run(7, 1, "CI", "main", "in_progress", "")}
		f.jobs["7"] = []map[string]any{{"id": 12, "name": "Test", "status": "in_progress", "conclusion": nil, "html_url": "https://example.test/jobs/12", "steps": []any{map[string]any{"number": 1, "name": "Checkout", "status": "completed", "conclusion": "success"}, map[string]any{"number": 2, "name": "Run tests", "status": "in_progress", "conclusion": nil}}}}
	})
	if st := syncNow(t, env); st.LastError != nil {
		t.Fatalf("sync: %+v", st)
	}
	var rs []api.GitHubRun
	env.MustDo(http.MethodGet, "/github/runs", nil, &rs)
	if rs[0].StepsTotal == nil || *rs[0].StepsTotal != 2 || *rs[0].StepsDone != 1 || *rs[0].CurrentStep != "Run tests" {
		t.Fatalf("progress: %+v", rs[0])
	}
	before, _ := gh.counts()
	var jobs []api.GitHubJob
	for range 2 {
		env.MustDo(http.MethodGet, "/github/runs/7/jobs?repo="+repo, nil, &jobs)
	}
	if after, _ := gh.counts(); after != before {
		t.Fatalf("jobs TTL: %d -> %d", before, after)
	}
	if len(jobs) != 1 || len(jobs[0].Steps) != 2 {
		t.Fatalf("jobs: %+v", jobs)
	}
	m := githubModule(t, env)
	future := time.Now().Add(6 * time.Second)
	m.SetNow(func() time.Time { return future })
	env.MustDo(http.MethodGet, "/github/runs/7/jobs?repo="+repo, nil, &jobs)
	if after, _ := gh.counts(); after != before+1 {
		t.Fatalf("expired jobs: %d", after)
	}
	if s, _ := env.Do(http.MethodGet, "/github/runs/999/jobs?repo="+repo, nil, nil); s != 404 {
		t.Fatalf("missing run: %d", s)
	}

	other := newFakeGitHub(t)
	env.Elevate()
	ca := createConnection(t, env, map[string]any{"kind": "github", "name": "limited", "baseUrl": gh.URL(), "token": gh.token})
	cb := createConnection(t, env, map[string]any{"kind": "github", "name": "healthy", "baseUrl": other.URL(), "token": other.token})
	other.set(func(f *fakeGitHub) { f.pulls[repo] = []map[string]any{pull(3, "healthy", "feat", "")} })
	env.MustDo(http.MethodPut, "/github/config", map[string]any{"watches": []api.RepoWatch{{ConnectionId: ca.ID, Repo: repo}, {ConnectionId: cb.ID, Repo: repo}}}, nil)
	gh.set(func(f *fakeGitHub) { f.rateLimited = true })
	syncNow(t, env)
	ps := pulls(t, env)
	if len(ps) != 1 || *ps[0].ConnectionId != cb.ID {
		t.Fatalf("one account blocks another: %+v", ps)
	}
}

func TestLegacyCacheMigrationIsIdempotent(t *testing.T) {
	env, gh := setup(t)
	ctx := context.Background()
	if err := env.App.Deps.Settings.Delete(ctx, "github.migrated_b70"); err != nil {
		t.Fatal(err)
	}
	if err := env.App.Deps.Settings.Delete(ctx, "github.watches"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.App.Deps.DB.Exec(`INSERT INTO github_pulls(repo,number,title,url,head_ref,base_ref,created_at,updated_at,synced_at) VALUES(?,99,'legacy','https://example.test/pr/99','feat','main',?,?,?)`, repo, time.Now(), time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	m := githubModule(t, env)
	if err := m.MigrateB70(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.MigrateB70(ctx); err != nil {
		t.Fatal(err)
	}
	if ps := pulls(t, env); len(ps) != 1 || ps[0].Number != 99 {
		t.Fatalf("migrated cache: %+v", ps)
	}
	gh.set(func(f *fakeGitHub) { f.pulls[repo] = []map[string]any{pull(99, "fresh", "feat", "")} })
	syncNow(t, env)
	if err := m.MigrateB70(ctx); err != nil {
		t.Fatal(err)
	}
	if ps := pulls(t, env); ps[0].Title != "fresh" {
		t.Fatalf("migration restored stale cache: %+v", ps)
	}
	var oldCount int
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM github_pulls WHERE number=99").Scan(&oldCount); err != nil || oldCount != 1 {
		t.Fatalf("old cache removed: %d %v", oldCount, err)
	}
}

type fakeForge struct {
	srv           *httptest.Server
	mu            sync.Mutex
	actionsStatus int
	requests      int
}

func newFakeForge(t *testing.T) *fakeForge {
	f := &fakeForge{actionsStatus: 404}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests++
		if r.Header.Get("Authorization") != "token fj-token" {
			w.WriteHeader(401)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/api/v1")
		var out any
		switch {
		case p == "/user":
			out = map[string]any{"login": "jo"}
		case p == "/user/repos":
			if r.URL.Query().Get("limit") == "" {
				t.Error("Forgejo missing limit")
			}
			out = []any{map[string]any{"full_name": repo, "private": true, "pushed_at": "2026-10-01T10:00:00Z"}}
		case p == "/repos/"+repo:
			out = map[string]any{"default_branch": "main", "html_url": "https://forge.test/" + repo, "private": true, "description": "forge", "open_issues_count": 0}
		case strings.HasSuffix(p, "/pulls") || strings.HasSuffix(p, "/issues") || strings.HasSuffix(p, "/releases"):
			out = []any{}
		case strings.HasSuffix(p, "/commits"):
			out = []any{commit("abc", "forge commit")}
		case strings.HasSuffix(p, "/status"):
			out = map[string]any{"state": "failure", "total_count": 1, "statuses": []any{map[string]any{"id": 8, "context": "test", "state": "failure", "target_url": "https://forge.test/ci/8", "created_at": "2026-10-01T10:00:00Z", "updated_at": "2026-10-01T10:01:00Z"}}}
		case strings.HasSuffix(p, "/actions/runs"):
			if f.actionsStatus != 200 {
				w.WriteHeader(f.actionsStatus)
				return
			}
			out = map[string]any{"workflow_runs": []any{run(7, 1, "CI", "main", "success", "")}}
		case strings.HasSuffix(p, "/jobs"):
			w.WriteHeader(404)
			return
		default:
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func TestForgejoFallbackAndRunsWithoutJobs(t *testing.T) {
	for _, status := range []int{404, 200, 403, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			env := testutil.New(t)
			fj := newFakeForge(t)
			fj.actionsStatus = status
			env.Elevate()
			c := createConnection(t, env, map[string]any{"kind": "forgejo", "name": "Forgejo", "baseUrl": fj.srv.URL, "token": "fj-token"})
			env.MustDo(http.MethodPut, "/github/config", map[string]any{"watches": []api.RepoWatch{{ConnectionId: c.ID, Repo: repo}}}, nil)
			var available api.GitHubAvailableRepos
			env.MustDo(http.MethodGet, fmt.Sprintf("/github/available-repos?connectionId=%d", c.ID), nil, &available)
			if len(available.Repos) != 1 {
				t.Fatalf("available: %+v", available)
			}
			st := syncNow(t, env)
			var rs []api.GitHubRun
			env.MustDo(http.MethodGet, "/github/runs", nil, &rs)
			if status == 403 || status == 500 {
				if st.LastError == nil || len(rs) != 0 {
					t.Fatalf("failure must not fallback: %+v %+v", st, rs)
				}
				return
			}
			if st.LastError != nil || len(rs) != 1 {
				t.Fatalf("sync: %+v %+v", st, rs)
			}
			if rs[0].Forge == nil || *rs[0].Forge != "forgejo" || rs[0].Status != "completed" {
				t.Fatalf("run: %+v", rs[0])
			}
			if status == 404 && (rs[0].Id >= 0 || rs[0].Conclusion != "failure") {
				t.Fatalf("fallback: %+v", rs[0])
			}
			if s, _ := env.Do(http.MethodGet, fmt.Sprintf("/github/runs/%d/jobs?repo=%s&connectionId=%d", rs[0].Id, repo, c.ID), nil, nil); s != 501 {
				t.Fatalf("jobs unsupported: %d", s)
			}
		})
	}
}
