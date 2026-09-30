package ai_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestAiProviders(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer secret-value" {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"test-model"},{"id":"test-model"},{"id":"second"}]}`)
	}))
	defer remote.Close()
	env := testutil.New(t)
	input := map[string]any{"name": " Test ", "baseUrl": remote.URL + "/v1/", "apiKey": "secret-value"}
	if status, _ := env.Do("POST", "/ai/providers", input, nil); status != 403 {
		t.Fatalf("create without elevation: %d", status)
	}
	env.Elevate()
	var created api.AiProvider
	env.MustDo("POST", "/ai/providers", input, &created)
	if created.Name != "Test" || created.BaseUrl != remote.URL+"/v1" || !created.HasApiKey {
		t.Fatalf("created: %+v", created)
	}
	var sealed string
	if err := env.App.Deps.DB.QueryRow(`SELECT api_key_enc FROM ai_providers WHERE id=?`, created.Id).Scan(&sealed); err != nil || sealed == "secret-value" || !strings.Contains(sealed, "=") {
		t.Fatalf("key was not encrypted: %v", err)
	}
	var providers []api.AiProvider
	env.MustDo("GET", "/ai/providers", nil, &providers)
	if len(providers) != 1 || providers[0].Id != created.Id {
		t.Fatalf("providers: %+v", providers)
	}
	var result api.AiProviderTest
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/test", created.Id), nil, &result)
	if !result.Ok || result.ModelCount == nil || *result.ModelCount != 2 {
		t.Fatalf("test: %+v", result)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		env.MustDo("GET", "/ai/providers", nil, &providers)
		if providers[0].ModelCount == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if providers[0].ModelCount != 2 {
		t.Fatalf("background refresh: %+v", providers)
	}
	path := fmt.Sprintf("/ai/providers/%d", created.Id)
	env.MustDo("PATCH", path, map[string]any{"apiKey": ""}, &created)
	if created.HasApiKey {
		t.Fatal("key was not cleared")
	}
	if status, _ := env.Do("PATCH", path, map[string]any{"baseUrl": "file:///tmp"}, nil); status != 400 {
		t.Fatalf("invalid URL: %d", status)
	}
	env.MustDo("DELETE", path, nil, nil)
	if status, _ := env.Do("DELETE", path, nil, nil); status != 404 {
		t.Fatalf("deleted provider: %d", status)
	}
}
