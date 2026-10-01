package ai_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestRunToolsForBuiltinAgent(t *testing.T) {
	env := testutil.New(t)
	runner, ok := module.Lookup[contracts.ToolRunner](env.App.Deps.Registry, contracts.ToolRunnerKey)
	if !ok {
		t.Fatal("contracts.ToolRunner not provided")
	}
	fake := llm.NewFake()
	runner.(*ai.Module).SetLLMForTest(fake)
	fake.Queue(llm.Result{ToolCalls: []llm.ToolCall{
		{ID: "c1", Name: "notes__create", Arguments: json.RawMessage(`{"body":"# 来自 Agent"}`)},
		{ID: "c2", Name: "notes__delete", Arguments: json.RawMessage(`{"id":1}`)},
	}}, nil)
	fake.Queue(llm.Result{Text: " 写好了一条笔记。 "}, nil)
	session := &auth.Session{ID: "ai_agent:1", Username: "agent:1", ViaToken: true, Token: &auth.TokenInfo{Name: "a", Access: "write"}}
	ctx := auth.WithSession(context.Background(), session)

	if _, err := runner.RunTools(ctx, contracts.ToolRun{Model: "gpt", Access: "write"}); err == nil {
		t.Fatal("bad model accepted")
	}
	text, err := runner.RunTools(ctx, contracts.ToolRun{Model: "1:gpt-5", System: "sys", Prompt: "写笔记", Access: "write", Source: "ai_agent", Ref: "1"})
	if err != nil || text != "写好了一条笔记。" {
		t.Fatalf("run: %q %v", text, err)
	}
	var n int
	if err := env.App.Deps.DB.QueryRow(`SELECT count(*) FROM notes`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("notes: %d %v", n, err)
	}
	tools := map[string]bool{}
	for _, tool := range fake.Calls[0].Tools {
		tools[tool.Name] = true
	}
	if !tools["notes__create"] || tools["notes__delete"] || tools["hosts__exec"] {
		t.Fatalf("tools offered: %v", tools)
	}
	second := fake.Calls[1].Messages
	last := second[len(second)-1]
	if last.Role != "tool" || last.ToolCallID != "c2" || !strings.Contains(last.Content, "不能用") {
		t.Fatalf("refused tool result: %+v", last)
	}

	// The loop stops after MaxTurns.
	for i := 0; i < 3; i++ {
		fake.Queue(llm.Result{ToolCalls: []llm.ToolCall{{ID: "x", Name: "notes__search", Arguments: json.RawMessage(`{}`)}}}, nil)
	}
	if _, err := runner.RunTools(ctx, contracts.ToolRun{Model: "1:m", Access: "read", MaxTurns: 2}); err == nil {
		t.Fatal("no turn limit")
	}
}
