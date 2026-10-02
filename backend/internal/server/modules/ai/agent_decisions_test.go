package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type agent struct{ ID int64 }

func newCard(t *testing.T, env *testutil.Env) struct{ Key string } {
	var p struct{ ID int64 }
	env.MustDo("POST", "/projects", map[string]any{"key": "AD", "name": "确认测试"}, &p)
	var c struct{ Key string }
	env.MustDo("POST", fmt.Sprintf("/projects/%d/issues", p.ID), map[string]any{"title": "处理数据"}, &c)
	return c
}

func TestDangerousAgentDecisionRequiresElevation(t *testing.T) {
	env := testutil.New(t)
	c := newCard(t, env)
	var count atomic.Int32
	env.App.Deps.Actions.Register(actions.Action{Name: "test.dangerous", Title: "危险操作", Effect: actions.Dangerous, Input: json.RawMessage(`{"type":"object"}`), Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
		if err := auth.RequireElevated(ctx); err != nil {
			return nil, err
		}
		count.Add(1)
		return "ok", nil
	}})
	fake := llm.NewFake()
	runner, _ := module.Lookup[contracts.ToolRunner](env.App.Deps.Registry, contracts.ToolRunnerKey)
	runner.(*ai.Module).SetLLMForTest(fake)
	fake.Queue(llm.Result{ToolCalls: []llm.ToolCall{{ID: "danger", Name: "test__dangerous", Arguments: json.RawMessage(`{}`)}}}, nil)
	fake.Queue(llm.Result{Text: "做完"}, nil)
	var a agent
	env.MustDo("POST", "/ai-agents", map[string]any{"name": "危险测试", "kind": "builtin", "model": "1:test"}, &a)
	env.MustDo("POST", fmt.Sprintf("/ai-agents/%d/assign", a.ID), map[string]any{"issueKey": c.Key}, nil)
	var ds []api.AiAgentDecision
	waitFor(t, func() bool { env.MustDo("GET", "/ai-agents/decisions", nil, &ds); return len(ds) == 1 })
	if s, _ := env.Do("POST", "/ai-agents/decisions/"+ds[0].Id, map[string]any{"approve": true}, nil); s != 403 {
		t.Fatalf("unelevated approval: %d", s)
	}
	if count.Load() != 0 {
		t.Fatal("ran before approval")
	}
	env.Elevate()
	env.MustDo("POST", "/ai-agents/decisions/"+ds[0].Id, map[string]any{"approve": true}, nil)
	waitFor(t, func() bool { return count.Load() == 1 })
}
func waitFor(t *testing.T, f func() bool) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for !f() {
		if time.Now().After(until) {
			t.Fatal("timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestBuiltinDeletionWaitsForDecision(t *testing.T) {
	for _, approve := range []bool{true, false} {
		t.Run(fmt.Sprint(approve), func(t *testing.T) {
			env := testutil.New(t)
			env.Elevate()
			c := newCard(t, env)
			var note struct{ ID int64 }
			env.MustDo("POST", "/notes", map[string]any{"body": "不要提前删除"}, &note)
			fake := llm.NewFake()
			runner, _ := module.Lookup[contracts.ToolRunner](env.App.Deps.Registry, contracts.ToolRunnerKey)
			runner.(*ai.Module).SetLLMForTest(fake)
			fake.Queue(llm.Result{ToolCalls: []llm.ToolCall{{ID: "delete", Name: "notes__delete", Arguments: json.RawMessage(fmt.Sprintf(`{"id":%d}`, note.ID))}}}, nil)
			fake.Queue(llm.Result{Text: "已处理"}, nil)
			var a agent
			env.MustDo("POST", "/ai-agents", map[string]any{"name": "整理员", "kind": "builtin", "model": "1:test", "access": "write_delete"}, &a)
			env.MustDo("POST", fmt.Sprintf("/ai-agents/%d/assign", a.ID), map[string]any{"issueKey": c.Key}, nil)
			var decisions []api.AiAgentDecision
			waitFor(t, func() bool { env.MustDo("GET", "/ai-agents/decisions", nil, &decisions); return len(decisions) == 1 })
			if decisions[0].Kind != api.Permission {
				t.Fatal(decisions)
			}
			env.MustDo("GET", fmt.Sprintf("/notes/%d", note.ID), nil, nil)
			var runs []api.AiAgentRun
			env.MustDo("GET", "/ai-agents/runs", nil, &runs)
			if runs[0].Status != api.Waiting {
				t.Fatal(runs)
			}
			env.MustDo("POST", "/ai-agents/decisions/"+decisions[0].Id, map[string]any{"approve": approve}, nil)
			waitFor(t, func() bool { env.MustDo("GET", "/ai-agents/runs", nil, &runs); return runs[0].Status == api.Done })
			status, _ := env.Do("GET", fmt.Sprintf("/notes/%d", note.ID), nil, nil)
			want := 200
			if approve {
				want = 404
			}
			if status != want {
				t.Fatalf("deleted before approval or rejection ignored: %d", status)
			}
			if status, _ := env.Do("POST", "/ai-agents/decisions/"+decisions[0].Id, map[string]any{"approve": true}, nil); status != 409 {
				t.Fatal(status)
			}
			env.MustDo("GET", "/ai-agents/decisions", nil, &decisions)
			if len(decisions) != 0 {
				t.Fatal(decisions)
			}
		})
	}
}

func TestBuiltinQuestionAndNotificationSwitches(t *testing.T) {
	env := testutil.New(t)
	c := newCard(t, env)
	fake := llm.NewFake()
	runner, _ := module.Lookup[contracts.ToolRunner](env.App.Deps.Registry, contracts.ToolRunnerKey)
	runner.(*ai.Module).SetLLMForTest(fake)
	fake.Queue(llm.Result{ToolCalls: []llm.ToolCall{{ID: "ask", Name: "ask_user", Arguments: json.RawMessage(`{"title":"选哪个？","options":["A","B"]}`)}}}, nil)
	fake.Queue(llm.Result{Text: "按回答做完了"}, nil)
	settings := api.AiAgentNotify{Received: true, Started: true, Decision: true, Done: false, Failed: false, PrOpened: false}
	env.MustDo("PUT", "/ai-agents/notify", settings, nil)
	var a agent
	env.MustDo("POST", "/ai-agents", map[string]any{"name": "提问者", "kind": "builtin", "model": "1:test"}, &a)
	env.MustDo("POST", fmt.Sprintf("/ai-agents/%d/assign", a.ID), map[string]any{"issueKey": c.Key}, nil)
	var ds []api.AiAgentDecision
	waitFor(t, func() bool { env.MustDo("GET", "/ai-agents/decisions", nil, &ds); return len(ds) == 1 })
	if ds[0].Kind != api.Question || len(*ds[0].Options) != 2 {
		t.Fatal(ds)
	}
	if s, _ := env.Do("POST", "/ai-agents/decisions/"+ds[0].Id, map[string]any{"answer": " "}, nil); s != 400 {
		t.Fatal(s)
	}
	env.MustDo("POST", "/ai-agents/decisions/"+ds[0].Id, map[string]any{"answer": "B"}, nil)
	var runs []api.AiAgentRun
	waitFor(t, func() bool { env.MustDo("GET", "/ai-agents/runs", nil, &runs); return runs[0].Status == api.Done })
	for _, kind := range []string{"received", "started", "decision", "done", "failed", "pr_opened"} {
		var n int
		err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM notifications WHERE kind=?", "ai_agent."+kind).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		if kind == "received" || kind == "started" || kind == "decision" {
			want = 1
		}
		if n != want {
			t.Errorf("%s: %d want %d", kind, n, want)
		}
	}
	// The answered option is passed to the model.
	last := fake.Calls[1].Messages
	if last[len(last)-1].Content != "B" {
		t.Fatal(last)
	}
}
