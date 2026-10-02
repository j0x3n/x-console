package coding_test

import (
	"fmt"
	"testing"

	agentapi "github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/api"
)

func TestCLIAgentQuestionResumesSameTask(t *testing.T) {
	needGit(t)
	env, _ := setup(t)
	_, path, _ := newRepo(t)
	runner, _ := startAgent(t, env)
	repo := register(t, env, runner, path)
	var project struct{ ID int64 }
	env.MustDo("POST", "/projects", map[string]any{"key": "CQ", "name": "问题测试"}, &project)
	var card struct{ Key string }
	env.MustDo("POST", fmt.Sprintf("/projects/%d/issues", project.ID), map[string]any{"title": "ASKONCE"}, &card)
	var agent struct{ ID int64 }
	env.MustDo("POST", "/ai-agents", map[string]any{"name": "开发者", "kind": "claude_code", "repoIds": []int64{repo.Id}, "autoBuild": false}, &agent)
	var task struct{ TaskID int64 }
	env.MustDo("POST", fmt.Sprintf("/ai-agents/%d/assign", agent.ID), map[string]any{"issueKey": card.Key, "repoId": repo.Id, "openPr": false}, &task)
	waitStatus(t, env, task.TaskID, "review")
	var ds []agentapi.AiAgentDecision
	waitFor(t, "question", func() bool { env.MustDo("GET", "/ai-agents/decisions", nil, &ds); return len(ds) == 1 })
	if ds[0].Title != "选择数据库" {
		t.Fatal(ds)
	}
	env.MustDo("POST", "/ai-agents/decisions/"+ds[0].Id, map[string]any{"answer": "SQLite"}, nil)
	got := waitStatus(t, env, task.TaskID, "review")
	if len(got.ChangedFiles) == 0 {
		t.Fatal("did not resume")
	}
	var runs []agentapi.AiAgentRun
	env.MustDo("GET", "/ai-agents/runs", nil, &runs)
	if len(runs) != 1 || *runs[0].TaskId != task.TaskID {
		t.Fatal(runs)
	}
	env.MustDo("GET", "/ai-agents/decisions", nil, &ds)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
}
