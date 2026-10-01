package notes

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
)

// B40: the editor asks for these on the user's click. They only return
// text; the editor decides whether to use it. The user sends the body
// explicitly, so this works for hidden notes too.

const maxPolishRunes = 20000

var errAINotConfigured = httpx.NewError(http.StatusConflict, "ai_not_configured", "请先在设置里配置 AI 模型")

func (m *Module) llmClient(ctx context.Context) (contracts.LLM, error) {
	client, ok := module.Lookup[contracts.LLM](m.d.Registry, contracts.LLMKey)
	if !ok || !client.Available(ctx) {
		return nil, errAINotConfigured
	}
	return client, nil
}

func noteText(body string, limit int) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", httpx.Invalid("正文是空的")
	}
	if utf8.RuneCountInString(body) > limit {
		return "", httpx.Invalid("正文太长，最多 20000 字")
	}
	return body, nil
}

const polishSystem = `你是笔记编辑。把用户的 Markdown 笔记润色得通顺、清楚、好读：
- 改错字和不通的句子，保留原意和所有信息，不编造内容。
- 整理格式：合适的标题层级、列表、段落，代码和链接保持原样。
- 图片和附件链接（![...](...)、[...](...)）原样保留。
- 用笔记原来的语言。
只输出润色后的完整 Markdown 正文，不要解释，不要用代码块包起来。`

func (m *Module) PolishNoteBody(w http.ResponseWriter, r *http.Request) {
	var in api.PolishNoteBodyJSONBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	body, err := noteText(in.Body, maxPolishRunes)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	client, err := m.llmClient(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	system := polishSystem
	if in.Prompt != nil && strings.TrimSpace(*in.Prompt) != "" {
		system += "\n这次用户的要求（优先按它来）：" + strings.TrimSpace(*in.Prompt)
	}
	out, err := client.CompleteText(contracts.WithAIUsage(r.Context(), "notes", ""), "fast", system, body)
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "ai_failed", "AI 润色失败："+err.Error()))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"body": stripFence(out)})
}

// stripFence removes a ```markdown fence some models add despite being told.
func stripFence(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") || !strings.HasSuffix(text, "```") {
		return text
	}
	first := strings.IndexByte(text, '\n')
	if first < 0 {
		return text
	}
	return strings.TrimSpace(text[first+1 : len(text)-3])
}

func (m *Module) SuggestNoteTitle(w http.ResponseWriter, r *http.Request) {
	var in api.SuggestNoteTitleJSONBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	body, err := noteText(in.Body, maxPolishRunes)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	client, err := m.llmClient(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var answer struct {
		Title string `json:"title"`
	}
	schema := json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}},"required":["title"],"additionalProperties":false}`)
	if err := client.CompleteJSON(contracts.WithAIUsage(r.Context(), "notes", ""), "fast", "给笔记起一个 20 字以内的标题，用笔记的语言。只返回 JSON。", clip(body, maxNoteAIInput), schema, &answer); err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "ai_failed", "生成标题失败："+err.Error()))
		return
	}
	title := strings.TrimSpace(answer.Title)
	if runes := []rune(title); len(runes) > 20 {
		title = string(runes[:20])
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"title": title})
}

func (m *Module) SuggestNoteTags(w http.ResponseWriter, r *http.Request) {
	var in api.SuggestNoteTagsJSONBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	body, err := noteText(in.Body, maxPolishRunes)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	client, err := m.llmClient(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	existing, err := m.tagCounts(r.Context(), false)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	names := make([]string, 0, min(len(existing), 200))
	for _, tag := range existing[:min(len(existing), 200)] {
		names = append(names, tag.Tag)
	}
	var answer struct {
		Tags []string `json:"tags"`
	}
	schema := json.RawMessage(`{"type":"object","properties":{"tags":{"type":"array","items":{"type":"string"},"maxItems":3}},"required":["tags"],"additionalProperties":false}`)
	system := "给笔记挑最多 3 个标签。尽量从已有标签中选，只有很确定时才新建。只返回 JSON。已有标签：" + strings.Join(names, "、")
	if err := client.CompleteJSON(contracts.WithAIUsage(r.Context(), "notes", ""), "fast", system, clip(body, maxNoteAIInput), schema, &answer); err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "ai_failed", "生成标签失败："+err.Error()))
		return
	}
	tags, err := cleanTags(answer.Tags)
	if err != nil {
		tags = nil
	}
	if len(tags) > 3 {
		tags = tags[:3]
	}
	if tags == nil {
		tags = []string{}
	}
	httpx.JSON(w, http.StatusOK, map[string][]string{"tags": tags})
}

func clip(text string, limit int) string {
	if runes := []rune(text); len(runes) > limit {
		return string(runes[:limit])
	}
	return text
}
