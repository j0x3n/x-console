package inbox

import (
	"context"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/inbox/api"
)

// HandleTelegramMessage takes the plain messages the read-later module left
// alone: text without a link, and voice messages (B134).
func (m *Module) HandleTelegramMessage(ctx context.Context, msg contracts.TelegramMessage) (string, bool) {
	text := strings.TrimSpace(msg.Text)
	source := "telegram"
	if msg.Voice != nil {
		source = "telegram_voice"
		if msg.Voice.Seconds > maxVoiceSecs {
			return "语音太长了，最多 5 分钟。", true
		}
		if msg.Download == nil {
			return "", false
		}
		audio, err := msg.Download(ctx)
		if err != nil {
			return "收不到这段语音：" + err.Error(), true
		}
		spoken, err := m.transcribe(ctx, audio, msg.Voice.Mime, "")
		if err != nil {
			return "语音没转成文字：" + errMessage(err), true
		}
		text = strings.TrimSpace(spoken + "\n" + text)
	}
	if text == "" || strings.HasPrefix(text, "/") {
		return "", false
	}
	row, err := m.capture(ctx, text, source, true)
	if err != nil {
		return "没收进收集箱：" + errMessage(err), true
	}
	return "已收进收集箱：" + clip(row.Text, 30), true
}

func errMessage(err error) string {
	if e, ok := err.(*httpx.Error); ok {
		return e.Message
	}
	return err.Error()
}

// handleAction runs the buttons of the suggestion notification.
func (m *Module) handleAction(ctx context.Context, id string) error {
	rest := strings.TrimPrefix(id, actionPrefix)
	verb, num, ok := strings.Cut(rest, ":")
	n, err := strconv.ParseInt(num, 10, 64)
	if !ok || err != nil {
		return httpx.Invalid("不认识的按钮")
	}
	switch verb {
	case "dismiss":
		_, err := m.dismiss(ctx, n)
		return err
	case "file":
		row, err := m.item(ctx, n)
		if err != nil {
			return err
		}
		item := toItem(row)
		if item.Suggestion == nil || item.Suggestion.Kind == api.None {
			return httpx.Invalid("这条没有建议，去面板里选")
		}
		s := item.Suggestion
		in := api.InboxFileInput{Kind: s.Kind, Title: &s.Title, Body: &s.Body, At: &s.At, Url: &s.Url}
		if s.Kind == api.Reminder && s.At == "" {
			return httpx.Invalid("建议里没有时间，去面板里补")
		}
		if s.Kind == api.Card && m.options(ctx).CardProjectId == 0 {
			return httpx.Invalid("看板卡片要选项目，去面板里归档，或在设置里选默认项目")
		}
		_, err = m.file(ctx, n, in)
		return err
	}
	return httpx.Invalid("不认识的按钮")
}
