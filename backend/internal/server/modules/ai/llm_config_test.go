package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestAiLLMSelectionAndUsage(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
		case "/v1/chat/completions":
			if r.Header.Get("Authorization") != "Bearer secret-value" {
				http.Error(w, "bad key", 401)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"chat_1","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"好"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	env := testutil.New(t)
	env.Elevate()
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Test", "baseUrl": remote.URL + "/v1", "apiKey": "secret-value"}, &provider)
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	catalog := map[string]any{"openai": map[string]any{"test-model": map[string]any{"name": "Test Model", "cost": map[string]any{"input": 2, "output": 4}}}}
	if err := env.App.Deps.Settings.Set(context.Background(), "ai.modelsdev", catalog); err != nil {
		t.Fatal(err)
	}
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}, "fast": nil, "reasoningEffort": "off"}, nil)
	created, err := ai.New(env.App.Deps)
	if err != nil {
		t.Fatal(err)
	}
	client := created.(*ai.Module).LLMForTest()
	result, err := client.Complete(context.Background(), llm.Request{Purpose: "fast", Messages: []llm.Message{{Role: "user", Content: "测试"}}})
	if err != nil || result.Text != "好" {
		t.Fatalf("completion: %+v %v", result, err)
	}
	var usage api.AiUsage
	env.MustDo("GET", "/ai/usage", nil, &usage)
	if usage.Calls != 1 || usage.InputTokens != 100 || usage.OutputTokens != 50 || usage.Cost == nil || math.Abs(float64(*usage.Cost)-0.0004) > 0.0000001 || len(usage.ByModel) != 1 || usage.ByModel[0].Purpose == nil || *usage.ByModel[0].Purpose != "fast" {
		raw, _ := json.Marshal(usage)
		t.Fatalf("usage: %s", raw)
	}
}
