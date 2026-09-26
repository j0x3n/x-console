package coding_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	agentcoding "github.com/j0x3n/x-console/backend/internal/agent/coding"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/api"
)

var httpNotFound = httpx.ErrNotFound

func TestLogic(t *testing.T) {
	for in, want := range map[string]string{
		"Add a hello file!": "add-a-hello-file",
		"修复登录":              "",
		"Refactor the very long module name handling": "refactor-the-very-long-module",
		"Supercalifragilisticexpialidocious-and-more": "supercalifragilisticexpialidocio",
	} {
		if got := coding.Slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
	if got := coding.BranchName(12, "修复登录", "", "XC-3"); got != "xc/12-xc-3" {
		t.Errorf("branch %q", got)
	}
	if got := coding.BranchName(5, "修复"); got != "xc/5-task" {
		t.Errorf("branch %q", got)
	}
	for in, want := range map[string]string{
		"git@github.com:jo/demo.git":           "jo/demo",
		"https://github.com/jo/x-console":      "jo/x-console",
		"https://token@github.com/jo/demo.git": "jo/demo",
		"ssh://git@github.com/jo/demo.git":     "jo/demo",
		"https://gitlab.com/jo/demo.git":       "",
		"/srv/git/demo.git":                    "",
	} {
		if got := coding.GithubRepo(in); got != want {
			t.Errorf("githubRepo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReposAndExecutors(t *testing.T) {
	needGit(t)
	env, _ := setup(t)
	root, repoPath, _ := newRepo(t)
	agentID, _ := startAgent(t, env)

	var execs []api.Executor
	env.MustDo(http.MethodGet, "/coding/executors?agentId="+agentID, nil, &execs)
	if len(execs) != 2 || !execs[0].Available || *execs[0].Version != "9.9.9 (Fake Claude)" || execs[1].Available {
		t.Fatalf("executors %+v", execs)
	}
	if status, _ := env.Do(http.MethodGet, "/coding/executors?agentId=nope", nil, nil); status != http.StatusNotFound {
		t.Fatalf("unknown agent: %d", status)
	}

	var found api.DiscoverResult
	env.MustDo(http.MethodGet, "/coding/repos/discover?agentId="+agentID+"&root="+root, nil, &found)
	if len(found.Items) != 1 || found.Items[0].Path != repoPath || found.Items[0].RepoId != nil {
		t.Fatalf("discover %+v", found)
	}

	repo := register(t, env, agentID, repoPath)
	if repo.Name != "demo" || repo.DefaultBranch != "main" || repo.GithubRepo != "jo/demo" || !repo.AgentOnline || repo.AgentName != "desk" {
		t.Fatalf("repo %+v", repo)
	}
	env.MustDo(http.MethodGet, "/coding/repos/discover?agentId="+agentID+"&root="+root, nil, &found)
	if found.Items[0].RepoId == nil || *found.Items[0].RepoId != repo.Id {
		t.Fatalf("discover after register %+v", found.Items)
	}
	if status, raw := env.Do(http.MethodPost, "/coding/repos", api.CreateRepo{AgentId: agentID, Path: repoPath}, nil); status != http.StatusConflict {
		t.Fatalf("duplicate: %d %s", status, raw)
	}
	if status, _ := env.Do(http.MethodPost, "/coding/repos", api.CreateRepo{AgentId: agentID, Path: root}, nil); status != http.StatusBadRequest {
		t.Fatalf("not a repo: %d", status)
	}
	var repos []api.Repo
	env.MustDo(http.MethodGet, "/coding/repos", nil, &repos)
	if len(repos) != 1 {
		t.Fatalf("repos %+v", repos)
	}
	env.MustDo(http.MethodDelete, "/coding/repos/"+itoa(repo.Id), nil, nil)
	if status, _ := env.Do(http.MethodDelete, "/coding/repos/"+itoa(repo.Id), nil, nil); status != http.StatusNotFound {
		t.Fatalf("second delete: %d", status)
	}
}

func TestTaskLifecycle(t *testing.T) {
	needGit(t)
	env, _ := setup(t)
	_, repoPath, bare := newRepo(t)
	agentID, _ := startAgent(t, env)
	repo := register(t, env, agentID, repoPath)
	output, cancel := env.App.Deps.Bus.Subscribe("coding_task.output", 256)
	defer cancel()

	task := createTask(t, env, api.CreateTask{RepoId: repo.Id, Executor: "claude", Prompt: ptr("Add a hello file\nwith details")})
	if task.Branch != "xc/"+itoa(task.Id)+"-add-a-hello-file" || task.BaseBranch != "main" || task.Title != "Add a hello file" || task.TimeoutMinutes != 60 {
		t.Fatalf("task %+v", task)
	}
	task = waitStatus(t, env, task.Id, "review")
	if task.ExitCode == nil || *task.ExitCode != 0 || len(task.ChangedFiles) != 2 || task.BaseCommit == nil || task.StartedAt == nil || task.FinishedAt == nil {
		t.Fatalf("finished task %+v", task)
	}
	worktree := agentcoding.WorktreePath(repoPath, task.Id)
	if !exists(worktree) {
		t.Fatal("worktree missing after the run")
	}

	// Output was stored in order and published in batches.
	evs := events(t, env, task.Id)
	kinds := map[api.TaskEventKind]int{}
	for i, ev := range evs {
		if ev.Seq != int64(i+1) {
			t.Fatalf("seq %d at %d", ev.Seq, i)
		}
		kinds[ev.Kind]++
	}
	last := evs[len(evs)-1]
	if kinds["text"] < 3 || kinds["tool"] != 2 || kinds["status"] < 3 || last.Kind != "done" || last.ExitCode == nil || *last.ExitCode != 0 {
		t.Fatalf("events %+v", evs)
	}
	var later api.TaskEventList
	env.MustDo(http.MethodGet, "/coding/tasks/"+itoa(task.Id)+"/events?after=3", nil, &later)
	if len(later.Items) != len(evs)-3 || later.Items[0].Seq != 4 || later.LastSeq != last.Seq {
		t.Fatalf("after=3: %+v", later)
	}
	published := 0
	for len(output) > 0 {
		ev := <-output
		raw, _ := json.Marshal(ev.Data)
		var payload struct {
			TaskID int64           `json:"taskId"`
			Events []api.TaskEvent `json:"events"`
		}
		_ = json.Unmarshal(raw, &payload)
		if payload.TaskID != task.Id || len(payload.Events) == 0 {
			t.Fatalf("output payload %s", raw)
		}
		published += len(payload.Events)
	}
	if published != len(evs) {
		t.Fatalf("published %d events, stored %d", published, len(evs))
	}

	// The review notification was sent.
	var notes struct {
		Items []struct{ Kind, Link string }
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) == 0 || notes.Items[0].Kind != "coding_task.review" || notes.Items[0].Link != "/coding/"+itoa(task.Id) {
		t.Fatalf("notifications %+v", notes)
	}

	var diff api.TaskDiff
	env.MustDo(http.MethodGet, "/coding/tasks/"+itoa(task.Id)+"/diff", nil, &diff)
	if len(diff.Files) != 2 || !strings.Contains(diff.Diff, "+changed") || !strings.Contains(diff.Diff, "hello.txt") {
		t.Fatalf("diff %+v", diff)
	}

	// Commit needs elevation.
	relogin(t, env)
	if status, raw := env.Do(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/commit", nil, nil); status != http.StatusForbidden || errCode(raw) != "elevation_required" {
		t.Fatalf("commit without elevation: %d %s", status, raw)
	}
	env.Elevate()
	env.MustDo(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/commit", api.CommitRequest{Push: ptr(true)}, &task)
	if task.Status != "pushed" || len(task.CommitSha) != 40 {
		t.Fatalf("after commit+push %+v", task)
	}
	if got := git(t, bare, "rev-parse", "refs/heads/"+task.Branch); got != task.CommitSha {
		t.Fatalf("remote branch at %q", got)
	}
	if subject := git(t, repoPath, "log", "-1", "--format=%s", task.Branch); subject != "Add a hello file" {
		t.Fatalf("commit subject %q", subject)
	}
	if exists(worktree) {
		t.Fatal("worktree kept after commit")
	}
	env.MustDo(http.MethodGet, "/coding/tasks/"+itoa(task.Id)+"/diff", nil, &diff)
	if len(diff.Files) != 2 {
		t.Fatalf("diff after commit %+v", diff)
	}

	// Pull requests need the GitHub module.
	var settings api.CodingSettings
	env.MustDo(http.MethodGet, "/coding/settings", nil, &settings)
	if settings.PrAvailable || settings.MaxConcurrent != 2 || settings.DefaultTimeoutMinutes != 60 {
		t.Fatalf("settings %+v", settings)
	}
	if status, raw := env.Do(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/pr", nil, nil); status != http.StatusNotImplemented || errCode(raw) != "feature_unavailable" {
		t.Fatalf("pr without github: %d %s", status, raw)
	}
	gh := &fakeGitHub{}
	module.Provide[contracts.GitHub](env.App.Deps.Registry, contracts.GitHubKey, gh)
	env.MustDo(http.MethodGet, "/coding/settings", nil, &settings)
	if !settings.PrAvailable {
		t.Fatal("prAvailable stays false")
	}
	env.MustDo(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/pr", api.PullRequestRequest{Draft: ptr(true)}, &task)
	if task.Status != "pr_opened" || task.PrUrl != "https://github.com/jo/demo/pull/34" {
		t.Fatalf("after pr %+v", task)
	}
	if len(gh.prs) != 1 || gh.prs[0].Repo != "jo/demo" || gh.prs[0].Head != task.Branch || gh.prs[0].Base != "main" ||
		gh.prs[0].Title != "Add a hello file" || !gh.prs[0].Draft {
		t.Fatalf("pr request %+v", gh.prs)
	}
	if status, _ := env.Do(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/discard", nil, nil); status != http.StatusConflict {
		t.Fatalf("discard after pr: %d", status)
	}
}

func TestDiscardAndFailure(t *testing.T) {
	needGit(t)
	env, _ := setup(t)
	_, repoPath, _ := newRepo(t)
	agentID, _ := startAgent(t, env)
	repo := register(t, env, agentID, repoPath)

	task := createTask(t, env, api.CreateTask{RepoId: repo.Id, Executor: "claude", Prompt: ptr("please FAIL")})
	task = waitStatus(t, env, task.Id, "failed")
	if task.ExitCode == nil || *task.ExitCode != 3 || !strings.Contains(task.Error, "3") {
		t.Fatalf("failed task %+v", task)
	}
	var notes struct{ Items []struct{ Kind string } }
	env.MustDo(http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) == 0 || notes.Items[0].Kind != "coding_task.failed" {
		t.Fatalf("notifications %+v", notes)
	}
	worktree := agentcoding.WorktreePath(repoPath, task.Id)
	if !exists(worktree) {
		t.Fatal("a failed run keeps its worktree for review")
	}
	env.MustDo(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/discard", nil, &task)
	if task.Status != "discarded" || exists(worktree) {
		t.Fatalf("after discard %+v, worktree exists %v", task, exists(worktree))
	}
	if b := git(t, repoPath, "branch", "--list", "xc/*"); b != "" {
		t.Fatalf("branches left: %q", b)
	}
	if st := git(t, repoPath, "status", "--porcelain"); st != "" {
		t.Fatalf("main checkout changed: %q", st)
	}
	if status, _ := env.Do(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/commit", nil, nil); status != http.StatusConflict {
		t.Fatalf("commit after discard: %d", status)
	}

	// A task whose executor changes nothing cannot be committed.
	task = createTask(t, env, api.CreateTask{RepoId: repo.Id, Executor: "claude", Prompt: ptr("NOEDIT")})
	task = waitStatus(t, env, task.Id, "review")
	if len(task.ChangedFiles) != 0 {
		t.Fatalf("files %+v", task.ChangedFiles)
	}
	if status, _ := env.Do(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/commit", nil, nil); status != http.StatusBadRequest {
		t.Fatalf("empty commit: %d", status)
	}

	// Codex is not installed on the agent: the task fails to start.
	task = createTask(t, env, api.CreateTask{RepoId: repo.Id, Executor: "codex", Prompt: ptr("x")})
	task = waitStatus(t, env, task.Id, "failed")
	if !strings.Contains(task.Error, "无法启动") {
		t.Fatalf("codex task %+v", task)
	}
}

func TestQueueLimitAndCancel(t *testing.T) {
	needGit(t)
	env, m := setup(t)
	_, repoPath, _ := newRepo(t)
	agentID, _ := startAgent(t, env)
	repo := register(t, env, agentID, repoPath)

	var ids []int64
	for i := 0; i < 3; i++ {
		ids = append(ids, createTask(t, env, api.CreateTask{RepoId: repo.Id, Executor: "claude", Prompt: ptr("SLEEP " + itoa(int64(i)))}).Id)
	}
	waitStatus(t, env, ids[0], "running")
	waitStatus(t, env, ids[1], "running")
	third := getTask(t, env, ids[2])
	if third.Status != "queued" || third.QueuePosition == nil || *third.QueuePosition != 1 || m.Running() != 2 {
		t.Fatalf("third %+v, running %d", third, m.Running())
	}
	var list api.TaskList
	env.MustDo(http.MethodGet, "/coding/tasks?status=queued&status=running", nil, &list)
	if len(list.Items) != 3 || list.Items[2].Id != ids[2] {
		t.Fatalf("list %+v", list.Items)
	}

	// Wait until the first task really runs, then cancel it: the third starts.
	waitFor(t, "executor output", func() bool { return len(events(t, env, ids[0])) >= 4 })
	env.MustDo(http.MethodPost, "/coding/tasks/"+itoa(ids[0])+"/cancel", nil, nil)
	first := waitStatus(t, env, ids[0], "canceled")
	if first.Error == "" {
		t.Fatalf("canceled %+v", first)
	}
	if exists(agentcoding.WorktreePath(repoPath, ids[0])) {
		t.Fatal("worktree kept after cancel")
	}
	waitStatus(t, env, ids[2], "running")

	// Canceling a queued task needs no agent.
	fourth := createTask(t, env, api.CreateTask{RepoId: repo.Id, Executor: "claude", Prompt: ptr("SLEEP 4")})
	if fourth.Status != "queued" {
		t.Fatalf("fourth %+v", fourth)
	}
	env.MustDo(http.MethodPost, "/coding/tasks/"+itoa(fourth.Id)+"/cancel", nil, &fourth)
	if fourth.Status != "canceled" {
		t.Fatalf("fourth after cancel %+v", fourth)
	}
	if status, _ := env.Do(http.MethodPost, "/coding/tasks/"+itoa(fourth.Id)+"/cancel", nil, nil); status != http.StatusConflict {
		t.Fatalf("second cancel: %d", status)
	}
	if status, _ := env.Do(http.MethodDelete, "/coding/repos/"+itoa(repo.Id), nil, nil); status != http.StatusConflict {
		t.Fatalf("delete repo with running tasks: %d", status)
	}

	for _, id := range ids[1:] {
		env.MustDo(http.MethodPost, "/coding/tasks/"+itoa(id)+"/cancel", nil, nil)
		waitStatus(t, env, id, "canceled")
		if exists(agentcoding.WorktreePath(repoPath, id)) {
			t.Fatalf("worktree of %d kept", id)
		}
	}
	if b := git(t, repoPath, "branch", "--list", "xc/*"); b != "" {
		t.Fatalf("branches left: %q", b)
	}
}

func TestTimeout(t *testing.T) {
	needGit(t)
	env, m := setup(t)
	m.SetTimeoutUnit(time.Second) // timeoutMinutes counts seconds
	_, repoPath, _ := newRepo(t)
	agentID, _ := startAgent(t, env)
	repo := register(t, env, agentID, repoPath)
	task := createTask(t, env, api.CreateTask{RepoId: repo.Id, Executor: "claude", Prompt: ptr("SLEEP"), TimeoutMinutes: ptr(1)})
	task = waitStatus(t, env, task.Id, "failed")
	if !strings.Contains(task.Error, "超过") {
		t.Fatalf("timeout task %+v", task)
	}
	if exists(agentcoding.WorktreePath(repoPath, task.Id)) {
		t.Fatal("worktree kept after timeout")
	}
}

func TestAgentOffline(t *testing.T) {
	needGit(t)
	env, _ := setup(t)
	_, repoPath, _ := newRepo(t)
	agentID, stop := startAgent(t, env)
	repo := register(t, env, agentID, repoPath)
	task := createTask(t, env, api.CreateTask{RepoId: repo.Id, Executor: "claude", Prompt: ptr("SLEEP")})
	waitFor(t, "executor output", func() bool { return len(events(t, env, task.Id)) >= 4 })
	stop()
	task = waitStatus(t, env, task.Id, "failed")
	if !strings.Contains(task.Error, "代理断开") {
		t.Fatalf("offline task %+v", task)
	}
	// The agent stops the executor and cleans up when the stream ends.
	waitFor(t, "worktree removed", func() bool { return !exists(agentcoding.WorktreePath(repoPath, task.Id)) })

	// Tasks for an offline agent wait in the queue.
	queued := createTask(t, env, api.CreateTask{RepoId: repo.Id, Executor: "claude", Prompt: ptr("NOEDIT")})
	if queued.Status != "queued" {
		t.Fatalf("queued %+v", queued)
	}
	var repos []api.Repo
	env.MustDo(http.MethodGet, "/coding/repos", nil, &repos)
	if repos[0].AgentOnline {
		t.Fatal("agent still online")
	}
	if status, raw := env.Do(http.MethodGet, "/coding/executors?agentId="+agentID, nil, nil); status != http.StatusServiceUnavailable {
		t.Fatalf("executors offline: %d %s", status, raw)
	}
	env.MustDo(http.MethodPost, "/coding/tasks/"+itoa(queued.Id)+"/cancel", nil, nil)
}

func TestIssueLinking(t *testing.T) {
	needGit(t)
	env, _ := setup(t)
	issues := &fakeIssues{}
	module.Provide[contracts.Issues](env.App.Deps.Registry, contracts.IssuesKey, contracts.Issues(issues))
	gh := &fakeGitHub{}
	module.Provide[contracts.GitHub](env.App.Deps.Registry, contracts.GitHubKey, contracts.GitHub(gh))
	_, repoPath, _ := newRepo(t)
	agentID, _ := startAgent(t, env)
	repo := register(t, env, agentID, repoPath)

	if status, _ := env.Do(http.MethodPost, "/coding/tasks", api.CreateTask{RepoId: repo.Id, Executor: "claude", IssueKey: ptr("XC-404")}, nil); status != http.StatusBadRequest {
		t.Fatalf("unknown issue: %d", status)
	}
	task := createTask(t, env, api.CreateTask{RepoId: repo.Id, Executor: "claude", IssueKey: ptr("xc-7")})
	if task.IssueKey == nil || *task.IssueKey != "XC-7" || task.Title != "XC-7: Fix the login bug" ||
		!strings.Contains(task.Prompt, "Users cannot log in.") || task.Branch != "xc/"+itoa(task.Id)+"-fix-the-login-bug" {
		t.Fatalf("task %+v", task)
	}
	task = waitStatus(t, env, task.Id, "review")
	statuses, links := issues.snapshot()
	if len(statuses) != 1 || statuses[0] != "XC-7=in_progress" || len(links) != 1 || links[0].Kind != "coding_task" ||
		links[0].URL != "/coding/"+itoa(task.Id) || links[0].Ref != itoa(task.Id) {
		t.Fatalf("on start: %v %+v", statuses, links)
	}
	var list api.TaskList
	env.MustDo(http.MethodGet, "/coding/tasks?issueKey=XC-7", nil, &list)
	if len(list.Items) != 1 {
		t.Fatalf("by issue %+v", list.Items)
	}

	env.Elevate()
	env.MustDo(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/commit", nil, &task)
	if task.Status != "committed" {
		t.Fatalf("commit %+v", task)
	}
	if subject := git(t, repoPath, "log", "-1", "--format=%s%n%b", task.Branch); !strings.HasPrefix(subject, "Fix the login bug\n") || !strings.Contains(subject, "Issue: XC-7") {
		t.Fatalf("commit message %q", subject)
	}
	env.MustDo(http.MethodPost, "/coding/tasks/"+itoa(task.Id)+"/pr", nil, &task) // pushes first
	if task.Status != "pr_opened" {
		t.Fatalf("pr %+v", task)
	}
	statuses, links = issues.snapshot()
	if len(statuses) != 2 || statuses[1] != "XC-7=in_review" || len(links) != 2 || links[1].Kind != "pull_request" ||
		links[1].URL != task.PrUrl || links[1].Ref != "jo/demo#34" {
		t.Fatalf("after pr: %v %+v", statuses, links)
	}
}

func TestValidationAndActions(t *testing.T) {
	needGit(t)
	env, _ := setup(t)
	_, repoPath, _ := newRepo(t)
	agentID, _ := startAgent(t, env)
	repo := register(t, env, agentID, repoPath)

	for _, body := range []any{
		map[string]any{"repoId": repo.Id, "executor": "vim", "prompt": "x"},
		map[string]any{"repoId": 999, "executor": "claude", "prompt": "x"},
		map[string]any{"repoId": repo.Id, "executor": "claude", "prompt": "  "},
		map[string]any{"repoId": repo.Id, "executor": "claude", "prompt": "x", "timeoutMinutes": 5000},
		map[string]any{"repoId": repo.Id, "executor": "claude", "prompt": "x", "extra": 1},
	} {
		if status, raw := env.Do(http.MethodPost, "/coding/tasks", body, nil); status != http.StatusBadRequest {
			t.Errorf("%v: %d %s", body, status, raw)
		}
	}
	for _, path := range []string{"/coding/tasks/999", "/coding/tasks/999/events", "/coding/tasks/999/diff"} {
		if status, _ := env.Do(http.MethodGet, path, nil, nil); status != http.StatusNotFound {
			t.Errorf("%s: %d", path, status)
		}
	}
	if status, _ := env.Do(http.MethodGet, "/coding/tasks?status=bogus", nil, nil); status != http.StatusBadRequest {
		t.Errorf("bad status filter: %d", status)
	}
	if status, _ := env.Do(http.MethodPut, "/coding/settings", map[string]int{"maxConcurrent": 0}, nil); status != http.StatusBadRequest {
		t.Errorf("bad settings: %d", status)
	}
	var settings api.CodingSettings
	env.MustDo(http.MethodPut, "/coding/settings", map[string]int{"maxConcurrent": 3, "defaultTimeoutMinutes": 30}, &settings)
	if settings.MaxConcurrent != 3 || settings.DefaultTimeoutMinutes != 30 {
		t.Fatalf("settings %+v", settings)
	}

	// Actions: launch through the catalog and list.
	ctx := context.Background()
	out, err := env.App.Deps.Actions.Run(ctx, "coding.launch", json.RawMessage(`{"repoId":`+itoa(repo.Id)+`,"prompt":"NOEDIT from an action"}`))
	if err != nil {
		t.Fatal(err)
	}
	launched := out.(api.Task)
	if launched.Executor != "claude" || launched.TimeoutMinutes != 30 {
		t.Fatalf("launched %+v", launched)
	}
	waitStatus(t, env, launched.Id, "review")
	if status, _ := env.Do(http.MethodPost, "/coding/tasks/"+itoa(launched.Id)+"/cancel", nil, nil); status != http.StatusConflict {
		t.Errorf("cancel a finished task: %d", status)
	}
	out, err = env.App.Deps.Actions.Run(ctx, "coding.list_tasks", json.RawMessage(`{"status":["review"]}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), `"title":"NOEDIT from an action"`) || !strings.Contains(string(raw), `"link":"/coding/`) {
		t.Fatalf("list action %s", raw)
	}

	// contracts.Coding.Launch
	c, _ := module.Lookup[contracts.Coding](env.App.Deps.Registry, contracts.CodingKey)
	id, err := c.Launch(ctx, contracts.LaunchCoding{RepoID: repo.Id, Executor: "claude", Prompt: "NOEDIT via contract"})
	if err != nil || id == 0 {
		t.Fatalf("launch: %d %v", id, err)
	}
	waitStatus(t, env, id, "review")
}

func TestServerRestartFailsRunningTasks(t *testing.T) {
	env, m := setup(t)
	// A task left "running" by a previous process.
	db := env.App.Deps.DB
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO agents (id, name, kind, token_hash, created_at) VALUES ('a1', 'desk', 'desktop', 'h', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO coding_repos (id, agent_id, path, name, created_at) VALUES (1, 'a1', '/r', 'r', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO coding_tasks (id, repo_id, executor, prompt, status, created_at, updated_at) VALUES (5, 1, 'claude', 'p', 'running', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	// Start again, as a new server process would.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	task := getTask(t, env, 5)
	if task.Status != "failed" || !strings.Contains(task.Error, "服务重启") {
		t.Fatalf("stale task %+v", task)
	}
}

func ptr[T any](v T) *T { return &v }
