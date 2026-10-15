package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
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

func TestAiReasoningFallbackPersists(t *testing.T) {
	var rejected, accepted atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		if body["reasoning_effort"] != nil {
			rejected.Add(1)
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":{"message":"reasoning_effort unsupported"}}`)
			return
		}
		accepted.Add(1)
		fmt.Fprint(w, `{"id":"chat_1","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"好"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer remote.Close()
	env := testutil.New(t)
	env.Elevate()
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Test", "baseUrl": remote.URL + "/v1"}, &provider)
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}, "reasoningEffort": "high"}, nil)
	built, err := ai.New(env.App.Deps)
	if err != nil {
		t.Fatal(err)
	}
	client := built.(*ai.Module).LLMForTest()
	for range 2 {
		if _, err := client.Complete(context.Background(), llm.Request{Purpose: "agent", Messages: []llm.Message{{Role: "user", Content: "测试"}}}); err != nil {
			t.Fatal(err)
		}
	}
	var settings api.AiModelSettings
	env.MustDo("GET", "/ai/model-settings", nil, &settings)
	if rejected.Load() != 1 || accepted.Load() != 2 || !settings.ReasoningUnsupported {
		t.Fatalf("reasoning fallback rejected=%d accepted=%d settings=%+v", rejected.Load(), accepted.Load(), settings)
	}
}

// B212: a feature's own model and effort win over Settings → AI for both
// purposes. A model that is gone is "not configured", not swapped.
func TestAiFeatureModelChoice(t *testing.T) {
	type seen struct {
		model  string
		effort any
	}
	got := make(chan seen, 10)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"test-model"},{"id":"vision-model"}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		model, _ := body["model"].(string)
		got <- seen{model: model, effort: body["reasoning_effort"]}
		fmt.Fprint(w, `{"id":"chat_1","object":"chat.completion","created":1,"model":"x","choices":[{"index":0,"message":{"role":"assistant","content":"好"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer remote.Close()
	env := testutil.New(t)
	env.Elevate()
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Test", "baseUrl": remote.URL + "/v1"}, &provider)
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}, "reasoningEffort": "off", "fastReasoningEffort": "off"}, nil)
	built, err := ai.New(env.App.Deps)
	if err != nil {
		t.Fatal(err)
	}
	client := built.(*ai.Module).LLMForTest()
	call := func(ctx context.Context, purpose string) (seen, error) {
		_, err := client.Complete(ctx, llm.Request{Purpose: purpose, Messages: []llm.Message{{Role: "user", Content: "测试"}}})
		if err != nil {
			return seen{}, err
		}
		return <-got, nil
	}
	vision := fmt.Sprintf("%d:vision-model", provider.Id)
	for _, purpose := range []string{"agent", "fast"} {
		s, err := call(contracts.WithAIModel(context.Background(), vision, "high"), purpose)
		if err != nil || s.model != "vision-model" || s.effort != "high" {
			t.Fatalf("%s with a chosen model: %+v %v", purpose, s, err)
		}
	}
	if s, err := call(contracts.WithAIModel(context.Background(), "", "low"), "fast"); err != nil || s.model != "test-model" || s.effort != "low" {
		t.Fatalf("effort only: %+v %v", s, err)
	}
	if s, err := call(context.Background(), "agent"); err != nil || s.model != "test-model" || s.effort != nil {
		t.Fatalf("no choice: %+v %v", s, err)
	}
	for _, gone := range []string{fmt.Sprintf("%d:deleted-model", provider.Id), "999:test-model", "bad"} {
		if _, err := call(contracts.WithAIModel(context.Background(), gone, ""), "agent"); !errors.Is(err, llm.ErrNotConfigured) {
			t.Fatalf("%s: %v", gone, err)
		}
	}
}
