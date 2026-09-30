package ai_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestAiModelSettings(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"tools"},{"id":"no-tools"}]}`)
	}))
	defer remote.Close()
	env := testutil.New(t)
	if status, _ := env.Do("PUT", "/ai/model-settings", map[string]any{"agent": nil}, nil); status != 403 {
		t.Fatalf("settings without elevation: %d", status)
	}
	env.Elevate()
	if status, _ := env.Do("GET", "/ai/settings", nil, nil); status != http.StatusNotFound {
		t.Fatalf("legacy settings endpoint should be gone: %d", status)
	}
	if err := env.App.Deps.Settings.SetSecret(context.Background(), "ai.api_key", "old-secret"); err != nil {
		t.Fatal(err)
	}
	var settings api.AiModelSettings
	env.MustDo("GET", "/ai/model-settings", nil, &settings)
	if settings.LegacyAnthropic == nil || !*settings.LegacyAnthropic {
		t.Fatalf("legacy marker: %+v", settings)
	}
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Test", "baseUrl": remote.URL}, &provider)
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	noTools := false
	env.MustDo("PUT", "/ai/model-specs", api.AiModelSpecInput{ProviderId: provider.Id, ModelId: "no-tools", ToolCall: &noTools}, nil)
	invalidAgent := map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "no-tools"}}
	if status, _ := env.Do("PUT", "/ai/model-settings", invalidAgent, nil); status != 400 {
		t.Fatalf("agent without tools: %d", status)
	}
	if status, _ := env.Do("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "missing"}}, nil); status != 400 {
		t.Fatalf("unknown model: %d", status)
	}
	input := map[string]any{
		"agent":           map[string]any{"providerId": provider.Id, "model": "tools"},
		"fast":            map[string]any{"providerId": provider.Id, "model": "no-tools"},
		"reasoningEffort": "high", "confirmAllWrites": true,
	}
	env.MustDo("PUT", "/ai/model-settings", input, &settings)
	if settings.Agent == nil || settings.Fast == nil || settings.ReasoningEffort != "high" || !settings.ConfirmAllWrites {
		t.Fatalf("saved: %+v", settings)
	}
	settings = api.AiModelSettings{}
	env.MustDo("GET", "/ai/model-settings", nil, &settings)
	if settings.LegacyAnthropic != nil || settings.Agent.Model != "tools" {
		t.Fatalf("read: %+v", settings)
	}
	env.MustDo("DELETE", fmt.Sprintf("/ai/providers/%d", provider.Id), nil, nil)
	settings = api.AiModelSettings{}
	env.MustDo("GET", "/ai/model-settings", nil, &settings)
	if settings.Agent != nil || settings.Fast != nil {
		t.Fatalf("delete should clear selections: %+v", settings)
	}
}

func TestAiUsageMonth(t *testing.T) {
	env := testutil.New(t)
	month := time.Now().In(env.App.Deps.Config.Location).Format("2006-01")
	when, err := time.ParseInLocation("2006-01", month, env.App.Deps.Config.Location)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		model string
		cost  any
		at    time.Time
	}{{"priced", 0.25, when.Add(time.Hour)}, {"priced", 0.5, when.Add(2 * time.Hour)}, {"unknown", nil, when.Add(3 * time.Hour)}, {"old", 1.0, when.AddDate(0, -1, 0)}} {
		_, err := env.App.Deps.DB.Exec(`INSERT INTO ai_usage(provider_id,provider_name,model,purpose,input_tokens,output_tokens,duration_ms,cost,created_at) VALUES(1,'Test',?,'agent',100,20,50,?,?)`, item.model, item.cost, item.at.UTC())
		if err != nil {
			t.Fatal(err)
		}
	}
	var usage api.AiUsage
	env.MustDo("GET", "/ai/usage?month="+month, nil, &usage)
	if usage.Calls != 3 || usage.InputTokens != 300 || usage.OutputTokens != 60 || usage.Cost == nil || *usage.Cost != 0.75 || len(usage.ByModel) != 2 || usage.ByModel[0].Model != "priced" || usage.ByModel[1].Cost != nil {
		t.Fatalf("usage: %+v", usage)
	}
	if status, _ := env.Do("GET", "/ai/usage?month=2026-13", nil, nil); status != 400 {
		t.Fatalf("invalid month: %d", status)
	}
}
