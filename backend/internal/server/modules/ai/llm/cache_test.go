package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

func cacheResponse(w http.ResponseWriter, style string, stream bool) {
	if style == "responses" {
		object := map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "完成"}}}}}
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			sse(w, map[string]any{"type": "response.completed", "response": object})
		} else {
			json.NewEncoder(w).Encode(object)
		}
		return
	}
	if !stream {
		completion(w, "完成")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprint(w, "data: {\"id\":\"chat_1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"完成\"}}]}\n\ndata: [DONE]\n\n")
}

func cacheCall(t *testing.T, client llm.Client, req llm.Request, stream bool) {
	t.Helper()
	if !stream {
		result, err := client.Complete(context.Background(), req)
		if err != nil || result.Text != "完成" {
			t.Fatalf("响应：%+v %v", result, err)
		}
		return
	}
	s, err := client.Stream(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for s.Next() {
	}
	if s.Err() != nil || s.Result().Text != "完成" {
		t.Fatalf("流响应：%+v %v", s.Result(), s.Err())
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPromptCacheKeyAndStablePrefix(t *testing.T) {
	for _, style := range []string{"chat", "responses"} {
		for _, stream := range []bool{false, true} {
			for _, reject := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/reject=%t", style, stream, reject), func(t *testing.T) {
					var mu sync.Mutex
					var bodies []map[string]any
					remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
						}
						mu.Lock()
						bodies = append(bodies, body)
						mu.Unlock()
						if reject && body["prompt_cache_key"] != nil {
							http.Error(w, `{"error":{"message":"Unsupported parameter: prompt_cache_key"}}`, 400)
							return
						}
						cacheResponse(w, style, stream)
					}))
					defer remote.Close()
					client := llm.New(func(context.Context, string) (llm.Config, error) {
						return llm.Config{BaseURL: remote.URL + "/v1", Model: "test-model", APIStyle: style}, nil
					}, nil, nil)
					req := llm.Request{Purpose: "agent", CacheKey: "assistant:17", System: "固定规则", Tools: []llm.Tool{{Name: "read", Description: "读取", Parameters: json.RawMessage(`{"type":"object"}`)}}}
					for i := 0; i < 2; i++ {
						req.Messages = append(req.Messages, llm.Message{Role: "user", Content: fmt.Sprintf("第 %d 轮", i)})
						cacheCall(t, client, req, stream)
					}
					mu.Lock()
					defer mu.Unlock()
					step := 1
					if reject {
						step = 2
					}
					if len(bodies) != 2*step {
						t.Fatalf("请求数：%d", len(bodies))
					}
					for i, b := range bodies {
						if i%step == 0 && b["prompt_cache_key"] != "assistant:17" {
							t.Fatalf("缓存键：%+v", b)
						}
						if reject && i%step == 1 && b["prompt_cache_key"] != nil {
							t.Fatal("重试没有移除缓存键")
						}
					}
					first, second := bodies[step-1], bodies[2*step-1]
					system := func(b map[string]any) any {
						if style == "responses" {
							return b["instructions"]
						}
						return b["messages"].([]any)[0]
					}
					if !reflect.DeepEqual(system(first), system(second)) || !reflect.DeepEqual(first["tools"], second["tools"]) {
						t.Fatal("连续两轮的 system 或 tools 改变")
					}
				})
			}
		}
	}
}

func TestPromptCacheKeyFallsBackWithOtherUnsupportedFields(t *testing.T) {
	for _, style := range []string{"chat", "responses"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", style, stream), func(t *testing.T) {
				var mu sync.Mutex
				calls := 0
				remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					mu.Lock()
					calls++
					mu.Unlock()
					if body["prompt_cache_key"] != nil {
						if body["prompt_cache_key"] != "fast" {
							t.Errorf("用途键：%v", body["prompt_cache_key"])
						}
						http.Error(w, `{"error":{"message":"Unsupported parameter: prompt_cache_key"}}`, 400)
						return
					}
					if body["reasoning"] != nil || body["reasoning_effort"] != nil {
						http.Error(w, `{"error":{"message":"Unsupported parameter: reasoning"}}`, 400)
						return
					}
					if body["text"] != nil || body["response_format"] != nil {
						field := "response_format"
						if style == "responses" {
							field = "json_schema"
						}
						http.Error(w, fmt.Sprintf(`{"error":{"message":"Unsupported parameter: %s"}}`, field), 400)
						return
					}
					cacheResponse(w, style, stream)
				}))
				defer remote.Close()
				client := llm.New(func(context.Context, string) (llm.Config, error) {
					return llm.Config{BaseURL: remote.URL + "/v1", Model: "test-model", APIStyle: style, ReasoningEffort: "high"}, nil
				}, nil, nil)
				cacheCall(t, client, llm.Request{Purpose: "fast", System: "固定说明", Messages: []llm.Message{{Role: "user", Content: "变化内容"}}, JSONSchema: json.RawMessage(`{"type":"object"}`)}, stream)
				mu.Lock()
				defer mu.Unlock()
				if calls != 4 {
					t.Fatalf("请求数：%d", calls)
				}
			})
		}
	}
}
