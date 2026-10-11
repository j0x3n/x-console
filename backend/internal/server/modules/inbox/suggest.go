package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/inbox/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/inbox/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

var urlOnly = regexp.MustCompile(`^https?://\S+$`)

// onlyURL reports whether the whole text is one http(s) address.
func onlyURL(text string) (string, bool) {
	t := strings.TrimSpace(text)
	if !urlOnly.MatchString(t) {
		return "", false
	}
	if u, err := url.Parse(t); err != nil || u.Host == "" {
		return "", false
	}
	return t, true
}

var kindLabels = map[string]string{"reminder": "提醒", "card": "看板卡片", "note": "笔记", "readlater": "稍后读", "none": "拿不准"}

const suggestSystem = `你是收集箱助手。用户随手记了一句话，你判断它该归到哪里，只给建议，不要执行。
kind 取值：
- reminder：有明确时间要被提醒的事，比如“明天下午三点给医生打电话”。
- card：要做的一件事，没有明确时间，适合放进看板，比如“重构登录页”。
- note：想法、资料、要记下来的内容，不是待办。
- readlater：内容主要是一个要稍后读的网址。
- none：拿不准。
规则：
- title 简短，保留用户的原意，不要编造内容。
- body 是补充说明，没有就留空字符串。
- at 只在 reminder 时填，RFC 3339 格式并带时区偏移，按下面给的现在时间推算；没有说清楚时间就留空字符串，不要猜。
- url 只在 readlater 时填，没有就留空字符串。
- reason 用一句话说明为什么这么归。`

var suggestSchema = json.RawMessage(`{"type":"object","properties":{
"kind":{"type":"string","enum":["reminder","card","note","readlater","none"]},
"title":{"type":"string"},"body":{"type":"string"},"at":{"type":"string"},"url":{"type":"string"},"reason":{"type":"string"}},
"required":["kind","title","body","at","url","reason"],"additionalProperties":false}`)

// normalize cleans what the model returned. A time that is not RFC 3339 or is
// in the past is dropped, so the user fills it in instead of trusting it.
func (m *Module) normalize(s api.InboxSuggestion) api.InboxSuggestion {
	if _, ok := kindLabels[string(s.Kind)]; !ok {
		s.Kind = api.None
	}
	s.Title, s.Body, s.Reason = strings.TrimSpace(s.Title), strings.TrimSpace(s.Body), strings.TrimSpace(s.Reason)
	if s.Kind != api.Reminder {
		s.At = ""
	} else if t, err := time.Parse(time.RFC3339, strings.TrimSpace(s.At)); err != nil || !t.After(m.now()) {
		s.At = ""
	} else {
		s.At = t.Format(time.RFC3339)
	}
	if s.Kind != api.Readlater {
		s.Url = ""
	} else if u, ok := onlyURL(s.Url); ok {
		s.Url = u
	} else {
		s.Url = ""
	}
	return s
}

// makeSuggestion works out the suggestion for a text: directly for a bare
// address, otherwise with the AI.
func (m *Module) makeSuggestion(ctx context.Context, id int64, text string) (api.InboxSuggestion, error) {
	if u, ok := onlyURL(text); ok {
		return api.InboxSuggestion{Kind: api.Readlater, Title: u, Url: u, Reason: "内容只有一个网址"}, nil
	}
	llm, ok := module.Lookup[contracts.LLM](m.d.Registry, contracts.LLMKey)
	if !ok || !llm.Available(ctx) {
		return api.InboxSuggestion{}, httpx.NewError(http.StatusConflict, "ai_unavailable", "还没有配置 AI 供应商")
	}
	now := m.now().In(m.loc())
	user := fmt.Sprintf("现在是 %s（%s，时区 %s）。\n用户记的内容：\n%s", now.Format(time.RFC3339), weekdayZh(now.Weekday()), m.loc().String(), text)
	var s api.InboxSuggestion
	ctx = contracts.WithAIUsage(ctx, "inbox", fmt.Sprint(id))
	if err := llm.CompleteJSON(ctx, "fast", suggestSystem, user, suggestSchema, &s); err != nil {
		return api.InboxSuggestion{}, err
	}
	return m.normalize(s), nil
}

func weekdayZh(d time.Weekday) string {
	return [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}[d]
}

// suggestAsync makes the suggestion in the background.
func (m *Module) suggestAsync(id int64, announce bool) {
	m.async.Add(1)
	go func() {
		defer m.async.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if _, err := m.runSuggest(ctx, id, announce); err != nil {
			m.d.Log.Warn("inbox suggestion", "id", id, "err", err)
		}
	}()
}

// runSuggest makes and stores the suggestion for an open item.
func (m *Module) runSuggest(ctx context.Context, id int64, announce bool) (db.InboxItem, error) {
	row, err := m.item(ctx, id)
	if err != nil || row.Status != "open" {
		return row, err
	}
	s, serr := m.makeSuggestion(ctx, id, row.Text)
	state, raw := "ready", ""
	if serr != nil {
		state = "failed"
	} else {
		b, _ := json.Marshal(s)
		raw = string(b)
	}
	if err := m.q.SetInboxSuggestion(ctx, db.SetInboxSuggestionParams{SuggestState: state, Suggestion: raw, UpdatedAt: m.now().UTC(), ID: id}); err != nil {
		return row, err
	}
	m.publish()
	if serr != nil {
		return row, serr
	}
	if announce && s.Kind != api.None {
		m.announce(ctx, id, row.Text, s)
	}
	fresh, err := m.item(ctx, id)
	return fresh, err
}

// announce sends a suggestion as a notification with two buttons.
func (m *Module) announce(ctx context.Context, id int64, text string, s api.InboxSuggestion) {
	detail := kindLabels[string(s.Kind)] + "：" + s.Title
	if s.At != "" {
		if t, err := time.Parse(time.RFC3339, s.At); err == nil {
			detail += "，" + t.In(m.loc()).Format("1月2日 15:04")
		}
	}
	_, err := m.d.Notify.Send(ctx, notify.Notification{
		Kind: "inbox.suggest", Title: "收集箱：" + clip(text, 40), Body: "建议归到 " + detail, Link: "/inbox", Source: "inbox",
		Actions:   []notify.Action{{ID: fmt.Sprintf("%sfile:%d", actionPrefix, id), Label: "按建议归档"}, {ID: fmt.Sprintf("%sdismiss:%d", actionPrefix, id), Label: "忽略"}},
		Immediate: true,
	})
	if err != nil {
		m.d.Log.Warn("inbox announce", "err", err)
	}
}

func clip(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

func (m *Module) SuggestInboxItem(w http.ResponseWriter, r *http.Request, id api.InboxItemId) {
	ctx := r.Context()
	row, err := m.item(ctx, id)
	if fail(w, r, err) {
		return
	}
	if row.Status != "open" {
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "conflict", "这条已经处理过了"))
		return
	}
	if _, onlyLink := onlyURL(row.Text); !onlyLink && !m.suggestAvailable(ctx) {
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "ai_unavailable", "AI 建议没有开，或者还没有配置 AI 供应商"))
		return
	}
	fresh, err := m.runSuggest(ctx, id, false)
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "suggest_failed", "生成建议失败："+err.Error()))
		return
	}
	httpx.JSON(w, http.StatusOK, toItem(fresh))
}
