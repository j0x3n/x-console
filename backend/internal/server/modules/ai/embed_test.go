package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestEmbed(t *testing.T) {
	var gotModel, gotAuth string
	var requests, maxBatch int
	shuffle := false
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
		case "/v1/embeddings":
			var body struct {
				Model string
				Input []string
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotModel, gotAuth = body.Model, r.Header.Get("Authorization")
			requests++
			maxBatch = max(maxBatch, len(body.Input))
			type item struct {
				Object    string    `json:"object"`
				Index     int       `json:"index"`
				Embedding []float64 `json:"embedding"`
			}
			data := make([]item, len(body.Input))
			for i, in := range body.Input {
				// The vector is made of the text length, so tests can tell the texts apart.
				data[i] = item{Object: "embedding", Index: i, Embedding: []float64{float64(len([]rune(in))), 1}}
			}
			if shuffle && len(data) > 1 {
				data[0], data[len(data)-1] = data[len(data)-1], data[0]
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "model": body.Model, "data": data, "usage": map[string]int{"prompt_tokens": 1, "total_tokens": 1}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	env := testutil.New(t)
	env.Elevate()
	em, ok := module.Lookup[contracts.Embedder](env.App.Deps.Registry, contracts.EmbedderKey)
	if !ok {
		t.Fatal("embedder not registered")
	}
	ctx := context.Background()
	if em.Available(ctx, 0) {
		t.Fatal("available without a provider")
	}
	if _, err := em.Embed(ctx, 0, "", []string{"x"}); err == nil || !strings.Contains(err.Error(), "供应商") {
		t.Fatalf("without provider: %v", err)
	}
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Test", "baseUrl": remote.URL + "/v1", "apiKey": "secret-value"}, &provider)
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	catalog := map[string]any{"openai": map[string]any{"test-model": map[string]any{"name": "Test Model", "cost": map[string]any{"input": 2, "output": 4}}}}
	if err := env.App.Deps.Settings.Set(ctx, "ai.modelsdev", catalog); err != nil {
		t.Fatal(err)
	}
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}, "fast": nil, "reasoningEffort": "off"}, nil)
	if !em.Available(ctx, 0) {
		t.Fatal("not available with a provider")
	}

	vecs, err := em.Embed(ctx, 0, "", []string{"一二", "一二三四"})
	if err != nil || len(vecs) != 2 || vecs[0][0] != 2 || vecs[1][0] != 4 {
		t.Fatalf("embed: %v %v", vecs, err)
	}
	if gotModel != "text-embedding-3-small" || gotAuth != "Bearer secret-value" {
		t.Errorf("request: model %q auth %q", gotModel, gotAuth)
	}
	if _, err := em.Embed(ctx, 0, "other-model", []string{"x"}); err != nil || gotModel != "other-model" {
		t.Errorf("model override: %v %q", err, gotModel)
	}
	// 250 texts go in three requests of at most 100 (Google's limit), and the order is kept.
	many := make([]string, 250)
	for i := range many {
		many[i] = strings.Repeat("字", i+1)
	}
	requests, maxBatch = 0, 0
	vecs, err = em.Embed(ctx, 0, "", many)
	if err != nil || len(vecs) != 250 || requests != 3 || maxBatch != 100 || vecs[249][0] != 250 || vecs[0][0] != 1 {
		t.Fatalf("batches: %d vectors, %d requests, max %d, %v", len(vecs), requests, maxBatch, err)
	}
	// The API may answer out of order; the index field decides.
	shuffle = true
	vecs, err = em.Embed(ctx, 0, "", []string{"一", "一二", "一二三"})
	if err != nil || vecs[0][0] != 1 || vecs[1][0] != 2 || vecs[2][0] != 3 {
		t.Fatalf("shuffled: %v %v", vecs, err)
	}
	if v, err := em.Embed(ctx, 0, "", nil); err != nil || v != nil {
		t.Errorf("empty: %v %v", v, err)
	}

	// B197: another provider than fast AI, picked by id.
	var gotPath, otherAuth, otherModel string
	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Model string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotPath, otherAuth, otherModel = r.URL.Path, r.Header.Get("Authorization"), body.Model
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[3,4]}]}`)
	}))
	defer google.Close()
	var other api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Google", "baseUrl": google.URL + "/v1beta/openai", "apiKey": "google-key"}, &other)
	if !em.Available(ctx, other.Id) || em.Available(ctx, other.Id+100) {
		t.Fatal("availability by provider")
	}
	vecs, err = em.Embed(ctx, other.Id, "my-embed", []string{"x"})
	if err != nil || len(vecs) != 1 || vecs[0][0] != 3 {
		t.Fatalf("other provider: %v %v", vecs, err)
	}
	if gotPath != "/v1beta/openai/embeddings" || otherAuth != "Bearer google-key" || otherModel != "my-embed" {
		t.Errorf("other provider request: %q %q %q", gotPath, otherAuth, otherModel)
	}
	if _, err := em.Embed(ctx, other.Id+100, "", []string{"x"}); err == nil || !strings.Contains(err.Error(), "删除") {
		t.Errorf("deleted provider: %v", err)
	}
}

func TestEmbedGoogleDefaultModel(t *testing.T) {
	if !ai.IsGoogle("https://generativelanguage.googleapis.com/v1beta/openai/") || ai.IsGoogle("https://api.openai.com/v1") || ai.IsGoogle("::") {
		t.Fatal("isGoogle")
	}
}
