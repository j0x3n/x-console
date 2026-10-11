package cards

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/cards/api"
)

// The AI only drafts a card. The text the user selected goes to the AI
// provider the user set up under Settings → AI. It can be switched off in the
// card settings; hand-written cards never need it.

const draftSchema = `{"type":"object","properties":{"question":{"type":"string"},"answer":{"type":"string"}},"required":["question","answer"],"additionalProperties":false}`

const qaPrompt = `你帮用户把一段文字做成一张记忆复习卡片。
- 只问一个知识点。问题要具体，看到问题就知道要答什么，不能问“这段讲了什么”。
- 答案短，一两句话，能直接核对对错。
- 用原文的语言，不编造原文没有的内容。
- 问题和答案都是纯文本，不要加“问：”“答：”这样的前缀。`

const clozePrompt = `你帮用户把一段文字做成一张填空记忆卡片。
- question 是原文里的一句话，把一个关键词或短语换成 ____（四个下划线）。只挖一处。
- answer 是被挖掉的词或短语，不带其他字。
- 选最值得记住的那个词，不要挖虚词。
- 用原文的语言，不改原句的意思。`

// aiAvailable says whether an AI model is set up. The cards switch is not
// part of it.
func (m *Module) aiAvailable(ctx context.Context) bool {
	client, ok := module.Lookup[contracts.LLM](m.d.Registry, contracts.LLMKey)
	return ok && client.Available(ctx)
}

// generate asks the AI for a draft.
func (m *Module) generate(ctx context.Context, in api.CardGenerateInput) (api.CardDraft, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return api.CardDraft{}, httpx.Invalid("没有选中文字")
	}
	if utf8.RuneCountInString(text) > maxText {
		return api.CardDraft{}, httpx.Invalid("选中的文字太长，最多 4000 字")
	}
	if !m.options(ctx).AIEnabled {
		return api.CardDraft{}, httpx.NewError(http.StatusConflict, "ai_disabled", "做卡片的 AI 已在设置里关掉")
	}
	client, ok := module.Lookup[contracts.LLM](m.d.Registry, contracts.LLMKey)
	if !ok || !client.Available(ctx) {
		return api.CardDraft{}, httpx.NewError(http.StatusConflict, "ai_not_configured", "请先在设置 → AI 里配置模型")
	}
	system := qaPrompt
	if in.Style != nil && *in.Style == api.Cloze {
		system = clozePrompt
	}
	user := text
	if title := strings.TrimSpace(deref(in.SourceTitle)); title != "" {
		user = "来源：" + title + "\n\n" + text
	}
	ctx, cancel := context.WithTimeout(contracts.WithAIUsage(ctx, "cards", ""), 60*time.Second)
	defer cancel()
	var draft api.CardDraft
	if err := client.CompleteJSON(ctx, "fast", system, user, json.RawMessage(draftSchema), &draft); err != nil {
		return api.CardDraft{}, httpx.NewError(http.StatusBadGateway, "ai_failed", "AI 出卡片失败："+err.Error())
	}
	draft.Question = strings.TrimSpace(draft.Question)
	draft.Answer = strings.TrimSpace(draft.Answer)
	if draft.Question == "" || draft.Answer == "" {
		return api.CardDraft{}, httpx.NewError(http.StatusBadGateway, "ai_failed", "AI 没有给出完整的卡片，请再试一次")
	}
	return draft, nil
}
