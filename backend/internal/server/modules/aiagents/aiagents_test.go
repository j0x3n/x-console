package aiagents_test

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

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const secretToken = "forgejo-token-ABC123"

// fakeGit is a Forgejo (prefix "/api/v1") or GitHub (no prefix) server.
type fakeGit struct {
	*httptest.Server
	mu    sync.Mutex
	pulls []map[string]any
	auth  []string
}

func newFakeGit(t *testing.T, prefix, authScheme string) *fakeGit {
	f := &fakeGit{}
	mux := http.NewServeMux()
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	ok := func(w http.ResponseWriter, r *http.Request) bool {
		f.mu.Lock()
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		f.mu.Unlock()
		if r.Header.Get("Authorization") != authScheme+" "+secretToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"bad credentials"}`))
			return false
		}
		return true
	}
	repo := func(owner, name string, updated string) map[string]any {
		return map[string]any{"owner": map[string]any{"login": owner}, "name": name, "full_name": owner + "/" + name,
			"clone_url": f.URL + "/" + owner + "/" + name + ".git", "html_url": f.URL + "/" + owner + "/" + name,
			"default_branch": "main", "private": true, "updated_at": updated}
	}
	mux.HandleFunc("GET "+prefix+"/user", func(w http.ResponseWriter, r *http.Request) {
		if ok(w, r) {
			_, _ = w.Write([]byte(`{"login":"alice","username":"alice"}`))
		}
	})
	repos := []map[string]any{repo("alice", "old-tool", "2026-01-01T00:00:00Z"), repo("team", "x-console", "2026-09-01T00:00:00Z")}
	mux.HandleFunc("GET "+prefix+"/repos/search", func(w http.ResponseWriter, r *http.Request) {
		if ok(w, r) {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": repos})
		}
	})
	mux.HandleFunc("GET "+prefix+"/user/repos", func(w http.ResponseWriter, r *http.Request) {
		if ok(w, r) {
			_ = json.NewEncoder(w).Encode(repos)
		}
	})
	mux.HandleFunc("POST "+prefix+"/repos/{owner}/{repo}/pulls", func(w http.ResponseWriter, r *http.Request) {
		if !ok(w, r) {
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["repo"] = r.PathValue("owner") + "/" + r.PathValue("repo")
		f.mu.Lock()
		f.pulls = append(f.pulls, body)
		n := len(f.pulls)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"number": n, "html_url": fmt.Sprintf("%s/%s/pulls/%d", f.URL, body["repo"], n)})
	})
	return f
}

type connection struct {
	ID              int64
	Kind, Name      string
	BaseURL         string `json:"baseUrl"`
	Username        string
	HasToken        bool
	LastError       string
	WebhookPath     string
	UseGithubModule bool
}

func TestForgejoConnection(t *testing.T) {
	env := testutil.New(t)
	git := newFakeGit(t, "/api/v1", "token")
	body := map[string]any{"kind": "forgejo", "name": "自建 Forgejo", "baseUrl": git.URL + "/api/v1/", "token": secretToken}
	// Saving a token needs a fresh verification.
	if s, _ := env.Do(http.MethodPost, "/git-connections", body, nil); s != http.StatusForbidden {
		t.Fatalf("create without verification: %d", s)
	}
	env.Elevate()
	var created struct {
		Connection    connection
		WebhookSecret string
	}
	env.MustDo(http.MethodPost, "/git-connections", body, &created)
	c := created.Connection
	if c.Username != "alice" || c.LastError != "" || c.BaseURL != git.URL || !c.HasToken || created.WebhookSecret == "" ||
		c.WebhookPath != fmt.Sprintf("/hooks/git/%d", c.ID) {
		t.Fatalf("connection: %+v", created)
	}
	// The token is stored encrypted and never returned.
	var enc, hook string
	if err := env.App.Deps.DB.QueryRow(`SELECT token_enc, webhook_secret_enc FROM git_connections WHERE id = ?`, c.ID).Scan(&enc, &hook); err != nil {
		t.Fatal(err)
	}
	if enc == "" || strings.Contains(enc, secretToken) || strings.Contains(hook, created.WebhookSecret) {
		t.Fatalf("stored in plain text: %q %q", enc, hook)
	}
	_, raw := env.Do(http.MethodGet, "/git-connections", nil, nil)
	if strings.Contains(string(raw), secretToken) || strings.Contains(string(raw), created.WebhookSecret) {
		t.Fatalf("list leaks a secret: %s", raw)
	}
	var wh struct{ Path, Secret string }
	env.MustDo(http.MethodGet, fmt.Sprintf("/git-connections/%d/webhook", c.ID), nil, &wh)
	if wh.Secret != created.WebhookSecret {
		t.Fatalf("webhook: %+v", wh)
	}

	// Repositories, newest first, filtered by name.
	var repos []struct {
		Owner, Name, FullName, CloneURL, DefaultBranch string
		Private                                        bool
	}
	env.MustDo(http.MethodGet, fmt.Sprintf("/git-connections/%d/repos", c.ID), nil, &repos)
	if len(repos) != 2 || repos[0].FullName != "team/x-console" || !repos[0].Private || repos[0].DefaultBranch != "main" {
		t.Fatalf("repos: %+v", repos)
	}
	env.MustDo(http.MethodGet, fmt.Sprintf("/git-connections/%d/repos?q=old", c.ID), nil, &repos)
	if len(repos) != 1 || repos[0].Name != "old-tool" {
		t.Fatalf("filtered repos: %+v", repos)
	}

	// The coding module opens pull requests and clones through the contract.
	conns, ok := module.Lookup[contracts.GitConnections](env.App.Deps.Registry, contracts.GitConnectionsKey)
	if !ok {
		t.Fatal("contracts.GitConnections not provided")
	}
	url, number, err := conns.CreatePR(context.Background(), c.ID, contracts.CreatePR{Repo: "team/x-console", Head: "xc/1-fix", Base: "main", Title: "修好登录"})
	if err != nil || number != 1 || !strings.HasSuffix(url, "/team/x-console/pulls/1") {
		t.Fatalf("pr: %q %d %v", url, number, err)
	}
	if git.pulls[0]["head"] != "xc/1-fix" || git.pulls[0]["title"] != "修好登录" {
		t.Fatalf("pr body: %+v", git.pulls[0])
	}
	user, token, err := conns.CloneAuth(context.Background(), c.ID)
	if err != nil || user != "alice" || token != secretToken {
		t.Fatalf("clone auth: %q %q %v", user, token, err)
	}

	// A wrong token is saved with the error, and fixed by a new token.
	env.MustDo(http.MethodPatch, fmt.Sprintf("/git-connections/%d", c.ID), map[string]any{"token": "wrong"}, &c)
	if c.LastError == "" || !strings.Contains(c.LastError, "401") {
		t.Fatalf("bad token: %+v", c)
	}
	if s, _ := env.Do(http.MethodGet, fmt.Sprintf("/git-connections/%d/repos", c.ID), nil, nil); s != http.StatusBadGateway {
		t.Fatalf("repos with bad token: %d", s)
	}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/git-connections/%d", c.ID), map[string]any{"token": secretToken, "name": "家里的 Forgejo"}, &c)
	if c.LastError != "" || c.Name != "家里的 Forgejo" {
		t.Fatalf("fixed token: %+v", c)
	}

	env.MustDo(http.MethodDelete, fmt.Sprintf("/git-connections/%d", c.ID), nil, nil)
	var list []connection
	env.MustDo(http.MethodGet, "/git-connections", nil, &list)
	if len(list) != 0 {
		t.Fatalf("after delete: %+v", list)
	}
}

func TestGitHubConnection(t *testing.T) {
	env := testutil.New(t)
	git := newFakeGit(t, "", "Bearer")
	env.Elevate()
	if s, _ := env.Do(http.MethodPost, "/git-connections", map[string]any{"kind": "forgejo", "name": "x", "token": "t"}, nil); s != 400 {
		t.Fatalf("forgejo without address: %d", s)
	}
	if s, _ := env.Do(http.MethodPost, "/git-connections", map[string]any{"kind": "forgejo", "name": "x", "baseUrl": "https://a", "useGithubModule": true}, nil); s != 400 {
		t.Fatalf("forgejo with the GitHub module token: %d", s)
	}
	var created struct{ Connection connection }
	env.MustDo(http.MethodPost, "/git-connections", map[string]any{"kind": "github", "name": "GitHub", "baseUrl": git.URL, "token": secretToken}, &created)
	if created.Connection.Username != "alice" {
		t.Fatalf("github: %+v", created)
	}
	var repos []struct{ FullName string }
	env.MustDo(http.MethodGet, fmt.Sprintf("/git-connections/%d/repos", created.Connection.ID), nil, &repos)
	if len(repos) != 2 {
		t.Fatalf("github repos: %+v", repos)
	}
	conns, _ := module.Lookup[contracts.GitConnections](env.App.Deps.Registry, contracts.GitConnectionsKey)
	if user, _, _ := conns.CloneAuth(context.Background(), created.Connection.ID); user != "x-access-token" {
		t.Fatalf("github clone user: %q", user)
	}
	if _, _, err := conns.CreatePR(context.Background(), created.Connection.ID, contracts.CreatePR{Repo: "alice/old-tool", Head: "xc/2-a", Base: "main", Title: "t", Draft: true}); err != nil {
		t.Fatal(err)
	}
	if git.pulls[0]["draft"] != true {
		t.Fatalf("draft: %+v", git.pulls[0])
	}
	// The GitHub module has no token: the connection saves with an error.
	env.MustDo(http.MethodPost, "/git-connections", map[string]any{"kind": "github", "name": "共用", "useGithubModule": true}, &created)
	if !created.Connection.UseGithubModule || created.Connection.LastError == "" {
		t.Fatalf("shared token: %+v", created.Connection)
	}
}

type agent struct {
	ID               int64
	Name, Kind       string
	Model            string
	RunnerAgentID    *string `json:"runnerAgentId"`
	RunnerName       *string
	CliPermission    string
	RepoIDs          []int64 `json:"repoIds"`
	MonthlyBudgetUsd *float64
	MonthCostUsd     float64
	OverBudget       bool
	RunningTasks     int
	QueuedTasks      int
	Enabled          bool
}

func TestAgents(t *testing.T) {
	env := testutil.New(t)
	// Full CLI permission needs a fresh verification.
	if s, _ := env.Do(http.MethodPost, "/ai-agents", map[string]any{"name": "全能", "kind": "codex", "cliPermission": "full"}, nil); s != http.StatusForbidden {
		t.Fatalf("full without verification: %d", s)
	}
	if s, _ := env.Do(http.MethodPost, "/ai-agents", map[string]any{"name": "整理员", "kind": "builtin"}, nil); s != 400 {
		t.Fatalf("builtin without model: %d", s)
	}
	var b agent
	env.MustDo(http.MethodPost, "/ai-agents", map[string]any{"name": "整理员", "kind": "builtin", "model": "1:gpt-5", "avatar": "🧹"}, &b)
	if b.Kind != "builtin" || b.Model != "1:gpt-5" || !b.Enabled || b.CliPermission != "workspace" || len(b.RepoIDs) != 0 {
		t.Fatalf("builtin: %+v", b)
	}

	runner := env.Agent("server", []string{protocol.CapSystemInfo, protocol.CapCoding}, nil)
	noCoding := env.Agent("server", []string{protocol.CapSystemInfo}, nil)
	if s, _ := env.Do(http.MethodPost, "/ai-agents", map[string]any{"name": "x", "kind": "claude_code", "runnerAgentId": noCoding}, nil); s != 400 {
		t.Fatalf("runner without coding: %d", s)
	}
	db := env.App.Deps.DB
	res, err := db.Exec(`INSERT INTO coding_repos (agent_id, path, name, created_at) VALUES (?, '/srv/x', 'x', ?)`, runner, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	repoID, _ := res.LastInsertId()
	var c agent
	env.MustDo(http.MethodPost, "/ai-agents", map[string]any{"name": "后端开发", "kind": "claude_code", "runnerAgentId": runner,
		"repoIds": []int64{repoID, repoID}, "monthlyBudgetUsd": 5}, &c)
	if c.RunnerAgentID == nil || *c.RunnerAgentID != runner || c.RunnerName == nil || len(c.RepoIDs) != 1 || *c.MonthlyBudgetUsd != 5 {
		t.Fatalf("cli agent: %+v", c)
	}
	if s, _ := env.Do(http.MethodPost, "/ai-agents", map[string]any{"name": "x", "kind": "codex", "repoIds": []int64{999}}, nil); s != 400 {
		t.Fatalf("unknown repo: %d", s)
	}

	// Patch: null clears, missing keeps; the kind cannot change.
	id := c.ID
	c = agent{}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/ai-agents/%d", id), map[string]any{"monthlyBudgetUsd": nil, "name": "后端"}, &c)
	if c.MonthlyBudgetUsd != nil || c.Name != "后端" || c.RunnerAgentID == nil {
		t.Fatalf("patch: %+v", c)
	}
	if s, _ := env.Do(http.MethodPatch, fmt.Sprintf("/ai-agents/%d", c.ID), map[string]any{"kind": "codex"}, nil); s != 400 {
		t.Fatalf("change kind: %d", s)
	}

	// Budget: the month's cost comes from the usage records of its tasks.
	res, err = db.Exec(`INSERT INTO coding_tasks (repo_id, executor, prompt, created_at, updated_at, ai_agent_id, status)
		VALUES (?, 'claude', 'p', ?, ?, ?, 'queued')`, repoID, time.Now(), time.Now(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	taskID, _ := res.LastInsertId()
	now := time.Now().UTC()
	for _, u := range []struct {
		source, ref string
		cost        float64
		at          time.Time
	}{{"coding", fmt.Sprint(taskID), 2.5, now}, {"ai_agent", fmt.Sprint(c.ID), 1, now}, {"coding", fmt.Sprint(taskID), 100, now.AddDate(0, -2, 0)}, {"assistant", "1", 50, now}} {
		if _, err := db.Exec(`INSERT INTO ai_usage (provider_name, model, purpose, cost, created_at, source, ref) VALUES ('p', 'm', 'agent', ?, ?, ?, ?)`,
			u.cost, u.at, u.source, u.ref); err != nil {
			t.Fatal(err)
		}
	}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/ai-agents/%d", c.ID), map[string]any{"monthlyBudgetUsd": 3}, &c)
	if c.MonthCostUsd != 3.5 || !c.OverBudget || c.QueuedTasks != 1 {
		t.Fatalf("budget: %+v", c)
	}
	agents, _ := module.Lookup[contracts.AIAgents](env.App.Deps.Registry, contracts.AIAgentsKey)
	got, err := agents.Get(context.Background(), c.ID)
	if err != nil || !got.OverBudget || got.RunnerAgentID != runner || len(got.RepoIDs) != 1 {
		t.Fatalf("contract: %+v %v", got, err)
	}

	// An agent with queued work cannot be deleted; afterwards its tasks stay.
	if s, _ := env.Do(http.MethodDelete, fmt.Sprintf("/ai-agents/%d", c.ID), nil, nil); s != http.StatusConflict {
		t.Fatalf("delete busy: %d", s)
	}
	if _, err := db.Exec(`UPDATE coding_tasks SET status = 'review' WHERE id = ?`, taskID); err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/ai-agents/%d", c.ID), nil, nil)
	var left *int64
	if err := db.QueryRow(`SELECT ai_agent_id FROM coding_tasks WHERE id = ?`, taskID).Scan(&left); err != nil || left != nil {
		t.Fatalf("task after delete: %v %v", left, err)
	}
	var list []agent
	env.MustDo(http.MethodGet, "/ai-agents", nil, &list)
	if len(list) != 1 || list[0].ID != b.ID {
		t.Fatalf("list: %+v", list)
	}
}
