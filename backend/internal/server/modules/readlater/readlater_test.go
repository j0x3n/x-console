package readlater_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/modules/readlater"
	"github.com/j0x3n/x-console/backend/internal/server/modules/readlater/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

const articleHTML = `<!doctype html><html lang="zh"><head><meta charset="utf-8">
<title>鲸鱼协议详解 - 示例博客</title>
<meta property="og:site_name" content="示例博客">
<meta name="description" content="一篇讲鲸鱼协议的文章">
</head><body>
<nav>首页 关于 订阅</nav>
<article><h1>鲸鱼协议详解</h1>
<p>鲸鱼协议是一种让节点在不稳定网络里保持同步的办法。它先用心跳确认对方在线，再按版本号补齐缺的数据。</p>
<p>实际部署时要注意两点。第一，心跳间隔不能短于往返时间的两倍。第二，补齐数据要限速，否则会拖垮主库。</p>
<p>测试结果表明，在丢包率 10% 的网络里，同步延迟比旧办法低了一半，所以推荐在新项目里直接使用它，不用再自己写重试逻辑。</p>
</article><footer>版权所有</footer></body></html>`

type site struct {
	*httptest.Server
	mu    sync.Mutex
	hits  map[string]int
	fail  map[string]bool
	agent string
}

func newSite(t *testing.T) *site {
	s := &site{hits: map[string]int{}, fail: map[string]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/article", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits["/article"]++
		s.agent = r.UserAgent()
		s.mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, articleHTML)
	})
	mux.HandleFunc("/flaky", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits["/flaky"]++
		bad := s.fail["/flaky"]
		s.mu.Unlock()
		if bad {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, articleHTML)
	})
	mux.HandleFunc("/paper.pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 fake"))
	})
	mux.HandleFunc("/notes.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "第一行\n\n\n\n第二行  \n")
	})
	mux.HandleFunc("/gbk", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// 没声明编码，也不是 UTF-8 的页面不应该让抓取失败
		_, _ = w.Write([]byte("<html><head><title>plain</title></head><body><p>hello</p></body></html>"))
	})
	mux.HandleFunc("/hop", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/article", http.StatusFound)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func (s *site) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}

func setup(t *testing.T) (*testutil.Env, *readlater.Module, *site) {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*readlater.Module](env.App.Deps.Registry, readlater.ServiceKey)
	if !ok {
		t.Fatal("readlater module missing")
	}
	readlater.AllowPrivate(m)
	return env, m, newSite(t)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(15 * time.Millisecond)
	}
}

func add(t *testing.T, env *testutil.Env, body map[string]any) (int, api.ReadItemResult) {
	t.Helper()
	var out api.ReadItemResult
	status, raw := env.Do(http.MethodPost, "/readlater", body, &out)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("add %v: %d %s", body, status, raw)
	}
	return status, out
}

func get(t *testing.T, env *testutil.Env, id int64) api.ReadItem {
	t.Helper()
	var it api.ReadItem
	env.MustDo(http.MethodGet, "/readlater/"+strconv.FormatInt(id, 10), nil, &it)
	return it
}

func waitStatus(t *testing.T, env *testutil.Env, id int64, want api.ReadStatus) api.ReadItem {
	t.Helper()
	var it api.ReadItem
	waitFor(t, "status "+string(want), func() bool {
		it = get(t, env, id)
		return it.Status == want
	})
	return it
}

func list(t *testing.T, env *testutil.Env, query string) api.ReadList {
	t.Helper()
	var out api.ReadList
	env.MustDo(http.MethodGet, "/readlater"+query, nil, &out)
	return out
}

// ---- 保存和抓取 ----

func TestAddFetchesTheArticle(t *testing.T) {
	env, _, s := setup(t)
	status, res := add(t, env, map[string]any{"url": s.URL + "/article?id=7&utm_source=x&fbclid=abc#section", "note": "同事推荐"})
	if status != http.StatusCreated || res.Duplicate {
		t.Fatalf("create: %d %+v", status, res)
	}
	if res.Item.Url != s.URL+"/article?id=7" || res.Item.Status != api.Queued || res.Item.Source != api.ReadItemSourceWeb {
		t.Fatalf("saved: %+v", res.Item)
	}
	it := waitStatus(t, env, res.Item.Id, api.Ready)
	if it.Title != "鲸鱼协议详解 - 示例博客" && !strings.Contains(it.Title, "鲸鱼协议") {
		t.Fatalf("title: %q", it.Title)
	}
	if it.Content == nil || !strings.Contains(*it.Content, "心跳间隔不能短于往返时间的两倍") || strings.Contains(*it.Content, "版权所有") {
		t.Fatalf("content: %v", it.Content)
	}
	if !it.HasContent || it.FetchedAt == nil || it.Note != "同事推荐" || it.Error != "" {
		t.Fatalf("detail: %+v", it)
	}
	if !strings.Contains(s.agent, "X-Console") {
		t.Fatalf("user agent: %q", s.agent)
	}

	// 列表里不带正文，但能按正文搜到
	l := list(t, env, "")
	if len(l.Items) != 1 || l.Items[0].Content != nil || !l.Items[0].HasContent {
		t.Fatalf("list: %+v", l.Items)
	}
	if l := list(t, env, "?q=往返时间"); len(l.Items) != 1 {
		t.Fatalf("content search: %+v", l.Items)
	}
	if l := list(t, env, "?q=不存在的词"); len(l.Items) != 0 {
		t.Fatalf("search miss: %+v", l.Items)
	}
	if l := list(t, env, "?q=_"); len(l.Items) != 0 {
		t.Fatalf("underscore must not match everything: %+v", l.Items)
	}
	if l := list(t, env, "?q=10%25"); len(l.Items) != 1 {
		t.Fatalf("percent is a plain character: %+v", l.Items)
	}
}

func TestDuplicateReturnsTheSameItem(t *testing.T) {
	env, _, s := setup(t)
	_, first := add(t, env, map[string]any{"url": s.URL + "/article?utm_medium=a"})
	status, second := add(t, env, map[string]any{"url": s.URL + "/article#top"})
	if status != http.StatusOK || !second.Duplicate || second.Item.Id != first.Item.Id {
		t.Fatalf("duplicate: %d %+v", status, second)
	}
	waitStatus(t, env, first.Item.Id, api.Ready)
	if n := list(t, env, "?view=all").Counts.All; n != 1 {
		t.Fatalf("count: %d", n)
	}
	if s.count("/article") != 1 {
		t.Fatalf("fetched %d times", s.count("/article"))
	}
}

func TestFailedFetchCanBeRetried(t *testing.T) {
	env, _, s := setup(t)
	s.mu.Lock()
	s.fail["/flaky"] = true
	s.mu.Unlock()
	_, res := add(t, env, map[string]any{"url": s.URL + "/flaky"})
	it := waitStatus(t, env, res.Item.Id, api.Failed)
	if !strings.Contains(it.Error, "404") {
		t.Fatalf("error: %q", it.Error)
	}
	if c := list(t, env, "").Counts; c.Failed != 1 || c.Unread != 1 {
		t.Fatalf("counts: %+v", c)
	}
	s.mu.Lock()
	s.fail["/flaky"] = false
	s.mu.Unlock()
	var again api.ReadItem
	env.MustDo(http.MethodPost, fmt.Sprintf("/readlater/%d/refetch", res.Item.Id), nil, &again)
	it = waitStatus(t, env, res.Item.Id, api.Ready)
	if it.Error != "" || it.Content == nil || !strings.Contains(*it.Content, "鲸鱼协议") {
		t.Fatalf("after retry: %+v", it)
	}
	if status, _ := env.Do(http.MethodPost, "/readlater/9999/refetch", nil, nil); status != http.StatusNotFound {
		t.Fatalf("refetch missing: %d", status)
	}
}

func TestFollowsRedirectsAndReadsOtherTypes(t *testing.T) {
	env, _, s := setup(t)
	_, hop := add(t, env, map[string]any{"url": s.URL + "/hop"})
	it := waitStatus(t, env, hop.Item.Id, api.Ready)
	if it.Content == nil || !strings.Contains(*it.Content, "鲸鱼协议") {
		t.Fatalf("redirect: %+v", it)
	}

	// PDF 只留文件名，没有正文
	_, pdf := add(t, env, map[string]any{"url": s.URL + "/paper.pdf"})
	it = waitStatus(t, env, pdf.Item.Id, api.Ready)
	if it.Title != "paper.pdf" || it.HasContent {
		t.Fatalf("pdf: %+v", it)
	}

	// 纯文本留全文，空行合并
	_, txt := add(t, env, map[string]any{"url": s.URL + "/notes.txt"})
	it = waitStatus(t, env, txt.Item.Id, api.Ready)
	if it.Content == nil || *it.Content != "第一行\n\n第二行" {
		t.Fatalf("text: %q", *it.Content)
	}

	// 没有声明编码的页面也能抓
	_, plain := add(t, env, map[string]any{"url": s.URL + "/gbk"})
	waitStatus(t, env, plain.Item.Id, api.Ready)
}

// ---- 安全 ----

func TestRefusesAddressesOnThisMachine(t *testing.T) {
	env := testutil.New(t) // 不开私网例外
	for _, u := range []string{
		"http://127.0.0.1:8080/x", "http://localhost/x", "http://app.localhost/x", "http://[::1]/x",
		"http://169.254.169.254/latest/meta-data", "http://10.0.0.5/x", "http://192.168.1.1/", "http://172.16.0.1/",
		"http://100.64.1.1/", "http://0.0.0.0/", "http://nas.local/x", "http://db.internal/x",
		"ftp://example.com/x", "javascript:alert(1)", "file:///etc/passwd", "example.com/no-scheme", "   ", "http:///x",
	} {
		status, raw := env.Do(http.MethodPost, "/readlater", map[string]any{"url": u}, nil)
		if status != http.StatusBadRequest {
			t.Errorf("%q: %d %s", u, status, raw)
		}
	}
	if n := list(t, env, "?view=all").Counts.All; n != 0 {
		t.Fatalf("nothing should be saved: %d", n)
	}
}

func TestBlockedAddr(t *testing.T) {
	blocked := []string{"127.0.0.1", "::1", "10.1.2.3", "172.31.0.1", "192.168.0.9", "169.254.169.254", "fe80::1", "100.100.1.1", "0.0.0.0", "224.0.0.1", "::ffff:127.0.0.1", "fd00::1"}
	for _, s := range blocked {
		if !readlater.BlockedAddr(netip.MustParseAddr(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:4700:4700::1111"} {
		if readlater.BlockedAddr(netip.MustParseAddr(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

// 名字解析到本机、或者被跳转到本机时，连接本身会被拦下。
func TestFetchClientBlocksLocalConnections(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "secret") }))
	defer local.Close()
	client := readlater.NewFetchClient(false)
	if resp, err := client.Get(local.URL); err == nil {
		resp.Body.Close()
		t.Fatal("a local server must not be reachable")
	} else if !strings.Contains(err.Error(), "不能访问这个地址") {
		t.Fatalf("error: %v", err)
	}
	// 公网页面跳转到本机
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, local.URL, http.StatusFound)
	}))
	defer redirect.Close()
	if resp, err := readlater.NewFetchClient(false).Get(redirect.URL); err == nil {
		resp.Body.Close()
		t.Fatal("redirect to a local server must not be followed")
	}
}

func TestFetchOfPrivateTargetFailsInTheBackground(t *testing.T) {
	// 主机名看起来正常，但连接时才会发现是本机：用没有例外的客户端抓测试站点
	env, m, s := setup(t)
	_, res := add(t, env, map[string]any{"url": s.URL + "/article"})
	waitStatus(t, env, res.Item.Id, api.Ready)
	readlater.UseStrictClient(m)
	var again api.ReadItem
	env.MustDo(http.MethodPost, fmt.Sprintf("/readlater/%d/refetch", res.Item.Id), nil, &again)
	it := waitStatus(t, env, res.Item.Id, api.Failed)
	if !strings.Contains(it.Error, "不能访问") {
		t.Fatalf("error: %q", it.Error)
	}
}

// ---- 地址和链接的整理 ----

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"https://Example.com/a?b=1#frag":                     "https://example.com/a?b=1",
		"https://example.com/a?utm_source=x&b=1&utm_term=2":  "https://example.com/a?b=1",
		"https://example.com/a?UTM_Source=x":                 "https://example.com/a",
		"https://example.com/a?fbclid=1&gclid=2&q=%E4%BD%A0": "https://example.com/a?q=%E4%BD%A0",
		"https://user:pass@example.com/a":                    "https://example.com/a",
		" http://example.com ":                               "http://example.com",
		"https://example.com/a?id=1&id=2":                    "https://example.com/a?id=1&id=2",
	}
	for in, want := range cases {
		got, err := readlater.NormalizeURL(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x.com", "x.com", "http://", "mailto:a@b.c", "https://example.com/" + strings.Repeat("a", 2000)} {
		if _, err := readlater.NormalizeURL(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
}

func TestExtractLinks(t *testing.T) {
	got := readlater.ExtractLinks("看这个 https://a.com/x。还有（https://b.com/y?z=1）以及 https://a.com/x", []string{"https://c.com/", "https://b.com/y?z=1"})
	want := []string{"https://a.com/x", "https://b.com/y?z=1", "https://c.com/"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("links: %v", got)
	}
	many := strings.Repeat("https://a.com/1 https://a.com/2 https://a.com/3 ", 3) + "https://a.com/4 https://a.com/5 https://a.com/6 https://a.com/7"
	if got := readlater.ExtractLinks(many, nil); len(got) != 5 {
		t.Fatalf("limit: %v", got)
	}
	if got := readlater.ExtractLinks("没有链接的话", nil); len(got) != 0 {
		t.Fatalf("none: %v", got)
	}
}

// ---- 编辑、列表、删除 ----

func TestEditTagsReadAndDelete(t *testing.T) {
	env, _, s := setup(t)
	_, res := add(t, env, map[string]any{"url": s.URL + "/article"})
	id := res.Item.Id
	waitStatus(t, env, id, api.Ready)
	path := "/readlater/" + strconv.FormatInt(id, 10)

	if status, _ := env.Do(http.MethodPatch, path, map[string]any{"title": "  "}, nil); status != http.StatusBadRequest {
		t.Fatalf("empty title: %d", status)
	}
	if status, _ := env.Do(http.MethodPatch, path, map[string]any{"note": strings.Repeat("字", 2001)}, nil); status != http.StatusBadRequest {
		t.Fatalf("long note: %d", status)
	}
	var it api.ReadItem
	env.MustDo(http.MethodPatch, path, map[string]any{"title": "我的标题", "tags": []string{" 网络 ", "#协议", "网络", "", "Go"}, "note": " 备注 "}, &it)
	if it.Title != "我的标题" || fmt.Sprint(it.Tags) != "[网络 协议 Go]" || it.Note != "备注" || it.Read {
		t.Fatalf("patched: %+v", it)
	}

	// 重新抓取不会改掉用户改过的标题
	env.MustDo(http.MethodPost, path+"/refetch", nil, nil)
	waitFor(t, "refetch", func() bool { return s.count("/article") >= 2 })
	it = waitStatus(t, env, id, api.Ready)
	if it.Title != "我的标题" {
		t.Fatalf("title overwritten: %q", it.Title)
	}

	// 标记已读
	l := list(t, env, "")
	if l.Counts.Unread != 1 || l.Counts.Read != 0 || len(l.Items) != 1 {
		t.Fatalf("unread: %+v", l)
	}
	env.MustDo(http.MethodPatch, path, map[string]any{"read": true}, &it)
	if !it.Read || it.ReadAt == nil {
		t.Fatalf("read: %+v", it)
	}
	if l := list(t, env, ""); len(l.Items) != 0 || l.Counts.Read != 1 {
		t.Fatalf("unread view after read: %+v", l)
	}
	if l := list(t, env, "?view=read"); len(l.Items) != 1 {
		t.Fatalf("read view: %+v", l.Items)
	}
	if l := list(t, env, "?view=all&tag=go"); len(l.Items) != 1 || len(l.Tags) != 3 {
		t.Fatalf("tag filter: %+v", l)
	}
	if l := list(t, env, "?view=all&tag=没有"); len(l.Items) != 0 {
		t.Fatalf("tag miss: %+v", l.Items)
	}
	it = api.ReadItem{}
	env.MustDo(http.MethodPatch, path, map[string]any{"read": false}, &it)
	if it.Read || it.ReadAt != nil {
		t.Fatalf("unread again: %+v", it)
	}

	if status, _ := env.Do(http.MethodDelete, path, nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if status, _ := env.Do(http.MethodGet, path, nil, nil); status != http.StatusNotFound {
		t.Fatalf("after delete: %d", status)
	}
	if status, _ := env.Do(http.MethodDelete, path, nil, nil); status != http.StatusNotFound {
		t.Fatalf("delete again: %d", status)
	}
	if status, _ := env.Do(http.MethodPatch, "/readlater/9999", map[string]any{"read": true}, nil); status != http.StatusNotFound {
		t.Fatalf("patch missing: %d", status)
	}
}

func TestSharedSourceIsRemembered(t *testing.T) {
	env, _, s := setup(t)
	_, res := add(t, env, map[string]any{"url": s.URL + "/paper.pdf", "source": "share"})
	if res.Item.Source != api.ReadItemSourceShare {
		t.Fatalf("source: %+v", res.Item)
	}
}

// ---- AI 摘要和标签 ----

type fakeLLM struct {
	*llm.Fake
	mu      sync.Mutex
	prompts []string
}

func (f *fakeLLM) Available(context.Context) bool { return true }
func (f *fakeLLM) CompleteText(ctx context.Context, purpose, system, user string) (string, error) {
	return "", nil
}
func (f *fakeLLM) CompleteJSON(ctx context.Context, purpose, system, user string, schema json.RawMessage, out any) error {
	f.mu.Lock()
	f.prompts = append(f.prompts, user)
	f.mu.Unlock()
	return json.Unmarshal([]byte(`{"summary":"第一行讲协议。\n\n第二行讲部署。\n第三行讲结论。\n第四行不该留下。","tags":["协议","网络","协议","#同步"]}`), out)
}

func TestSummaryAndTagsFromAI(t *testing.T) {
	env, _, s := setup(t)
	fake := &fakeLLM{Fake: llm.NewFake()}

	// 没配 AI：抓取照常，摘要接口回 409
	_, res := add(t, env, map[string]any{"url": s.URL + "/article"})
	id := res.Item.Id
	it := waitStatus(t, env, id, api.Ready)
	if it.Summary != "" || len(it.Tags) != 0 {
		t.Fatalf("no AI, no summary: %+v", it)
	}
	status, raw := env.Do(http.MethodPost, fmt.Sprintf("/readlater/%d/summarize", id), nil, nil)
	if status != http.StatusConflict || !strings.Contains(string(raw), "ai_not_configured") {
		t.Fatalf("summarize without AI: %d %s", status, raw)
	}

	// 配了 AI：手动写摘要，已有标签保留，新标签合并去重
	env.MustDo(http.MethodPatch, fmt.Sprintf("/readlater/%d", id), map[string]any{"tags": []string{"阅读"}}, nil)
	module.Provide[contracts.LLM](env.App.Deps.Registry, contracts.LLMKey, fake)
	env.MustDo(http.MethodPost, fmt.Sprintf("/readlater/%d/summarize", id), nil, &it)
	if it.Summary != "第一行讲协议。\n第二行讲部署。\n第三行讲结论。" {
		t.Fatalf("summary: %q", it.Summary)
	}
	if fmt.Sprint(it.Tags) != "[阅读 协议 网络 同步]" {
		t.Fatalf("tags: %v", it.Tags)
	}
	if len(fake.prompts) != 1 || !strings.Contains(fake.prompts[0], "鲸鱼协议详解") || !strings.Contains(fake.prompts[0], "已有标签：阅读") {
		t.Fatalf("prompt: %v", fake.prompts)
	}

	// 新存一篇：抓完自动写摘要，并且提示里带上已有标签
	_, second := add(t, env, map[string]any{"url": s.URL + "/hop"})
	waitFor(t, "auto summary", func() bool { return get(t, env, second.Item.Id).Summary != "" })
	if len(fake.prompts) != 2 || !strings.Contains(fake.prompts[1], "已有标签：") || !strings.Contains(fake.prompts[1], "协议") {
		t.Fatalf("second prompt: %v", fake.prompts)
	}

	// 没有正文的条目没法写摘要
	_, pdf := add(t, env, map[string]any{"url": s.URL + "/paper.pdf"})
	waitStatus(t, env, pdf.Item.Id, api.Ready)
	status, raw = env.Do(http.MethodPost, fmt.Sprintf("/readlater/%d/summarize", pdf.Item.Id), nil, nil)
	if status != http.StatusConflict || !strings.Contains(string(raw), "no_content") {
		t.Fatalf("summarize pdf: %d %s", status, raw)
	}
	if status, _ := env.Do(http.MethodPost, "/readlater/9999/summarize", nil, nil); status != http.StatusNotFound {
		t.Fatalf("summarize missing: %d", status)
	}
}

func TestAIActions(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	out, err := env.App.Deps.Actions.Run(ctx, "readlater.add", json.RawMessage(fmt.Sprintf(`{"url":%q,"note":"AI 存的"}`, s.URL+"/article")))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	var added struct {
		ID        int64
		Duplicate bool
	}
	_ = json.Unmarshal(raw, &added)
	if added.ID == 0 || added.Duplicate {
		t.Fatalf("add: %s", raw)
	}
	waitStatus(t, env, added.ID, api.Ready)
	if it := get(t, env, added.ID); it.Source != "ai" || it.Note != "AI 存的" {
		t.Fatalf("source: %+v", it)
	}
	if _, err := env.App.Deps.Actions.Run(ctx, "readlater.add", json.RawMessage(`{"url":"ftp://x"}`)); err == nil {
		t.Fatal("bad url must fail")
	}
	out, err = env.App.Deps.Actions.Run(ctx, "readlater.search", json.RawMessage(`{"q":"心跳"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(out)
	if !strings.Contains(string(raw), "鲸鱼协议") || strings.Contains(string(raw), "心跳间隔不能短于") {
		t.Fatalf("search result must hold the item but not the text: %s", raw)
	}
}

// ---- 没抓完的重新排队 ----

func TestRetryStaleQueuesUnfinishedItems(t *testing.T) {
	env, m, s := setup(t)
	now := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	readlater.SetNow(m, func() time.Time { return now })
	// 手动写入一条一直排着的条目，模拟服务器重启时丢了任务
	if _, err := env.App.Deps.DB.ExecContext(context.Background(),
		`INSERT INTO read_items (url, title, site, source, status, created_at, updated_at) VALUES (?, 'x', 'x', 'web', 'queued', ?, ?)`,
		s.URL+"/article", now.Add(-10*time.Minute), now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := readlater.RetryStale(m, context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "stale item fetched", func() bool { return s.count("/article") == 1 })
}

// ---- 每周提醒 ----

func notices(t *testing.T, env *testutil.Env) []struct{ Kind, Title, Body, Link string } {
	t.Helper()
	var out struct {
		Items []struct{ Kind, Title, Body, Link string }
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &out)
	var digest []struct{ Kind, Title, Body, Link string }
	for _, n := range out.Items {
		if n.Kind == "readlater.digest" {
			digest = append(digest, n)
		}
	}
	return digest
}

func TestWeeklyDigest(t *testing.T) {
	env, m, s := setup(t)
	ctx := context.Background()
	loc := env.App.Deps.Scheduler.Location()
	at := func(day, hour int) func() time.Time {
		return func() time.Time { return time.Date(2026, 10, day, hour, 30, 0, 0, loc) }
	}
	// 2026-10-10 是星期六
	readlater.SetNow(m, at(10, 10))

	if err := readlater.Digest(m, ctx); err != nil || len(notices(t, env)) != 0 {
		t.Fatalf("nothing unread must not push: %v %+v", err, notices(t, env))
	}
	_, a := add(t, env, map[string]any{"url": s.URL + "/article"})
	_, b := add(t, env, map[string]any{"url": s.URL + "/paper.pdf"})
	waitStatus(t, env, a.Item.Id, api.Ready)
	waitStatus(t, env, b.Item.Id, api.Ready)

	readlater.SetNow(m, at(9, 11)) // 星期五
	_ = readlater.Digest(m, ctx)
	readlater.SetNow(m, func() time.Time { return time.Date(2026, 10, 10, 9, 59, 0, 0, loc) }) // 星期六 10 点前
	_ = readlater.Digest(m, ctx)
	if len(notices(t, env)) != 0 {
		t.Fatal("only Saturday from 10:00")
	}

	readlater.SetNow(m, at(10, 10))
	if err := readlater.Digest(m, ctx); err != nil {
		t.Fatal(err)
	}
	got := notices(t, env)
	if len(got) != 1 || got[0].Title != "稍后阅读还有 2 篇没看" || got[0].Link != "/readlater" || !strings.Contains(got[0].Body, "paper.pdf") || !strings.Contains(got[0].Body, "1. ") {
		t.Fatalf("digest: %+v", got)
	}

	// 同一周不再发，下一周再发
	readlater.SetNow(m, at(10, 18))
	_ = readlater.Digest(m, ctx)
	if len(notices(t, env)) != 1 {
		t.Fatal("sent twice in one week")
	}
	readlater.SetNow(m, at(17, 10))
	_ = readlater.Digest(m, ctx)
	if len(notices(t, env)) != 2 {
		t.Fatal("next week should push again")
	}

	// 都读完了就不推
	for _, id := range []int64{a.Item.Id, b.Item.Id} {
		env.MustDo(http.MethodPatch, fmt.Sprintf("/readlater/%d", id), map[string]any{"read": true}, nil)
	}
	readlater.SetNow(m, at(24, 10))
	_ = readlater.Digest(m, ctx)
	if len(notices(t, env)) != 2 {
		t.Fatal("all read, no push")
	}
}

// ---- 隐藏内容 ----

func TestHiddenModuleGate(t *testing.T) {
	env, _, s := setup(t)
	add(t, env, map[string]any{"url": s.URL + "/paper.pdf"})
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, nil)
	env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"readlater"}}, nil)
	if status, _ := env.Do(http.MethodGet, "/readlater", nil, nil); status != http.StatusOK {
		t.Fatalf("unlocked: %d", status)
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	if status, _ := env.Do(http.MethodGet, "/readlater", nil, nil); status != http.StatusNotFound {
		t.Fatalf("locked list: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/readlater", map[string]any{"url": s.URL + "/article"}, nil); status != http.StatusNotFound {
		t.Fatalf("locked create: %d", status)
	}
	var avail struct{ Modules []string }
	env.MustDo(http.MethodGet, "/app/modules", nil, &avail)
	for _, id := range avail.Modules {
		if id == "readlater" {
			t.Fatalf("readlater still available: %v", avail.Modules)
		}
	}
}

// ---- Telegram ----

type fakeTelegram struct {
	*httptest.Server
	mu    sync.Mutex
	calls map[string][]string
}

func newTelegram(t *testing.T) *fakeTelegram {
	f := &fakeTelegram{calls: map[string][]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.calls[r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]] = append(f.calls[r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]], string(raw))
		f.mu.Unlock()
		_, _ = io.WriteString(w, `{"ok":true,"result":{}}`)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeTelegram) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls["sendMessage"]...)
}

func TestTelegramMessagesSaveLinks(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	tg := newTelegram(t)
	env.Elevate()
	env.MustDo(http.MethodPut, "/notify/channels/telegram", map[string]any{"values": map[string]string{"bot_token": "123456:SECRET", "chat_id": "42", "api_base": tg.URL}}, nil)
	if err := env.App.Deps.Settings.SetSecret(ctx, "telegram.webhook_secret", "hook-secret"); err != nil {
		t.Fatal(err)
	}

	post := func(secret string, update map[string]any) int {
		raw, _ := json.Marshal(update)
		req, _ := http.NewRequest(http.MethodPost, env.URL("/integrations/telegram/webhook"), bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	message := func(chat int, text string, entities ...map[string]any) map[string]any {
		msg := map[string]any{"message_id": 5, "chat": map[string]any{"id": chat}, "text": text}
		if len(entities) > 0 {
			msg["entities"] = entities
		}
		return map[string]any{"update_id": 1, "message": msg}
	}

	// 密钥不对：拒绝，不保存
	if status := post("wrong", message(42, s.URL+"/article")); status != http.StatusUnauthorized {
		t.Fatalf("bad secret: %d", status)
	}
	// 别的聊天：悄悄忽略
	if status := post("hook-secret", message(7, s.URL+"/article")); status != http.StatusOK {
		t.Fatalf("foreign chat: %d", status)
	}
	if list(t, env, "?view=all").Counts.All != 0 || len(tg.sent()) != 0 {
		t.Fatal("another chat must not save links or get answers")
	}
	// 没有链接的话：不保存，不回复
	if status := post("hook-secret", message(42, "今天吃什么")); status != http.StatusOK {
		t.Fatalf("plain text: %d", status)
	}
	if len(tg.sent()) != 0 {
		t.Fatalf("plain text answered: %v", tg.sent())
	}

	// 一条链接加几个字：存下来，字当备注
	if status := post("hook-secret", message(42, "这篇不错 "+s.URL+"/article?utm_source=tg")); status != http.StatusOK {
		t.Fatalf("one link: %d", status)
	}
	waitFor(t, "first reply", func() bool { return len(tg.sent()) == 1 })
	if !strings.Contains(tg.sent()[0], "已存 1 条") || !strings.Contains(tg.sent()[0], `"chat_id":42`) {
		t.Fatalf("reply: %v", tg.sent())
	}
	l := list(t, env, "")
	if len(l.Items) != 1 || l.Items[0].Source != "telegram" || l.Items[0].Note != "这篇不错" {
		t.Fatalf("saved: %+v", l.Items)
	}
	waitStatus(t, env, l.Items[0].Id, api.Ready)

	// 两条链接：一条重复，一条是链接文字背后的地址
	hidden := s.URL + "/paper.pdf"
	text := "文章 " + s.URL + "/article 和这个"
	entity := map[string]any{"type": "text_link", "offset": 0, "length": 2, "url": hidden}
	if status := post("hook-secret", message(42, text, entity)); status != http.StatusOK {
		t.Fatalf("two links: %d", status)
	}
	waitFor(t, "second reply", func() bool { return len(tg.sent()) == 2 })
	if !strings.Contains(tg.sent()[1], "已存 1 条") || !strings.Contains(tg.sent()[1], "1 条之前已经存过") {
		t.Fatalf("reply: %v", tg.sent()[1])
	}
	if n := list(t, env, "?view=all").Counts.All; n != 2 {
		t.Fatalf("count: %d", n)
	}

	// 全是旧的
	_ = post("hook-secret", message(42, s.URL+"/article"))
	waitFor(t, "third reply", func() bool { return len(tg.sent()) == 3 })
	if !strings.Contains(tg.sent()[2], "这 1 条之前已经存过") {
		t.Fatalf("reply: %v", tg.sent()[2])
	}
	// 网址不合格（太长）：不存，回复说明
	_ = post("hook-secret", message(42, "https://example.com/"+strings.Repeat("a", 2100)))
	waitFor(t, "fourth reply", func() bool { return len(tg.sent()) == 4 })
	if !strings.Contains(tg.sent()[3], "1 条网址不能存") {
		t.Fatalf("reply: %v", tg.sent()[3])
	}
}
