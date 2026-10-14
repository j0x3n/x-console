package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// ---------- helpers ----------

// responsesServer is a provider with /models and /responses. Every request body
// of /responses is kept.
type responsesServer struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []map[string]any
	reply  func(body map[string]any) (int, any)
}

func newResponsesServer(t *testing.T) *responsesServer {
	t.Helper()
	s := &responsesServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
		case "/v1/responses":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.mu.Lock()
			s.bodies = append(s.bodies, body)
			reply := s.reply
			s.mu.Unlock()
			status, out := http.StatusOK, any(map[string]any{"status": "completed", "output": []any{}})
			if reply != nil {
				status, out = reply(body)
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(out)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *responsesServer) lastBody() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bodies[len(s.bodies)-1]
}

// useProvider adds the server as a provider with the style and makes its model the main model.
func useProvider(t *testing.T, env *testutil.Env, s *responsesServer, style string) {
	t.Helper()
	env.Elevate()
	var p api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": style, "baseUrl": s.URL + "/v1", "apiKey": "provider-key", "apiStyle": style}, &p)
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", p.Id), nil, nil)
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": p.Id, "model": "test-model"}, "reasoningEffort": "off"}, nil)
}

func webSearchOf(t *testing.T, env *testutil.Env) contracts.WebSearch {
	t.Helper()
	ws, ok := module.Lookup[contracts.WebSearch](env.App.Deps.Registry, contracts.WebSearchKey)
	if !ok {
		t.Fatal("WebSearch not provided")
	}
	return ws
}

func setMode(t *testing.T, env *testutil.Env, mode string) {
	t.Helper()
	if err := env.App.Deps.Settings.Set(context.Background(), "ai.web_search_mode", mode); err != nil {
		t.Fatal(err)
	}
}

func setBraveKey(t *testing.T, env *testutil.Env, key string) {
	t.Helper()
	if err := env.App.Deps.Settings.SetSecret(context.Background(), "ai.brave_key", key); err != nil {
		t.Fatal(err)
	}
}

// fakeBrave answers like the Brave Search API. results maps a search term to its results.
type fakeBrave struct {
	*httptest.Server
	mu      sync.Mutex
	results map[string][]map[string]string
	status  int
	queries []url.Values
	tokens  []string
}

func newFakeBrave(t *testing.T) *fakeBrave {
	t.Helper()
	b := &fakeBrave{results: map[string][]map[string]string{}}
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.queries = append(b.queries, r.URL.Query())
		b.tokens = append(b.tokens, r.Header.Get("X-Subscription-Token"))
		if b.status != 0 {
			http.Error(w, "no", b.status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"web": map[string]any{"results": b.results[r.URL.Query().Get("q")]}})
	}))
	t.Cleanup(b.Close)
	restore := ai.SetBraveURLForTest(b.URL + "/res/v1/web/search")
	t.Cleanup(restore)
	return b
}

func result(title, url, desc string) map[string]string {
	return map[string]string{"title": title, "url": url, "description": desc}
}

type fakeReader struct {
	mu    sync.Mutex
	pages map[string]contracts.WebPage
	read  []string
}

func (f *fakeReader) Read(_ context.Context, url string) (contracts.WebPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.read = append(f.read, url)
	if p, ok := f.pages[url]; ok {
		return p, nil
	}
	return contracts.WebPage{}, errors.New("打不开")
}

var dishSchema = json.RawMessage(`{"type":"object","properties":{"dishes":{"type":"array","items":{"type":"object"}}},"required":["dishes"]}`)

type dishes struct {
	Dishes []struct {
		Title string `json:"title"`
	} `json:"dishes"`
}

// ---------- settings ----------

func TestAiWebSearchSettings(t *testing.T) {
	env := testutil.New(t)
	var got api.AiWebSearch
	env.MustDo("GET", "/ai/web-search", nil, &got)
	if got.Mode != "off" || got.BraveKeySet {
		t.Fatalf("default: %+v", got)
	}
	if status, _ := env.Do("PUT", "/ai/web-search", map[string]any{"mode": "off"}, nil); status != 403 {
		t.Fatalf("without elevation: %d", status)
	}
	if status, _ := env.Do("POST", "/ai/web-search/test", nil, nil); status == 403 {
		t.Fatal("the test button must not need elevation")
	}
	env.Elevate()

	fail := func(body map[string]any, want string) {
		t.Helper()
		status, raw := env.Do("PUT", "/ai/web-search", body, nil)
		if status != 400 || !strings.Contains(string(raw), want) {
			t.Fatalf("PUT %v: %d %s, want 400 with %q", body, status, raw, want)
		}
	}
	fail(map[string]any{"mode": "google"}, "联网搜索的方式")
	fail(map[string]any{"mode": "brave"}, "Brave 的密钥")
	// no main model yet
	fail(map[string]any{"mode": "openai"}, "Responses 接口")

	// the main model's provider uses the chat style
	remote := newResponsesServer(t)
	useProvider(t, env, remote, "chat")
	fail(map[string]any{"mode": "openai"}, "OpenAI 自带的联网搜索要用 Responses 接口的服务商")
	env.MustDo("GET", "/ai/web-search", nil, &got)
	if got.Mode != "off" {
		t.Fatalf("a refused save changed the mode: %+v", got)
	}

	// with a Responses provider it works
	useProvider(t, env, remote, "responses")
	env.MustDo("PUT", "/ai/web-search", map[string]any{"mode": "openai"}, &got)
	if got.Mode != "openai" || got.BraveKeySet {
		t.Fatalf("openai: %+v", got)
	}

	// the key is stored encrypted and never given back
	status, raw := env.Do("PUT", "/ai/web-search", map[string]any{"braveKey": " brave-secret-123 "}, &got)
	if status != 200 || !got.BraveKeySet || got.Mode != "openai" || strings.Contains(string(raw), "brave-secret") {
		t.Fatalf("key: %d %s", status, raw)
	}
	_, raw = env.Do("GET", "/ai/web-search", nil, nil)
	if strings.Contains(string(raw), "brave-secret") || !strings.Contains(string(raw), `"braveKeySet":true`) {
		t.Fatalf("get: %s", raw)
	}
	var stored string
	var encrypted int
	if err := env.App.Deps.DB.QueryRow(`SELECT value, encrypted FROM settings WHERE key = 'ai.brave_key'`).Scan(&stored, &encrypted); err != nil ||
		encrypted != 1 || strings.Contains(stored, "brave-secret") {
		t.Fatalf("stored key: %q %d %v", stored, encrypted, err)
	}
	var key string
	if err := env.App.Deps.Settings.Get(context.Background(), "ai.brave_key", &key); err != nil || key != "brave-secret-123" {
		t.Fatalf("key after trim: %q %v", key, err)
	}

	// brave works now that there is a key; the key cannot be removed while brave is on
	env.MustDo("PUT", "/ai/web-search", map[string]any{"mode": "brave"}, &got)
	if got.Mode != "brave" {
		t.Fatalf("brave: %+v", got)
	}
	fail(map[string]any{"braveKey": ""}, "Brave 的密钥")
	env.MustDo("GET", "/ai/web-search", nil, &got)
	if !got.BraveKeySet {
		t.Fatal("a refused save removed the key")
	}
	env.MustDo("PUT", "/ai/web-search", map[string]any{"mode": "off", "braveKey": ""}, &got)
	if got.Mode != "off" || got.BraveKeySet {
		t.Fatalf("off and no key: %+v", got)
	}
	// mode and key in one request
	env.MustDo("PUT", "/ai/web-search", map[string]any{"mode": "brave", "braveKey": "k2"}, &got)
	if got.Mode != "brave" || !got.BraveKeySet {
		t.Fatalf("both: %+v", got)
	}

	// the audit log does not hold the key
	var n int
	if err := env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'ai.web_search.update'`).Scan(&n); err != nil || n == 0 {
		t.Fatalf("audit: %d %v", n, err)
	}
	if err := env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE detail LIKE '%brave-secret%' OR detail LIKE '%k2%'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("the key is in the audit log: %d %v", n, err)
	}
}

func TestAiWebSearchTestEndpoint(t *testing.T) {
	env := testutil.New(t)
	env.Elevate()
	if status, raw := env.Do("POST", "/ai/web-search/test", nil, nil); status != 501 || !strings.Contains(string(raw), "还没有打开联网搜索") {
		t.Fatalf("off: %d %s", status, raw)
	}
	brave := newFakeBrave(t)
	brave.results["西红柿炒鸡蛋 做法"] = []map[string]string{
		result("做法一", "https://a.example/1", "…"), result("做法二", "https://b.example/2", "…"),
		result("做法三", "https://c.example/3", "…"), result("做法四", "https://d.example/4", "…"),
		result("坏地址", "file:///etc/passwd", "…"),
	}
	env.MustDo("PUT", "/ai/web-search", map[string]any{"mode": "brave", "braveKey": "key-test"}, nil)

	var out struct {
		Sources []api.AiWebSource `json:"sources"`
	}
	env.MustDo("POST", "/ai/web-search/test", nil, &out)
	if len(out.Sources) != 3 || out.Sources[0].Title != "做法一" || out.Sources[2].Url != "https://c.example/3" {
		t.Fatalf("sources: %+v", out.Sources)
	}
	brave.mu.Lock()
	token, count, q := brave.tokens[0], brave.queries[0].Get("count"), brave.queries[0].Get("q")
	brave.mu.Unlock()
	if token != "key-test" || count != "3" || q != "西红柿炒鸡蛋 做法" {
		t.Fatalf("brave request: %q %q %q", token, count, q)
	}

	// Brave refuses the key
	brave.mu.Lock()
	brave.status = http.StatusUnauthorized
	brave.mu.Unlock()
	if status, raw := env.Do("POST", "/ai/web-search/test", nil, nil); status != 502 || !strings.Contains(string(raw), "Brave 的密钥不对或者过期了") {
		t.Fatalf("401: %d %s", status, raw)
	}
	brave.mu.Lock()
	brave.status = http.StatusTooManyRequests
	brave.mu.Unlock()
	if status, raw := env.Do("POST", "/ai/web-search/test", nil, nil); status != 502 || !strings.Contains(string(raw), "Brave 搜索次数用完了") {
		t.Fatalf("429: %d %s", status, raw)
	}
	// nothing found
	brave.mu.Lock()
	brave.status = 0
	delete(brave.results, "西红柿炒鸡蛋 做法")
	brave.mu.Unlock()
	if status, raw := env.Do("POST", "/ai/web-search/test", nil, nil); status != 502 || !strings.Contains(string(raw), "没有找到结果") {
		t.Fatalf("empty: %d %s", status, raw)
	}
}

func TestAiWebSearchTestEndpointOpenAI(t *testing.T) {
	env := testutil.New(t)
	remote := newResponsesServer(t)
	useProvider(t, env, remote, "responses")
	env.MustDo("PUT", "/ai/web-search", map[string]any{"mode": "openai"}, nil)
	remote.reply = func(map[string]any) (int, any) {
		cite := func(u, title string) map[string]any {
			return map[string]any{"type": "url_citation", "url": u, "title": title}
		}
		return 200, map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{
			map[string]any{"type": "output_text", "text": `{"summary":"先炒蛋再炒番茄"}`, "annotations": []any{
				cite("https://a.example/1", "一"), cite("https://b.example/2", "二"), cite("https://c.example/3", "三"), cite("https://d.example/4", "四")}}}}}}
	}
	var out struct {
		Sources []api.AiWebSource `json:"sources"`
	}
	env.MustDo("POST", "/ai/web-search/test", nil, &out)
	if len(out.Sources) != 3 || out.Sources[0].Url != "https://a.example/1" {
		t.Fatalf("sources: %+v", out.Sources)
	}
}

// ---------- Research: openai ----------

func TestWebSearchResearchOpenAI(t *testing.T) {
	env := testutil.New(t)
	remote := newResponsesServer(t)
	useProvider(t, env, remote, "responses")
	env.MustDo("PUT", "/ai/web-search", map[string]any{"mode": "openai"}, nil)
	remote.reply = func(map[string]any) (int, any) {
		cite := func(u, title string) map[string]any {
			return map[string]any{"type": "url_citation", "url": u, "title": title}
		}
		return 200, map[string]any{"status": "completed", "output": []any{
			map[string]any{"type": "web_search_call", "id": "ws_1", "status": "completed"},
			map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": `{"dishes":[{"title":"凉拌黄瓜"}]}`,
				"annotations": []any{cite("https://a.example/1", "黄瓜"), cite("https://a.example/1", "黄瓜"), cite("https://b.example/2", "拍黄瓜")}}}},
		}, "usage": map[string]any{"input_tokens": 10, "output_tokens": 20}}
	}
	ws := webSearchOf(t, env)
	if ws.Mode(t.Context()) != "openai" {
		t.Fatalf("mode: %s", ws.Mode(t.Context()))
	}
	ctx := contracts.WithAIUsage(t.Context(), "recipes", "")
	var out dishes
	sources, err := ws.Research(ctx, contracts.WebResearch{Purpose: "recipes.discover", System: "找菜谱", Task: "找清淡的凉菜", Schema: dishSchema}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Dishes) != 1 || out.Dishes[0].Title != "凉拌黄瓜" {
		t.Fatalf("out: %+v", out)
	}
	if len(sources) != 2 || sources[0] != (contracts.WebSource{Title: "黄瓜", URL: "https://a.example/1"}) || sources[1].URL != "https://b.example/2" {
		t.Fatalf("sources: %+v", sources)
	}
	body := remote.lastBody()
	tools, _ := body["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["type"] != "web_search" {
		t.Fatalf("tools: %+v", body["tools"])
	}
	text, _ := body["text"].(map[string]any)
	if text == nil || text["format"].(map[string]any)["type"] != "json_schema" {
		t.Fatalf("schema: %+v", body["text"])
	}
	if instructions, _ := body["instructions"].(string); !strings.Contains(instructions, "找菜谱") {
		t.Fatalf("instructions: %q", instructions)
	}
	raw, _ := json.Marshal(body["input"])
	if !strings.Contains(string(raw), "找清淡的凉菜") {
		t.Fatalf("input: %s", raw)
	}

	// the call is in the usage records: the main model, the caller's source, the purpose as reference
	var purpose, source, ref string
	var input int
	if err := env.App.Deps.DB.QueryRow(`SELECT purpose, source, ref, input_tokens FROM ai_usage ORDER BY id DESC LIMIT 1`).Scan(&purpose, &source, &ref, &input); err != nil ||
		purpose != "agent" || source != "recipes" || ref != "recipes.discover" || input != 10 {
		t.Fatalf("usage: %q %q %q %d %v", purpose, source, ref, input, err)
	}
	// without a caller source, the usage says web_search
	if _, err := ws.Research(t.Context(), contracts.WebResearch{Purpose: "x.y", Task: "t", Schema: dishSchema}, &out); err != nil {
		t.Fatal(err)
	}
	if err := env.App.Deps.DB.QueryRow(`SELECT source, ref FROM ai_usage ORDER BY id DESC LIMIT 1`).Scan(&source, &ref); err != nil || source != "web_search" || ref != "x.y" {
		t.Fatalf("usage: %q %q %v", source, ref, err)
	}

	// a provider that refuses the schema together with the search: one retry without it
	remote.reply = func(body map[string]any) (int, any) {
		if body["text"] != nil {
			return 400, map[string]any{"error": map[string]any{"message": "Web Search cannot be used with structured output"}}
		}
		return 200, map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{
			map[string]any{"type": "output_text", "text": "好的：\n{\"dishes\":[{\"title\":\"皮蛋豆腐\"}]}"}}}}}
	}
	out = dishes{}
	if _, err := ws.Research(t.Context(), contracts.WebResearch{Task: "t", Schema: dishSchema}, &out); err != nil || len(out.Dishes) != 1 || out.Dishes[0].Title != "皮蛋豆腐" {
		t.Fatalf("fallback: %+v %v", out, err)
	}
	if body := remote.lastBody(); body["text"] != nil || body["tools"] == nil || !strings.Contains(body["instructions"].(string), "只输出 JSON") {
		t.Fatalf("retry body: %+v", body)
	}

	// an answer that is not JSON
	remote.reply = func(map[string]any) (int, any) {
		return 200, map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{
			map[string]any{"type": "output_text", "text": "不是 JSON"}}}}}
	}
	if _, err := ws.Research(t.Context(), contracts.WebResearch{Task: "t", Schema: dishSchema}, &out); err == nil || !strings.Contains(err.Error(), "不是正确的 JSON") {
		t.Fatalf("bad answer: %v", err)
	}
}

func TestWebSearchResearchOpenAIChatStyle(t *testing.T) {
	env := testutil.New(t)
	remote := newResponsesServer(t)
	useProvider(t, env, remote, "chat")
	setMode(t, env, "openai") // set around the check of the save
	var out dishes
	_, err := webSearchOf(t, env).Research(t.Context(), contracts.WebResearch{Task: "t", Schema: dishSchema}, &out)
	if err == nil || !strings.Contains(err.Error(), "Responses 接口") {
		t.Fatalf("chat provider: %v", err)
	}
	if len(remote.bodies) != 0 {
		t.Fatal("a request was sent to a chat provider")
	}
}

func TestWebSearchResearchOff(t *testing.T) {
	env := testutil.New(t)
	ws := webSearchOf(t, env)
	var out dishes
	if _, err := ws.Research(t.Context(), contracts.WebResearch{Task: "t", Schema: dishSchema}, &out); !errors.Is(err, contracts.ErrWebSearchOff) {
		t.Fatalf("off: %v", err)
	}
	if ws.Mode(t.Context()) != "off" {
		t.Fatalf("mode: %s", ws.Mode(t.Context()))
	}
	if _, err := ws.Research(t.Context(), contracts.WebResearch{Schema: dishSchema}, &out); err == nil {
		t.Fatal("a request without a task was accepted")
	}
}

// ---------- Research: brave ----------

func braveEnv(t *testing.T) (*testutil.Env, *fakeBrave, *fakeReader, *llm.Fake) {
	t.Helper()
	env := testutil.New(t)
	brave := newFakeBrave(t)
	setMode(t, env, "brave")
	setBraveKey(t, env, "brave-key-1")
	reader := &fakeReader{pages: map[string]contracts.WebPage{}}
	module.Provide[contracts.PageReader](env.App.Deps.Registry, contracts.PageReaderKey, reader)
	fake := llm.NewFake()
	svc, _ := module.Lookup[contracts.LLM](env.App.Deps.Registry, contracts.LLMKey)
	svc.(*ai.Module).SetLLMForTest(fake)
	return env, brave, reader, fake
}

func TestWebSearchResearchBrave(t *testing.T) {
	env, brave, reader, fake := braveEnv(t)
	brave.results["土豆 做法"] = []map[string]string{
		result("土豆一", "https://a.example/1", "<strong>土豆</strong> 做法一 &amp; 技巧"), result("土豆二", "https://b.example/2", "摘要二"), result("土豆三", "https://c.example/3", "摘要三")}
	brave.results["土豆 家常 菜谱"] = []map[string]string{
		result("土豆二（重复）", "https://B.example/2/", "摘要二"), result("土豆四", "https://d.example/4", "摘要四"), result("土豆五", "https://e.example/5", "摘要五")}
	reader.pages["https://a.example/1"] = contracts.WebPage{Title: "土豆一 - 示例站", Text: strings.Repeat("甲", 6000) + "乙乙乙"}
	reader.pages["https://c.example/3"] = contracts.WebPage{Title: "", Text: "土豆三的正文"}
	// b.example/2 cannot be read
	fake.Queue(llm.Result{Text: `{"queries":["土豆 做法"," 土豆 家常 菜谱","土豆 做法","","土豆 三","土豆 四"]}`}, nil)
	fake.Queue(llm.Result{Text: `{"dishes":[{"title":"炒土豆丝"}]}`}, nil)

	ctx := contracts.WithAIUsage(t.Context(), "recipes", "")
	var out dishes
	sources, err := webSearchOf(t, env).Research(ctx, contracts.WebResearch{Purpose: "recipes.discover", System: "整理成菜谱", Task: "找土豆做的菜", Schema: dishSchema, MaxSources: 3}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Dishes) != 1 || out.Dishes[0].Title != "炒土豆丝" {
		t.Fatalf("out: %+v", out)
	}

	// step 1: the fast model writes the search terms; step 4: the main model answers
	if len(fake.Calls) != 2 || fake.Calls[0].Purpose != "fast" || fake.Calls[1].Purpose != "agent" {
		t.Fatalf("model calls: %+v", fake.Calls)
	}
	if !strings.Contains(fake.Calls[0].Messages[0].Content, "找土豆做的菜") || len(fake.Calls[0].JSONSchema) == 0 {
		t.Fatalf("terms request: %+v", fake.Calls[0])
	}
	// step 2: one Brave call per term, five results each, the key in the header
	brave.mu.Lock()
	if len(brave.queries) != 3 || brave.queries[0].Get("q") != "土豆 做法" || brave.queries[1].Get("q") != "土豆 家常 菜谱" || brave.queries[2].Get("q") != "土豆 三" ||
		brave.queries[0].Get("count") != "5" || brave.tokens[0] != "brave-key-1" {
		t.Fatalf("brave calls: %v %v", brave.queries, brave.tokens)
	}
	brave.mu.Unlock()
	// step 3: the first three different pages are read, the unreadable one is skipped
	reader.mu.Lock()
	readCount := len(reader.read)
	reader.mu.Unlock()
	if readCount != 3 {
		t.Fatalf("pages read: %v", reader.read)
	}
	if len(sources) != 2 || sources[0].URL != "https://a.example/1" || sources[0].Title != "土豆一 - 示例站" || sources[1].URL != "https://c.example/3" {
		t.Fatalf("sources are the pages that were read: %+v", sources)
	}
	// step 4 input: tidy snippets, the page text cut at 6000 characters, no page that failed
	req := fake.Calls[1]
	material := req.Messages[0].Content
	if !strings.Contains(material, "找土豆做的菜") || !strings.Contains(material, "土豆 做法一 & 技巧") || strings.Contains(material, "<strong>") ||
		!strings.Contains(material, strings.Repeat("甲", 6000)) || strings.Contains(material, "乙") ||
		!strings.Contains(material, "土豆三的正文") || strings.Contains(material, "b.example") || strings.Contains(material, "d.example") {
		t.Fatalf("material:\n%.600s", material)
	}
	if !strings.Contains(req.System, "整理成菜谱") || string(req.JSONSchema) != string(dishSchema) {
		t.Fatalf("main request: %q %s", req.System, req.JSONSchema)
	}
	// the usage keeps the caller's source for both calls
	if u := contracts.AIUsageFrom(ctx); u.Source != "recipes" {
		t.Fatal("context changed")
	}
}

func TestWebSearchResearchBraveErrors(t *testing.T) {
	env, brave, reader, fake := braveEnv(t)
	ws := webSearchOf(t, env)
	research := func() error {
		var out dishes
		_, err := ws.Research(t.Context(), contracts.WebResearch{Task: "找菜", Schema: dishSchema}, &out)
		return err
	}
	brave.results["找菜"] = []map[string]string{result("一", "https://a.example/1", "摘要")}

	for status, want := range map[int]string{401: "Brave 的密钥不对或者过期了", 403: "Brave 的密钥不对或者过期了", 429: "Brave 搜索次数用完了"} {
		brave.mu.Lock()
		brave.status = status
		brave.mu.Unlock()
		fake.Queue(llm.Result{Text: `{"queries":["找菜"]}`}, nil)
		if err := research(); err == nil || err.Error() != want {
			t.Fatalf("status %d: %v, want %q", status, err, want)
		}
	}
	// another status is an error with the number
	brave.mu.Lock()
	brave.status = 500
	brave.mu.Unlock()
	fake.Queue(llm.Result{Text: `{"queries":["找菜"]}`}, nil)
	if err := research(); err == nil || !strings.Contains(err.Error(), "状态码 500") {
		t.Fatalf("500: %v", err)
	}
	brave.mu.Lock()
	brave.status = 0
	brave.mu.Unlock()

	// no result at all
	fake.Queue(llm.Result{Text: `{"queries":["没有结果的词"]}`}, nil)
	if err := research(); err == nil || !strings.Contains(err.Error(), "没有找到结果") {
		t.Fatalf("empty: %v", err)
	}
	// the model gives no usable term: the task itself is searched
	fake.Queue(llm.Result{Text: `{"queries":[" "]}`}, nil)
	fake.Queue(llm.Result{Text: `{"dishes":[]}`}, nil)
	if err := research(); err != nil {
		t.Fatalf("fallback term: %v", err)
	}
	brave.mu.Lock()
	lastQ := brave.queries[len(brave.queries)-1].Get("q")
	brave.mu.Unlock()
	if lastQ != "找菜" {
		t.Fatalf("searched %q", lastQ)
	}
	// every page fails to read: the answer rests on the snippets and all results are sources
	if len(reader.read) == 0 {
		t.Fatal("no page was tried")
	}
	var out dishes
	fake.Queue(llm.Result{Text: `{"queries":["找菜"]}`}, nil)
	fake.Queue(llm.Result{Text: `{"dishes":[]}`}, nil)
	sources, err := ws.Research(t.Context(), contracts.WebResearch{Task: "找菜", Schema: dishSchema}, &out)
	if err != nil || len(sources) != 1 || sources[0].URL != "https://a.example/1" {
		t.Fatalf("snippets only: %+v %v", sources, err)
	}
	if material := fake.Calls[len(fake.Calls)-1].Messages[0].Content; !strings.Contains(material, "摘要：摘要") || strings.Contains(material, "正文：") {
		t.Fatalf("material:\n%s", material)
	}

	// no page reader at all (the read-later module is missing)
	module.Provide[any](env.App.Deps.Registry, contracts.PageReaderKey, nil)
	before := len(reader.read)
	fake.Queue(llm.Result{Text: `{"queries":["找菜"]}`}, nil)
	fake.Queue(llm.Result{Text: `{"dishes":[]}`}, nil)
	if sources, err := ws.Research(t.Context(), contracts.WebResearch{Task: "找菜", Schema: dishSchema}, &out); err != nil || len(sources) != 1 || len(reader.read) != before {
		t.Fatalf("without a reader: %+v %v", sources, err)
	}

	// the key is gone
	if err := env.App.Deps.Settings.Delete(t.Context(), "ai.brave_key"); err != nil {
		t.Fatal(err)
	}
	if err := research(); err == nil || err.Error() != "Brave 的密钥不对或者过期了" {
		t.Fatalf("no key: %v", err)
	}
}
