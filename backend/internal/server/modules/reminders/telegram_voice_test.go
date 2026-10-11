package reminders_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

// speechFake is the speech-to-text boundary.
type speechFake struct {
	mu    sync.Mutex
	audio [][]byte
}

func (s *speechFake) Available(context.Context) bool { return true }
func (s *speechFake) Transcribe(_ context.Context, in contracts.Transcribe) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audio = append(s.audio, in.Audio)
	return "明天买牛奶", nil
}

// B134: a voice message goes to Telegram's getFile, the file is downloaded,
// turned into text and kept by the capture inbox.
func TestTelegramVoiceReachesTheInbox(t *testing.T) {
	env, _ := setup(t)
	ctx := context.Background()
	tg := newFake(t, func(path string) (int, string) {
		switch {
		case strings.HasSuffix(path, "/getFile"):
			return 200, `{"ok":true,"result":{"file_id":"f1","file_path":"voice/f1.oga","file_size":8}}`
		case strings.Contains(path, "/file/bot"):
			return 200, "OGG-DATA"
		}
		return 200, `{"ok":true,"result":{}}`
	})
	configureTelegram(t, env, tg)
	if err := env.App.Deps.Settings.SetSecret(ctx, "telegram.webhook_secret", "hook-secret"); err != nil {
		t.Fatal(err)
	}
	speech := &speechFake{}
	module.Provide[contracts.Transcriber](env.App.Deps.Registry, contracts.TranscriberKey, speech)

	post := func(chat int, msg map[string]any) {
		msg["message_id"], msg["chat"] = 9, map[string]any{"id": chat}
		raw, _ := json.Marshal(map[string]any{"update_id": 1, "message": msg})
		req, _ := http.NewRequest(http.MethodPost, env.URL("/integrations/telegram/webhook"), bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "hook-secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	inboxItems := func() []struct{ Text, Source string } {
		var out struct{ Items []struct{ Text, Source string } }
		env.MustDo(http.MethodGet, "/inbox", nil, &out)
		return out.Items
	}

	// Another chat is ignored: no download, no item.
	post(7, map[string]any{"voice": map[string]any{"file_id": "f1", "duration": 3, "mime_type": "audio/ogg"}})
	if len(inboxItems()) != 0 || len(tg.find("/getFile")) != 0 {
		t.Fatal("a foreign chat must not reach the inbox")
	}

	post(42, map[string]any{"voice": map[string]any{"file_id": "f1", "duration": 3, "mime_type": "audio/ogg", "file_size": 8}})
	waitFor(t, "reply", func() bool { return len(tg.find("/sendMessage")) == 1 })
	if body := tg.find("/sendMessage")[0].Body; !strings.Contains(body, "已收进收集箱") || !strings.Contains(body, "明天买牛奶") {
		t.Fatalf("reply: %s", body)
	}
	items := inboxItems()
	if len(items) != 1 || items[0].Text != "明天买牛奶" || items[0].Source != "telegram_voice" {
		t.Fatalf("items: %+v", items)
	}
	if len(speech.audio) != 1 || string(speech.audio[0]) != "OGG-DATA" {
		t.Fatalf("audio: %q", speech.audio)
	}

	// Plain text without a link goes to the inbox too; a link still goes to read-later.
	post(42, map[string]any{"text": "记得买牛奶"})
	waitFor(t, "second reply", func() bool { return len(tg.find("/sendMessage")) == 2 })
	if n := len(inboxItems()); n != 2 {
		t.Fatalf("items after text: %d", n)
	}
}
