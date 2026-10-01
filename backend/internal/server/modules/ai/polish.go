package ai

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

// maxPolishRunes is the longest text POST /ai/polish accepts.
const maxPolishRunes = 20000

// polishBase is shared by every scene. The scene adds what the result is for.
const polishBase = `你是中文编辑，润色用户给的一段 Markdown：
- 改错字和不通的句子，保留原意和全部信息，不编造内容。
- 用原文的语言。中文要像母语者写的，一句一个意思，不用破折号插入语，不用比喻。
- 代码、链接、图片和附件（![...](...)、[...](...)）原样保留。
只输出润色后的完整 Markdown，不要解释，不要用代码块包起来。`

// polishScenes are the extra instructions per scene (B56).
var polishScenes = map[api.PolishScene]string{
	api.Note: `这是一篇笔记。目标是好读：
- 整理格式：合适的标题层级、列表、段落。
- 可以调整顺序让结构更清楚，但不删内容。`,
	api.Card: `这是项目看板上一张卡片的描述。目标是好读，并且专业、简洁：
- 去掉口语、重复和客套话，用行业标准术语。
- 能用列表就用列表。有步骤的写成有序列表，有待办的写成 - [ ] 清单。
- 不加原文没有的需求。`,
	api.Comment: `这是卡片下的一条评论。目标是简洁、专业、语气平和。保持简短，不要扩写。`,
	api.Task: `这是交给编码 Agent 的任务说明。整理成三段，用二级标题：
## 目标
## 要改什么
## 验收标准
原文没有提到的部分写“未说明”，不要自己编。具体的文件名、命令、报错原样保留。`,
	api.Reminder: `这是一条提醒的备注。写短，一两句说清楚要做什么。`,
	api.Event:    `这是日程的备注。写短，列出时间地点和要带的东西这类关键信息。`,
	api.General:  `整理成通顺、清楚、简洁的文字。`,
}

// polishPrompt is the system prompt for a scene and an optional user request.
func polishPrompt(scene api.PolishScene, request string) (string, bool) {
	extra, ok := polishScenes[scene]
	if !ok {
		return "", false
	}
	system := polishBase + "\n\n" + extra
	if request = strings.TrimSpace(request); request != "" {
		system += "\n\n这次用户的要求（优先按它来）：" + request
	}
	return system, true
}

// PolishText is POST /ai/polish (B56).
func (m *Module) PolishText(w http.ResponseWriter, r *http.Request) {
	var in api.PolishRequest
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	text := strings.TrimSpace(in.Text)
	switch {
	case text == "":
		httpx.Fail(w, r, httpx.Invalid("内容是空的"))
		return
	case utf8.RuneCountInString(text) > maxPolishRunes:
		httpx.Fail(w, r, httpx.Invalid("内容太长，最多 20000 字"))
		return
	}
	request := ""
	if in.Prompt != nil {
		request = *in.Prompt
	}
	system, ok := polishPrompt(in.Scene, request)
	if !ok {
		httpx.Fail(w, r, httpx.Invalid("不认识这个场景"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	out, err := m.CompleteText(contracts.WithAIUsage(ctx, "polish", string(in.Scene)), "fast", system, text)
	if errors.Is(err, llm.ErrNotConfigured) {
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "ai_not_configured", "请先在设置 → AI 里配置模型"))
		return
	}
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "ai_failed", "AI 润色失败："+err.Error()))
		return
	}
	httpx.JSON(w, http.StatusOK, api.PolishResult{Text: stripFence(out)})
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
