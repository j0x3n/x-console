package readlater_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/readlater"
	"github.com/j0x3n/x-console/backend/internal/server/modules/readlater/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func pngBytes(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// x is a fake X: the three interfaces, the pictures, and a log of what came in.
type x struct {
	*httptest.Server
	mu       sync.Mutex
	syn      string // body of the public interface; "" gives 404
	gql      string
	gqlCode  int
	fx       string
	cookies  []string
	csrf     []string
	tokens   []string
	gqlPaths []string
	hits     map[string]int
}

func newX(t *testing.T) *x {
	f := &x{hits: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/syndication/tweet-result", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits["syn"]++
		f.tokens = append(f.tokens, r.URL.Query().Get("token"))
		body := f.syn
		f.mu.Unlock()
		if body == "" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, body)
	})
	mux.HandleFunc("/graphql/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits["gql"]++
		f.cookies = append(f.cookies, r.Header.Get("Cookie"))
		f.csrf = append(f.csrf, r.Header.Get("X-Csrf-Token"))
		f.gqlPaths = append(f.gqlPaths, r.URL.Path)
		code, body := f.gqlCode, f.gql
		f.mu.Unlock()
		if code != 0 {
			w.WriteHeader(code)
			return
		}
		_, _ = io.WriteString(w, body)
	})
	mux.HandleFunc("/fx/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits["fx"]++
		body := f.fx
		f.mu.Unlock()
		if body == "" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, body)
	})
	mux.HandleFunc("/img/photo.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes(color.RGBA{R: 200, A: 255}))
	})
	mux.HandleFunc("/img/avatar.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes(color.RGBA{G: 200, A: 255}))
	})
	mux.HandleFunc("/img/evil.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *x) count(k string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[k]
}

func (f *x) wire(m *readlater.Module) {
	readlater.SetXBases(m, map[string]string{"syndication": f.URL + "/syndication", "graphql": f.URL + "/graphql", "fxtwitter": f.URL + "/fx"})
}

func (f *x) synTweet(text string, extra map[string]any) string {
	t := map[string]any{
		"__typename": "Tweet", "id_str": "1850000000000000001", "text": text, "created_at": "2026-09-01T08:30:00.000Z",
		"user":           map[string]any{"name": "测试用户", "screen_name": "tester", "profile_image_url_https": f.URL + "/img/avatar.png"},
		"favorite_count": 12,
		"mediaDetails":   []any{map[string]any{"type": "photo", "media_url_https": f.URL + "/img/photo.png", "ext_alt_text": "一张红色的图", "url": "https://t.co/pic"}},
		"entities":       map[string]any{"urls": []any{map[string]any{"url": "https://t.co/abc", "expanded_url": "https://example.org/post?a=1&b=2"}}},
	}
	for k, v := range extra {
		t[k] = v
	}
	raw, _ := json.Marshal(t)
	return string(raw)
}

const tweetURL = "https://twitter.com/tester/status/1850000000000000001?s=20&t=abc"
const tweetCanonical = "https://x.com/tester/status/1850000000000000001"

func setupX(t *testing.T) (*testutil.Env, *readlater.Module, *x) {
	t.Helper()
	env, m, _ := setup(t)
	f := newX(t)
	f.wire(m)
	return env, m, f
}

func fileCount(t *testing.T, env *testutil.Env) int {
	t.Helper()
	n := 0
	for _, err := range env.App.Deps.Files.For("readlater").List(t.Context(), "") {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	return n
}

func TestTweetParsingAndCanonicalAddress(t *testing.T) {
	cases := map[string][3]string{
		"https://x.com/jack/status/20":                    {"jack", "20", "y"},
		"https://twitter.com/jack/status/20?s=20&t=x#top": {"jack", "20", "y"},
		"https://mobile.twitter.com/jack/status/20":       {"jack", "20", "y"},
		"https://fxtwitter.com/jack/status/20":            {"jack", "20", "y"},
		"https://x.com/i/status/20":                       {"", "20", "y"},
		"https://x.com/i/web/status/20":                   {"", "20", "y"},
		"https://x.com/jack/status/20/photo/1":            {"jack", "20", "y"},
		"https://x.com/jack":                              {"", "", "n"},
		"https://x.com/jack/status/abc":                   {"", "", "n"},
		"https://example.org/jack/status/20":              {"", "", "n"},
		"https://x.com.evil.example/jack/status/20":       {"", "", "n"},
	}
	for raw, want := range cases {
		h, id, ok := readlater.ParseTweetURL(raw)
		if (want[2] == "y") != ok || h != want[0] || id != want[1] {
			t.Errorf("%s: got %q %q %v", raw, h, id, ok)
		}
	}
	got, err := readlater.NormalizeURL(tweetURL)
	if err != nil || got != tweetCanonical {
		t.Fatalf("normalize: %q %v", got, err)
	}
	if tok := readlater.SyndicationToken("1850000000000000001"); tok == "" || strings.ContainsAny(tok, ".0") {
		t.Fatalf("token: %q", tok)
	}
}

func TestSanitizeKeepsReadingMarkupOnly(t *testing.T) {
	in := `<h2 onclick="x()">标题</h2><p style="color:red">正文<script>alert(1)</script><a href="javascript:alert(1)">坏链接</a>
<a href="/rel?x=1">好链接</a><iframe src="https://evil.example"></iframe><form><input name=a></form></p>
<img src="/a.png" onerror="alert(1)" alt="图"><img src="data:image/png;base64,AAAA"><svg><circle/></svg><style>p{}</style>
<pre><code class="evil xc-keep">code</code></pre><table><tr><td colspan="2" onclick="x">a</td></tr></table><custom-tag>里面的字</custom-tag>`
	out := readlater.Sanitize(in, "https://site.example/dir/page")
	for _, bad := range []string{"<script", "onclick", "onerror", "javascript:", "<iframe", "<form", "<input", "<svg", "<style", "style=", "data:image", "custom-tag", "evil "} {
		if strings.Contains(out, bad) {
			t.Errorf("%q survived: %s", bad, out)
		}
	}
	for _, good := range []string{"<h2>标题</h2>", `href="https://site.example/rel?x=1"`, `rel="noopener noreferrer nofollow"`, `src="https://site.example/a.png"`, `class="xc-keep"`, `colspan="2"`, "里面的字", "正文"} {
		if !strings.Contains(out, good) {
			t.Errorf("%q lost: %s", good, out)
		}
	}
}

func TestCookieOnlyGoesToX(t *testing.T) {
	for _, ok := range []string{"https://x.com/i/api/graphql/abc/TweetResultByRestId", "https://api.x.com/graphql/abc/x"} {
		if err := readlater.CookieHostOK(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"https://evil.example/x", "https://x.com.evil.example/", "http://127.0.0.1/", "https://twitter.com/i/api"} {
		if err := readlater.CookieHostOK(bad); err == nil {
			t.Errorf("%s was allowed", bad)
		}
	}
}

// ---- 读推文 ----

func TestTweetViaPublicInterfaceKeepsTextAndPictures(t *testing.T) {
	env, _, f := setupX(t)
	f.syn = f.synTweet("第一行 https://t.co/abc https://t.co/pic\n第二行", map[string]any{
		"display_text_range": []int{0, 51},
		"quoted_tweet": map[string]any{
			"__typename": "Tweet", "id_str": "1850000000000000002", "text": "被引用的话", "created_at": "2026-08-30T00:00:00.000Z",
			"user": map[string]any{"name": "另一个人", "screen_name": "other"},
		},
	})
	status, res := add(t, env, map[string]any{"url": tweetURL})
	if status != http.StatusCreated || res.Item.Url != tweetCanonical {
		t.Fatalf("saved: %d %+v", status, res.Item)
	}
	it := waitStatus(t, env, res.Item.Id, api.Ready)
	if it.Kind != api.Tweet || it.Site != "X" || !strings.HasPrefix(it.Title, "测试用户：第一行") || !it.HasHtml {
		t.Fatalf("item: %+v", it)
	}
	if it.Meta["via"] != "syndication" || it.Meta["handle"] != "tester" || it.Meta["incomplete"] != nil {
		t.Fatalf("meta: %v", it.Meta)
	}
	text := *it.Content
	if !strings.Contains(text, "https://example.org/post?a=1&b=2") || strings.Contains(text, "t.co") || !strings.Contains(text, "被引用的话") || !strings.Contains(text, "一张红色的图") {
		t.Fatalf("text: %q", text)
	}
	body := *it.ContentHtml
	if strings.Contains(body, f.URL+"/img/") || strings.Count(body, "/api/v1/readlater/") != 2 {
		t.Fatalf("pictures were not saved here: %s", body)
	}
	for _, want := range []string{`<a href="https://example.org/post?a=1&amp;b=2"`, "<br/>", "测试用户", "@tester", "另一个人", "<blockquote"} {
		if !strings.Contains(body, want) {
			t.Errorf("%q missing: %s", want, body)
		}
	}
	// 搜得到推文里的话
	if l := list(t, env, "?q=被引用"); len(l.Items) != 1 {
		t.Fatalf("search: %+v", l.Items)
	}

	// 图片从本站取，登录才能看
	start := strings.Index(body, "/api/v1")
	imgPath := body[start+len("/api/v1") : start+strings.Index(body[start:], `"`)]
	resp, err := http.Get(env.URL(imgPath))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("pictures must need a login, got %d", resp.StatusCode)
	}
	var raw []byte
	status, raw = env.Do(http.MethodGet, imgPath, nil, nil)
	if status != http.StatusOK || !bytes.Equal(raw, pngBytes(color.RGBA{R: 200, A: 255})) && !bytes.Equal(raw, pngBytes(color.RGBA{G: 200, A: 255})) {
		t.Fatalf("picture: %d %d bytes", status, len(raw))
	}
	if status, _ := env.Do(http.MethodGet, "/readlater/"+strconv.FormatInt(it.Id, 10)+"/assets/"+strings.Repeat("a", 64), nil, nil); status != http.StatusNotFound {
		t.Fatalf("unknown picture: %d", status)
	}

	// 导出的单文件：图片是内联的，没有指向本站的地址
	status, raw = env.Do(http.MethodGet, "/readlater/"+strconv.FormatInt(it.Id, 10)+"/export", nil, nil)
	page := string(raw)
	if status != http.StatusOK || strings.Contains(page, "/api/v1/readlater") || strings.Count(page, "data:image/png;base64,") != 2 || !strings.Contains(page, "<title>测试用户") {
		t.Fatalf("export: %d %.300s", status, page)
	}
	if !strings.Contains(page, base64.StdEncoding.EncodeToString(pngBytes(color.RGBA{R: 200, A: 255}))) {
		t.Fatal("export lost the picture")
	}

	// 删除后图片文件也删掉
	if n := fileCount(t, env); n != 2 {
		t.Fatalf("files before delete: %d", n)
	}
	if status, _ := env.Do(http.MethodDelete, "/readlater/"+strconv.FormatInt(it.Id, 10), nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if n := fileCount(t, env); n != 0 {
		t.Fatalf("files after delete: %d", n)
	}
}

func TestLongTweetNeedsTheCookie(t *testing.T) {
	env, m, f := setupX(t)
	_ = m
	f.syn = f.synTweet("被截断的开头…", map[string]any{"note_tweet": map[string]any{"id": "note1"}})
	f.gql = `{"data":{"tweetResult":{"result":{"__typename":"Tweet","rest_id":"1850000000000000001",
	  "core":{"user_results":{"result":{"core":{"name":"测试用户","screen_name":"tester"},"avatar":{"image_url":"` + f.URL + `/img/avatar.png"}}}},
	  "legacy":{"full_text":"被截断的开头…","created_at":"Mon Sep 01 08:30:00 +0000 2026","favorite_count":5,"reply_count":1,"retweet_count":2,
	    "extended_entities":{"media":[{"type":"photo","media_url_https":"` + f.URL + `/img/photo.png","url":"https://t.co/pic"}]}},
	  "note_tweet":{"note_tweet_results":{"result":{"text":"这是完整的长文，不会被截断。 https://t.co/x1 https://t.co/pic","entity_set":{"urls":[{"url":"https://t.co/x1","expanded_url":"https://example.org/full"}]}}}}}}}}`

	_, res := add(t, env, map[string]any{"url": tweetURL})
	it := waitStatus(t, env, res.Item.Id, api.Ready)
	if it.Meta["incomplete"] != true || f.count("gql") != 0 {
		t.Fatalf("without a cookie the post is kept as incomplete: %v gql=%d", it.Meta, f.count("gql"))
	}

	// 填 Cookie：要先提权，保存后接口不返回内容
	env.Elevate()
	var st api.XAuth
	env.MustDo(http.MethodPut, "/readlater/x-auth", map[string]any{"authToken": "tok-ABC123def456", "ct0": "csrf-XYZ789abc012"}, &st)
	raw, _ := json.Marshal(st)
	if !st.Configured || string(st.Status) != "unverified" || strings.Contains(string(raw), "ABC123") || strings.Contains(string(raw), "XYZ789") {
		t.Fatalf("state: %s", raw)
	}
	var again []byte
	_, again = env.Do(http.MethodGet, "/readlater/x-auth", nil, nil)
	if strings.Contains(string(again), "ABC123") {
		t.Fatal("cookie leaked in GET")
	}

	env.MustDo(http.MethodPost, "/readlater/"+strconv.FormatInt(it.Id, 10)+"/refetch", nil, nil)
	waitFor(t, "full text", func() bool {
		it = get(t, env, res.Item.Id)
		return it.Status == api.Ready && it.Content != nil && strings.Contains(*it.Content, "完整的长文")
	})
	if it.Meta["via"] != "cookie" || it.Meta["incomplete"] != nil || !strings.Contains(*it.Content, "https://example.org/full") || strings.Contains(*it.Content, "t.co") {
		t.Fatalf("after cookie: %v %q", it.Meta, *it.Content)
	}
	f.mu.Lock()
	gotCookie, gotCSRF, path := f.cookies[0], f.csrf[0], f.gqlPaths[0]
	f.mu.Unlock()
	if gotCookie != "auth_token=tok-ABC123def456; ct0=csrf-XYZ789abc012" || gotCSRF != "csrf-XYZ789abc012" || !strings.HasSuffix(path, "/TweetResultByRestId") {
		t.Fatalf("request: %q %q %q", gotCookie, gotCSRF, path)
	}
	// 图片只剩一份：refetch 后换掉旧的
	if n := fileCount(t, env); n != 2 {
		t.Fatalf("files: %d", n)
	}

	// 审计里没有 Cookie
	var logs struct {
		Items []map[string]any `json:"items"`
	}
	_, auditRaw := env.Do(http.MethodGet, "/audit", nil, &logs)
	if strings.Contains(string(auditRaw), "ABC123") || strings.Contains(string(auditRaw), "XYZ789") {
		t.Fatal("cookie in audit log")
	}

	// 清除
	env.MustDo(http.MethodDelete, "/readlater/x-auth", nil, &st)
	if st.Configured {
		t.Fatalf("not cleared: %+v", st)
	}
}

func TestCookieFormatAndElevation(t *testing.T) {
	env, _, _ := setupX(t)
	// saving and clearing the cookie need the elevated window
	if status, raw := env.Do(http.MethodPut, "/readlater/x-auth", map[string]any{"authToken": "tok-ABC123def456", "ct0": "csrf-XYZ789abc012"}, nil); status != http.StatusForbidden {
		t.Fatalf("save without elevation: %d %s", status, raw)
	}
	if status, _ := env.Do(http.MethodDelete, "/readlater/x-auth", nil, nil); status != http.StatusForbidden {
		t.Fatalf("clear without elevation: %d", status)
	}
	env.Elevate()
	for _, body := range []map[string]any{
		{"authToken": "only-one-1234567"},
		{"authToken": "a b c d e f g h", "ct0": "csrf-XYZ789abc012"},
		{"authToken": "auth_token=abc12345; ct0=x", "ct0": "csrf-XYZ789abc012"},
		{"queryId": "bad id!"},
	} {
		if status, raw := env.Do(http.MethodPut, "/readlater/x-auth", body, nil); status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
			t.Errorf("%v: %d %s", body, status, raw)
		}
	}
}

func TestExpiredCookieIsReportedOnce(t *testing.T) {
	env, _, f := setupX(t)
	f.syn = f.synTweet("被截断…", map[string]any{"note_tweet": map[string]any{"id": "n"}})
	f.gqlCode = http.StatusUnauthorized
	env.Elevate()
	env.MustDo(http.MethodPut, "/readlater/x-auth", map[string]any{"authToken": "tok-ABC123def456", "ct0": "csrf-XYZ789abc012"}, nil)
	_, res := add(t, env, map[string]any{"url": tweetURL})
	waitStatus(t, env, res.Item.Id, api.Ready)
	var st api.XAuth
	env.MustDo(http.MethodGet, "/readlater/x-auth", nil, &st)
	if string(st.Status) != "expired" || st.Message == nil {
		t.Fatalf("state: %+v", st)
	}
	var list struct {
		Items []struct {
			Kind string `json:"kind"`
		} `json:"items"`
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &list)
	n := 0
	for _, it := range list.Items {
		if it.Kind == "readlater.xauth" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("notifications: %d", n)
	}

	// 测试按钮也会报告
	env.MustDo(http.MethodPost, "/readlater/x-auth/test", nil, &st)
	if string(st.Status) != "expired" {
		t.Fatalf("test: %+v", st)
	}
	f.mu.Lock()
	f.gqlCode, f.gql = 0, `{"data":{"tweetResult":{"result":{"__typename":"Tweet","rest_id":"20","legacy":{"full_text":"just setting up my twttr"}}}}}`
	f.mu.Unlock()
	env.MustDo(http.MethodPost, "/readlater/x-auth/test", nil, &st)
	if string(st.Status) != "ok" || st.VerifiedAt == nil {
		t.Fatalf("test ok: %+v", st)
	}
}

func TestCookieIsNotSentToRedirectTargets(t *testing.T) {
	env, m, f := setupX(t)
	var leaked sync.Map
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store("hit", r.Header.Get("Cookie"))
	}))
	defer other.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusFound)
	}))
	defer redirect.Close()
	readlater.SetXBases(m, map[string]string{"syndication": f.URL + "/syndication", "graphql": redirect.URL, "fxtwitter": f.URL + "/fx"})
	f.syn = f.synTweet("被截断…", map[string]any{"note_tweet": map[string]any{"id": "n"}})
	env.Elevate()
	env.MustDo(http.MethodPut, "/readlater/x-auth", map[string]any{"authToken": "tok-ABC123def456", "ct0": "csrf-XYZ789abc012"}, nil)
	_, res := add(t, env, map[string]any{"url": tweetURL})
	waitStatus(t, env, res.Item.Id, api.Ready)
	if _, ok := leaked.Load("hit"); ok {
		t.Fatal("the redirect was followed")
	}
}

func TestThirdPartyConverterIsOffUnlessTurnedOn(t *testing.T) {
	env, _, f := setupX(t)
	f.fx = `{"code":200,"tweet":{"id":"1850000000000000001","text":"第三方给的全文","created_timestamp":1788000000,
	  "likes":3,"author":{"name":"测试用户","screen_name":"tester","avatar_url":"` + f.URL + `/img/avatar.png"},
	  "media":{"all":[{"type":"video","url":"https://video.example/v.mp4","thumbnail_url":"` + f.URL + `/img/photo.png"}]}}}`

	_, res := add(t, env, map[string]any{"url": tweetURL})
	it := waitStatus(t, env, res.Item.Id, api.Failed)
	if f.count("fx") != 0 || !strings.Contains(it.Error, "公开接口") || !strings.Contains(it.Error, "Cookie") {
		t.Fatalf("converter must stay off: hits=%d error=%q", f.count("fx"), it.Error)
	}

	env.Elevate()
	env.MustDo(http.MethodPut, "/readlater/x-auth", map[string]any{"fxtwitter": true}, nil)
	env.MustDo(http.MethodPost, "/readlater/"+strconv.FormatInt(it.Id, 10)+"/refetch", nil, nil)
	it = waitStatus(t, env, res.Item.Id, api.Ready)
	if it.Meta["via"] != "fxtwitter" || !strings.Contains(*it.Content, "第三方给的全文") || !strings.Contains(*it.ContentHtml, "视频，只存了封面") || strings.Contains(*it.ContentHtml, "video.example/v.mp4") {
		t.Fatalf("via converter: %v %s", it.Meta, *it.ContentHtml)
	}
}

func TestDeletedTweetFails(t *testing.T) {
	env, _, f := setupX(t)
	f.syn = `{"__typename":"TweetTombstone","tombstone":{"text":{"text":"这条推文不可用"}}}`
	_, res := add(t, env, map[string]any{"url": tweetURL})
	it := waitStatus(t, env, res.Item.Id, api.Failed)
	if !strings.Contains(it.Error, "已删除") {
		t.Fatalf("error: %q", it.Error)
	}
}

// ---- 普通网页的完整保存 ----

func TestArchiveKeepsPicturesAndDropsActiveContent(t *testing.T) {
	env, m, _ := setup(t)
	f := newX(t) // 只用它的图片
	mux := http.NewServeMux()
	var mu sync.Mutex
	withPicture := true
	mux.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		pic := withPicture
		mu.Unlock()
		imgs := fmt.Sprintf(`<img src="%s/img/photo.png" alt="配图" onerror="alert(1)">`, f.URL)
		if !pic {
			imgs = ""
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><head><title>带图的文章</title></head><body><article><h1>带图的文章</h1>
<p>第一段内容，讲了一件很长的事情，需要足够多的字让正文被识别出来。这一段要写得长一点，才不会被当成页面的杂项。</p>%s
<p>第二段内容，里面有一个<a href="javascript:alert(1)">坏链接</a>和一个<a href="/other">相对链接</a>。同样要写得长一点，让抽取器认定这是正文。</p>
<img src="%s/img/evil.svg" alt="svg">
<p>第三段内容，收尾。这里再写一些字，保证正文足够长，抽取器不会把它丢掉，也不会只留下一小段。</p>
<script>alert(2)</script></article></body></html>`, imgs, f.URL)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	_ = m

	_, res := add(t, env, map[string]any{"url": srv.URL + "/post"})
	it := waitStatus(t, env, res.Item.Id, api.Ready)
	if it.Kind != api.Page || !it.HasHtml {
		t.Fatalf("item: %+v", it)
	}
	body := *it.ContentHtml
	for _, bad := range []string{"<script", "onerror", "javascript:"} {
		if strings.Contains(body, bad) {
			t.Errorf("%q survived: %s", bad, body)
		}
	}
	if !strings.Contains(body, fmt.Sprintf("/api/v1/readlater/%d/assets/", it.Id)) || strings.Contains(body, f.URL+"/img/photo.png") {
		t.Errorf("picture not saved here: %s", body)
	}
	if !strings.Contains(body, `href="`+srv.URL+`/other"`) {
		t.Errorf("relative link: %s", body)
	}
	// SVG 不存，保留原地址，条目不算失败
	if !strings.Contains(body, f.URL+"/img/evil.svg") {
		t.Errorf("svg should keep its address: %s", body)
	}
	if n := fileCount(t, env); n != 1 {
		t.Fatalf("files: %d", n)
	}

	// 页面改了、不再有这张图：重抓后旧图被删
	mu.Lock()
	withPicture = false
	mu.Unlock()
	env.MustDo(http.MethodPost, "/readlater/"+strconv.FormatInt(it.Id, 10)+"/refetch", nil, nil)
	waitFor(t, "refetch", func() bool { return fileCount(t, env) == 0 })
}

func TestArchiveRefusesInternalPictures(t *testing.T) {
	_, m, _ := setup(t)
	internal := newX(t)
	readlater.UseStrictClient(m)
	out, kept := readlater.Archive(m, t.Context(), 1, "https://site.example/post",
		fmt.Sprintf(`<p>字</p><img src="%s/img/photo.png" alt="内网"><img src="http://169.254.169.254/latest/meta-data" alt="元数据">`, internal.URL))
	if strings.Contains(out, "<img") || strings.Contains(out, "127.0.0.1") || strings.Contains(out, "169.254") || len(kept) != 0 {
		t.Fatalf("internal pictures must be dropped: %q %v", out, kept)
	}
}
