package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func memoryPrompt(t *testing.T, env *testutil.Env) string {
	t.Helper()
	mem, ok := module.Lookup[contracts.Memories](env.App.Deps.Registry, contracts.MemoriesKey)
	if !ok {
		t.Fatal("contracts.Memories not provided")
	}
	return mem.Prompt(context.Background())
}

func TestMemoryCRUDAndLimit(t *testing.T) {
	env := testutil.New(t)
	var a, b api.AiMemory
	env.MustDo(http.MethodPost, "/ai/memories", map[string]any{"text": "主路由是 OpenWrt，地址 192.168.1.1"}, &a)
	env.MustDo(http.MethodPost, "/ai/memories", map[string]any{"text": "周报周五下午写"}, &b)
	if a.Source != api.AiMemorySourceUser {
		t.Fatalf("source: %+v", a)
	}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/ai/memories/%d", b.Id), map[string]any{"text": "周报周四下午写"}, &b)
	env.MustDo(http.MethodDelete, fmt.Sprintf("/ai/memories/%d", a.Id), nil, nil)
	var list api.AiMemories
	env.MustDo(http.MethodGet, "/ai/memories", nil, &list)
	if !list.Enabled || len(list.Items) != 1 || list.Items[0].Text != "周报周四下午写" || list.UsedChars != 7 || list.LimitChars != 4000 {
		t.Fatalf("list: %+v", list)
	}
	if p := memoryPrompt(t, env); !strings.Contains(p, fmt.Sprintf("[%d] 周报周四下午写", b.Id)) || !strings.HasPrefix(p, "## 关于用户的记忆") {
		t.Fatalf("prompt: %q", p)
	}

	status, raw := env.Do(http.MethodPost, "/ai/memories", map[string]any{"text": strings.Repeat("长", 501)}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("501 chars: %d %s", status, raw)
	}
	for i := 0; i < 7; i++ {
		env.MustDo(http.MethodPost, "/ai/memories", map[string]any{"text": strings.Repeat("字", 500)}, nil)
	}
	status, raw = env.Do(http.MethodPost, "/ai/memories", map[string]any{"text": strings.Repeat("多", 494)}, nil)
	if status != http.StatusBadRequest || !strings.Contains(string(raw), "记忆总共最多 4000 字") {
		t.Fatalf("over the limit: %d %s", status, raw)
	}
	env.MustDo(http.MethodPost, "/ai/memories", map[string]any{"text": strings.Repeat("刚", 493)}, nil) // exactly 4000

	env.MustDo(http.MethodPut, "/ai/memories/enabled", map[string]any{"enabled": false}, &list)
	if list.Enabled || len(list.Items) != 9 {
		t.Fatalf("off keeps the items: %+v", list.Enabled)
	}
	if p := memoryPrompt(t, env); p != "" {
		t.Fatalf("prompt while off: %q", p)
	}
}

// The panel AI reads the memory and can write it; agents only read.
func TestMemoryInPanelAndAgents(t *testing.T) {
	env := testutil.New(t)
	m, _ := module.Lookup[contracts.ToolRunner](env.App.Deps.Registry, contracts.ToolRunnerKey)
	fake := llm.NewFake()
	m.(*ai.Module).SetLLMForTest(fake)
	env.MustDo(http.MethodPost, "/ai/memories", map[string]any{"text": "主路由是 OpenWrt"}, nil)
	models := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
	}))
	defer models.Close()
	env.Elevate()
	var provider api.AiProvider
	env.MustDo(http.MethodPost, "/ai/providers", map[string]any{"name": "Test", "baseUrl": models.URL + "/v1", "apiKey": "k"}, &provider)
	env.MustDo(http.MethodPost, fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	env.MustDo(http.MethodPut, "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}}, nil)

	fake.Queue(llm.Result{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "memory__save", Arguments: json.RawMessage(`{"text":"周报周五写"}`)}}}, nil)
	fake.Queue(llm.Result{Text: "记住了"}, nil)
	var conversation api.Conversation
	env.MustDo(http.MethodPost, "/ai/conversations", map[string]any{}, &conversation)
	env.MustDo(http.MethodPost, fmt.Sprintf("/ai/conversations/%d/messages", conversation.Id), map[string]any{"text": "记住周报周五写"}, nil)
	await(t, env, conversation.Id, func(d api.ConversationDetail) bool { return !d.Running && len(fake.Calls) == 2 })
	first := fake.Calls[0]
	if !strings.Contains(first.System, "主路由是 OpenWrt") || !strings.Contains(first.System, "memory__save") {
		t.Fatalf("panel system prompt: %q", first.System)
	}
	tools := map[string]bool{}
	for _, tool := range first.Tools {
		tools[tool.Name] = true
	}
	if !tools["memory__save"] || !tools["memory__update"] || !tools["memory__delete"] {
		t.Fatalf("panel tools: %v", tools)
	}
	var list api.AiMemories
	env.MustDo(http.MethodGet, "/ai/memories", nil, &list)
	if len(list.Items) != 2 || list.Items[1].Source != api.AiMemorySourceAi || list.Items[1].Text != "周报周五写" {
		t.Fatalf("saved by the AI: %+v", list.Items)
	}

	// A machine conversation reads the memory; its tools are machine tools only.
	if sys := m.(*ai.Module).HostSystemForTest(context.Background(), "nope"); !strings.Contains(sys, "周报周五写") {
		t.Fatalf("machine prompt: %q", sys)
	}

	// An agent run gets no memory tools.
	fake.Queue(llm.Result{Text: "好"}, nil)
	if _, err := m.RunTools(context.Background(), contracts.ToolRun{Model: "1:gpt", System: "sys", Prompt: "x", Access: "write_delete"}); err != nil {
		t.Fatal(err)
	}
	for _, tool := range fake.Calls[2].Tools {
		if strings.HasPrefix(tool.Name, "memory__") {
			t.Fatalf("agent got %s", tool.Name)
		}
	}
}
