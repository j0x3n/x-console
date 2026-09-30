package llm_test

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

	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

func sse(w http.ResponseWriter, event map[string]any) {
	raw, _ := json.Marshal(event)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], raw)
}

func responsesClient(url string, effort string, record llm.RecordFunc, marked *atomic.Int32) llm.Client {
	return llm.New(func(context.Context, string) (llm.Config, error) {
		return llm.Config{ProviderID: 3, BaseURL: url + "/v1", APIKey: "test-key", Model: "gpt-test", ReasoningEffort: effort, APIStyle: "responses"}, nil
	}, record, func(context.Context, int64) error {
		if marked != nil {
			marked.Add(1)
		}
		return nil
	})
}

func TestResponsesStreamToolsImagesAndUsage(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	var rejected, marked atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "not found", 404)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["reasoning"] != nil {
			rejected.Add(1)
			http.Error(w, `{"error":{"message":"Unsupported parameter: reasoning.effort"}}`, 400)
			return
		}
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		sse(w, map[string]any{"type": "response.output_text.delta", "delta": "先看"})
		sse(w, map[string]any{"type": "response.output_text.delta", "delta": "一下"})
		sse(w, map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "call_id": "call_1", "name": "notes__search", "arguments": `{"q":"周报"}`}})
		sse(w, map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed",
			"output": []any{
				map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "先看一下"}}},
				map[string]any{"type": "function_call", "call_id": "call_1", "name": "notes__search", "arguments": `{"q":"周报"}`},
			},
			"usage": map[string]any{"input_tokens": 30, "output_tokens": 7}}})
	}))
	defer remote.Close()
	var recorded atomic.Int32
	client := responsesClient(remote.URL, "high", func(_ context.Context, _ llm.Config, _ string, result llm.Result, _ time.Duration, err error) {
		if err != nil || result.InputTokens != 30 || result.OutputTokens != 7 {
			t.Errorf("record: %+v %v", result, err)
		}
		recorded.Add(1)
	}, &marked)
	stream, err := client.Stream(context.Background(), llm.Request{Purpose: "agent", System: "你是助手",
		Messages: []llm.Message{
			{Role: "user", Content: "看看这张图", Images: []llm.Image{{MIME: "image/png", Data: []byte("png")}}},
			{Role: "assistant", Content: "好", ToolCalls: []llm.ToolCall{{ID: "call_0", Name: "notes__list", Arguments: json.RawMessage(`{}`)}}},
			{Role: "tool", ToolCallID: "call_0", Content: "[]"},
		},
		Tools: []llm.Tool{{Name: "notes__search", Description: "搜笔记", Parameters: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":[]}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for stream.Next() {
		text += stream.Current().Text
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	result := stream.Result()
	_ = stream.Close()
	if text != "先看一下" || result.Text != "先看一下" || len(result.ToolCalls) != 1 || result.ToolCalls[0].ID != "call_1" || string(result.ToolCalls[0].Arguments) != `{"q":"周报"}` || result.Truncated {
		t.Fatalf("stream: %q %+v", text, result)
	}
	if rejected.Load() != 1 || marked.Load() != 1 || recorded.Load() != 1 {
		t.Fatalf("reasoning retry: rejected=%d marked=%d recorded=%d", rejected.Load(), marked.Load(), recorded.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	raw, _ := json.Marshal(bodies[0])
	for _, want := range []string{`"instructions":"你是助手"`, `"type":"input_image"`, `data:image/png;base64,cG5n`, `"call_id":"call_0","name":"notes__list","type":"function_call"`,
		`"call_id":"call_0","output":"[]","type":"function_call_output"`, `"name":"notes__search","parameters"`, `"type":"function"`, `"stream":true`, `"store":false`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("request body misses %s: %s", want, raw)
		}
	}
}

func TestResponsesCompleteJSONAndTruncation(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["reasoning"].(map[string]any)["effort"] != "low" || body["text"] == nil || body["stream"] != false {
			t.Errorf("request: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "incomplete", "incomplete_details": map[string]any{"reason": "max_output_tokens"},
			"output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": `{"title":"半`}}}},
			"usage":  map[string]any{"input_tokens": 5, "output_tokens": 100}})
	}))
	defer remote.Close()
	client := responsesClient(remote.URL, "low", nil, nil)
	result, err := client.Complete(context.Background(), llm.Request{Purpose: "fast", Messages: []llm.Message{{Role: "user", Content: "起个标题"}},
		JSONSchema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}}}`)})
	if err != nil || !result.Truncated || result.OutputTokens != 100 {
		t.Fatalf("complete: %+v %v", result, err)
	}
}

func TestResponsesErrorCarriesBody(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Invalid schema for function 'x'"}`, 400)
	}))
	defer remote.Close()
	_, err := responsesClient(remote.URL, "", nil, nil).Complete(context.Background(), llm.Request{Purpose: "agent", Messages: []llm.Message{{Role: "user", Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "Invalid schema") || !strings.Contains(err.Error(), "/v1/responses") {
		t.Fatalf("error: %v", err)
	}
}
