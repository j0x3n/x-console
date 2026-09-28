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

	"github.com/go-chi/chi/v5"
	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type testActions struct{}

func (testActions) Name() string     { return "ai-test-actions" }
func (testActions) Mount(chi.Router) {}

var writes atomic.Int32

func withActions(d *module.Deps) (module.Module, error) {
	for _, name := range []string{"test.read", "test.write", "test.delete"} {
		name := name
		effect := actions.Read
		if name != "test.read" {
			effect = actions.Write
		}
		d.Actions.Register(actions.Action{Name: name, Title: name, Description: name, Input: actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`), Effect: effect, Run: func(context.Context, json.RawMessage) (any, error) {
			if name != "test.read" {
				writes.Add(1)
			}
			return map[string]any{"ok": true}, nil
		}})
	}
	return testActions{}, nil
}
func sendEvent(w http.ResponseWriter, kind string, data any) {
	raw, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, raw)
}
func streamText(w http.ResponseWriter, text string) {
	sendEvent(w, "message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": "msg_2", "type": "message", "role": "assistant", "model": "claude-opus-5-5", "content": []any{}, "stop_reason": nil, "usage": map[string]any{"input_tokens": 1, "output_tokens": 1}}})
	sendEvent(w, "content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
	sendEvent(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": text}})
	sendEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	sendEvent(w, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}, "usage": map[string]any{"output_tokens": 1}})
	sendEvent(w, "message_stop", map[string]any{"type": "message_stop"})
}
func streamTool(w http.ResponseWriter, name string) {
	sendEvent(w, "message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5", "content": []any{}, "stop_reason": nil, "usage": map[string]any{"input_tokens": 1, "output_tokens": 1}}})
	sendEvent(w, "content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": "toolu_1", "name": name, "input": map[string]any{}}})
	sendEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	sendEvent(w, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "tool_use"}, "usage": map[string]any{"output_tokens": 1}})
	sendEvent(w, "message_stop", map[string]any{"type": "message_stop"})
}
func await(t *testing.T, env *testutil.Env, id int64, pred func(api.ConversationDetail) bool) api.ConversationDetail {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var detail api.ConversationDetail
		env.MustDo("GET", fmt.Sprintf("/ai/conversations/%d", id), nil, &detail)
		if pred(detail) {
			return detail
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("助手回复超时")
	return api.ConversationDetail{}
}
func TestToolExecutionAndRejection(t *testing.T) {
	for _, tc := range []struct {
		name                string
		wantPending, reject bool
		wantWrites          int32
	}{{"test.read", false, false, 0}, {"test.write", false, false, 1}, {"test.delete", true, true, 0}, {"test.delete", true, false, 1}} {
		t.Run(fmt.Sprintf("%s/reject=%v", tc.name, tc.reject), func(t *testing.T) {
			writes.Store(0)
			var requests atomic.Int32
			var sawRejection atomic.Bool
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if requests.Add(1) == 1 {
					streamTool(w, strings.ReplaceAll(tc.name, ".", "__"))
					return
				}
				raw, _ := json.Marshal(body["messages"])
				if strings.Contains(string(raw), "用户拒绝了") {
					sawRejection.Store(true)
				}
				streamText(w, "完成")
			}))
			defer fake.Close()
			t.Setenv("XC_ANTHROPIC_BASE_URL", fake.URL)
			env := testutil.New(t, withActions)
			env.Elevate()
			env.MustDo("PUT", "/ai/settings", map[string]any{"apiKey": "fake-key"}, nil)
			var conversation api.Conversation
			env.MustDo("POST", "/ai/conversations", map[string]any{}, &conversation)
			env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/messages", conversation.Id), map[string]any{"text": "测试"}, nil)
			detail := await(t, env, conversation.Id, func(d api.ConversationDetail) bool {
				if tc.wantPending {
					return len(d.PendingActions) > 0
				}
				return !d.Running && len(d.Messages) >= 4
			})
			if tc.wantPending {
				if detail.PendingActions[0].Status != "pending" || writes.Load() != 0 {
					t.Fatalf("pending: %+v writes=%d", detail.PendingActions, writes.Load())
				}
				decision := "approve"
				if tc.reject {
					decision = "reject"
				}
				env.MustDo("POST", fmt.Sprintf("/ai/actions/%d/%s", detail.PendingActions[0].Id, decision), map[string]any{}, nil)
				await(t, env, conversation.Id, func(d api.ConversationDetail) bool { return !d.Running && len(d.Messages) >= 4 })
				if tc.reject && !sawRejection.Load() {
					t.Fatal("拒绝结果没有回给模型")
				}
				if writes.Load() != tc.wantWrites {
					t.Fatalf("writes=%d, want %d", writes.Load(), tc.wantWrites)
				}
			} else if writes.Load() != tc.wantWrites {
				t.Fatalf("writes=%d, want %d", writes.Load(), tc.wantWrites)
			}
		})
	}
}
