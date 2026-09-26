package coding_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	agentcoding "github.com/j0x3n/x-console/backend/internal/agent/coding"
	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

var desktopCaps = []string{protocol.CapSystemInfo, protocol.CapCoding}

// needGit skips tests that drive the fake executor script.
func needGit(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake executor is a shell script")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

// setup starts a server (coding is registered in app/modules.go) and
// returns the coding module.
func setup(t *testing.T) (*testutil.Env, *coding.Module) {
	t.Helper()
	env := testutil.New(t)
	c, ok := module.Lookup[contracts.Coding](env.App.Deps.Registry, contracts.CodingKey)
	if !ok {
		t.Fatal("contracts.Coding not provided")
	}
	return env, c.(*coding.Module)
}

// fakeExecutor is the fake Claude Code script of the agent package.
func fakeExecutor(t *testing.T) string {
	p, err := filepath.Abs("../../../agent/coding/testdata/fake-claude.sh")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// startAgent runs a desktop agent with the real coding package, using the
// fake executor as claude. stop takes it offline.
func startAgent(t *testing.T, env *testutil.Env) (string, func()) {
	t.Helper()
	env.Elevate()
	var pc struct{ Code string }
	env.MustDo(http.MethodPost, "/agents/pairing-codes", map[string]string{"name": "desk", "kind": "desktop"}, &pc)
	hello := protocol.Hello{AgentVersion: "test", OS: "windows", Arch: "amd64", Hostname: "desk", Capabilities: desktopCaps}
	id, token, err := conn.Pair(context.Background(), env.Server.URL, pc.Code, hello)
	if err != nil {
		t.Fatal(err)
	}
	client := conn.New(env.Server.URL, token, hello)
	agentcoding.New(agentcoding.Config{Executors: map[string]agentcoding.ExecutorConfig{
		"claude": {Path: fakeExecutor(t)},
		"codex":  {Path: "/nonexistent/codex"},
	}}).Register(client)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = client.Run(ctx)
		close(done)
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
	t.Cleanup(stop)
	waitFor(t, "agent online", func() bool { return env.App.Deps.Agents.Online(id) })
	return id, stop
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@x")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo makes <tmp>/code/demo with one commit on main. Its origin looks
// like GitHub (jo/demo) but pushes go to a local bare repository.
func newRepo(t *testing.T) (root, repo, bare string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "code")
	repo = filepath.Join(root, "demo")
	bare = filepath.Join(base, "demo.git")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, base, "init", "--quiet", "--bare", bare)
	git(t, repo, "init", "--quiet", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", ".")
	git(t, repo, "commit", "--quiet", "-m", "init")
	git(t, repo, "remote", "add", "origin", "https://github.com/jo/demo.git")
	git(t, repo, "config", "url."+bare+".pushInsteadOf", "https://github.com/jo/demo.git")
	git(t, repo, "config", "user.name", "T")
	git(t, repo, "config", "user.email", "t@x")
	return root, repo, bare
}

// register adds the repository through the API.
func register(t *testing.T, env *testutil.Env, agentID, path string) api.Repo {
	t.Helper()
	var repo api.Repo
	env.MustDo(http.MethodPost, "/coding/repos", api.CreateRepo{AgentId: agentID, Path: path}, &repo)
	return repo
}

func createTask(t *testing.T, env *testutil.Env, body api.CreateTask) api.Task {
	t.Helper()
	var task api.Task
	env.MustDo(http.MethodPost, "/coding/tasks", body, &task)
	return task
}

func getTask(t *testing.T, env *testutil.Env, id int64) api.Task {
	t.Helper()
	var task api.Task
	env.MustDo(http.MethodGet, "/coding/tasks/"+itoa(id), nil, &task)
	return task
}

// waitStatus waits until the task has one of the statuses.
func waitStatus(t *testing.T, env *testutil.Env, id int64, statuses ...api.TaskStatus) api.Task {
	t.Helper()
	var task api.Task
	deadline := time.Now().Add(20 * time.Second)
	for {
		task = getTask(t, env, id)
		for _, s := range statuses {
			if task.Status == s {
				return task
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("task %d: status %s, want %v (error %q)", id, task.Status, statuses, task.Error)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func events(t *testing.T, env *testutil.Env, id int64) []api.TaskEvent {
	t.Helper()
	var list api.TaskEventList
	env.MustDo(http.MethodGet, "/coding/tasks/"+itoa(id)+"/events", nil, &list)
	return list.Items
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// relogin starts a fresh session that is not elevated.
func relogin(t *testing.T, env *testutil.Env) {
	t.Helper()
	env.MustDo(http.MethodPost, "/auth/logout", nil, nil)
	env.MustDo(http.MethodPost, "/auth/login", map[string]string{"username": testutil.Username, "password": testutil.Password, "code": env.Code()}, nil)
}

func errCode(raw []byte) string {
	var e struct{ Code string }
	_ = json.Unmarshal(raw, &e)
	return e.Code
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fakeIssues records what the coding module does to issues.
type fakeIssues struct {
	mu       sync.Mutex
	statuses []string
	links    []contracts.IssueLink
}

func (f *fakeIssues) Get(_ context.Context, key string) (contracts.IssueRef, error) {
	if key != "XC-7" {
		return contracts.IssueRef{}, httpNotFound
	}
	return contracts.IssueRef{ID: 7, Key: "XC-7", Title: "Fix the login bug", Description: "Users cannot log in.", Status: "todo"}, nil
}
func (f *fakeIssues) Create(context.Context, contracts.CreateIssue) (contracts.IssueRef, error) {
	return contracts.IssueRef{}, nil
}
func (f *fakeIssues) SetStatus(_ context.Context, key, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = append(f.statuses, key+"="+status)
	return nil
}
func (f *fakeIssues) AttachLink(_ context.Context, key string, link contracts.IssueLink) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.links = append(f.links, link)
	return nil
}
func (f *fakeIssues) ListDue(context.Context, time.Time) ([]contracts.IssueRef, error) {
	return nil, nil
}

func (f *fakeIssues) snapshot() ([]string, []contracts.IssueLink) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.statuses...), append([]contracts.IssueLink{}, f.links...)
}

// fakeGitHub records pull requests.
type fakeGitHub struct {
	mu  sync.Mutex
	prs []contracts.CreatePR
}

func (f *fakeGitHub) CreatePR(_ context.Context, in contracts.CreatePR) (string, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prs = append(f.prs, in)
	return "https://github.com/" + in.Repo + "/pull/34", 34, nil
}
