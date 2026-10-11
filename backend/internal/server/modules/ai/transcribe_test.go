package ai_test

import (
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestTranscribe(t *testing.T) {
	var gotModel, gotLang, gotFile, gotAuth string
	var gotBytes int
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
		case "/v1/audio/transcriptions":
			_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			mr := multipart.NewReader(r.Body, params["boundary"])
			for {
				part, err := mr.NextPart()
				if err != nil {
					break
				}
				data, _ := io.ReadAll(part)
				switch part.FormName() {
				case "model":
					gotModel = string(data)
				case "language":
					gotLang = string(data)
				case "file":
					gotFile, gotBytes = part.FileName(), len(data)
				}
			}
			gotAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"text":" 明天下午三点开会 "}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	env := testutil.New(t)
	env.Elevate()
	tr, ok := module.Lookup[contracts.Transcriber](env.App.Deps.Registry, contracts.TranscriberKey)
	if !ok {
		t.Fatal("transcriber not registered")
	}
	ctx := context.Background()
	// No provider yet.
	if _, err := tr.Transcribe(ctx, contracts.Transcribe{Audio: []byte("x")}); err == nil || !strings.Contains(err.Error(), "供应商") {
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

	text, err := tr.Transcribe(ctx, contracts.Transcribe{Audio: []byte("fake-audio"), Filename: "voice.webm", Mime: "audio/webm", Language: "zh"})
	if err != nil || text != "明天下午三点开会" {
		t.Fatalf("transcribe: %q %v", text, err)
	}
	if gotModel != "whisper-1" || gotLang != "zh" || gotFile != "voice.webm" || gotBytes != len("fake-audio") || gotAuth != "Bearer secret-value" {
		t.Errorf("request: model %q lang %q file %q bytes %d auth %q", gotModel, gotLang, gotFile, gotBytes, gotAuth)
	}
	if _, err := tr.Transcribe(ctx, contracts.Transcribe{Audio: []byte("x"), Model: "gpt-4o-mini-transcribe"}); err != nil || gotModel != "gpt-4o-mini-transcribe" {
		t.Errorf("model override: %v %q", err, gotModel)
	}
	if _, err := tr.Transcribe(ctx, contracts.Transcribe{}); err == nil {
		t.Error("empty audio should fail")
	}
}
