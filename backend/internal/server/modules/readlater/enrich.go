package readlater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/readlater/db"
)

const (
	maxAIInput   = 12000
	maxSummary   = 3 // lines
	maxSumRunes  = 400
	summarySchem = `{"type":"object","properties":{"summary":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}}},"required":["summary","tags"],"additionalProperties":false}`
	summarySys   = "你帮用户整理收藏的文章。用中文写摘要：最多三行，每行一句话，说清文章讲什么、结论是什么。" +
		"再给 3 到 5 个标签，每个不超过 6 个字，优先从已有标签里选，没有合适的再新起。只返回 JSON。"
)

var (
	errAINotConfigured = httpx.NewError(http.StatusConflict, "ai_not_configured", "请先在设置里配置 AI 模型")
	errNoContent       = httpx.NewError(http.StatusConflict, "no_content", "还没有抓到正文，没法写摘要")
)

// summarize asks the AI for a short summary and tags. The tags it finds are
// added to the ones the item already has; a summary replaces the old one.
func (m *Module) summarize(ctx context.Context, row db.ReadItem) (db.ReadItem, error) {
	client, ok := module.Lookup[contracts.LLM](m.d.Registry, contracts.LLMKey)
	if !ok || !client.Available(ctx) {
		return row, errAINotConfigured
	}
	text := row.Content
	if text == "" {
		text = row.Excerpt
	}
	if text == "" {
		return row, errNoContent
	}
	var b strings.Builder
	fmt.Fprintf(&b, "标题：%s\n网址：%s\n", row.Title, row.Url)
	if tags := m.topTagNames(ctx); len(tags) > 0 {
		b.WriteString("已有标签：" + strings.Join(tags, "、") + "\n")
	}
	b.WriteString("\n正文：\n" + clipRunes(text, maxAIInput))
	var out struct {
		Summary string   `json:"summary"`
		Tags    []string `json:"tags"`
	}
	ai := contracts.WithAIUsage(ctx, "readlater", strconv.FormatInt(row.ID, 10))
	if err := client.CompleteJSON(ai, "fast", summarySys, b.String(), json.RawMessage(summarySchem), &out); err != nil {
		return row, err
	}
	summary := tidySummary(out.Summary)
	tags := normalizeTags(append(parseTags(row.TagsJson), out.Tags...))
	raw, _ := json.Marshal(tags)
	if err := m.q.SaveReadSummary(ctx, db.SaveReadSummaryParams{Summary: summary, TagsJson: string(raw), UpdatedAt: m.now().UTC(), ID: row.ID}); err != nil {
		return row, err
	}
	return m.get(ctx, row.ID)
}

// tidySummary keeps at most three non-empty lines.
func tidySummary(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
		if len(lines) == maxSummary {
			break
		}
	}
	return clipRunes(strings.Join(lines, "\n"), maxSumRunes)
}
