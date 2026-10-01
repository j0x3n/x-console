package coding_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	agentcoding "github.com/j0x3n/x-console/backend/internal/agent/coding"
	"github.com/j0x3n/x-console/backend/internal/agent/coding/gittest"
	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	agentfiles "github.com/j0x3n/x-console/backend/internal/agent/files"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const forgejoToken = "fj-token-XYZ"

// startRemoteAgent runs an agent that can clone (CapCodingRemote) into
// reposDir, with the fake executor as claude.
func startRemoteAgent(t *testing.T, env *testutil.Env, reposDir string) string {
	t.Helper()
	env.Elevate()
	var pc struct{ Code string }
	env.MustDo(http.MethodPost, "/agents/pairing-codes", map[string]string{"name": "builder", "kind": "server"}, &pc)
	hello := protocol.Hello{AgentVersion: "test", OS: "linux", Arch: "amd64", Hostname: "builder",
		Capabilities: []string{protocol.CapSystemInfo, protocol.CapFiles, protocol.CapCoding, protocol.CapCodingRemote}}
	id, token, err := conn.Pair(context.Background(), env.Server.URL, pc.Code, hello)
	if err != nil {
		t.Fatal(err)
	}
	client := conn.New(env.Server.URL, token, hello)
	agentcoding.New(agentcoding.Config{ReposDir: reposDir, Executors: map[string]agentcoding.ExecutorConfig{
		"claude": {Path: fakeExecutor(t)},
	}}).Register(client)
	agentfiles.Register(client)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = client.Run(ctx) }()
	waitFor(t, "agent online", func() bool { return env.App.Deps.Agents.Online(id) })
	return id
}

// fakeForgejo answers /api/v1/user and opens pull requests.
type fakeForgejo struct {
	*httptest.Server
	mu    sync.Mutex
	pulls []map[string]any
}

func newFakeForgejo(t *testing.T) *fakeForgejo {
	f := &fakeForgejo{}
	mux := http.NewServeMux()
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "token "+forgejoToken {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			_, _ = w.Write([]byte(`{"login":"alice"}`))
		}
	})
	mux.HandleFunc("POST /api/v1/repos/{owner}/{repo}/pulls", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["repo"] = r.PathValue("owner") + "/" + r.PathValue("repo")
		f.mu.Lock()
		f.pulls = append(f.pulls, body)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"number":5,"html_url":"%s/%s/pulls/5"}`, f.URL, body["repo"])
	})
	return f
}

func TestRemoteRepoWithAIAgent(t *testing.T) {
	needGit(t)
	env, _ := setup(t)
	_, work, bare := newRepo(t)
	git(t, work, "config", "--unset", "url."+bare+".pushInsteadOf")
	git(t, work, "remote", "set-url", "origin", bare)
	git(t, work, "push", "--quiet", "origin", "main")
	git(t, bare, "symbolic-ref", "HEAD", "refs/heads/main")
	git(t, bare, "config", "http.receivepack", "true")
	gitSrv := gittest.New(t, filepath.Dir(bare), "alice", forgejoToken)
	cloneURL := gitSrv.URL + "/demo.git"
	fj := newFakeForgejo(t)
	reposDir := t.TempDir()
	agentID := startRemoteAgent(t, env, reposDir)

	var conn struct{ Connection struct{ ID int64 } }
	env.MustDo(http.MethodPost, "/git-connections", map[string]any{"kind": "forgejo", "name": "fj", "baseUrl": fj.URL, "token": forgejoToken}, &conn)
	connID := conn.Connection.ID

	// Register by remote: the agent clones it into its repository folder.
	var repo api.Repo
	body := api.CreateRepo{AgentId: agentID, ConnectionId: &connID, RemoteRepo: ptr("team/demo"), CloneUrl: &cloneURL}
	env.MustDo(http.MethodPost, "/coding/repos", body, &repo)
	if repo.Path != filepath.Join(reposDir, fmt.Sprint(connID), "team", "demo") || repo.DefaultBranch != "main" ||
		repo.RemoteRepo == nil || *repo.RemoteRepo != "team/demo" || repo.ConnectionId == nil {
		t.Fatalf("repo: %+v", repo)
	}
	if s, _ := env.Do(http.MethodPost, "/coding/repos", body, nil); s != http.StatusConflict {
		t.Fatalf("second registration: %d", s)
	}
	bad := cloneURL
	bad = strings.Replace(bad, "http://", "http://u:p@", 1)
	if s, _ := env.Do(http.MethodPost, "/coding/repos", api.CreateRepo{AgentId: agentID, ConnectionId: &connID, RemoteRepo: ptr("team/x"), CloneUrl: &bad}, nil); s != 400 {
		t.Fatalf("credentials in clone url: %d", s)
	}

	// An AI agent allowed on this repository.
	var agent struct{ ID int64 }
	env.MustDo(http.MethodPost, "/ai-agents", map[string]any{"name": "后端", "kind": "claude_code", "instructions": "你是测试 Agent。",
		"model": "opus", "runnerAgentId": agentID, "repoIds": []int64{repo.Id}}, &agent)
	var other struct{ ID int64 }
	env.MustDo(http.MethodPost, "/ai-agents", map[string]any{"name": "别的", "kind": "claude_code"}, &other)
	if s, _ := env.Do(http.MethodPost, "/coding/tasks", api.CreateTask{RepoId: repo.Id, AiAgentId: &other.ID, Prompt: ptr("x")}, nil); s != 400 {
		t.Fatalf("agent without the repo: %d", s)
	}

	// A new upstream commit after the clone: the task still starts from it.
	git(t, work, "commit", "--quiet", "--allow-empty", "-m", "upstream")
	git(t, work, "push", "--quiet", "origin", "main")
	upstream := git(t, work, "rev-parse", "HEAD")

	task := createTask(t, env, api.CreateTask{RepoId: repo.Id, AiAgentId: &agent.ID, Prompt: ptr("Add a hello file")})
	if task.AiAgentId == nil || *task.AiAgentId != agent.ID || task.Model == nil || *task.Model != "opus" ||
		task.Executor != "claude" || !strings.HasPrefix(task.Prompt, "你是测试 Agent。") {
		t.Fatalf("task: %+v", task)
	}
	task = waitStatus(t, env, task.Id, "review")
	if task.BaseCommit == nil || *task.BaseCommit != upstream {
		t.Fatalf("base %v, want %s", task.BaseCommit, upstream)
	}

	// Commit, then the PR pushes with the token and opens on Forgejo.
	env.Elevate()
	env.MustDo(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/commit", map[string]any{}, nil)
	env.MustDo(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/pr", map[string]any{}, &task)
	if task.Status != "pr_opened" || !strings.HasSuffix(task.PrUrl, "/team/demo/pulls/5") {
		t.Fatalf("pr: %+v", task)
	}
	if fj.pulls[0]["head"] != task.Branch || fj.pulls[0]["base"] != "main" || fj.pulls[0]["repo"] != "team/demo" {
		t.Fatalf("pull: %+v", fj.pulls[0])
	}
	if out := git(t, bare, "branch", "--list", task.Branch); !strings.Contains(out, task.Branch) {
		t.Fatalf("branch not pushed: %q", out)
	}

	// The list filters by agent.
	var list api.TaskList
	env.MustDo(http.MethodGet, fmt.Sprintf("/coding/tasks?aiAgentId=%d", other.ID), nil, &list)
	if len(list.Items) != 0 {
		t.Fatalf("other agent's tasks: %+v", list.Items)
	}
	env.MustDo(http.MethodGet, fmt.Sprintf("/coding/tasks?aiAgentId=%d", agent.ID), nil, &list)
	if len(list.Items) != 1 {
		t.Fatalf("agent's tasks: %+v", list.Items)
	}

	// Disabled agents take no new tasks.
	env.MustDo(http.MethodPatch, fmt.Sprintf("/ai-agents/%d", agent.ID), map[string]any{"enabled": false}, nil)
	if s, _ := env.Do(http.MethodPost, "/coding/tasks", api.CreateTask{RepoId: repo.Id, AiAgentId: &agent.ID, Prompt: ptr("x")}, nil); s != http.StatusConflict {
		t.Fatalf("disabled agent: %d", s)
	}
}

// remoteRepo registers a Forgejo repository on a fresh agent.
func remoteRepo(t *testing.T) (*testutil.Env, api.Repo, string) {
	t.Helper()
	needGit(t)
	env, _ := setup(t)
	_, work, bare := newRepo(t)
	git(t, work, "config", "--unset", "url."+bare+".pushInsteadOf")
	git(t, work, "remote", "set-url", "origin", bare)
	git(t, work, "push", "--quiet", "origin", "main")
	git(t, bare, "symbolic-ref", "HEAD", "refs/heads/main")
	gitSrv := gittest.New(t, filepath.Dir(bare), "alice", forgejoToken)
	cloneURL := gitSrv.URL + "/demo.git"
	fj := newFakeForgejo(t)
	agentID := startRemoteAgent(t, env, t.TempDir())
	var conn struct{ Connection struct{ ID int64 } }
	env.MustDo(http.MethodPost, "/git-connections", map[string]any{"kind": "forgejo", "name": "fj", "baseUrl": fj.URL, "token": forgejoToken}, &conn)
	var repo api.Repo
	env.MustDo(http.MethodPost, "/coding/repos", api.CreateRepo{AgentId: agentID, ConnectionId: &conn.Connection.ID,
		RemoteRepo: ptr("team/demo"), CloneUrl: &cloneURL}, &repo)
	return env, repo, agentID
}

func TestBuildRetryAndArtifacts(t *testing.T) {
	env, repo, _ := remoteRepo(t)
	// The build passes only once the agent was told that it failed.
	cfg := api.BuildConfig{Windows: []api.BuildStep{}, Linux: []api.BuildStep{
		{Name: "检查", Command: "grep -q 构建失败 hello.txt"},
		{Name: "打包", Command: "mkdir -p dist && cp hello.txt dist/app.txt", Artifacts: &[]string{"dist/**"}},
	}}
	env.MustDo(http.MethodPut, "/coding/repos/"+itoa(repo.Id)+"/build-config", cfg, &repo)
	if repo.BuildConfig == nil || len(repo.BuildConfig.Linux) != 2 {
		t.Fatalf("config: %+v", repo.BuildConfig)
	}
	var agent struct{ ID int64 }
	env.MustDo(http.MethodPost, "/ai-agents", map[string]any{"name": "修构建", "kind": "claude_code", "repoIds": []int64{repo.Id}, "buildRetries": 1}, &agent)

	task := createTask(t, env, api.CreateTask{RepoId: repo.Id, AiAgentId: &agent.ID, Prompt: ptr("Add a hello file")})
	var got api.Task
	waitFor(t, "build passed", func() bool {
		got = getTask(t, env, task.Id)
		return got.BuildStatus != nil && *got.BuildStatus == "passed"
	})
	if got.Status != "review" || *got.BuildAttempts != 2 || got.Artifacts == nil || len(*got.Artifacts) != 1 || (*got.Artifacts)[0].Name != "dist/app.txt" {
		t.Fatalf("task: status %s attempts %v artifacts %+v error %v", got.Status, *got.BuildAttempts, got.Artifacts, *got.BuildError)
	}
	// The log has both builds and the fix run.
	var steps int
	for _, ev := range events(t, env, task.Id) {
		if ev.Kind == "status" && ev.Data != nil && (*ev.Data)["code"] == "build_step" {
			steps++
		}
	}
	if steps != 3 {
		t.Fatalf("build steps in the log: %d", steps)
	}
	// The artifact downloads with the content the agent wrote.
	resp, err := env.Client.Get(env.URL("/coding/tasks/" + itoa(task.Id) + "/artifacts/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := new(strings.Builder)
	_, _ = io.Copy(body, resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(body.String(), "上一次的改动构建失败了") {
		t.Fatalf("artifact: %d %q", resp.StatusCode, body.String())
	}
	if s, _ := env.Do(http.MethodGet, "/coding/tasks/"+itoa(task.Id)+"/artifacts/5", nil, nil); s != http.StatusNotFound {
		t.Fatalf("missing artifact: %d", s)
	}

	// A manual build without retries left that fails stays failed.
	cfg.Linux = []api.BuildStep{{Name: "坏", Command: "echo nope >&2; exit 2"}}
	env.MustDo(http.MethodPut, "/coding/repos/"+itoa(repo.Id)+"/build-config", cfg, nil)
	env.MustDo(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/build", nil, &got)
	waitFor(t, "build failed", func() bool {
		got = getTask(t, env, task.Id)
		return *got.BuildStatus == "failed"
	})
	if got.Status != "review" || *got.BuildAttempts != 3 || got.BuildError == nil || len(*got.Artifacts) != 0 {
		t.Fatalf("failed build: %+v", got)
	}
	// Commit removes the worktree: no more builds.
	env.MustDo(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/commit", map[string]any{}, nil)
	if s, _ := env.Do(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/build", nil, nil); s != http.StatusConflict {
		t.Fatalf("build after commit: %d", s)
	}
}
