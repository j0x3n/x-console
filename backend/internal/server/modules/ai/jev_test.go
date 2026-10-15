package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestJevProviderSortsMailAndCannotChat(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"tools"}],"models":[{"name":"jev-latest","description":"stable","release_date":"2026-09-15"},{"name":"jev-latest","description":"dup","release_date":"2026-09-15"},{"name":"jev-preview","description":"preview","release_date":"2026-10-14"}]}`)
		case "/v1/systemone":
			if r.Header.Get("Authorization") != "Bearer secret-value" {
				http.Error(w, "no", 401)
				return
			}
			raw, _ := io.ReadAll(r.Body)
			mu.Lock()
			bodies = append(bodies, string(raw))
			mu.Unlock()
			choice, noul, codeNoul, confidence := "action", 0.91, 0.1, 0.91
			switch {
			case strings.Contains(string(raw), "促销"):
				choice, noul, codeNoul = "promo", 0.99, 0.99
			case strings.Contains(string(raw), "拿不准"):
				choice, noul, confidence = "action", 0.5, 0.85
			case strings.Contains(string(raw), "验证码"):
				choice, noul, codeNoul = "notice", 0.1, 0.91
			}
			fmt.Fprintf(w, `{"model":"jev-1.13.0","answers":{"category":{"type":"choice","choice":%q,"probabilities":{},"confidence":%g},"decision":{"type":"noul","noul":%g},"code":{"type":"noul","noul":%g}},"usage":{"input_tokens":1000,"output_tokens":20}}`, choice, confidence, noul, codeNoul)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	env := testutil.New(t)
	env.Elevate()
	var chat api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Chat", "baseUrl": remote.URL + "/v1", "apiKey": "secret-value"}, &chat)
	var jev api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Jev", "baseUrl": remote.URL + "/v1", "apiKey": "secret-value", "apiStyle": "jev"}, &jev)
	if jev.ApiStyle != api.Jev {
		t.Fatalf("style: %s", jev.ApiStyle)
	}
	var models []api.AiModel
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", jev.Id), nil, &models)
	if len(models) != 2 || models[0].Id != "jev-latest" && models[1].Id != "jev-latest" {
		t.Fatalf("jev models: %+v", models)
	}
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", chat.Id), nil, &models)
	if len(models) != 1 || models[0].Id != "tools" {
		t.Fatalf("chat models: %+v", models)
	}
	agent := map[string]any{"providerId": jev.Id, "model": "jev-latest"}
	if status, _ := env.Do("PUT", "/ai/model-settings", map[string]any{"agent": agent}, nil); status != 400 {
		t.Fatalf("jev as agent: %d", status)
	}
	if status, _ := env.Do("PUT", "/ai/model-settings", map[string]any{"fast": agent}, nil); status != 400 {
		t.Fatalf("jev as fast: %d", status)
	}
	if status, _ := env.Do("PUT", "/ai/model-settings", map[string]any{"mailTriage": map[string]any{"providerId": chat.Id, "model": "tools"}}, nil); status != 400 {
		t.Fatalf("chat as mail: %d", status)
	}
	var settings api.AiModelSettings
	env.MustDo("PUT", "/ai/model-settings", map[string]any{
		"agent":      map[string]any{"providerId": chat.Id, "model": "tools"},
		"mailTriage": agent,
	}, &settings)
	if settings.MailTriage == nil || settings.MailTriage.Model != "jev-latest" || settings.Agent == nil || settings.Agent.Model != "tools" {
		t.Fatalf("saved: %+v", settings)
	}
	env.MustDo("PUT", "/ai/model-settings", map[string]any{
		"agent":           map[string]any{"providerId": chat.Id, "model": "tools"},
		"reasoningEffort": "low",
	}, nil)
	settings = api.AiModelSettings{}
	env.MustDo("GET", "/ai/model-settings", nil, &settings)
	if settings.MailTriage == nil || settings.MailTriage.Model != "jev-latest" || settings.Agent == nil || settings.Agent.Model != "tools" {
		t.Fatalf("omit should keep mail model: %+v", settings)
	}
	sorter, ok := module.Lookup[contracts.MailSorter](env.App.Deps.Registry, contracts.MailSortKey)
	if !ok || !sorter.Available(context.Background()) {
		t.Fatal("sorter should be available")
	}
	ctx := contracts.WithAIUsage(context.Background(), "mail", "batch")
	out, err := sorter.Sort(ctx, []contracts.MailSortItem{
		{ID: 1, From: "a@b.c", Subject: "请批准", Text: "请批准这笔报销"},
		{ID: 2, From: "a@b.c", Subject: "促销", Text: "全场促销"},
		{ID: 3, From: "a@b.c", Subject: "拿不准", Text: "也许要你看一下"},
		{ID: 4, From: "a@b.c", Subject: "验证码", Text: "你的验证码是 123456"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, kept := out[3]; kept || !out[1].Decision || out[1].Category != "action" || out[1].Code || out[2].Decision || out[2].Code || out[2].Category != "promo" || !out[4].Code || out[4].Decision || out[4].Category != "notice" {
		t.Fatalf("verdicts: %+v", out)
	}
	mu.Lock()
	got := append([]string(nil), bodies...)
	mu.Unlock()
	if len(got) != 4 {
		t.Fatalf("calls: %d", len(got))
	}
	var req map[string]any
	if err := json.Unmarshal([]byte(got[0]), &req); err != nil {
		t.Fatal(err)
	}
	if req["model"] != "jev-latest" {
		t.Fatalf("model: %v", req["model"])
	}
	questions, _ := req["questions"].(map[string]any)
	category, _ := questions["category"].(map[string]any)
	decision, _ := questions["decision"].(map[string]any)
	code, _ := questions["code"].(map[string]any)
	if category["type"] != "choice" || decision["type"] != "noul" || code["type"] != "noul" {
		t.Fatalf("questions: %v", questions)
	}
	var model, purpose, source, ref string
	var input int64
	var cost float64
	if err := env.App.Deps.DB.QueryRow(`SELECT model,purpose,source,ref,input_tokens,cost FROM ai_usage WHERE ref='1'`).Scan(&model, &purpose, &source, &ref, &input, &cost); err != nil {
		t.Fatal(err)
	}
	if model != "jev-1.13.0" || purpose != "fast" || source != "mail" || ref != "1" || input != 1000 || math.Abs(cost-0.000042) > 1e-9 {
		t.Fatalf("usage %s %s %s %s %d %g", model, purpose, source, ref, input, cost)
	}
	if err := env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM ai_usage WHERE ref='3'`).Scan(&input); err != nil || input != 1 {
		t.Fatalf("low confidence should still be recorded: %v %d", err, input)
	}
	env.MustDo("DELETE", fmt.Sprintf("/ai/providers/%d", jev.Id), nil, nil)
	settings = api.AiModelSettings{}
	env.MustDo("GET", "/ai/model-settings", nil, &settings)
	if settings.MailTriage != nil || settings.Agent == nil || settings.Agent.Model != "tools" {
		t.Fatalf("delete clears only jev: %+v", settings)
	}
	if sorter.Available(context.Background()) {
		t.Fatal("sorter should be off after delete")
	}
}
