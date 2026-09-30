package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
func sendEvent(w http.ResponseWriter, data any) {
	raw, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", raw)
}
func streamText(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/event-stream")
	sendEvent(w, map[string]any{"id": "chat_2", "object": "chat.completion.chunk", "created": 1, "model": "test-model", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": text}}}})
	sendEvent(w, map[string]any{"id": "chat_2", "object": "chat.completion.chunk", "created": 1, "model": "test-model", "choices": []any{}, "usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1}})
	fmt.Fprint(w, "data: [DONE]\n\n")
}
func streamTool(w http.ResponseWriter, name string) {
	w.Header().Set("Content-Type", "text/event-stream")
	sendEvent(w, map[string]any{"id": "chat_1", "object": "chat.completion.chunk", "created": 1, "model": "test-model", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "toolu_1", "type": "function", "function": map[string]any{"name": name, "arguments": "{}"}}}}}}})
	sendEvent(w, map[string]any{"id": "chat_1", "object": "chat.completion.chunk", "created": 1, "model": "test-model", "choices": []any{}, "usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1}})
	fmt.Fprint(w, "data: [DONE]\n\n")
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
				if r.URL.Path == "/v1/models" {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
					return
				}
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
			env := testutil.New(t, withActions)
			env.Elevate()
			var provider api.AiProvider
			env.MustDo("POST", "/ai/providers", map[string]any{"name": "Test", "baseUrl": fake.URL + "/v1", "apiKey": "fake-key"}, &provider)
			env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
			env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}}, nil)
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

func TestOldConversationToolHistory(t *testing.T) {
	var sawHistory atomic.Bool
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
			return
		}
		var body struct {
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, message := range body.Messages {
			if message["role"] == "assistant" && message["tool_calls"] != nil {
				for _, other := range body.Messages {
					if other["role"] == "tool" && other["tool_call_id"] == "old_call" {
						sawHistory.Store(true)
					}
				}
			}
		}
		streamText(w, "继续")
	}))
	defer fake.Close()
	env := testutil.New(t)
	env.Elevate()
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Test", "baseUrl": fake.URL + "/v1"}, &provider)
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}}, nil)
	var conversation api.Conversation
	env.MustDo("POST", "/ai/conversations", map[string]any{}, &conversation)
	for index, entry := range []struct{ role, content string }{
		{"user", `[{"type":"text","text":"旧问题"}]`},
		{"assistant", `[{"type":"tool_use","id":"old_call","name":"test__read","input":{}}]`},
		{"user", `[{"type":"tool_result","tool_use_id":"old_call","content":"旧结果"}]`},
	} {
		_, err := env.App.Deps.DB.Exec(`INSERT INTO ai_messages(conversation_id,seq,role,content,created_at) VALUES(?,?,?,?,?)`, conversation.Id, index+1, entry.role, entry.content, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
	}
	env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/messages", conversation.Id), map[string]any{"text": "继续"}, nil)
	await(t, env, conversation.Id, func(detail api.ConversationDetail) bool { return !detail.Running && len(detail.Messages) >= 5 })
	if !sawHistory.Load() {
		t.Fatal("旧工具消息未转换成 OpenAI 对话格式")
	}
}

type slowActions struct{}

func (slowActions) Name() string     { return "ai-test-slow-actions" }
func (slowActions) Mount(chi.Router) {}

var slowDone atomic.Int32

func withSlowDelete(d *module.Deps) (module.Module, error) {
	d.Actions.Register(actions.Action{Name: "test.slow.delete", Title: "慢删除", Description: "Slow delete", Input: actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`), Effect: actions.Write, Run: func(context.Context, json.RawMessage) (any, error) {
		time.Sleep(200 * time.Millisecond)
		return map[string]any{"deleted": slowDone.Add(1)}, nil
	}})
	return slowActions{}, nil
}

func TestConcurrentApprovalsWaitForEachOther(t *testing.T) {
	slowDone.Store(0)
	var requests atomic.Int32
	var resumedWith atomic.Value
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			var calls []any
			for i := 0; i < 2; i++ {
				calls = append(calls, map[string]any{"index": i, "id": fmt.Sprintf("toolu_%d", i+1), "type": "function", "function": map[string]any{"name": "test__slow__delete", "arguments": "{}"}})
			}
			sendEvent(w, map[string]any{"id": "chat_1", "object": "chat.completion.chunk", "created": 1, "model": "test-model", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": calls}}}})
			sendEvent(w, map[string]any{"id": "chat_1", "object": "chat.completion.chunk", "created": 1, "model": "test-model", "choices": []any{}, "usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1}})
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		resumedWith.Store(string(raw))
		streamText(w, "完成")
	}))
	defer fake.Close()
	env := testutil.New(t, withSlowDelete)
	env.Elevate()
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Test", "baseUrl": fake.URL + "/v1", "apiKey": "fake-key"}, &provider)
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}}, nil)
	var conversation api.Conversation
	env.MustDo("POST", "/ai/conversations", map[string]any{}, &conversation)
	env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/messages", conversation.Id), map[string]any{"text": "删两个"}, nil)
	detail := await(t, env, conversation.Id, func(d api.ConversationDetail) bool { return len(d.PendingActions) == 2 && !d.Running })
	done := make(chan int, 2)
	for _, action := range detail.PendingActions {
		go func(id int64) {
			status, _ := env.Do("POST", fmt.Sprintf("/ai/actions/%d/approve", id), map[string]any{}, nil)
			done <- status
		}(action.Id)
	}
	for i := 0; i < 2; i++ {
		if status := <-done; status != http.StatusNoContent {
			t.Fatalf("approve: %d", status)
		}
	}
	detail = await(t, env, conversation.Id, func(d api.ConversationDetail) bool { return !d.Running && requests.Load() == 2 })
	for _, action := range detail.PendingActions {
		if action.Status != "done" {
			t.Fatalf("action %d: %s", action.Id, action.Status)
		}
	}
	// The model resumes once, after both actions finished, and sees both results.
	sent, _ := resumedWith.Load().(string)
	if strings.Count(sent, "deleted") != 2 {
		t.Fatalf("resumed before both actions finished: %s", sent)
	}
}

func TestTruncatedToolCallIsNotRun(t *testing.T) {
	writes.Store(0)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		// The arguments stop in the middle: the model hit its output limit.
		sendEvent(w, map[string]any{"id": "chat_t", "object": "chat.completion.chunk", "created": 1, "model": "test-model", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "toolu_1", "type": "function", "function": map[string]any{"name": "test__write", "arguments": `{"title":"半`}}}}}}})
		sendEvent(w, map[string]any{"id": "chat_t", "object": "chat.completion.chunk", "created": 1, "model": "test-model", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "length"}}})
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer fake.Close()
	env := testutil.New(t, withActions)
	env.Elevate()
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Test", "baseUrl": fake.URL + "/v1", "apiKey": "fake-key"}, &provider)
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}}, nil)
	var conversation api.Conversation
	env.MustDo("POST", "/ai/conversations", map[string]any{}, &conversation)
	env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/messages", conversation.Id), map[string]any{"text": "写一个"}, nil)
	detail := await(t, env, conversation.Id, func(d api.ConversationDetail) bool { return !d.Running && len(d.Messages) >= 2 })
	if writes.Load() != 0 || len(detail.PendingActions) != 0 {
		t.Fatalf("cut-off tool call ran: writes=%d pending=%d", writes.Load(), len(detail.PendingActions))
	}
	raw, _ := json.Marshal(detail.Messages[len(detail.Messages)-1])
	if !strings.Contains(string(raw), "没有执行") {
		t.Fatalf("no truncation note: %s", raw)
	}
}

func TestPromptPrefixStaysTheSame(t *testing.T) {
	var mu sync.Mutex
	var systems []string
	var lastUser atomic.Value
	var requests atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
			return
		}
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body.Messages[0].Content)
		mu.Lock()
		systems = append(systems, string(raw))
		mu.Unlock()
		user, _ := json.Marshal(body.Messages[1].Content)
		lastUser.Store(string(user))
		if requests.Add(1) == 1 {
			streamTool(w, "test__read") // a second turn in the same reply
			return
		}
		streamText(w, "好")
	}))
	defer fake.Close()
	env := testutil.New(t, withActions)
	env.Elevate()
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Test", "baseUrl": fake.URL + "/v1", "apiKey": "fake-key"}, &provider)
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}}, nil)
	var conversation api.Conversation
	env.MustDo("POST", "/ai/conversations", map[string]any{}, &conversation)
	env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/messages", conversation.Id), map[string]any{"text": "你好"}, nil)
	await(t, env, conversation.Id, func(d api.ConversationDetail) bool { return !d.Running && requests.Load() == 2 })
	time.Sleep(1100 * time.Millisecond) // the clock moves on
	env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/messages", conversation.Id), map[string]any{"text": "再说一句"}, nil)
	await(t, env, conversation.Id, func(d api.ConversationDetail) bool { return !d.Running && requests.Load() == 3 })
	mu.Lock()
	defer mu.Unlock()
	if len(systems) != 3 || systems[0] != systems[1] || systems[1] != systems[2] || strings.Contains(systems[0], "当前时间") {
		t.Fatalf("system prompt changes between calls: %q", systems)
	}
	if user, _ := lastUser.Load().(string); !strings.Contains(user, "当前时间") {
		t.Fatalf("time missing from the user message: %s", user)
	}
}
