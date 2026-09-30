package llm_test

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

	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

func completion(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id": "chat_1", "object": "chat.completion", "created": 1, "model": "test-model",
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 4, "total_tokens": 14},
	})
}

func TestCompleteReasoningAndJSONFallback(t *testing.T) {
	var reasoningRejected, jsonRejected, recorded, marked atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "bad request", 404)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		if body["reasoning_effort"] != nil {
			reasoningRejected.Add(1)
			http.Error(w, `{"error":{"message":"reasoning_effort unsupported"}}`, 400)
			return
		}
		if body["response_format"] != nil {
			jsonRejected.Add(1)
			http.Error(w, `{"error":{"message":"response_format unsupported"}}`, 400)
			return
		}
		messages, _ := json.Marshal(body["messages"])
		if !strings.Contains(string(messages), "只输出 JSON") {
			t.Error("missing JSON fallback prompt")
		}
		completion(w, "结果：{\"title\":\"完成\"}。")
	}))
	defer remote.Close()
	client := llm.New(func(context.Context, string) (llm.Config, error) {
		return llm.Config{ProviderID: 2, BaseURL: remote.URL + "/v1", APIKey: "test-key", Model: "test-model", ReasoningEffort: "high"}, nil
	}, func(_ context.Context, _ llm.Config, _ string, result llm.Result, _ time.Duration, err error) {
		if err != nil || result.InputTokens != 10 || result.OutputTokens != 4 {
			t.Errorf("record: %+v %v", result, err)
		}
		recorded.Add(1)
	}, func(_ context.Context, id int64) error {
		if id != 2 {
			t.Errorf("provider ID %d", id)
		}
		marked.Add(1)
		return nil
	})
	result, err := client.Complete(context.Background(), llm.Request{Purpose: "agent", Messages: []llm.Message{{Role: "user", Content: "起标题"}}, JSONSchema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}}}`)})
	if err != nil || result.Text != `{"title":"完成"}` || reasoningRejected.Load() != 1 || jsonRejected.Load() != 1 || marked.Load() != 1 || recorded.Load() != 1 {
		t.Fatalf("result=%+v err=%v reasoning=%d json=%d marked=%d recorded=%d", result, err, reasoningRejected.Load(), jsonRejected.Load(), marked.Load(), recorded.Load())
	}
}

func TestStreamingToolsAndUsage(t *testing.T) {
	var recorded atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["stream"] != true || body["stream_options"] == nil || body["tools"] == nil {
			t.Errorf("stream request: %+v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, delta := range []string{
			`{"content":"开始"}`,
			`{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"test__read","arguments":"{\"x\":"}}]}`,
			`{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]}`,
		} {
			fmt.Fprintf(w, "data: {\"id\":\"chat_1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":%s}]}\n\n", delta)
		}
		fmt.Fprint(w, "data: {\"id\":\"chat_1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[],\"usage\":{\"prompt_tokens\":6,\"completion_tokens\":3,\"total_tokens\":9}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer remote.Close()
	client := llm.New(func(context.Context, string) (llm.Config, error) {
		return llm.Config{BaseURL: remote.URL + "/v1", Model: "test-model"}, nil
	}, func(_ context.Context, _ llm.Config, _ string, result llm.Result, _ time.Duration, err error) {
		if err != nil || result.InputTokens != 6 || result.OutputTokens != 3 {
			t.Errorf("record: %+v %v", result, err)
		}
		recorded.Add(1)
	}, nil)
	stream, err := client.Stream(context.Background(), llm.Request{Purpose: "agent", Messages: []llm.Message{{Role: "user", Content: "读数据"}}, Tools: []llm.Tool{{Name: "test__read", Description: "Read", Parameters: json.RawMessage(`{"type":"object"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for stream.Next() {
		text.WriteString(stream.Current().Text)
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	result := stream.Result()
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if text.String() != "开始" || len(result.ToolCalls) != 1 || result.ToolCalls[0].ID != "call_1" || result.ToolCalls[0].Name != "test__read" || string(result.ToolCalls[0].Arguments) != `{"x":1}` || recorded.Load() != 1 {
		t.Fatalf("text=%q result=%+v recorded=%d", text.String(), result, recorded.Load())
	}
}

func TestToolHistoryForSecondTurn(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Messages) != 3 || body.Messages[1]["role"] != "assistant" || body.Messages[2]["role"] != "tool" || body.Messages[2]["tool_call_id"] != "call_1" {
			t.Errorf("history: %+v", body.Messages)
		}
		calls, _ := body.Messages[1]["tool_calls"].([]any)
		if len(calls) != 1 {
			t.Errorf("tool calls: %+v", body.Messages[1])
		}
		completion(w, "完成")
	}))
	defer remote.Close()
	client := llm.New(func(context.Context, string) (llm.Config, error) {
		return llm.Config{BaseURL: remote.URL + "/v1", Model: "test-model"}, nil
	}, nil, nil)
	result, err := client.Complete(context.Background(), llm.Request{Purpose: "agent", Messages: []llm.Message{
		{Role: "user", Content: "读数据"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "test__read", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", ToolCallID: "call_1", Content: "数据"},
	}})
	if err != nil || result.Text != "完成" {
		t.Fatalf("second turn: %+v %v", result, err)
	}
}
