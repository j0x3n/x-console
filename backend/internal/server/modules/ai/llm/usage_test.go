package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

// B42：三种格式的缓存用量都能读出来，全部输入统一包括缓存部分。
func TestUsageReadsCacheFields(t *testing.T) {
	cases := []struct {
		name                                 string
		style                                string
		usage                                map[string]any
		input, cached, write, output, reason int64
	}{
		{"chat openai", "chat", map[string]any{"prompt_tokens": 100, "completion_tokens": 20,
			"prompt_tokens_details": map[string]any{"cached_tokens": 80}, "completion_tokens_details": map[string]any{"reasoning_tokens": 5}},
			100, 80, 0, 20, 5},
		{"chat anthropic style", "chat", map[string]any{"prompt_tokens": 10, "completion_tokens": 20,
			"cache_read_input_tokens": 70, "cache_creation_input_tokens": 20},
			100, 70, 20, 20, 0},
		{"responses", "responses", map[string]any{"input_tokens": 50, "output_tokens": 7,
			"input_tokens_details": map[string]any{"cached_tokens": 40}, "output_tokens_details": map[string]any{"reasoning_tokens": 3}},
			50, 40, 0, 7, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if c.style == "responses" {
					json.NewEncoder(w).Encode(map[string]any{"id": "resp_1", "object": "response", "status": "completed",
						"output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "好"}}}},
						"usage":  c.usage})
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"id": "chat_1", "object": "chat.completion", "created": 1, "model": "m",
					"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "好"}, "finish_reason": "stop"}},
					"usage":   c.usage})
			}))
			defer remote.Close()
			var got llm.Result
			client := llm.New(func(context.Context, string) (llm.Config, error) {
				return llm.Config{BaseURL: remote.URL + "/v1", Model: "m", APIStyle: c.style}, nil
			}, func(_ context.Context, _ llm.Config, _ string, r llm.Result, _ time.Duration, _ error) { got = r },
				func(context.Context, int64) error { return nil })
			if _, err := client.Complete(context.Background(), llm.Request{Purpose: "fast", Messages: []llm.Message{{Role: "user", Content: "x"}}}); err != nil {
				t.Fatal(err)
			}
			if got.InputTokens != c.input || got.CachedInputTokens != c.cached || got.CacheWriteTokens != c.write ||
				got.OutputTokens != c.output || got.ReasoningTokens != c.reason {
				t.Fatalf("usage %+v", got)
			}
		})
	}
}

func TestStreamUsageReadsCachedTokens(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"好\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[],\"usage\":{\"prompt_tokens\":60,\"completion_tokens\":3,\"total_tokens\":63,\"prompt_tokens_details\":{\"cached_tokens\":50}}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer remote.Close()
	done := make(chan llm.Result, 1)
	client := llm.New(func(context.Context, string) (llm.Config, error) {
		return llm.Config{BaseURL: remote.URL + "/v1", Model: "m"}, nil
	}, func(_ context.Context, _ llm.Config, _ string, r llm.Result, _ time.Duration, _ error) {
		select {
		case done <- r:
		default:
		}
	},
		func(context.Context, int64) error { return nil })
	stream, err := client.Stream(context.Background(), llm.Request{Purpose: "agent", Messages: []llm.Message{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	for stream.Next() {
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	got := <-done
	if got.InputTokens != 60 || got.CachedInputTokens != 50 {
		t.Fatalf("usage %+v", got)
	}
}
