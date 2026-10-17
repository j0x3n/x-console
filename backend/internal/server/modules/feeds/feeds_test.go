package feeds_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/netproxy"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// B241: the page reads a small service on a VM through an agent's http.proxy.

const goodToken = "feeds-token-SECRET-1234567890"

// fakeFeeds stands in for deploy/feeds-api. It records what it was asked.
type fakeFeeds struct {
	*httptest.Server
	mu     sync.Mutex
	reqs   []seen
	status map[string]int    // path -> forced status
	body   map[string]string // path -> forced raw body
}

type seen struct {
	Path  string
	Query url.Values
	Auth  string
}

func newFakeFeeds(t *testing.T) *fakeFeeds {
	f := &fakeFeeds{status: map[string]int{}, body: map[string]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeFeeds) requests() []seen {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seen(nil), f.reqs...)
}

func (f *fakeFeeds) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = nil
}

func (f *fakeFeeds) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.reqs = append(f.reqs, seen{Path: r.URL.Path, Query: r.URL.Query(), Auth: r.Header.Get("Authorization")})
	forcedStatus, forcedBody := f.status[r.URL.Path], f.body[r.URL.Path]
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Header.Get("Authorization") != "Bearer "+goodToken {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		return
	}
	if forcedStatus != 0 {
		w.WriteHeader(forcedStatus)
	}
	if forcedBody != "" {
		_, _ = w.Write([]byte(forcedBody))
		return
	}
	switch {
	case r.URL.Path == "/v1/health":
		_, _ = w.Write([]byte(`{"news":{"count":150,"lastFetchedAt":"2026-10-17T08:00:00+00:00"},"tweets":{"count":2,"lastAt":"2026-10-17T07:00:00.000Z"}}`))
	case r.URL.Path == "/v1/news":
		page(w, r, 150, func(id int) any {
			return map[string]any{
				"id": id, "title": fmt.Sprintf("title %d", id), "titleCn": fmt.Sprintf("标题 %d", id),
				"url": "https://example.com/" + strconv.Itoa(id), "source": "src", "category": "ai",
				"tags": []string{"llm"}, "importance": 4, "pushed": id%2 == 0,
				"publishedAt": "2026-10-17T00:00:00+00:00", "fetchedAt": "2026-10-17T00:00:00+00:00",
			}
		})
	case r.URL.Path == "/v1/news/facets":
		_, _ = w.Write([]byte(`{"categories":["ai","dev"],"tags":[{"tag":"llm","count":9}]}`))
	case strings.HasPrefix(r.URL.Path, "/v1/news/"):
		id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/v1/news/"))
		if err != nil || id < 1 || id > 150 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":%d,"title":"t","url":"https://example.com/x","source":"s","category":"ai","tags":["a"],"pushed":true,`+
			`"content":{"status":"ok","markdown":"# 正文","markdownCn":"# 译文","truncated":true},`+
			`"media":[{"url":"https://img.example.com/a.png","type":"image","width":10,"height":20}]}`, id)
	case r.URL.Path == "/v1/tweets":
		page(w, r, 120, func(id int) any {
			return map[string]any{"id": strconv.Itoa(1800000000000000000 + id), "author": "A @a", "time": "2026-10-17T07:00:00.000Z",
				"url": "https://x.com/i/web/status/" + strconv.Itoa(id), "text": "line1\nline2", "collectedAt": "2026-10-17T07:01:00.000Z"}
		})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// page serves ids total..1, newest first.
func page(w http.ResponseWriter, r *http.Request, total int, item func(int) any) {
	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		limit = v
	}
	start := total
	if b := r.URL.Query().Get("before"); b != "" {
		n, _ := strconv.Atoi(b)
		if n > 1800000000000000000 {
			n -= 1800000000000000000
		}
		start = n - 1
	}
	items := []any{}
	next := ""
	for id := start; id >= 1; id-- {
		if len(items) == limit {
			if _, ok := items[len(items)-1].(map[string]any)["id"].(int); ok {
				next = strconv.Itoa(id + 1)
			} else {
				next = items[len(items)-1].(map[string]any)["id"].(string)
			}
			break
		}
		items = append(items, item(id))
	}
	out := map[string]any{"items": items}
	if next != "" {
		out["next"] = next
	}
	_ = json.NewEncoder(w).Encode(out)
}

func proxyAgent(env *testutil.Env) string {
	return env.Agent("server", []string{protocol.CapProxy}, func(c *conn.Client) {
		c.Handle(protocol.MethodHTTPProxy, netproxy.HTTP)
	})
}

// pairedOffline pairs an agent that never connects.
func pairedOffline(env *testutil.Env, caps []string) string {
	env.T.Helper()
	env.Elevate()
	var pc struct{ Code string }
	env.MustDo(http.MethodPost, "/agents/pairing-codes", map[string]string{"name": "vm-1", "kind": "server"}, &pc)
	id, _, err := conn.Pair(context.Background(), env.Server.URL, pc.Code,
		protocol.Hello{AgentVersion: "test", OS: "linux", Arch: "amd64", Hostname: "vm", Capabilities: caps})
	if err != nil {
		env.T.Fatal(err)
	}
	return id
}

func errCode(raw []byte) string {
	var e struct{ Code string }
	_ = json.Unmarshal(raw, &e)
	return e.Code
}

type fixture struct {
	env     *testutil.Env
	feeds   *fakeFeeds
	agentID string
	bodies  []string // every response body, to prove the token never shows up
}

// do wraps env.Do and keeps the body for the token check.
func (f *fixture) do(method, path string, body, out any) (int, []byte) {
	f.env.T.Helper()
	code, raw := f.env.Do(method, path, body, out)
	f.bodies = append(f.bodies, string(raw))
	return code, raw
}

func (f *fixture) noToken(t *testing.T) {
	t.Helper()
	for _, b := range f.bodies {
		if strings.Contains(b, goodToken) || strings.Contains(b, "SECRET") {
			t.Fatalf("token leaked in a response: %s", b)
		}
	}
}

func setup(t *testing.T) *fixture {
	t.Helper()
	env := testutil.New(t)
	f := &fixture{env: env, feeds: newFakeFeeds(t)}
	f.agentID = proxyAgent(env)
	env.Elevate()
	code, raw := f.do(http.MethodPut, "/feeds/settings", map[string]any{
		"agentId": f.agentID, "baseUrl": f.feeds.URL + "/", "token": goodToken}, nil)
	if code != http.StatusOK {
		t.Fatalf("put settings: %d %s", code, raw)
	}
	return f
}

func TestNotConfigured(t *testing.T) {
	env := testutil.New(t)
	var s struct {
		AgentID    string `json:"agentId"`
		BaseURL    string `json:"baseUrl"`
		HasToken   bool   `json:"hasToken"`
		Configured bool   `json:"configured"`
	}
	env.MustDo(http.MethodGet, "/feeds/settings", nil, &s)
	if s.AgentID != "" || s.BaseURL != "http://127.0.0.1:8790" || s.HasToken || s.Configured {
		t.Fatalf("empty settings: %+v", s)
	}
	for _, p := range []string{"/feeds/news", "/feeds/news/facets", "/feeds/news/1", "/feeds/tweets"} {
		code, raw := env.Do(http.MethodGet, p, nil, nil)
		if code != http.StatusConflict || errCode(raw) != "not_configured" {
			t.Fatalf("GET %s: %d %s", p, code, raw)
		}
	}
	code, raw := env.Do(http.MethodPost, "/feeds/settings/test", nil, nil)
	if code != http.StatusConflict || errCode(raw) != "not_configured" {
		t.Fatalf("test: %d %s", code, raw)
	}
}

func TestSettings(t *testing.T) {
	env := testutil.New(t)
	feeds := newFakeFeeds(t)

	// Changing the settings needs a fresh second factor. Pairing an agent
	// elevates, so this comes first.
	code, raw := env.Do(http.MethodPut, "/feeds/settings", map[string]any{"agentId": "x", "baseUrl": feeds.URL, "token": goodToken}, nil)
	if code != http.StatusForbidden || errCode(raw) != "elevation_required" {
		t.Fatalf("unelevated put: %d %s", code, raw)
	}
	agentID := proxyAgent(env)
	body := map[string]any{"agentId": agentID, "baseUrl": feeds.URL, "token": goodToken}
	env.Elevate()

	noProxy := pairedOffline(env, nil)
	for name, bad := range map[string]map[string]any{
		"ftp address":    {"agentId": agentID, "baseUrl": "ftp://x", "token": "t"},
		"no scheme":      {"agentId": agentID, "baseUrl": "192.168.1.2:8790", "token": "t"},
		"address query":  {"agentId": agentID, "baseUrl": "http://x/?a=1", "token": "t"},
		"address user":   {"agentId": agentID, "baseUrl": "http://u:p@x", "token": "t"},
		"unknown agent":  {"agentId": "nope", "baseUrl": feeds.URL, "token": "t"},
		"no proxy cap":   {"agentId": noProxy, "baseUrl": feeds.URL, "token": "t"},
		"no token yet":   {"agentId": agentID, "baseUrl": feeds.URL},
		"unknown fields": {"agentId": agentID, "baseUrl": feeds.URL, "token": "t", "extra": 1},
	} {
		if code, raw := env.Do(http.MethodPut, "/feeds/settings", bad, nil); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, code, raw)
		}
	}

	var saved struct {
		AgentID    string `json:"agentId"`
		BaseURL    string `json:"baseUrl"`
		HasToken   bool   `json:"hasToken"`
		Configured bool   `json:"configured"`
	}
	code, raw = env.Do(http.MethodPut, "/feeds/settings", body, &saved)
	if code != http.StatusOK || saved.AgentID != agentID || saved.BaseURL != feeds.URL || !saved.HasToken || !saved.Configured {
		t.Fatalf("put: %d %s", code, raw)
	}
	if strings.Contains(string(raw), "SECRET") {
		t.Fatalf("put echoed the token: %s", raw)
	}
	_, raw = env.Do(http.MethodGet, "/feeds/settings", nil, &saved)
	if !saved.HasToken || strings.Contains(string(raw), "SECRET") {
		t.Fatalf("get: %s", raw)
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if _, has := m["token"]; has {
		t.Fatalf("get has a token field: %s", raw)
	}

	// The token is stored encrypted.
	var enc string
	if err := env.App.Deps.DB.QueryRow(`SELECT token_enc FROM feeds_settings WHERE id = 1`).Scan(&enc); err != nil {
		t.Fatal(err)
	}
	if enc == "" || strings.Contains(enc, "SECRET") {
		t.Fatalf("token_enc: %q", enc)
	}

	// An empty token keeps the stored one. A new base URL has no trailing slash.
	env.MustDo(http.MethodPut, "/feeds/settings", map[string]any{"agentId": agentID, "baseUrl": feeds.URL + "/", "token": ""}, &saved)
	if saved.BaseURL != feeds.URL || !saved.HasToken {
		t.Fatalf("keep token: %+v", saved)
	}
	var health map[string]any
	env.MustDo(http.MethodPost, "/feeds/settings/test", nil, &health)

	// The audit log records the change without the token.
	var detail, target string
	if err := env.App.Deps.DB.QueryRow(`SELECT detail, target FROM audit_log WHERE action = 'feeds.settings' AND result = 'ok' ORDER BY id LIMIT 1`).Scan(&detail, &target); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if target != agentID || strings.Contains(detail, "SECRET") || !strings.Contains(detail, `"tokenChanged":true`) {
		t.Fatalf("audit: %s %s", target, detail)
	}

	// An empty machine clears everything.
	env.MustDo(http.MethodPut, "/feeds/settings", map[string]any{"agentId": ""}, &saved)
	if saved.Configured || saved.HasToken || saved.AgentID != "" {
		t.Fatalf("clear: %+v", saved)
	}
	if code, raw := env.Do(http.MethodGet, "/feeds/news", nil, nil); code != http.StatusConflict {
		t.Fatalf("after clear: %d %s", code, raw)
	}
}

type newsItem struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

func TestNewsPaging(t *testing.T) {
	f := setup(t)
	var page struct {
		Items []newsItem `json:"items"`
		Next  string     `json:"next"`
	}
	seenIDs := map[int64]bool{}
	pages := 0
	next := ""
	for {
		p := "/feeds/news"
		if next != "" {
			p += "?before=" + next
		}
		code, raw := f.do(http.MethodGet, p, nil, &page)
		if code != http.StatusOK {
			t.Fatalf("page %d: %d %s", pages, code, raw)
		}
		pages++
		for _, it := range page.Items {
			if seenIDs[it.ID] {
				t.Fatalf("duplicate id %d", it.ID)
			}
			seenIDs[it.ID] = true
		}
		if page.Next == "" {
			break
		}
		next = page.Next
		page.Next = ""
		if pages > 10 {
			t.Fatal("never ends")
		}
	}
	if pages != 3 || len(seenIDs) != 150 {
		t.Fatalf("pages %d, items %d", pages, len(seenIDs))
	}
	// The newest come first, and the last page ends without a cursor.
	_, raw := f.do(http.MethodGet, "/feeds/news?limit=2", nil, &page)
	if len(page.Items) != 2 || page.Items[0].ID != 150 || page.Next != "149" {
		t.Fatalf("limit=2: %s", raw)
	}
	for _, r := range f.feeds.requests() {
		if r.Auth != "Bearer "+goodToken {
			t.Fatalf("service got auth %q", r.Auth)
		}
	}
	f.noToken(t)
}

func TestNewsParamsAreWhitelisted(t *testing.T) {
	f := setup(t)
	f.feeds.reset()
	q := url.Values{}
	q.Set("q", "  gpt  agent ")
	q.Set("tag", "llm")
	q.Set("category", "ai")
	q.Set("days", "7")
	q.Set("minImportance", "4")
	q.Set("pushed", "true")
	q.Set("before", "100")
	q.Set("limit", "20")
	// None of these may reach the service.
	q.Set("evil", "1")
	q.Set("url", "http://169.254.169.254/")
	q.Set("path", "../../etc/passwd")
	q.Set("Authorization", "Bearer x")
	q.Set("sql", "1; DROP TABLE items")
	code, raw := f.do(http.MethodGet, "/feeds/news?"+q.Encode(), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, raw)
	}
	reqs := f.feeds.requests()
	if len(reqs) != 1 || reqs[0].Path != "/v1/news" {
		t.Fatalf("requests: %+v", reqs)
	}
	var keys []string
	for k := range reqs[0].Query {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := "before,category,days,limit,minImportance,pushed,q,tag"
	if strings.Join(keys, ",") != want {
		t.Fatalf("forwarded keys %v, want %s", keys, want)
	}
	if got := reqs[0].Query.Get("q"); got != "gpt  agent" {
		t.Fatalf("q: %q", got)
	}
	if reqs[0].Query.Get("pushed") != "true" || reqs[0].Query.Get("days") != "7" {
		t.Fatalf("query: %v", reqs[0].Query)
	}

	// Empty values are dropped, pushed=false is kept.
	f.feeds.reset()
	f.do(http.MethodGet, "/feeds/news?q=&tag=%20&pushed=false", nil, nil)
	reqs = f.feeds.requests()
	if len(reqs) != 1 || len(reqs[0].Query) != 1 || reqs[0].Query.Get("pushed") != "false" {
		t.Fatalf("empty values: %+v", reqs)
	}

	// Values out of range or in the wrong shape never get forwarded.
	f.feeds.reset()
	for _, bad := range []string{
		"days=0", "days=99999", "minImportance=9", "limit=0", "limit=101", "limit=x",
		"before=abc", "before=1%26evil%3D1", "before=-5", "pushed=maybe",
		"q=" + strings.Repeat("a", 201), "tag=" + strings.Repeat("a", 101),
	} {
		if code, raw := f.do(http.MethodGet, "/feeds/news?"+bad, nil, nil); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", bad, code, raw)
		}
	}
	if n := len(f.feeds.requests()); n != 0 {
		t.Fatalf("%d bad requests reached the service", n)
	}
}

func TestNewsDetailPathIsFixed(t *testing.T) {
	f := setup(t)
	f.feeds.reset()
	var d struct {
		ID      int64 `json:"id"`
		Content struct {
			Status     string `json:"status"`
			Markdown   string `json:"markdown"`
			MarkdownCn string `json:"markdownCn"`
			Truncated  bool   `json:"truncated"`
		} `json:"content"`
		Media []struct {
			URL    string `json:"url"`
			Type   string `json:"type"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"media"`
	}
	code, raw := f.do(http.MethodGet, "/feeds/news/42", nil, &d)
	if code != http.StatusOK || d.ID != 42 || d.Content.Status != "ok" || d.Content.MarkdownCn != "# 译文" ||
		!d.Content.Truncated || len(d.Media) != 1 || d.Media[0].Width != 10 {
		t.Fatalf("detail: %d %s", code, raw)
	}
	// Parameters are not forwarded on a detail request.
	f.do(http.MethodGet, "/feeds/news/43?evil=1&url=http://x", nil, nil)
	for _, r := range f.feeds.requests() {
		if len(r.Query) != 0 {
			t.Fatalf("detail forwarded %v", r.Query)
		}
	}
	// Anything that is not a plain number never turns into a request.
	f.feeds.reset()
	for _, p := range []string{
		"/feeds/news/abc", "/feeds/news/1%2F..%2F..%2Fv1%2Fhealth", "/feeds/news/..%2Fhealth", "/feeds/news/1.5",
		"/feeds/news/%2e%2e", "/feeds/news/0", "/feeds/news/-1", "/feeds/news/99999999999999999999",
	} {
		code, _ := f.do(http.MethodGet, p, nil, nil)
		if code != http.StatusBadRequest && code != http.StatusNotFound {
			t.Fatalf("%s: %d", p, code)
		}
	}
	for _, r := range f.feeds.requests() {
		if r.Path != "/v1/news/1" {
			t.Fatalf("unexpected request %s", r.Path)
		}
	}
	// A news item the service does not know is a plain 404.
	if code, raw := f.do(http.MethodGet, "/feeds/news/9999", nil, nil); code != http.StatusNotFound || errCode(raw) != "not_found" {
		t.Fatalf("missing: %d %s", code, raw)
	}
	f.noToken(t)
}

func TestFacetsAndHealth(t *testing.T) {
	f := setup(t)
	var fc struct {
		Categories []string `json:"categories"`
		Tags       []struct {
			Tag   string `json:"tag"`
			Count int    `json:"count"`
		} `json:"tags"`
	}
	f.feeds.reset()
	if code, raw := f.do(http.MethodGet, "/feeds/news/facets?x=1", nil, &fc); code != http.StatusOK || len(fc.Categories) != 2 || fc.Tags[0].Tag != "llm" || fc.Tags[0].Count != 9 {
		t.Fatalf("facets: %d %s", code, raw)
	}
	if r := f.feeds.requests(); len(r) != 1 || r[0].Path != "/v1/news/facets" || len(r[0].Query) != 0 {
		t.Fatalf("facets request: %+v", r)
	}
	var h struct {
		News struct {
			Count         int64  `json:"count"`
			LastFetchedAt string `json:"lastFetchedAt"`
		} `json:"news"`
		Tweets struct {
			Count  int64  `json:"count"`
			LastAt string `json:"lastAt"`
		} `json:"tweets"`
	}
	if code, raw := f.do(http.MethodPost, "/feeds/settings/test", nil, &h); code != http.StatusOK || h.News.Count != 150 || h.Tweets.Count != 2 ||
		h.News.LastFetchedAt == "" || h.Tweets.LastAt == "" {
		t.Fatalf("health: %d %s", code, raw)
	}
	f.noToken(t)
}

func TestTweets(t *testing.T) {
	f := setup(t)
	f.feeds.reset()
	var page struct {
		Items []struct {
			ID     string `json:"id"`
			Author string `json:"author"`
			Text   string `json:"text"`
		} `json:"items"`
		Next string `json:"next"`
	}
	code, raw := f.do(http.MethodGet, "/feeds/tweets?author=%20A&q=hi&limit=50&evil=1&days=3", nil, &page)
	if code != http.StatusOK || len(page.Items) != 50 || page.Next == "" || page.Items[0].Text != "line1\nline2" {
		t.Fatalf("tweets: %d %.200s", code, raw)
	}
	// Tweet ids are big numbers and stay strings all the way.
	if len(page.Items[0].ID) != 19 {
		t.Fatalf("id: %s", page.Items[0].ID)
	}
	r := f.feeds.requests()
	if len(r) != 1 || r[0].Path != "/v1/tweets" || r[0].Query.Get("author") != "A" || r[0].Query.Get("q") != "hi" ||
		r[0].Query.Get("evil") != "" || r[0].Query.Get("days") != "" || len(r[0].Query) != 3 {
		t.Fatalf("forwarded: %+v", r)
	}
	var second struct {
		Items []struct{ ID string } `json:"items"`
		Next  string                `json:"next"`
	}
	if code, raw := f.do(http.MethodGet, "/feeds/tweets?before="+page.Next+"&limit=100", nil, &second); code != http.StatusOK || len(second.Items) != 70 || second.Next != "" {
		t.Fatalf("second page: %d %.200s", code, raw)
	}
	for _, bad := range []string{"before=abc", "before=1/../x", "limit=500", "author=" + strings.Repeat("a", 101)} {
		if code, _ := f.do(http.MethodGet, "/feeds/tweets?"+bad, nil, nil); code != http.StatusBadRequest {
			t.Fatalf("%s: %d", bad, code)
		}
	}
	f.noToken(t)
}

func TestErrors(t *testing.T) {
	f := setup(t)
	check := func(name string, wantStatus int, wantCode string) {
		t.Helper()
		for _, p := range []string{"/feeds/news", "/feeds/tweets", "/feeds/news/facets", "/feeds/news/1"} {
			code, raw := f.do(http.MethodGet, p, nil, nil)
			if code != wantStatus || errCode(raw) != wantCode {
				t.Fatalf("%s %s: %d %s", name, p, code, raw)
			}
		}
		code, raw := f.do(http.MethodPost, "/feeds/settings/test", nil, nil)
		if code != wantStatus || errCode(raw) != wantCode {
			t.Fatalf("%s test: %d %s", name, code, raw)
		}
	}

	// The service answers 500 or something that is not JSON.
	for _, p := range []string{"/v1/news", "/v1/tweets", "/v1/news/facets", "/v1/news/1", "/v1/health"} {
		f.feeds.status[p] = http.StatusInternalServerError
		f.feeds.body[p] = `{"error":"boom"}`
	}
	check("500", http.StatusBadGateway, "feeds_unreachable")
	for p := range f.feeds.status {
		f.feeds.status[p] = 0
		f.feeds.body[p] = "<html>not json</html>"
	}
	check("not json", http.StatusBadGateway, "feeds_unreachable")
	for p := range f.feeds.status {
		delete(f.feeds.status, p)
		delete(f.feeds.body, p)
	}

	// The service refuses the token.
	f.env.Elevate()
	f.env.MustDo(http.MethodPut, "/feeds/settings", map[string]any{"agentId": f.agentID, "baseUrl": f.feeds.URL, "token": "wrong-token-xyz"}, nil)
	check("wrong token", http.StatusBadGateway, "feeds_unauthorized")
	f.env.MustDo(http.MethodPut, "/feeds/settings", map[string]any{"agentId": f.agentID, "baseUrl": f.feeds.URL, "token": goodToken}, nil)

	// The address is not private: the agent refuses and the page says the service is unreachable.
	f.env.MustDo(http.MethodPut, "/feeds/settings", map[string]any{"agentId": f.agentID, "baseUrl": "http://8.8.8.8:8790"}, nil)
	check("public address", http.StatusBadGateway, "feeds_unreachable")

	// Nothing listens there.
	dead := newFakeFeeds(t)
	deadURL := dead.URL
	dead.Close()
	f.env.MustDo(http.MethodPut, "/feeds/settings", map[string]any{"agentId": f.agentID, "baseUrl": deadURL}, nil)
	check("connection refused", http.StatusBadGateway, "feeds_unreachable")
	if _, raw := f.do(http.MethodGet, "/feeds/news", nil, nil); !strings.Contains(string(raw), "x-console-feeds") {
		t.Fatalf("message should point at the service: %s", raw)
	}
	f.noToken(t)
}

func TestAgentOffline(t *testing.T) {
	env := testutil.New(t)
	id := pairedOffline(env, []string{protocol.CapProxy})
	env.Elevate()
	env.MustDo(http.MethodPut, "/feeds/settings", map[string]any{"agentId": id, "baseUrl": "http://127.0.0.1:8790", "token": goodToken}, nil)
	for _, p := range []string{"/feeds/news", "/feeds/tweets", "/feeds/news/facets", "/feeds/news/1"} {
		code, raw := env.Do(http.MethodGet, p, nil, nil)
		var e struct {
			Code    string
			Message string
			Details map[string]any
		}
		_ = json.Unmarshal(raw, &e)
		if code != http.StatusServiceUnavailable || e.Code != "agent_offline" || !strings.Contains(e.Message, "vm-1") ||
			e.Details["agentName"] != "vm-1" || e.Details["agentId"] != id {
			t.Fatalf("%s: %d %s", p, code, raw)
		}
		if strings.Contains(string(raw), "SECRET") {
			t.Fatalf("token leaked: %s", raw)
		}
	}
	if code, raw := env.Do(http.MethodPost, "/feeds/settings/test", nil, nil); code != http.StatusServiceUnavailable || errCode(raw) != "agent_offline" {
		t.Fatalf("test: %d %s", code, raw)
	}
}
