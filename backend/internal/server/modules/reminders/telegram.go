package reminders

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// telegramWebhookPath is the public callback path, relative to /api/v1.
const telegramWebhookPath = "/integrations/telegram/webhook"

// telegramSecretKey stores the secret_token Telegram echoes in
// X-Telegram-Bot-Api-Secret-Token.
const telegramSecretKey = "telegram.webhook_secret"

type telegramChannel struct{ m *Module }

func (c *telegramChannel) Name() string { return "telegram" }

func (c *telegramChannel) Configured(ctx context.Context) bool {
	return c.m.requiredSet(ctx, "telegram")
}

type tgButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type tgMarkup struct {
	InlineKeyboard [][]tgButton `json:"inline_keyboard"`
}

func (c *telegramChannel) Send(ctx context.Context, n notify.Stored) error {
	cfg, err := c.m.values(ctx, "telegram")
	if err != nil {
		return err
	}
	if cfg["bot_token"] == "" || cfg["chat_id"] == "" {
		return httpx.ErrIntegrationMissing
	}
	var text strings.Builder
	text.WriteString("<b>" + html.EscapeString(n.Title) + "</b>")
	if n.Body != "" {
		text.WriteString("\n" + html.EscapeString(n.Body))
	}
	msg := map[string]any{
		"chat_id":                  cfg["chat_id"],
		"text":                     text.String(),
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	var row []tgButton
	for _, a := range n.Actions {
		row = append(row, tgButton{Text: a.Label, CallbackData: a.ID})
	}
	// Telegram only accepts public https links in buttons.
	if link := c.m.publicLink(n.Link); strings.HasPrefix(link, "https://") {
		row = append(row, tgButton{Text: "打开", URL: link})
	}
	if len(row) > 0 {
		msg["reply_markup"] = tgMarkup{InlineKeyboard: [][]tgButton{row}}
	}
	return c.m.telegramCall(ctx, cfg, "sendMessage", msg)
}

// telegramCall posts a Bot API method. Errors never contain the token.
func (m *Module) telegramCall(ctx context.Context, cfg map[string]string, method string, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(cfg["api_base"], "/") + "/bot" + cfg["bot_token"] + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("telegram %s: bad request", method)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.http.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
	if !out.OK {
		if out.Description == "" {
			out.Description = resp.Status
		}
		return fmt.Errorf("telegram %s: %s", method, out.Description)
	}
	return nil
}

// telegramWebhookURL is where Telegram should send updates, or "" when the
// server has no public URL.
func (m *Module) telegramWebhookURL() string {
	if m.d.Config.PublicURL == "" {
		return ""
	}
	return m.d.Config.PublicURL + "/api/v1" + telegramWebhookPath
}

// registerTelegramWebhook calls setWebhook with a fresh secret token.
func (m *Module) registerTelegramWebhook(ctx context.Context) (string, error) {
	hook := m.telegramWebhookURL()
	if hook == "" {
		return "", httpx.Invalid("先设置 XC_PUBLIC_URL。Telegram 要能从公网访问这个地址")
	}
	cfg, err := m.values(ctx, "telegram")
	if err != nil {
		return "", err
	}
	if cfg["bot_token"] == "" {
		return "", httpx.ErrIntegrationMissing
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(buf)
	if err := m.d.Settings.SetSecret(ctx, telegramSecretKey, secret); err != nil {
		return "", err
	}
	err = m.telegramCall(ctx, cfg, "setWebhook", map[string]any{
		"url": hook, "secret_token": secret, "allowed_updates": []string{"callback_query", "message"},
	})
	if err != nil {
		return "", httpx.NewError(http.StatusBadGateway, "delivery_failed", "注册失败："+err.Error())
	}
	return hook, nil
}

// tgEntity is a piece of a message with its own meaning. A text_link hides
// its address behind other words.
type tgEntity struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type tgUpdate struct {
	Message *struct {
		MessageID int64  `json:"message_id"`
		Text      string `json:"text"`
		Caption   string `json:"caption"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Entities        []tgEntity `json:"entities"`
		CaptionEntities []tgEntity `json:"caption_entities"`
	} `json:"message"`
	CallbackQuery *struct {
		ID      string `json:"id"`
		Data    string `json:"data"`
		Message *struct {
			MessageID int64  `json:"message_id"`
			Text      string `json:"text"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			ReplyMarkup *tgMarkup `json:"reply_markup"`
		} `json:"message"`
	} `json:"callback_query"`
}

// handleTelegramUpdate verifies and runs a button press from Telegram.
func (m *Module) handleTelegramUpdate(ctx context.Context, secretHeader string, body io.Reader) error {
	var secret string
	if err := m.d.Settings.Get(ctx, telegramSecretKey, &secret); err != nil || secret == "" ||
		subtle.ConstantTimeCompare([]byte(secret), []byte(secretHeader)) != 1 {
		return httpx.NewError(http.StatusUnauthorized, "unauthorized", "Telegram 校验失败")
	}
	var upd tgUpdate
	if err := json.NewDecoder(io.LimitReader(body, 1<<20)).Decode(&upd); err != nil {
		return httpx.Invalid("请求体格式不正确")
	}
	cfg, err := m.values(ctx, "telegram")
	if err != nil {
		return err
	}
	if upd.Message != nil {
		m.handleTelegramMessage(ctx, cfg, upd)
		return nil
	}
	cq := upd.CallbackQuery
	if cq == nil {
		return nil
	}
	answer := func(text string) {
		if err := m.telegramCall(ctx, cfg, "answerCallbackQuery", map[string]any{"callback_query_id": cq.ID, "text": text}); err != nil {
			m.d.Log.Warn("telegram answerCallbackQuery", "err", err)
		}
	}
	if cq.Message == nil || strconv.FormatInt(cq.Message.Chat.ID, 10) != cfg["chat_id"] {
		answer("这个聊天没有权限")
		return nil
	}
	actx := audit.WithActor(ctx, "telegram")
	err = m.d.Notify.HandleAction(actx, cq.Data)
	m.d.Audit.Record(actx, "notify.action", cq.Data, map[string]any{"channel": "telegram"}, err)
	if err != nil {
		answer("操作失败：" + actionError(err))
		return nil
	}
	label := cq.Data
	if cq.Message.ReplyMarkup != nil {
		for _, row := range cq.Message.ReplyMarkup.InlineKeyboard {
			for _, b := range row {
				if b.CallbackData == cq.Data {
					label = b.Text
				}
			}
		}
	}
	answer("已处理")
	edit := map[string]any{
		"chat_id":    cq.Message.Chat.ID,
		"message_id": cq.Message.MessageID,
		"text":       strings.TrimSpace(cq.Message.Text) + "\n\n已处理：" + label,
	}
	if err := m.telegramCall(ctx, cfg, "editMessageText", edit); err != nil {
		m.d.Log.Warn("telegram editMessageText", "err", err)
	}
	return nil
}

// handleTelegramMessage gives a plain message from the configured chat to the
// module that takes them (B117). Other chats get no answer at all.
func (m *Module) handleTelegramMessage(ctx context.Context, cfg map[string]string, upd tgUpdate) {
	msg := upd.Message
	if strconv.FormatInt(msg.Chat.ID, 10) != cfg["chat_id"] {
		return
	}
	inbox, ok := module.Lookup[contracts.TelegramInbox](m.d.Registry, contracts.TelegramInboxKey)
	if !ok {
		return
	}
	text, entities := msg.Text, msg.Entities
	if text == "" {
		text, entities = msg.Caption, msg.CaptionEntities
	}
	var links []string
	for _, e := range entities {
		if e.Type == "text_link" && e.URL != "" {
			links = append(links, e.URL)
		}
	}
	actx := audit.WithActor(ctx, "telegram")
	reply, handled := inbox.HandleTelegramMessage(actx, contracts.TelegramMessage{Text: text, Links: links})
	if !handled || reply == "" {
		return
	}
	err := m.telegramCall(ctx, cfg, "sendMessage", map[string]any{
		"chat_id": msg.Chat.ID, "text": reply, "reply_to_message_id": msg.MessageID, "disable_web_page_preview": true,
	})
	if err != nil {
		m.d.Log.Warn("telegram reply", "err", err)
	}
}

// actionError turns an action error into a short message for the user.
func actionError(err error) string {
	var apiErr *httpx.Error
	if errors.As(err, &apiErr) {
		return apiErr.Message
	}
	return "服务器内部错误"
}
