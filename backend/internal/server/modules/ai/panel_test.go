package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// B60: the panel AI's permission levels, its own model, and machines only
// through a bound agent.

func streamToolArgs(w http.ResponseWriter, name string, args any) {
	w.Header().Set("Content-Type", "text/event-stream")
	raw, _ := json.Marshal(args)
	sendEvent(w, map[string]any{"id": "chat_t", "object": "chat.completion.chunk", "created": 1, "model": "test-model", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("tool-%s-%d", name, time.Now().UnixNano()), "type": "function", "function": map[string]any{"name": name, "arguments": string(raw)}}}}}}})
	sendEvent(w, map[string]any{"id": "chat_t", "object": "chat.completion.chunk", "created": 1, "model": "test-model", "choices": []any{}, "usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1}})
	fmt.Fprint(w, "data: [DONE]\n\n")
}

// scriptedLLM answers each chat request with the next step and keeps the
// request bodies.
type scriptedLLM struct {
	mu     sync.Mutex
	steps  []func(http.ResponseWriter)
	bodies []map[string]any
}

func (s *scriptedLLM) push(steps ...func(http.ResponseWriter)) {
	s.mu.Lock()
	s.steps = append(s.steps, steps...)
	s.mu.Unlock()
}

func (s *scriptedLLM) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

func (s *scriptedLLM) body(i int) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bodies[i]
}

func panelEnv(t *testing.T, modules ...func(*module.Deps) (module.Module, error)) (*testutil.Env, *scriptedLLM, int64) {
	t.Helper()
	s := &scriptedLLM{}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"test-model"},{"id":"other-model"}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.bodies = append(s.bodies, body)
		var step func(http.ResponseWriter)
		if len(s.steps) > 0 {
			step, s.steps = s.steps[0], s.steps[1:]
		}
		s.mu.Unlock()
		if step == nil {
			streamText(w, "好")
			return
		}
		step(w)
	}))
	t.Cleanup(fake.Close)
	env := testutil.New(t, modules...)
	env.Elevate()
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Test", "baseUrl": fake.URL + "/v1", "apiKey": "fake-key"}, &provider)
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}}, nil)
	return env, s, provider.Id
}

func toolNamesOf(body map[string]any) map[string]bool {
	out := map[string]bool{}
	tools, _ := body["tools"].([]any)
	for _, x := range tools {
		fn, _ := x.(map[string]any)["function"].(map[string]any)
		if name, ok := fn["name"].(string); ok {
			out[name] = true
		}
	}
	return out
}

func send(t *testing.T, env *testutil.Env, id int64, text string) {
	t.Helper()
	env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/messages", id), map[string]any{"text": text}, nil)
}

func TestPanelOperatesMachinesOnlyThroughAgents(t *testing.T) {
	env, llm, providerID := panelEnv(t)
	var executions atomic.Int32
	hostID := env.Agent("desktop", []string{protocol.CapExec}, func(c *conn.Client) {
		c.Handle(protocol.MethodExecRun, func(ctx context.Context, raw json.RawMessage) (any, error) {
			executions.Add(1)
			return protocol.ExecResult{ExitCode: 0, Stdout: "ok"}, nil
		})
	})

	// No agent is bound: the tool fails and nothing runs on the machine.
	var c api.Conversation
	env.MustDo("POST", "/ai/conversations", map[string]any{"permission": "all"}, &c)
	if c.PanelPermission == nil || *c.PanelPermission != api.AiPermissionAll {
		t.Fatalf("all at creation: %+v", c)
	}
	llm.push(func(w http.ResponseWriter) {
		streamToolArgs(w, "agents__operate_host", map[string]any{"host": hostID, "request": "打开记事本"})
	})
	send(t, env, c.Id, "在电脑上打开记事本")
	detail := await(t, env, c.Id, func(d api.ConversationDetail) bool { return !d.Running && llm.count() == 2 })
	tools := toolNamesOf(llm.body(0))
	if tools["hosts__exec"] || tools["hosts__power"] || !tools["agents__operate_host"] || !tools["hosts__list"] && !tools["hosts__metrics"] {
		t.Fatalf("panel tools: %v", tools)
	}
	if len(detail.PendingActions) != 1 || detail.PendingActions[0].Status != "failed" ||
		!strings.Contains(fmt.Sprint(detail.PendingActions[0].Result), "这台机器没有绑定 Agent") {
		t.Fatalf("unbound: %+v", detail.PendingActions)
	}
	if executions.Load() != 0 {
		t.Fatal("ran on the machine without an agent")
	}

	// Bind an agent. In manual the panel asks first; the machine
	// conversation then runs in confirm.
	status, raw := env.Do("POST", "/ai-agents", map[string]any{"name": "乱绑", "kind": "builtin",
		"model": fmt.Sprintf("%d:test-model", providerID), "hostIds": []string{"no-such-host"}}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("unknown machine: %d %s", status, raw)
	}
	var agent struct {
		ID        int64    `json:"id"`
		HostIDs   []string `json:"hostIds"`
		HostNames []string `json:"hostNames"`
	}
	env.MustDo("POST", "/ai-agents", map[string]any{"name": "电脑管家", "kind": "builtin",
		"model": fmt.Sprintf("%d:test-model", providerID), "hostIds": []string{hostID}}, &agent)
	if len(agent.HostIDs) != 1 || agent.HostIDs[0] != hostID || len(agent.HostNames) != 1 {
		t.Fatalf("agent hosts: %+v", agent)
	}
	var manual api.Conversation
	env.MustDo("POST", "/ai/conversations", map[string]any{"permission": "manual"}, &manual)
	if manual.PanelPermission == nil || *manual.PanelPermission != api.AiPermissionManual {
		t.Fatalf("manual: %+v", manual)
	}
	llm.push(func(w http.ResponseWriter) {
		streamToolArgs(w, "agents__operate_host", map[string]any{"host": hostID, "request": "看看磁盘"})
	}, func(w http.ResponseWriter) {
		streamHostTool(w, "df -h")
	})
	send(t, env, manual.Id, "看看电脑磁盘")
	detail = await(t, env, manual.Id, func(d api.ConversationDetail) bool { return !d.Running && len(d.PendingActions) == 1 })
	if detail.PendingActions[0].Status != "pending" || detail.PendingActions[0].Action != "agents.operate_host" {
		t.Fatalf("manual asks first: %+v", detail.PendingActions)
	}
	env.MustDo("POST", fmt.Sprintf("/ai/actions/%d/approve", detail.PendingActions[0].Id), nil, nil)
	detail = await(t, env, manual.Id, func(d api.ConversationDetail) bool {
		return !d.Running && len(d.PendingActions) == 1 && d.PendingActions[0].Status == "done"
	})
	result, _ := json.Marshal(detail.PendingActions[0].Result)
	if !strings.Contains(string(result), "电脑管家") || !strings.Contains(string(result), "/pc/agent?c=") || !strings.Contains(string(result), `"waiting":1`) {
		t.Fatalf("operate result: %s", result)
	}
	var hostConvs []api.Conversation
	env.MustDo("GET", "/ai/host-agent/"+hostID+"/conversations", nil, &hostConvs)
	if len(hostConvs) != 1 || hostConvs[0].Permission == nil || *hostConvs[0].Permission != api.Confirm || !strings.HasPrefix(hostConvs[0].Title, "电脑管家：") {
		t.Fatalf("machine conversation: %+v", hostConvs)
	}
	if executions.Load() != 0 {
		t.Fatal("the machine conversation ran a command without confirmation")
	}
}

func TestPanelPermissionLevelsAndModel(t *testing.T) {
	env, llm, providerID := panelEnv(t, withActions)
	m, _ := module.Lookup[contracts.ToolRunner](env.App.Deps.Registry, contracts.ToolRunnerKey)
	mod := m.(*ai.Module)
	writes.Store(0)
	// Without a chosen default, the old "confirm all writes" switch decides.
	var c api.Conversation
	env.MustDo("POST", "/ai/conversations", map[string]any{}, &c)
	if *c.PanelPermission != api.AiPermissionWrite {
		t.Fatalf("default with the switch off: %+v", c)
	}
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"confirmAllWrites": true,
		"agent": map[string]any{"providerId": providerID, "model": "test-model"}}, nil)
	env.MustDo("POST", "/ai/conversations", map[string]any{}, &c)
	if *c.PanelPermission != api.AiPermissionManual {
		t.Fatalf("default with the switch on: %+v", c)
	}

	// manual: a write waits.
	llm.push(func(w http.ResponseWriter) { streamToolArgs(w, "test__write", map[string]any{}) })
	send(t, env, c.Id, "写一下")
	detail := await(t, env, c.Id, func(d api.ConversationDetail) bool { return !d.Running && len(d.PendingActions) == 1 })
	if detail.PendingActions[0].Status != "pending" || writes.Load() != 0 {
		t.Fatalf("manual write: %+v", detail.PendingActions)
	}
	env.MustDo("POST", fmt.Sprintf("/ai/actions/%d/reject", detail.PendingActions[0].Id), nil, nil)
	await(t, env, c.Id, func(d api.ConversationDetail) bool { return !d.Running })

	// write: a write runs, a delete waits. Its own model is used.
	var updated api.Conversation
	env.MustDo("PATCH", fmt.Sprintf("/ai/conversations/%d/settings", c.Id), map[string]any{
		"permission": "write", "model": fmt.Sprintf("%d:other-model", providerID), "effort": "low"}, &updated)
	if *updated.PanelPermission != api.AiPermissionWrite || *updated.Model != fmt.Sprintf("%d:other-model", providerID) || *updated.Effort != "low" {
		t.Fatalf("settings: %+v", updated)
	}
	before := llm.count()
	llm.push(func(w http.ResponseWriter) { streamToolArgs(w, "test__write", map[string]any{}) }, func(w http.ResponseWriter) { streamToolArgs(w, "test__delete", map[string]any{}) })
	send(t, env, c.Id, "写完再删")
	detail = await(t, env, c.Id, func(d api.ConversationDetail) bool { return !d.Running && len(d.PendingActions) == 3 })
	if writes.Load() != 1 || detail.PendingActions[2].Status != "pending" || detail.PendingActions[2].Action != "test.delete" {
		t.Fatalf("write level: writes=%d %+v", writes.Load(), detail.PendingActions)
	}
	if got := llm.body(before)["model"]; got != "other-model" {
		t.Fatalf("model sent: %v", got)
	}

	// all needs elevation, and goes back to manual after 2 hours.
	if _, err := env.App.Deps.DB.Exec("UPDATE sessions SET elevated_until=?", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	status, raw := env.Do("PATCH", fmt.Sprintf("/ai/conversations/%d/settings", c.Id), map[string]any{"permission": "all"}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("all without elevation: %d %s", status, raw)
	}
	env.Elevate()
	env.MustDo("PATCH", fmt.Sprintf("/ai/conversations/%d/settings", c.Id), map[string]any{"permission": "all"}, &updated)
	if *updated.PanelPermission != api.AiPermissionAll || updated.PanelPermissionUntil == nil {
		t.Fatalf("all: %+v", updated)
	}
	later := time.Now().Add(2*time.Hour + time.Minute)
	mod.SetNowForTest(func() time.Time { return later })
	env.MustDo("GET", fmt.Sprintf("/ai/conversations/%d", c.Id), nil, &detail)
	if *detail.Conversation.PanelPermission != api.AiPermissionWrite {
		t.Fatalf("after 2 hours: %+v", detail.Conversation.PanelPermission)
	}

	// The default level for new conversations: manual or write, never all.
	status, _ = env.Do("PUT", "/ai/model-settings", map[string]any{"defaultPermission": "all"}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("all as default: %d", status)
	}
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"defaultPermission": "write",
		"agent": map[string]any{"providerId": providerID, "model": "test-model"}}, nil)
	var fresh api.Conversation
	env.MustDo("POST", "/ai/conversations", map[string]any{}, &fresh)
	if *fresh.PanelPermission != api.AiPermissionWrite {
		t.Fatalf("default write: %+v", fresh)
	}
}
