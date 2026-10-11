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
	if em.Available(ctx) {
		t.Fatal("available without a provider")
	}
	if _, err := em.Embed(ctx, "", []string{"x"}); err == nil || !strings.Contains(err.Error(), "供应商") {
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
	if !em.Available(ctx) {
		t.Fatal("not available with a provider")
	}

	vecs, err := em.Embed(ctx, "", []string{"一二", "一二三四"})
	if err != nil || len(vecs) != 2 || vecs[0][0] != 2 || vecs[1][0] != 4 {
		t.Fatalf("embed: %v %v", vecs, err)
	}
	if gotModel != "text-embedding-3-small" || gotAuth != "Bearer secret-value" {
		t.Errorf("request: model %q auth %q", gotModel, gotAuth)
	}
	if _, err := em.Embed(ctx, "other-model", []string{"x"}); err != nil || gotModel != "other-model" {
		t.Errorf("model override: %v %q", err, gotModel)
	}
	// 300 texts go in three requests of at most 128, and the order is kept.
	many := make([]string, 300)
	for i := range many {
		many[i] = strings.Repeat("字", i+1)
	}
	requests, maxBatch = 0, 0
	vecs, err = em.Embed(ctx, "", many)
	if err != nil || len(vecs) != 300 || requests != 3 || maxBatch != 128 || vecs[299][0] != 300 || vecs[0][0] != 1 {
		t.Fatalf("batches: %d vectors, %d requests, max %d, %v", len(vecs), requests, maxBatch, err)
	}
	// The API may answer out of order; the index field decides.
	shuffle = true
	vecs, err = em.Embed(ctx, "", []string{"一", "一二", "一二三"})
	if err != nil || vecs[0][0] != 1 || vecs[1][0] != 2 || vecs[2][0] != 3 {
		t.Fatalf("shuffled: %v %v", vecs, err)
	}
	if v, err := em.Embed(ctx, "", nil); err != nil || v != nil {
		t.Errorf("empty: %v %v", v, err)
	}
}
