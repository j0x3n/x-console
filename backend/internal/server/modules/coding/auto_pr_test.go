package coding_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/agent/coding/gittest"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/api"
)

type boardRepoStub struct{ repo contracts.GitRepository }

func (b boardRepoStub) RepositoryForIssue(context.Context, string) (contracts.GitRepository, error) {
	return b.repo, nil
}

func TestAgentAssignmentAutomaticallyOpensPR(t *testing.T) {
	needGit(t)
	for _, autoBuild := range []bool{false, true} {
		t.Run(fmt.Sprintf("autoBuild=%t", autoBuild), func(t *testing.T) {
			testAgentAssignmentAutomaticallyOpensPR(t, autoBuild)
		})
	}
}

func testAgentAssignmentAutomaticallyOpensPR(t *testing.T, autoBuild bool) {
	env, _ := setup(t)
	_, work, bare := newRepo(t)
	git(t, work, "config", "--unset", "url."+bare+".pushInsteadOf")
	git(t, work, "remote", "set-url", "origin", bare)
	git(t, work, "push", "--quiet", "origin", "main")
	git(t, bare, "symbolic-ref", "HEAD", "refs/heads/main")
	git(t, bare, "config", "http.receivepack", "true")
	gitSrv := gittest.New(t, filepath.Dir(bare), "alice", forgejoToken)
	fj := newFakeForgejo(t)
	runner := startRemoteAgent(t, env, t.TempDir())
	var conn struct{ Connection struct{ ID int64 } }
	env.MustDo("POST", "/git-connections", map[string]any{"kind": "forgejo", "name": "fj", "baseUrl": fj.URL, "token": forgejoToken}, &conn)
	cloneURL := gitSrv.URL + "/demo.git"
	var repo api.Repo
	env.MustDo("POST", "/coding/repos", api.CreateRepo{AgentId: runner, ConnectionId: &conn.Connection.ID, RemoteRepo: ptr("team/demo"), CloneUrl: &cloneURL}, &repo)
	if autoBuild {
		env.MustDo("PUT", fmt.Sprintf("/coding/repos/%d/build-config", repo.Id), api.BuildConfig{Linux: []api.BuildStep{{Name: "检查修改", Command: "test -s hello.txt"}}, Windows: []api.BuildStep{}}, &repo)
	}
	var p struct{ ID int64 }
	env.MustDo("POST", "/projects", map[string]any{"key": "AP", "name": "自动 PR"}, &p)
	var card struct{ Key string }
	env.MustDo("POST", fmt.Sprintf("/projects/%d/issues", p.ID), map[string]any{"title": "Add a hello file"}, &card)
	module.Provide[contracts.BoardGit](env.App.Deps.Registry, contracts.BoardGitKey, boardRepoStub{contracts.GitRepository{ConnectionID: conn.Connection.ID, FullName: "team/demo", CloneURL: cloneURL}})
	var agent struct{ ID int64 }
	env.MustDo("POST", "/ai-agents", map[string]any{"name": "开发者", "kind": "claude_code", "repoIds": []int64{repo.Id}, "autoBuild": autoBuild}, &agent)
	var task struct{ TaskID int64 }
	env.MustDo("POST", fmt.Sprintf("/ai-agents/%d/assign", agent.ID), map[string]any{"issueKey": card.Key}, &task)
	got := waitStatus(t, env, task.TaskID, "pr_opened")
	if got.PrUrl == "" {
		t.Fatal("missing PR")
	}
	if autoBuild && (got.BuildStatus == nil || *got.BuildStatus != "passed") {
		t.Fatalf("opened PR before build passed: %+v", got)
	}
	var runs []struct {
		Status string
		TaskID int64
	}
	env.MustDo("GET", "/ai-agents/runs?issueKey="+card.Key, nil, &runs)
	if len(runs) != 1 || runs[0].Status != "pr_opened" || runs[0].TaskID != task.TaskID {
		t.Fatalf("runs: %+v", runs)
	}
	fj.mu.Lock()
	if len(fj.pulls) != 1 || fj.pulls[0]["title"] != "Add a hello file" || !strings.Contains(fj.pulls[0]["body"].(string), card.Key) {
		t.Errorf("PR: %+v", fj.pulls)
	}
	fj.mu.Unlock()
	var links []struct{ URL string }
	env.MustDo("GET", "/issues/"+card.Key+"/links", nil, &links)
	found := false
	for _, link := range links {
		found = found || link.URL == got.PrUrl
	}
	if !found {
		t.Fatalf("links: %+v", links)
	}
	// Permission denial precedes cloning and executing.
	var denied struct{ ID int64 }
	env.MustDo("POST", "/ai-agents", map[string]any{"name": "无权限", "kind": "codex"}, &denied)
	if s, _ := env.Do("POST", fmt.Sprintf("/ai-agents/%d/assign", denied.ID), map[string]any{"issueKey": card.Key}, nil); s != 403 {
		t.Fatal(s)
	}
}
