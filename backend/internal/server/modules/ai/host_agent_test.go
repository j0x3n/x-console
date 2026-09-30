package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func streamHostTool(w http.ResponseWriter, command string) {
	w.Header().Set("Content-Type", "text/event-stream")
	args, _ := json.Marshal(map[string]string{"command": command, "reason": "检查机器"})
	sendEvent(w, map[string]any{"id": "host-chat", "object": "chat.completion.chunk", "created": 1, "model": "test-model", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "host-tool-1", "type": "function", "function": map[string]any{"name": "host__run_command", "arguments": string(args)}}}}}}})
	sendEvent(w, map[string]any{"id": "host-chat", "object": "chat.completion.chunk", "created": 1, "model": "test-model", "choices": []any{}, "usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1}})
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func hostScenario(t *testing.T, command string, handler func(context.Context, protocol.ExecParams) (any, error)) (*testutil.Env, api.Conversation, *atomic.Int32, *atomic.Bool) {
	t.Helper()
	var requests, executions atomic.Int32
	var sawResult atomic.Bool
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if requests.Add(1) == 1 {
			streamHostTool(w, command)
			return
		}
		raw, _ := json.Marshal(body["messages"])
		if strings.Contains(string(raw), "命令结果") || strings.Contains(string(raw), "用户拒绝了") {
			sawResult.Store(true)
		}
		streamText(w, "完成")
	}))
	t.Cleanup(fake.Close)
	env := testutil.New(t)
	env.Elevate()
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Test", "baseUrl": fake.URL + "/v1", "apiKey": "fake-key"}, &provider)
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}}, nil)
	hostID := env.Agent("server", []string{protocol.CapExec}, func(c *conn.Client) {
		c.Handle(protocol.MethodExecRun, func(ctx context.Context, raw json.RawMessage) (any, error) {
			var p protocol.ExecParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			executions.Add(1)
			return handler(ctx, p)
		})
	})
	var conversation api.Conversation
	env.MustDo("POST", "/ai/host-agent/"+hostID+"/conversations", map[string]any{}, &conversation)
	return env, conversation, &executions, &sawResult
}

func TestHostAgentReadAutoAndAudit(t *testing.T) {
	env, conversation, executions, sawResult := hostScenario(t, "df -h", func(_ context.Context, p protocol.ExecParams) (any, error) {
		if p.Command != "df -h" {
			t.Errorf("command: %s", p.Command)
		}
		return protocol.ExecResult{ExitCode: 0, Stdout: "命令结果", DurationMs: 12}, nil
	})
	env.MustDo("PUT", fmt.Sprintf("/ai/conversations/%d/permission", conversation.Id), map[string]any{"mode": "read_auto"}, nil)
	env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/messages", conversation.Id), map[string]any{"text": "看看磁盘"}, nil)
	detail := await(t, env, conversation.Id, func(d api.ConversationDetail) bool { return !d.Running && len(d.Messages) >= 4 })
	if executions.Load() != 1 || !sawResult.Load() {
		t.Fatalf("executions=%d result=%v", executions.Load(), sawResult.Load())
	}
	if len(detail.PendingActions) != 1 || detail.PendingActions[0].Effect == nil || *detail.PendingActions[0].Effect != "read" || detail.PendingActions[0].Status != "done" {
		t.Fatalf("actions: %+v", detail.PendingActions)
	}
	var count int
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM audit_log WHERE actor='ai:host-agent' AND action='ai.host_agent.exec'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit count=%d err=%v", count, err)
	}
}

func TestHostAgentWriteWaitsAndRejectionReturnsToModel(t *testing.T) {
	env, conversation, executions, sawResult := hostScenario(t, "rm -rf /tmp/x", func(_ context.Context, _ protocol.ExecParams) (any, error) {
		return protocol.ExecResult{ExitCode: 0}, nil
	})
	env.MustDo("PUT", fmt.Sprintf("/ai/conversations/%d/permission", conversation.Id), map[string]any{"mode": "read_auto"}, nil)
	env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/messages", conversation.Id), map[string]any{"text": "清理临时目录"}, nil)
	detail := await(t, env, conversation.Id, func(d api.ConversationDetail) bool { return len(d.PendingActions) > 0 })
	action := detail.PendingActions[0]
	if action.Status != "pending" || action.Effect == nil || *action.Effect != "write" || executions.Load() != 0 {
		t.Fatalf("action=%+v executions=%d", action, executions.Load())
	}
	env.MustDo("POST", fmt.Sprintf("/ai/actions/%d/reject", action.Id), map[string]any{}, nil)
	await(t, env, conversation.Id, func(d api.ConversationDetail) bool { return !d.Running && len(d.Messages) >= 4 })
	if executions.Load() != 0 || !sawResult.Load() {
		t.Fatalf("executions=%d result=%v", executions.Load(), sawResult.Load())
	}
}

func TestHostAgentDangerousRequiresElevationEvenAllAuto(t *testing.T) {
	env, conversation, executions, _ := hostScenario(t, "rm -rf /", func(_ context.Context, _ protocol.ExecParams) (any, error) {
		return protocol.ExecResult{ExitCode: 0}, nil
	})
	env.MustDo("PUT", fmt.Sprintf("/ai/conversations/%d/permission", conversation.Id), map[string]any{"mode": "all_auto"}, nil)
	env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/messages", conversation.Id), map[string]any{"text": "危险测试"}, nil)
	detail := await(t, env, conversation.Id, func(d api.ConversationDetail) bool { return len(d.PendingActions) > 0 })
	action := detail.PendingActions[0]
	if action.Status != "pending" || action.Effect == nil || *action.Effect != "dangerous" || executions.Load() != 0 {
		t.Fatalf("action=%+v executions=%d", action, executions.Load())
	}
	_, err := env.App.Deps.DB.Exec("UPDATE sessions SET elevated_until=?", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := env.Do("POST", fmt.Sprintf("/ai/actions/%d/approve", action.Id), map[string]any{}, nil); status != 403 {
		t.Fatalf("without elevation: %d", status)
	}
	if executions.Load() != 0 {
		t.Fatalf("dangerous executed without elevation")
	}
	env.Elevate()
	env.MustDo("POST", fmt.Sprintf("/ai/actions/%d/approve", action.Id), map[string]any{}, nil)
	await(t, env, conversation.Id, func(d api.ConversationDetail) bool { return !d.Running && len(d.Messages) >= 4 })
	if executions.Load() != 1 {
		t.Fatalf("executions=%d", executions.Load())
	}
}

func TestHostAgentStopCancelsAgentCall(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	env, conversation, _, _ := hostScenario(t, "df -h", func(ctx context.Context, _ protocol.ExecParams) (any, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	})
	env.MustDo("PUT", fmt.Sprintf("/ai/conversations/%d/permission", conversation.Id), map[string]any{"mode": "read_auto"}, nil)
	env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/messages", conversation.Id), map[string]any{"text": "检查磁盘"}, nil)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("agent call not started")
	}
	env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/stop", conversation.Id), map[string]any{}, nil)
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("agent context not canceled")
	}
}
