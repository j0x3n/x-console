package ai_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
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

func uploadAttachment(t *testing.T, env *testutil.Env, name string, data []byte) (int, api.AiAttachment) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, _ := form.CreateFormFile("file", name)
	_, _ = part.Write(data)
	_ = form.Close()
	req, _ := http.NewRequest(http.MethodPost, env.URL("/ai/attachments"), &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("X-Requested-With", "x-console")
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out api.AiAttachment
	if resp.StatusCode == http.StatusCreated {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)

func TestAttachmentsReachTheModel(t *testing.T) {
	var mu sync.Mutex
	var sent []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		sent = append(sent, string(raw))
		mu.Unlock()
		streamText(w, "看到了")
	}))
	defer fake.Close()
	env := testutil.New(t)
	env.Elevate()
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "Test", "baseUrl": fake.URL + "/v1", "apiKey": "k"}, &provider)
	if provider.ApiStyle != api.Chat {
		t.Fatalf("default api style: %q", provider.ApiStyle)
	}
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}}, nil)

	status, image := uploadAttachment(t, env, "截图.png", pngBytes)
	if status != http.StatusCreated || image.Kind != api.Image || image.Mime != "image/png" {
		t.Fatalf("image upload: %d %+v", status, image)
	}
	status, text := uploadAttachment(t, env, "main.go", []byte("package main\n\nfunc main() {}\n"))
	if status != http.StatusCreated || text.Kind != api.Text {
		t.Fatalf("text upload: %d %+v", status, text)
	}
	if status, _ := uploadAttachment(t, env, "a.bin", []byte{0, 1, 2, 3, 0xff, 0xfe}); status != http.StatusUnsupportedMediaType {
		t.Fatalf("binary upload: %d", status)
	}
	var conversation api.Conversation
	env.MustDo("POST", "/ai/conversations", map[string]any{}, &conversation)
	env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/messages", conversation.Id), map[string]any{"text": "看看", "attachmentIds": []int64{image.Id, text.Id}}, nil)
	detail := await(t, env, conversation.Id, func(d api.ConversationDetail) bool { return !d.Running && len(d.Messages) >= 2 })
	mu.Lock()
	body := sent[len(sent)-1]
	mu.Unlock()
	if !strings.Contains(body, "data:image/png;base64,") || !strings.Contains(body, "func main()") || !strings.Contains(body, "main.go") {
		t.Fatalf("attachments not sent to the model: %s", body)
	}
	raw, _ := json.Marshal(detail.Messages[0])
	if !strings.Contains(string(raw), `"type":"image"`) || !strings.Contains(string(raw), `"type":"file"`) {
		t.Fatalf("attachment blocks not saved: %s", raw)
	}
	// Already used in this conversation: not usable in another one.
	var other api.Conversation
	env.MustDo("POST", "/ai/conversations", map[string]any{}, &other)
	if status, _ := env.Do("POST", fmt.Sprintf("/ai/conversations/%d/messages", other.Id), map[string]any{"text": "再看", "attachmentIds": []int64{image.Id}}, nil); status != http.StatusBadRequest {
		t.Fatalf("attachment reused: %d", status)
	}
	if status, _ := env.Do("GET", fmt.Sprintf("/ai/attachments/%d", image.Id), nil, nil); status != http.StatusOK {
		t.Fatalf("download: %d", status)
	}
	env.MustDo("DELETE", fmt.Sprintf("/ai/conversations/%d", conversation.Id), nil, nil)
	if status, _ := env.Do("GET", fmt.Sprintf("/ai/attachments/%d", image.Id), nil, nil); status != http.StatusNotFound {
		t.Fatalf("attachment after conversation delete: %d", status)
	}
}

func TestResponsesStyleAndFastReasoning(t *testing.T) {
	var mu sync.Mutex
	paths := map[string]map[string]any{}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		paths[r.URL.Path] = body
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"title\":\"周报\",\"tags\":[]}"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer fake.Close()
	env := testutil.New(t)
	env.Elevate()
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "R", "baseUrl": fake.URL + "/v1", "apiStyle": "responses"}, &provider)
	if provider.ApiStyle != api.Responses {
		t.Fatalf("api style: %q", provider.ApiStyle)
	}
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	ref := map[string]any{"providerId": provider.Id, "model": "test-model"}
	var settings api.AiModelSettings
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": ref, "fast": ref, "reasoningEffort": "off", "fastReasoningEffort": "medium"}, &settings)
	if settings.FastReasoningEffort != api.Medium || settings.ReasoningEffort != api.Off {
		t.Fatalf("settings: %+v", settings)
	}
	var out struct{ Title string }
	client, _ := module.Lookup[contracts.LLM](env.App.Deps.Registry, contracts.LLMKey)
	if err := client.CompleteJSON(t.Context(), "fast", "起标题", "本周完成了很多事情", json.RawMessage(`{"type":"object"}`), &out); err != nil || out.Title != "周报" {
		t.Fatalf("fast call: %+v %v", out, err)
	}
	mu.Lock()
	defer mu.Unlock()
	body := paths["/v1/responses"]
	if body == nil || body["reasoning"] == nil || body["reasoning"].(map[string]any)["effort"] != "medium" {
		t.Fatalf("fast call went to %v with %+v", paths, body)
	}
}
