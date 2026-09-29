package ai_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestAiModelSpecsAndRefresh(t *testing.T) {
	fixture, err := os.ReadFile("testdata/modelsdev.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := ai.ParseModelsDevForTest(fixture)
	if err != nil {
		t.Fatal(err)
	}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"gpt-4o"},{"id":"openai/gpt-4o"},{"id":"mystery"}]}`)
	}))
	defer remote.Close()
	env := testutil.New(t)
	if err := env.App.Deps.Settings.Set(context.Background(), "ai.modelsdev", catalog); err != nil {
		t.Fatal(err)
	}
	env.Elevate()
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "OpenAI mirror", "baseUrl": remote.URL}, &provider)
	var models []api.AiModel
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, &models)
	if len(models) != 3 {
		t.Fatalf("models: %+v", models)
	}
	byID := map[string]api.AiModel{}
	for _, model := range models {
		byID[model.Id] = model
	}
	if byID["gpt-4o"].SpecSource != "stripped" || byID["openai/gpt-4o"].SpecSource != "stripped" || byID["mystery"].SpecSource != "unknown" {
		t.Fatalf("sources: %+v", byID)
	}
	env.MustDo("PATCH", fmt.Sprintf("/ai/providers/%d", provider.Id), map[string]any{"baseUrl": "https://api.openai.com/v1"}, nil)
	env.MustDo("GET", fmt.Sprintf("/ai/models?providerId=%d", provider.Id), nil, &models)
	byID = map[string]api.AiModel{}
	for _, model := range models {
		byID[model.Id] = model
	}
	if byID["gpt-4o"].SpecSource != "exact" || byID["openai/gpt-4o"].SpecSource != "stripped" {
		t.Fatalf("exact source: %+v", byID)
	}
	contextWindow := 8192
	toolCall := false
	var manual api.AiModel
	env.MustDo("PUT", "/ai/model-specs", api.AiModelSpecInput{ProviderId: provider.Id, ModelId: "mystery", ContextWindow: &contextWindow, ToolCall: &toolCall}, &manual)
	if manual.SpecSource != "manual" || manual.ContextWindow == nil || *manual.ContextWindow != contextWindow || manual.ToolCall == nil || *manual.ToolCall {
		t.Fatalf("manual spec: %+v", manual)
	}
	if status, _ := env.Do("PUT", "/ai/model-specs", map[string]any{"providerId": provider.Id, "modelId": "missing"}, nil); status != 404 {
		t.Fatalf("missing model: %d", status)
	}
	env.MustDo("GET", "/ai/models", nil, &models)
	if len(models) != 3 {
		t.Fatalf("list: %+v", models)
	}
}
