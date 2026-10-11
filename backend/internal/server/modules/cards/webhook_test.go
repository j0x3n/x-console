package cards_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/cards"
	"github.com/j0x3n/x-console/backend/internal/server/modules/cards/api"
)

// A fake Telegram Bot API that records what is sent to it.
type fakeBotAPI struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []botRequest
}

type botRequest struct{ Method, Body string }

func newFakeBotAPI(t *testing.T) *fakeBotAPI {
	t.Helper()
	f := &fakeBotAPI{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, botRequest{Method: path.Base(req.URL.Path), Body: string(raw)})
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeBotAPI) find(method string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.reqs {
		if r.Method == method {
			out = append(out, r.Body)
		}
	}
	return out
}

// The whole Telegram review through the real webhook, the real Telegram
// channel and a fake Bot API.
func TestTelegramWebhookReview(t *testing.T) {
	r := setup(t)
	ctx := context.Background()
	bot := newFakeBotAPI(t)
	set := r.env.App.Deps.Settings
	for key, v := range map[string]string{"telegram.chat_id": "42", "telegram.api_base": bot.URL} {
		if err := set.Set(ctx, key, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := set.SetSecret(ctx, "telegram.bot_token", "123:TOKEN"); err != nil {
		t.Fatal(err)
	}
	if err := set.SetSecret(ctx, "telegram.webhook_secret", "hook-secret"); err != nil {
		t.Fatal(err)
	}
	card := r.create(t, "Go 的零值是什么？", "变量声明后没赋值时的默认值。")

	press := func(chat int, data string) int {
		update := map[string]any{"update_id": 1, "callback_query": map[string]any{
			"id": "cb", "data": data,
			"message": map[string]any{"message_id": 5, "text": "旧消息", "chat": map[string]any{"id": chat}},
		}}
		raw, _ := json.Marshal(update)
		req, _ := http.NewRequest(http.MethodPost, r.env.URL("/integrations/telegram/webhook"), bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "hook-secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// The daily notice is stored and has the "开始" button.
	r.at(20, 0)
	if err := cards.PushJob(r.m, ctx); err != nil {
		t.Fatal(err)
	}
	if n := r.notices(t); len(n) != 1 {
		t.Fatalf("push: %+v", n)
	}

	// The notice also goes to Telegram, with the "开始" button. Delivery runs
	// in the background.
	for i := 0; i < 100 && len(bot.find("sendMessage")) == 0; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if first := bot.find("sendMessage"); len(first) != 1 || !strings.Contains(first[0], "cards.start") || !strings.Contains(first[0], "今天有 1 张卡待复习") {
		t.Fatalf("daily notice in Telegram: %v", first)
	}

	// Another chat cannot start a review.
	if press(7, "cards.start") != http.StatusOK || len(bot.find("sendMessage")) != 1 {
		t.Fatal("a foreign chat started a review")
	}
	if press(42, "cards.start") != http.StatusOK {
		t.Fatal("webhook failed")
	}
	sent := bot.find("sendMessage")[1:]
	if len(sent) != 1 || !strings.Contains(sent[0], "Go 的零值是什么？") || !strings.Contains(sent[0], fmt.Sprintf("cards.show:%d", card.Id)) || strings.Contains(sent[0], "变量声明后") {
		t.Fatalf("question message: %v", sent)
	}
	if edits := bot.find("editMessageText"); len(edits) != 1 || !strings.Contains(edits[0], "已处理") {
		t.Fatalf("the old message must be marked as handled: %v", edits)
	}

	press(42, fmt.Sprintf("cards.show:%d", card.Id))
	sent = bot.find("sendMessage")[1:]
	if len(sent) != 2 || !strings.Contains(sent[1], "变量声明后") || strings.Count(sent[1], "cards.rate:") != 4 {
		t.Fatalf("answer message: %v", sent)
	}
	rate := fmt.Sprintf("cards.rate:%d:0:3", card.Id)
	press(42, rate)
	press(42, rate) // pressed twice
	var c api.Card
	r.env.MustDo(http.MethodGet, fmt.Sprintf("/cards/%d", card.Id), nil, &c)
	if c.Reps != 1 {
		t.Fatalf("a rating pressed twice counted %d times", c.Reps)
	}
	sent = bot.find("sendMessage")[1:]
	if len(sent) != 3 || !strings.Contains(sent[2], "复习完了") {
		t.Fatalf("closing message: %v", sent)
	}
	answers := bot.find("answerCallbackQuery")
	if len(answers) != 5 || !strings.Contains(answers[len(answers)-1], "这张卡已经答过了") {
		t.Fatalf("callback answers: %v", answers)
	}
}
