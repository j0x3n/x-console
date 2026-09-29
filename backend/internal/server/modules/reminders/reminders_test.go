package reminders_test

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// fakeService records every request an external service receives.
type fakeService struct {
	*httptest.Server
	mu    sync.Mutex
	calls []fakeCall
	reply func(path string) (int, string)
}

type fakeCall struct {
	Path string
	Body string
}

func newFake(t *testing.T, reply func(path string) (int, string)) *fakeService {
	f := &fakeService{reply: reply}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.calls = append(f.calls, fakeCall{Path: r.URL.Path, Body: string(raw)})
		f.mu.Unlock()
		status, body := f.reply(r.URL.Path)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeService) find(suffix string) []fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []fakeCall
	for _, c := range f.calls {
		if strings.HasSuffix(c.Path, suffix) {
			out = append(out, c)
		}
	}
	return out
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func setup(t *testing.T) (*testutil.Env, *reminders.Module) {
	env := testutil.New(t)
	svc, ok := module.Lookup[contracts.Reminders](env.App.Deps.Registry, contracts.RemindersKey)
	if !ok {
		t.Fatal("contracts.Reminders not provided")
	}
	return env, svc.(*reminders.Module)
}

func fakeTelegram(t *testing.T) *fakeService {
	return newFake(t, func(string) (int, string) { return 200, `{"ok":true,"result":{}}` })
}

func configureTelegram(t *testing.T, env *testutil.Env, tg *fakeService) {
	t.Helper()
	body := map[string]any{"values": map[string]string{"bot_token": "123456:SECRET-TOKEN-XYZ", "chat_id": "42", "api_base": tg.URL}}
	status, raw := env.Do(http.MethodPut, "/notify/channels/telegram", body, nil)
	if status != http.StatusForbidden || !strings.Contains(string(raw), "elevation_required") {
		t.Fatalf("token change without elevation: %d %s", status, raw)
	}
	env.Elevate()
	var ch api.NotifyChannel
	env.MustDo(http.MethodPut, "/notify/channels/telegram", body, &ch)
	if !ch.Configured {
		t.Fatalf("not configured: %+v", ch)
	}
	for _, f := range ch.Fields {
		if f.Key == "bot_token" && (f.Value != "••••-XYZ" || !f.Set) {
			t.Fatalf("token not masked: %+v", f)
		}
	}
}

func TestReminderDueTelegramDone(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	tg := fakeTelegram(t)
	configureTelegram(t, env, tg)

	// Validation and 404.
	if status, _ := env.Do(http.MethodPost, "/reminders", map[string]any{"title": " ", "at": time.Now()}, nil); status != http.StatusBadRequest {
		t.Fatalf("empty title: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/reminders", map[string]any{"title": "x", "at": time.Now(), "rrule": "FREQ=BAD"}, nil); status != http.StatusBadRequest {
		t.Fatalf("bad rrule: %d", status)
	}
	if status, _ := env.Do(http.MethodGet, "/reminders/999", nil, nil); status != http.StatusNotFound {
		t.Fatalf("missing: %d", status)
	}

	// A reminder one minute from now.
	now := time.Now()
	var rem api.Reminder
	env.MustDo(http.MethodPost, "/reminders", map[string]any{"title": "喝水", "at": now.Add(time.Minute)}, &rem)
	if rem.Status != api.ReminderStatusScheduled {
		t.Fatalf("created: %+v", rem)
	}
	var list, upcoming struct{ Items []api.Reminder }
	env.MustDo(http.MethodGet, "/reminders?range=today", nil, &list)
	env.MustDo(http.MethodGet, "/reminders?range=upcoming", nil, &upcoming)
	if len(list.Items)+len(upcoming.Items) != 1 {
		t.Fatalf("today %+v upcoming %+v", list.Items, upcoming.Items)
	}

	if err := reminders.Scan(m, ctx, now); err != nil {
		t.Fatal(err)
	}
	if len(tg.find("/sendMessage")) != 0 {
		t.Fatal("sent before due")
	}
	if err := reminders.Scan(m, ctx, now.Add(61*time.Second)); err != nil {
		t.Fatal(err)
	}
	// In-app notification.
	var notes struct {
		Items []struct{ Kind, Title string }
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) != 1 || notes.Items[0].Kind != "reminder.due" || notes.Items[0].Title != "喝水" {
		t.Fatalf("in-app: %+v", notes.Items)
	}
	// Telegram message with buttons.
	waitFor(t, "telegram sendMessage", func() bool { return len(tg.find("/sendMessage")) == 1 })
	sent := tg.find("/sendMessage")[0]
	if !strings.Contains(sent.Path, "/bot123456:SECRET-TOKEN-XYZ/") {
		t.Fatalf("path: %s", sent.Path)
	}
	doneID := fmt.Sprintf("reminder.done:%d", rem.Id)
	if !strings.Contains(sent.Body, doneID) || !strings.Contains(sent.Body, `"chat_id":"42"`) {
		t.Fatalf("message: %s", sent.Body)
	}
	env.MustDo(http.MethodGet, fmt.Sprintf("/reminders/%d", rem.Id), nil, &rem)
	if rem.Status != api.ReminderStatusPending {
		t.Fatalf("after firing: %s", rem.Status)
	}
	// Scanning again does not resend.
	_ = reminders.Scan(m, ctx, now.Add(2*time.Minute))
	env.MustDo(http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) != 1 {
		t.Fatalf("resent: %d", len(notes.Items))
	}

	// Telegram presses "完成". The webhook is public: no cookie, no CSRF header.
	if err := env.App.Deps.Settings.SetSecret(ctx, reminders.TelegramSecretKey, "hook-secret"); err != nil {
		t.Fatal(err)
	}
	update := map[string]any{"update_id": 1, "callback_query": map[string]any{
		"id": "cb1", "data": doneID,
		"message": map[string]any{"message_id": 77, "text": "喝水", "chat": map[string]any{"id": 42},
			"reply_markup": map[string]any{"inline_keyboard": [][]map[string]string{{{"text": "完成", "callback_data": doneID}}}}},
	}}
	post := func(secret string) int {
		raw, _ := json.Marshal(update)
		req, _ := http.NewRequest(http.MethodPost, env.URL("/integrations/telegram/webhook"), bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if secret != "" {
			req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if status := post("wrong"); status != http.StatusUnauthorized {
		t.Fatalf("bad secret: %d", status)
	}
	if status := post("hook-secret"); status != http.StatusOK {
		t.Fatalf("webhook: %d", status)
	}
	env.MustDo(http.MethodGet, fmt.Sprintf("/reminders/%d", rem.Id), nil, &rem)
	if rem.Status != api.ReminderStatusDone {
		t.Fatalf("after telegram done: %s", rem.Status)
	}
	if len(tg.find("/answerCallbackQuery")) != 1 {
		t.Fatal("callback not answered")
	}
	edits := tg.find("/editMessageText")
	if len(edits) != 1 || !strings.Contains(edits[0].Body, "已处理：完成") || !strings.Contains(edits[0].Body, `"message_id":77`) {
		t.Fatalf("edit: %+v", edits)
	}
	env.MustDo(http.MethodGet, "/reminders?range=done", nil, &list)
	if len(list.Items) != 1 {
		t.Fatalf("done tab: %+v", list.Items)
	}

	// Another chat cannot press buttons.
	update["callback_query"].(map[string]any)["message"].(map[string]any)["chat"] = map[string]any{"id": 7}
	if status := post("hook-secret"); status != http.StatusOK {
		t.Fatalf("foreign chat: %d", status)
	}
	if len(tg.find("/editMessageText")) != 1 {
		t.Fatal("foreign chat edited a message")
	}

	// Register webhook needs a public URL.
	if status, _ := env.Do(http.MethodPost, "/notify/telegram/register-webhook", nil, nil); status != http.StatusBadRequest {
		t.Fatalf("register without public url: %d", status)
	}
}

func TestRegisterTelegramWebhook(t *testing.T) {
	env, m := setup(t)
	reminders.SetPublicURL(m, "https://console.example.com")
	tg := fakeTelegram(t)
	configureTelegram(t, env, tg)
	var out struct{ URL string }
	env.MustDo(http.MethodPost, "/notify/telegram/register-webhook", nil, &out)
	if out.URL != "https://console.example.com/api/v1/integrations/telegram/webhook" {
		t.Fatal(out.URL)
	}
	calls := tg.find("/setWebhook")
	if len(calls) != 1 || !strings.Contains(calls[0].Body, out.URL) || !strings.Contains(calls[0].Body, "secret_token") {
		t.Fatalf("setWebhook: %+v", calls)
	}
	var ch api.NotifyChannel
	env.MustDo(http.MethodGet, "/notify/channels/telegram", nil, &ch)
	if ch.WebhookUrl == nil || *ch.WebhookUrl != out.URL {
		t.Fatalf("channel: %+v", ch)
	}
}

func TestDailyReminderCompleteMovesToTomorrow(t *testing.T) {
	_, m := setup(t)
	ctx := context.Background()
	eight := local(2026, 10, 1, 8, 0)
	row, err := reminders.CreateAt(m, ctx, contracts.CreateReminder{Title: "吃药", At: local(2026, 10, 1, 9, 0), RRule: "FREQ=DAILY"}, eight)
	if err != nil {
		t.Fatal(err)
	}
	row, err = reminders.CompleteAt(m, ctx, row.ID, eight.Add(30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !row.NextAt.Equal(local(2026, 10, 2, 9, 0)) {
		t.Fatalf("next: %v", row.NextAt.In(shanghai))
	}
	// Upcoming lists every occurrence in the window.
	refs, err := m.Upcoming(ctx, local(2026, 10, 4, 12, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 3 || !refs[0].At.Equal(local(2026, 10, 2, 9, 0)) {
		t.Fatalf("upcoming: %+v", refs)
	}
}

func TestMissedReminderBody(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	start := time.Now().Add(-3 * time.Hour)
	row, err := reminders.CreateAt(m, ctx, contracts.CreateReminder{Title: "开会", At: start}, start.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := reminders.Scan(m, ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	var notes struct {
		Items []struct{ Title, Body string }
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) != 1 || !strings.HasPrefix(notes.Items[0].Body, "错过的提醒") {
		t.Fatalf("missed body: %+v", notes.Items)
	}
	// Snooze through the notification action endpoint (Web Push buttons).
	env.MustDo(http.MethodPost, "/notify/actions", map[string]string{"actionId": fmt.Sprintf("reminder.snooze:%d:10", row.ID)}, nil)
	var rem api.Reminder
	env.MustDo(http.MethodGet, fmt.Sprintf("/reminders/%d", row.ID), nil, &rem)
	if rem.Status != api.ReminderStatusSnoozed || rem.DueAt == nil || time.Until(*rem.DueAt) < 9*time.Minute {
		t.Fatalf("snoozed: %+v", rem)
	}
	if status, _ := env.Do(http.MethodPost, "/notify/actions", map[string]string{"actionId": "nothing.here:1"}, nil); status != http.StatusNotFound {
		t.Fatalf("unknown action: %d", status)
	}
}

func TestQuietHoursBark(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	bark := newFake(t, func(string) (int, string) { return 200, `{"code":200,"message":"success"}` })
	env.Elevate()
	env.MustDo(http.MethodPut, "/notify/channels/bark", map[string]any{"values": map[string]string{"device_key": "DEVKEY123456", "server": bark.URL}}, nil)
	// Quiet hours around now.
	nowLocal := time.Now().In(shanghai)
	q := api.QuietHours{Enabled: true, Start: nowLocal.Add(-time.Hour).Format("15:04"), End: nowLocal.Add(time.Hour).Format("15:04")}
	env.MustDo(http.MethodPut, "/notify/quiet-hours", q, nil)
	if status, _ := env.Do(http.MethodPut, "/notify/quiet-hours", api.QuietHours{Enabled: true, Start: "25:00", End: "07:00"}, nil); status != http.StatusBadRequest {
		t.Fatalf("bad quiet hours: %d", status)
	}

	r := reminders.Router(m)
	normal := notify.Stored{Notification: notify.Notification{Kind: "test.x", Title: "普通", Priority: notify.PriorityNormal}}
	if got := r.Route(ctx, normal); len(got) != 0 {
		t.Fatalf("normal during quiet hours: %v", got)
	}
	urgent := notify.Stored{Notification: notify.Notification{Kind: "test.x", Title: "紧急", Priority: notify.PriorityUrgent}}
	if got := r.Route(ctx, urgent); len(got) != 1 || got[0] != "bark" {
		t.Fatalf("urgent during quiet hours: %v", got)
	}
	if _, err := env.App.Deps.Notify.Send(ctx, normal.Notification); err != nil {
		t.Fatal(err)
	}
	if _, err := env.App.Deps.Notify.Send(ctx, urgent.Notification); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "bark push", func() bool { return len(bark.find("/push")) == 1 })
	time.Sleep(100 * time.Millisecond)
	calls := bark.find("/push")
	if len(calls) != 1 || !strings.Contains(calls[0].Body, "紧急") || !strings.Contains(calls[0].Body, "timeSensitive") {
		t.Fatalf("bark calls: %+v", calls)
	}

	// Outside quiet hours a normal notification goes out.
	env.MustDo(http.MethodPut, "/notify/quiet-hours", api.QuietHours{Enabled: false, Start: q.Start, End: q.End}, nil)
	if got := r.Route(ctx, normal); len(got) != 1 {
		t.Fatalf("normal outside quiet hours: %v", got)
	}
}

func TestRoutesAndChannels(t *testing.T) {
	env, _ := setup(t)
	var routes []api.NotifyRoute
	env.MustDo(http.MethodGet, "/notify/routes", nil, &routes)
	if len(routes) != 1 || routes[0].KindPattern != "*" {
		t.Fatalf("default routes: %+v", routes)
	}
	next := []api.NotifyRoute{
		{KindPattern: "host.alert*", MinPriority: "high", Channels: []string{"telegram"}, Enabled: true},
		{KindPattern: "*", MinPriority: "normal", Channels: []string{"webpush"}, Enabled: true},
	}
	env.MustDo(http.MethodPut, "/notify/routes", next, &routes)
	if len(routes) != 2 || routes[0].KindPattern != "host.alert*" || routes[1].Channels[0] != "webpush" {
		t.Fatalf("saved routes: %+v", routes)
	}
	bad := []api.NotifyRoute{{KindPattern: "*", MinPriority: "normal", Channels: []string{"pigeon"}, Enabled: true}}
	if status, _ := env.Do(http.MethodPut, "/notify/routes", bad, nil); status != http.StatusBadRequest {
		t.Fatalf("bad channel: %d", status)
	}

	var channels []api.NotifyChannel
	env.MustDo(http.MethodGet, "/notify/channels", nil, &channels)
	if len(channels) != 4 || channels[0].Name != "webpush" || channels[1].Configured {
		t.Fatalf("channels: %+v", channels)
	}
	if status, _ := env.Do(http.MethodPost, "/notify/channels/bark/test", nil, nil); status != http.StatusPreconditionFailed {
		t.Fatalf("test unconfigured: %d", status)
	}
	if status, _ := env.Do(http.MethodPut, "/notify/channels/bark", map[string]any{"values": map[string]string{"nope": "x"}}, nil); status != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", status)
	}
	if status, _ := env.Do(http.MethodGet, "/notify/channels/pigeon", nil, nil); status != http.StatusNotFound {
		t.Fatalf("unknown channel: %d", status)
	}

	// ServerChan test message through a fake server.
	sc := newFake(t, func(string) (int, string) { return 200, `{"code":0,"message":""}` })
	env.Elevate()
	env.MustDo(http.MethodPut, "/notify/channels/serverchan", map[string]any{"values": map[string]string{"send_key": "SCTKEY999", "api_base": sc.URL}}, nil)
	env.MustDo(http.MethodPost, "/notify/channels/serverchan/test", nil, nil)
	calls := sc.find("/SCTKEY999.send")
	if len(calls) != 1 || !strings.Contains(calls[0].Body, "title=") {
		t.Fatalf("serverchan: %+v", calls)
	}
	// A failing service turns into a readable 502.
	scFail := newFake(t, func(string) (int, string) { return 200, `{"code":40001,"message":"bad key"}` })
	env.MustDo(http.MethodPut, "/notify/channels/serverchan", map[string]any{"values": map[string]string{"api_base": scFail.URL}}, nil)
	status, raw := env.Do(http.MethodPost, "/notify/channels/serverchan/test", nil, nil)
	if status != http.StatusBadGateway || !strings.Contains(string(raw), "bad key") || strings.Contains(string(raw), "SCTKEY999") {
		t.Fatalf("serverchan failure: %d %s", status, raw)
	}
}

var shanghai, _ = time.LoadLocation("Asia/Shanghai")

func local(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, shanghai)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func TestWebPush(t *testing.T) {
	env, _ := setup(t)
	var key struct{ PublicKey string }
	env.MustDo(http.MethodGet, "/notify/webpush/vapid-public-key", nil, &key)
	if len(key.PublicKey) < 80 {
		t.Fatalf("vapid key: %q", key.PublicKey)
	}
	gone := false
	var mu sync.Mutex
	push := newFake(t, func(string) (int, string) {
		mu.Lock()
		defer mu.Unlock()
		if gone {
			return http.StatusGone, ""
		}
		return http.StatusCreated, ""
	})
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	sub := map[string]any{"endpoint": push.URL + "/push/abc", "keys": map[string]string{"p256dh": b64(priv.PublicKey().Bytes()), "auth": b64(auth)}}
	if status, _ := env.Do(http.MethodPost, "/notify/webpush/subscriptions", map[string]any{"endpoint": "ftp://x", "keys": map[string]string{"p256dh": "a", "auth": "b"}}, nil); status != http.StatusBadRequest {
		t.Fatalf("bad subscription: %d", status)
	}
	env.MustDo(http.MethodPost, "/notify/webpush/subscriptions", sub, nil)
	env.MustDo(http.MethodPost, "/notify/webpush/subscriptions", sub, nil) // upsert, not a duplicate
	var ch api.NotifyChannel
	env.MustDo(http.MethodGet, "/notify/channels/webpush", nil, &ch)
	if !ch.Configured || ch.Subscriptions == nil || *ch.Subscriptions != 1 {
		t.Fatalf("webpush channel: %+v", ch)
	}
	env.MustDo(http.MethodPost, "/notify/channels/webpush/test", nil, nil)
	calls := push.find("/push/abc")
	if len(calls) != 1 || len(calls[0].Body) == 0 {
		t.Fatalf("push calls: %+v", calls)
	}
	// The push service says the subscription is gone: it is removed.
	mu.Lock()
	gone = true
	mu.Unlock()
	env.MustDo(http.MethodPost, "/notify/channels/webpush/test", nil, nil)
	env.MustDo(http.MethodGet, "/notify/channels/webpush", nil, &ch)
	if ch.Configured || *ch.Subscriptions != 0 {
		t.Fatalf("gone subscription kept: %+v", ch)
	}
	env.MustDo(http.MethodPost, "/notify/webpush/subscriptions", sub, nil)
	env.MustDo(http.MethodDelete, "/notify/webpush/subscriptions?endpoint="+push.URL+"/push/abc", nil, nil)
	if status, _ := env.Do(http.MethodDelete, "/notify/webpush/subscriptions?endpoint=nope", nil, nil); status != http.StatusNotFound {
		t.Fatalf("delete missing: %d", status)
	}
}

func TestReminderCRUD(t *testing.T) {
	env, _ := setup(t)
	var rem api.Reminder
	at := time.Now().Add(48 * time.Hour)
	env.MustDo(http.MethodPost, "/reminders", map[string]any{"title": "交房租", "at": at, "rrule": "FREQ=MONTHLY", "link": "/notes"}, &rem)
	var list struct{ Items []api.Reminder }
	env.MustDo(http.MethodGet, "/reminders?range=upcoming", nil, &list)
	if len(list.Items) != 1 || list.Items[0].Rrule != "FREQ=MONTHLY" {
		t.Fatalf("upcoming: %+v", list.Items)
	}
	if status, _ := env.Do(http.MethodPatch, fmt.Sprintf("/reminders/%d", rem.Id), map[string]any{"link": "javascript:alert(1)"}, nil); status != http.StatusBadRequest {
		t.Fatalf("bad link: %d", status)
	}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/reminders/%d", rem.Id), map[string]any{"title": "交房租和水电", "enabled": false}, &rem)
	if rem.Title != "交房租和水电" || rem.Enabled {
		t.Fatalf("patched: %+v", rem)
	}
	env.MustDo(http.MethodPost, fmt.Sprintf("/reminders/%d/snooze", rem.Id), map[string]int{"minutes": 30}, &rem)
	if rem.Status != api.ReminderStatusSnoozed {
		t.Fatalf("snooze: %+v", rem)
	}
	if status, _ := env.Do(http.MethodPost, fmt.Sprintf("/reminders/%d/snooze", rem.Id), map[string]int{"minutes": 0}, nil); status != http.StatusBadRequest {
		t.Fatalf("snooze 0: %d", status)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/reminders/%d", rem.Id), nil, nil)
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/reminders/%d", rem.Id), nil, nil); status != http.StatusNotFound {
		t.Fatalf("delete twice: %d", status)
	}
}

func TestActions(t *testing.T) {
	env, _ := setup(t)
	ctx := context.Background()
	run := func(name, input string) any {
		t.Helper()
		out, err := env.App.Deps.Actions.Run(ctx, name, json.RawMessage(input))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	at := time.Now().Add(time.Hour).Format(time.RFC3339)
	created := run("reminders.create", `{"title":"打电话","at":"`+at+`"}`).(map[string]any)
	id := created["id"].(int64)
	list := run("reminders.list", `{"range":"today"}`).([]api.Reminder)
	upcoming := run("reminders.list", `{"range":"upcoming"}`).([]api.Reminder)
	if len(list)+len(upcoming) != 1 {
		t.Fatalf("list: %+v %+v", list, upcoming)
	}
	done := run("reminders.complete", fmt.Sprintf(`{"id":%d}`, id)).(api.Reminder)
	if done.Status != api.ReminderStatusDone {
		t.Fatalf("complete: %+v", done)
	}
	sent := run("notify.send", `{"title":"自动化消息","priority":"high"}`).(map[string]any)
	if sent["id"].(int64) == 0 {
		t.Fatal("notify.send returned no id")
	}
	if _, err := env.App.Deps.Actions.Run(ctx, "notify.send", json.RawMessage(`{"title":"x","priority":"loud"}`)); err == nil {
		t.Fatal("bad priority accepted")
	}
	if _, err := env.App.Deps.Actions.Run(ctx, "reminders.create", json.RawMessage(`{"title":"x"}`)); err == nil {
		t.Fatal("missing time accepted")
	}
}

func TestPushService(t *testing.T) {
	cases := map[string]api.WebPushService{
		"https://fcm.googleapis.com/fcm/send/abc":                api.WebPushServiceGoogle,
		"https://android.googleapis.com/gcm/send/abc":            api.WebPushServiceGoogle,
		"https://hk2p.notify.windows.com/w/?token=abc":           api.WebPushServiceMicrosoft,
		"https://updates.push.services.mozilla.com/wpush/v2/abc": api.WebPushServiceMozilla,
		"https://autopush.prod.mozaws.net/wpush/v1/abc":          api.WebPushServiceMozilla,
		"https://web.push.apple.com/abc":                         api.WebPushServiceApple,
		"https://example.com/push":                               api.WebPushServiceOther,
		"https://evilgoogleapis.com/push":                        api.WebPushServiceOther,
		"not a url":                                              api.WebPushServiceOther,
	}
	for endpoint, want := range cases {
		if got := reminders.PushService(endpoint); got != want {
			t.Errorf("%s: got %s, want %s", endpoint, got, want)
		}
	}
}

func TestWebPushStatusAndTest(t *testing.T) {
	env, _ := setup(t)
	var mu sync.Mutex
	replies := map[string]int{"/push/ok": http.StatusCreated, "/push/denied": http.StatusForbidden, "/push/gone": http.StatusGone}
	push := newFake(t, func(path string) (int, string) {
		mu.Lock()
		defer mu.Unlock()
		return replies[path], ""
	})
	subscribe := func(path string) {
		priv, err := ecdh.P256().GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		auth := make([]byte, 16)
		_, _ = rand.Read(auth)
		env.MustDo(http.MethodPost, "/notify/webpush/subscriptions", map[string]any{
			"endpoint": push.URL + path, "userAgent": "Test/1.0",
			"keys": map[string]string{"p256dh": b64(priv.PublicKey().Bytes()), "auth": b64(auth)},
		}, nil)
	}
	var none []api.WebPushTestResult
	env.MustDo(http.MethodPost, "/notify/webpush/test", nil, &none)
	if len(none) != 0 {
		t.Fatalf("no subscriptions: %+v", none)
	}

	subscribe("/push/ok")
	subscribe("/push/denied")
	subscribe("/push/gone")
	var results []api.WebPushTestResult
	env.MustDo(http.MethodPost, "/notify/webpush/test", nil, &results)
	if len(results) != 3 {
		t.Fatalf("results: %+v", results)
	}
	byID := map[int64]api.WebPushTestResult{}
	for _, r := range results {
		byID[r.Id] = r
	}
	var subs []api.WebPushSubscriptionInfo
	env.MustDo(http.MethodGet, "/notify/webpush/subscriptions", nil, &subs)
	if len(subs) != 2 {
		t.Fatalf("410 should remove its subscription: %+v", subs)
	}
	for _, s := range subs {
		r := byID[s.Id]
		switch {
		case strings.HasSuffix(s.Endpoint, "/push/ok"):
			if !r.Ok || r.Status == nil || *r.Status != 201 || s.LastOkAt == nil || s.LastError != nil {
				t.Fatalf("ok subscription: %+v %+v", r, s)
			}
		case strings.HasSuffix(s.Endpoint, "/push/denied"):
			if r.Ok || r.Status == nil || *r.Status != 403 || r.Error == nil || *r.Error != "403 Forbidden" ||
				s.LastError == nil || *s.LastError != "403 Forbidden" || s.LastErrorAt == nil || s.LastOkAt != nil {
				t.Fatalf("denied subscription: %+v %+v", r, s)
			}
		default:
			t.Fatalf("unexpected subscription %s", s.Endpoint)
		}
		if s.Service != api.WebPushServiceOther || s.UserAgent != "Test/1.0" {
			t.Fatalf("info: %+v", s)
		}
	}
	gone := 0
	for _, r := range results {
		if !r.Ok && r.Status != nil && *r.Status == 410 {
			gone++
		}
	}
	if gone != 1 {
		t.Fatalf("410 result missing: %+v", results)
	}

	// A later success clears the error.
	mu.Lock()
	replies["/push/denied"] = http.StatusCreated
	mu.Unlock()
	env.MustDo(http.MethodPost, "/notify/webpush/test", nil, &results)
	subs = nil // decoding into old elements would keep their omitted fields
	env.MustDo(http.MethodGet, "/notify/webpush/subscriptions", nil, &subs)
	for _, s := range subs {
		if s.LastError != nil || s.LastErrorAt != nil || s.LastOkAt == nil {
			t.Fatalf("after recovery: %+v", s)
		}
	}

	// Delete by id.
	env.MustDo(http.MethodDelete, fmt.Sprintf("/notify/webpush/subscriptions/%d", subs[0].Id), nil, nil)
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/notify/webpush/subscriptions/%d", subs[0].Id), nil, nil); status != http.StatusNotFound {
		t.Fatalf("delete twice: %d", status)
	}
	env.MustDo(http.MethodGet, "/notify/webpush/subscriptions", nil, &subs)
	if len(subs) != 1 {
		t.Fatalf("after delete: %+v", subs)
	}
}

func TestWebPushConnectionError(t *testing.T) {
	env, _ := setup(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "http://" + l.Addr().String() + "/push/x"
	_ = l.Close() // nothing listens here any more
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	env.MustDo(http.MethodPost, "/notify/webpush/subscriptions", map[string]any{
		"endpoint": endpoint, "keys": map[string]string{"p256dh": b64(priv.PublicKey().Bytes()), "auth": b64(auth)},
	}, nil)
	var results []api.WebPushTestResult
	env.MustDo(http.MethodPost, "/notify/webpush/test", nil, &results)
	if len(results) != 1 || results[0].Ok || results[0].Status != nil || results[0].Error == nil || *results[0].Error != "127.0.0.1 拒绝连接" {
		t.Fatalf("refused: %+v", results)
	}
}
