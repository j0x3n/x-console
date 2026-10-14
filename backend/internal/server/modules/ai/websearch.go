package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Web search (B211): other modules ask for facts from the internet through
// contracts.WebSearch. The mode is off, openai (the provider's own search
// tool on the Responses API) or brave (Brave Search plus our own page reader).

const (
	webSearchModeKey = "ai.web_search_mode"
	braveKeySetting  = "ai.brave_key"

	modeOff    = "off"
	modeOpenAI = "openai"
	modeBrave  = "brave"

	braveTimeout        = 15 * time.Second
	braveResultsPerTerm = 5
	maxSearchTerms      = 3
	maxSearchTermRunes  = 200
	defaultMaxSources   = 4
	limitMaxSources     = 8
	// pageRunes is how much text of one page goes to the model.
	pageRunes = 6000
	// testTask is what the test button searches for.
	testTask = "西红柿炒鸡蛋 做法"
)

// braveSearchURL is the Brave Search endpoint. Tests point it at a local server.
var braveSearchURL = "https://api.search.brave.com/res/v1/web/search"

var (
	errBraveAuth   = errors.New("Brave 的密钥不对或者过期了")
	errBraveQuota  = errors.New("Brave 搜索次数用完了")
	errNoResults   = errors.New("搜索没有找到结果，换个说法试试")
	errBadResponse = errors.New("AI 返回的内容不是正确的 JSON")
)

const webSearchStyleMessage = "OpenAI 自带的联网搜索要用 Responses 接口的服务商"

// webSearch implements contracts.WebSearch on top of the module.
type webSearch struct{ m *Module }

var _ contracts.WebSearch = webSearch{}

// ---------- settings ----------

func (m *Module) webSearchMode(ctx context.Context) string {
	var mode string
	if err := m.d.Settings.Get(ctx, webSearchModeKey, &mode); err != nil {
		return modeOff
	}
	switch mode {
	case modeOpenAI, modeBrave:
		return mode
	}
	return modeOff
}

func (m *Module) braveKey(ctx context.Context) (string, error) {
	var key string
	err := m.d.Settings.Get(ctx, braveKeySetting, &key)
	if errors.Is(err, settings.ErrNotSet) {
		return "", nil
	}
	return key, err
}

func (m *Module) webSearchSettings(ctx context.Context) (api.AiWebSearch, error) {
	has, err := m.d.Settings.Has(ctx, braveKeySetting)
	return api.AiWebSearch{Mode: api.AiWebSearchMode(m.webSearchMode(ctx)), BraveKeySet: has}, err
}

// GetAiWebSearch implements api.ServerInterface.
func (m *Module) GetAiWebSearch(w http.ResponseWriter, r *http.Request) {
	out, err := m.webSearchSettings(r.Context())
	if m.fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// PutAiWebSearch implements api.ServerInterface.
func (m *Module) PutAiWebSearch(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	var body api.AiWebSearchInput
	if m.fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	err := m.saveWebSearch(ctx, body)
	changed := body.BraveKey != nil
	var mode any
	if body.Mode != nil {
		mode = string(*body.Mode)
	}
	m.d.Audit.Record(ctx, "ai.web_search.update", "", map[string]any{"mode": mode, "keyChanged": changed}, err)
	if m.fail(w, r, err) {
		return
	}
	out, err := m.webSearchSettings(ctx)
	if m.fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// saveWebSearch checks everything first, then writes, so a refused body
// changes nothing.
func (m *Module) saveWebSearch(ctx context.Context, in api.AiWebSearchInput) error {
	if in.Mode != nil && !(*in.Mode == modeOff || *in.Mode == modeOpenAI || *in.Mode == modeBrave) {
		return httpx.Invalid("联网搜索的方式只能是 off、openai 或 brave")
	}
	mode := m.webSearchMode(ctx)
	if in.Mode != nil {
		mode = string(*in.Mode)
	}
	keySet, err := m.d.Settings.Has(ctx, braveKeySetting)
	if err != nil {
		return err
	}
	var key string
	if in.BraveKey != nil {
		key = strings.TrimSpace(*in.BraveKey)
		keySet = key != ""
	}
	if mode == modeBrave && !keySet {
		return httpx.Invalid("用 Brave Search 要先填 Brave 的密钥")
	}
	if mode == modeOpenAI && in.Mode != nil {
		if err := m.requireResponsesStyle(ctx); err != nil {
			return err
		}
	}
	if in.BraveKey != nil {
		if key == "" {
			err = m.d.Settings.Delete(ctx, braveKeySetting)
		} else {
			err = m.d.Settings.SetSecret(ctx, braveKeySetting, key)
		}
		if err != nil {
			return err
		}
	}
	if in.Mode != nil {
		return m.d.Settings.Set(ctx, webSearchModeKey, mode)
	}
	return nil
}

// requireResponsesStyle checks that the provider of the main model uses the
// Responses API, the only one with a web search tool.
func (m *Module) requireResponsesStyle(ctx context.Context) error {
	sel, err := m.modelSettings(ctx)
	if err != nil {
		return err
	}
	if sel.Agent == nil {
		return httpx.Invalid("先选好主模型。" + webSearchStyleMessage)
	}
	p, err := m.provider(ctx, sel.Agent.ProviderId)
	if errors.Is(err, httpx.ErrNotFound) {
		return httpx.Invalid("先选好主模型。" + webSearchStyleMessage)
	}
	if err != nil {
		return err
	}
	if p.ApiStyle != api.Responses {
		return httpx.Invalid(webSearchStyleMessage)
	}
	return nil
}

// TestAiWebSearch implements api.ServerInterface. It searches once with the
// current settings and gives back the first three pages.
func (m *Module) TestAiWebSearch(w http.ResponseWriter, r *http.Request) {
	ctx := contracts.WithAIUsage(r.Context(), "web_search", "test")
	sources, err := m.testWebSearch(ctx)
	if err != nil {
		httpx.Fail(w, r, webSearchHTTPError(err))
		return
	}
	if len(sources) > 3 {
		sources = sources[:3]
	}
	out := make([]api.AiWebSource, len(sources))
	for i, s := range sources {
		out[i] = api.AiWebSource{Title: s.Title, Url: s.URL}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"sources": out})
}

func (m *Module) testWebSearch(ctx context.Context) ([]contracts.WebSource, error) {
	switch m.webSearchMode(ctx) {
	case modeBrave:
		// Only Brave is called: the test should not cost model tokens.
		key, err := m.braveKey(ctx)
		if err != nil {
			return nil, err
		}
		found, err := braveSearch(ctx, key, testTask, 3)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, errNoResults
		}
		out := make([]contracts.WebSource, len(found))
		for i, f := range found {
			out[i] = contracts.WebSource{Title: f.Title, URL: f.URL}
		}
		return out, nil
	case modeOpenAI:
		var answer struct {
			Summary string `json:"summary"`
		}
		return webSearch{m}.Research(ctx, contracts.WebResearch{
			Purpose: "ai.web_search_test",
			System:  "你是搜索助手。上网搜索，用一句话回答。",
			Task:    testTask,
			Schema:  json.RawMessage(`{"type":"object","properties":{"summary":{"type":"string"}},"required":["summary"],"additionalProperties":false}`),
		}, &answer)
	}
	return nil, contracts.ErrWebSearchOff
}

// webSearchHTTPError turns a research error into one the page can show.
func webSearchHTTPError(err error) error {
	var he *httpx.Error
	switch {
	case errors.As(err, &he):
		return err
	case errors.Is(err, contracts.ErrWebSearchOff):
		return httpx.NewError(http.StatusNotImplemented, "feature_unavailable", err.Error())
	case errors.Is(err, llm.ErrWebSearchStyle):
		return httpx.NewError(http.StatusBadRequest, "web_search_style", err.Error())
	}
	return httpx.NewError(http.StatusBadGateway, "web_search_failed", "联网搜索失败："+err.Error())
}

// ---------- contracts.WebSearch ----------

// Mode implements contracts.WebSearch.
func (w webSearch) Mode(ctx context.Context) string { return w.m.webSearchMode(ctx) }

// Research implements contracts.WebSearch.
func (w webSearch) Research(ctx context.Context, in contracts.WebResearch, out any) ([]contracts.WebSource, error) {
	m := w.m
	if strings.TrimSpace(in.Task) == "" || len(in.Schema) == 0 {
		return nil, errors.New("联网搜索要有任务和返回格式")
	}
	if in.MaxSources <= 0 {
		in.MaxSources = defaultMaxSources
	}
	if in.MaxSources > limitMaxSources {
		in.MaxSources = limitMaxSources
	}
	// The usage record keeps the caller's source and takes the purpose as the reference.
	use := contracts.AIUsageFrom(ctx)
	if use.Source == "" {
		use.Source = "web_search"
	}
	if in.Purpose != "" {
		use.Ref = in.Purpose
	}
	ctx = contracts.WithAIUsage(ctx, use.Source, use.Ref)
	switch m.webSearchMode(ctx) {
	case modeOpenAI:
		return m.researchOpenAI(ctx, in, out)
	case modeBrave:
		return m.researchBrave(ctx, in, out)
	}
	return nil, contracts.ErrWebSearchOff
}

func fillJSON(text string, out any) error {
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return fmt.Errorf("%w：%v", errBadResponse, err)
	}
	return nil
}

// researchOpenAI is one Responses call with the web search tool and the schema.
func (m *Module) researchOpenAI(ctx context.Context, in contracts.WebResearch, out any) ([]contracts.WebSource, error) {
	result, err := m.llm.Complete(ctx, llm.Request{
		Purpose: "agent", CacheKey: cacheKey(ctx, "agent"), WebSearch: true,
		System:     in.System + "\n先上网搜索，再按要求回答。",
		Messages:   []llm.Message{{Role: "user", Content: in.Task}},
		JSONSchema: in.Schema,
	})
	if err != nil {
		return nil, err
	}
	if result.Truncated {
		return nil, errTruncated
	}
	if err := fillJSON(result.Text, out); err != nil {
		return nil, err
	}
	sources := make([]contracts.WebSource, len(result.Sources))
	for i, s := range result.Sources {
		sources[i] = contracts.WebSource{Title: s.Title, URL: s.URL}
	}
	return sources, nil
}

// ---------- Brave ----------

type braveResult struct{ Title, URL, Snippet string }

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// braveSearch asks Brave for the first count results of one search term.
func braveSearch(ctx context.Context, key, term string, count int) ([]braveResult, error) {
	ctx, cancel := context.WithTimeout(ctx, braveTimeout)
	defer cancel()
	q := url.Values{"q": {term}, "count": {fmt.Sprint(count)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, braveSearchURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Subscription-Token", key)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("Brave 搜索超时了")
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("连不上 Brave 搜索：%v", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, errBraveAuth
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, errBraveQuota
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("Brave 搜索失败，状态码 %d", resp.StatusCode)
	}
	var body struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, errors.New("Brave 搜索的结果看不懂")
	}
	var out []braveResult
	for _, r := range body.Web.Results {
		u, err := url.Parse(r.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}
		out = append(out, braveResult{Title: cleanSnippet(r.Title), URL: r.URL, Snippet: cleanSnippet(r.Description)})
	}
	return out, nil
}

func cleanSnippet(s string) string {
	s = htmlTag.ReplaceAllString(s, "")
	s = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#x27;", "'", "&#39;", "'").Replace(s)
	return strings.TrimSpace(s)
}

const searchTermsSystem = `你是搜索助手。根据任务写 2 到 3 个适合搜索引擎的搜索词，只返回 JSON。
- 每个词从不同角度找，不要重复。
- 用简短的关键词，不要写成句子。`

const searchTermsSchema = `{"type":"object","properties":{"queries":{"type":"array","items":{"type":"string"}}},"required":["queries"],"additionalProperties":false}`

// searchTerms asks the fast model for the words to search.
func (m *Module) searchTerms(ctx context.Context, task string) ([]string, error) {
	var answer struct {
		Queries []string `json:"queries"`
	}
	if err := m.CompleteJSON(ctx, "fast", searchTermsSystem, task, json.RawMessage(searchTermsSchema), &answer); err != nil {
		return nil, err
	}
	var terms []string
	seen := map[string]bool{}
	for _, t := range answer.Queries {
		t = strings.TrimSpace(t)
		if r := []rune(t); len(r) > maxSearchTermRunes {
			t = string(r[:maxSearchTermRunes])
		}
		if k := strings.ToLower(t); t != "" && !seen[k] && len(terms) < maxSearchTerms {
			seen[k] = true
			terms = append(terms, t)
		}
	}
	if len(terms) == 0 {
		// The model gave nothing usable: search the task itself.
		t := strings.TrimSpace(task)
		if r := []rune(t); len(r) > maxSearchTermRunes {
			t = string(r[:maxSearchTermRunes])
		}
		terms = []string{t}
	}
	return terms, nil
}

func sameURL(u string) string { return strings.ToLower(strings.TrimRight(u, "/")) }

// researchBrave: the fast model writes the search terms, Brave answers, the
// first pages are read, and the main model writes the answer from them.
func (m *Module) researchBrave(ctx context.Context, in contracts.WebResearch, out any) ([]contracts.WebSource, error) {
	key, err := m.braveKey(ctx)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, errBraveAuth
	}
	terms, err := m.searchTerms(ctx, in.Task)
	if err != nil {
		return nil, fmt.Errorf("写搜索词失败：%w", err)
	}
	var found []braveResult
	seen := map[string]bool{}
	var lastErr error
	for _, term := range terms {
		results, err := braveSearch(ctx, key, term, braveResultsPerTerm)
		if errors.Is(err, errBraveAuth) || errors.Is(err, errBraveQuota) {
			return nil, err
		}
		if err != nil {
			lastErr = err
			continue
		}
		for _, r := range results {
			if k := sameURL(r.URL); !seen[k] {
				seen[k] = true
				found = append(found, r)
			}
		}
	}
	if len(found) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, errNoResults
	}
	if len(found) > in.MaxSources {
		found = found[:in.MaxSources]
	}

	// Read the pages side by side; one that cannot be read is skipped.
	texts := make([]string, len(found))
	read := make([]bool, len(found))
	if reader, ok := module.Lookup[contracts.PageReader](m.d.Registry, contracts.PageReaderKey); ok {
		var wg sync.WaitGroup
		for i, f := range found {
			wg.Add(1)
			go func() {
				defer wg.Done()
				page, err := reader.Read(ctx, f.URL)
				if text := clipText(page.Text, pageRunes); err == nil && text != "" {
					texts[i], read[i] = text, true
					if page.Title != "" {
						found[i].Title = page.Title
					}
				}
			}()
		}
		wg.Wait()
	}
	var sources []contracts.WebSource
	var material strings.Builder
	material.WriteString("任务：" + in.Task + "\n\n下面是搜到的网页。只依据这些内容回答，不要编造里面没有的信息。\n")
	anyRead := false
	for i := range found {
		if read[i] {
			anyRead = true
		}
	}
	for i, f := range found {
		// With no readable page at all the answer rests on the snippets,
		// and every result counts as a source.
		if anyRead && !read[i] {
			continue
		}
		sources = append(sources, contracts.WebSource{Title: f.Title, URL: f.URL})
		fmt.Fprintf(&material, "\n[%d] %s\n地址：%s\n摘要：%s\n", len(sources), f.Title, f.URL, f.Snippet)
		if read[i] {
			material.WriteString("正文：\n" + texts[i] + "\n")
		}
	}
	system := in.System + "\n只能依据用户给出的网页内容回答。网页里没有的信息不要编。"
	if err := m.CompleteJSON(ctx, "agent", system, material.String(), in.Schema, out); err != nil {
		return nil, err
	}
	return sources, nil
}

func clipText(s string, max int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		return strings.TrimSpace(string(r[:max]))
	}
	return s
}
