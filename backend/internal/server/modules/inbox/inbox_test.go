package inbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/inbox"
	"github.com/j0x3n/x-console/backend/internal/server/modules/inbox/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// fakeAI is the AI boundary: it answers every suggestion with what reply says.
type fakeAI struct {
	mu        sync.Mutex
	available bool
	reply     func(user string) (string, error)
	calls     []string
	speech    string
	speechErr error
	audio     [][]byte
}

func (f *fakeAI) Available(context.Context) bool { return f.available }
func (f *fakeAI) CompleteText(context.Context, string, string, string) (string, error) {
	return "", errors.New("unused")
}
func (f *fakeAI) CompleteJSON(_ context.Context, _, _, user string, _ json.RawMessage, out any) error {
	f.mu.Lock()
	f.calls = append(f.calls, user)
	reply := f.reply
	f.mu.Unlock()
	text, err := reply(user)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(text), out)
}
func (f *fakeAI) Transcribe(_ context.Context, in contracts.Transcribe) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audio = append(f.audio, in.Audio)
	return f.speech, f.speechErr
}
func (f *fakeAI) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type fixture struct {
	env *testutil.Env
	m   *inbox.Module
	ai  *fakeAI
	now time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*inbox.Module](env.App.Deps.Registry, inbox.ServiceKey)
	if !ok {
		t.Fatal("inbox module not registered")
	}
	ai := &fakeAI{available: true, speech: "买牛奶", reply: func(string) (string, error) {
		return `{"kind":"card","title":"买牛奶","body":"","at":"","url":"","reason":"一件要做的事"}`, nil
	}}
	module.Provide[contracts.LLM](env.App.Deps.Registry, contracts.LLMKey, ai)
	module.Provide[contracts.Transcriber](env.App.Deps.Registry, contracts.TranscriberKey, ai)
	loc := inbox.Loc(m)
	f := &fixture{env: env, m: m, ai: ai, now: time.Date(2026, 11, 11, 10, 0, 0, 0, loc)}
	inbox.SetNow(m, func() time.Time { return f.now })
	return f
}

func (f *fixture) add(t *testing.T, text string) api.InboxItem {
	t.Helper()
	var it api.InboxItem
	f.env.MustDo(http.MethodPost, "/inbox", map[string]any{"text": text}, &it)
	inbox.WaitSuggestions(f.m)
	return f.get(t, it.Id)
}

func (f *fixture) get(t *testing.T, id int64) api.InboxItem {
	t.Helper()
	for _, status := range []string{"open", "closed"} {
		var out struct{ Items []api.InboxItem }
		f.env.MustDo(http.MethodGet, "/inbox?status="+status, nil, &out)
		for _, it := range out.Items {
			if it.Id == id {
				return it
			}
		}
	}
	t.Fatalf("item %d not found", id)
	return api.InboxItem{}
}

func (f *fixture) notifications(t *testing.T, kind string) int {
	t.Helper()
	var out struct{ Items []struct{ Kind string } }
	f.env.MustDo(http.MethodGet, "/notifications?limit=100", nil, &out)
	n := 0
	for _, it := range out.Items {
		if it.Kind == kind {
			n++
		}
	}
	return n
}

func (f *fixture) reply(kind, title, body, at, url string) {
	f.ai.mu.Lock()
	defer f.ai.mu.Unlock()
	f.ai.reply = func(string) (string, error) {
		b, _ := json.Marshal(map[string]string{"kind": kind, "title": title, "body": body, "at": at, "url": url, "reason": "测试"})
		return string(b), nil
	}
}

func TestCaptureAndSuggestion(t *testing.T) {
	f := setup(t)
	it := f.add(t, "  买牛奶  ")
	if it.Text != "买牛奶" || it.Source != api.Web || it.Status != api.InboxItemStatusOpen || it.SuggestState != api.Ready {
		t.Fatalf("item: %+v", it)
	}
	if it.Suggestion == nil || it.Suggestion.Kind != api.Card || it.Suggestion.Title != "买牛奶" {
		t.Fatalf("suggestion: %+v", it.Suggestion)
	}
	if f.ai.callCount() != 1 || !strings.Contains(f.ai.calls[0], "2026-11-11T10:00:00") || !strings.Contains(f.ai.calls[0], "买牛奶") {
		t.Fatalf("prompt: %q", f.ai.calls)
	}
	// Empty, too long.
	for _, text := range []string{"  ", strings.Repeat("字", 5001)} {
		if status, _ := f.env.Do(http.MethodPost, "/inbox", map[string]any{"text": text}, nil); status != http.StatusBadRequest {
			t.Errorf("text %d chars: %d", len(text), status)
		}
	}
}

func TestSuggestionIsCleaned(t *testing.T) {
	f := setup(t)
	// A time in the past is dropped; a time in the future keeps its offset.
	f.reply("reminder", "给医生打电话", "", "2026-11-10T15:00:00+08:00", "")
	past := f.add(t, "给医生打电话")
	if past.Suggestion.Kind != api.Reminder || past.Suggestion.At != "" {
		t.Errorf("past time: %+v", past.Suggestion)
	}
	f.reply("reminder", "给医生打电话", "", "2026-11-12T15:00:00+08:00", "https://ignored.example")
	future := f.add(t, "明天下午三点给医生打电话")
	if future.Suggestion.At != "2026-11-12T15:00:00+08:00" || future.Suggestion.Url != "" {
		t.Errorf("future time: %+v", future.Suggestion)
	}
	// A bad kind becomes none; a bad address is dropped.
	f.reply("task", "x", "", "", "")
	if it := f.add(t, "随便"); it.Suggestion.Kind != api.None {
		t.Errorf("bad kind: %+v", it.Suggestion)
	}
	f.reply("readlater", "文章", "", "", "ftp://x/y")
	if it := f.add(t, "那篇文章"); it.Suggestion.Url != "" {
		t.Errorf("bad address: %+v", it.Suggestion)
	}
	// A failing model leaves the item with state failed, and the item stays.
	f.ai.mu.Lock()
	f.ai.reply = func(string) (string, error) { return "", errors.New("boom") }
	f.ai.mu.Unlock()
	if it := f.add(t, "会失败"); it.SuggestState != api.Failed || it.Suggestion != nil {
		t.Errorf("failed: %+v", it)
	}
}

func TestBareURLNeedsNoAI(t *testing.T) {
	f := setup(t)
	it := f.add(t, "https://example.com/a?b=1")
	if f.ai.callCount() != 0 {
		t.Fatalf("the AI was called %d times", f.ai.callCount())
	}
	if it.SuggestState != api.Ready || it.Suggestion.Kind != api.Readlater || it.Suggestion.Url != "https://example.com/a?b=1" {
		t.Fatalf("item: %+v", it)
	}
	// With AI off the URL still gets the shortcut.
	f.ai.available = false
	if it := f.add(t, "https://example.com/b"); it.Suggestion == nil || it.Suggestion.Kind != api.Readlater {
		t.Fatalf("without AI: %+v", it)
	}
}

func TestNoAIOrSwitchedOff(t *testing.T) {
	f := setup(t)
	f.ai.available = false
	it := f.add(t, "买牛奶")
	if it.SuggestState != api.Off || it.Suggestion != nil || f.ai.callCount() != 0 {
		t.Fatalf("no provider: %+v", it)
	}
	if status, _ := f.env.Do(http.MethodPost, fmt.Sprintf("/inbox/%d/suggest", it.Id), nil, nil); status != http.StatusConflict {
		t.Errorf("suggest without provider: %d", status)
	}
	f.ai.available = true
	f.env.MustDo(http.MethodPut, "/inbox/settings", api.InboxSettings{Suggest: false, ReviewReminder: true, ReviewTime: "21:00"}, nil)
	if it := f.add(t, "再买一个"); it.SuggestState != api.Off || f.ai.callCount() != 0 {
		t.Fatalf("switched off: %+v", it)
	}
	f.env.MustDo(http.MethodPut, "/inbox/settings", api.InboxSettings{Suggest: true, ReviewReminder: true, ReviewTime: "21:00"}, nil)
	var again api.InboxItem
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/inbox/%d/suggest", it.Id), nil, &again)
	if again.SuggestState != api.Ready || again.Suggestion == nil {
		t.Errorf("manual suggestion: %+v", again)
	}
}

func TestSettingsValidation(t *testing.T) {
	f := setup(t)
	var s api.InboxSettings
	f.env.MustDo(http.MethodGet, "/inbox/settings", nil, &s)
	if !s.Suggest || !s.ReviewReminder || s.ReviewTime != "21:00" {
		t.Fatalf("defaults: %+v", s)
	}
	good := api.InboxSettings{Suggest: true, ReviewReminder: false, ReviewTime: "20:30", TranscribeModel: " gpt-4o-mini-transcribe ", Language: "ZH", CardProjectId: 3}
	f.env.MustDo(http.MethodPut, "/inbox/settings", good, &s)
	if s.TranscribeModel != "gpt-4o-mini-transcribe" || s.Language != "zh" || s.ReviewReminder {
		t.Fatalf("saved: %+v", s)
	}
	for name, mutate := range map[string]func(*api.InboxSettings){
		"time":     func(x *api.InboxSettings) { x.ReviewTime = "9pm" },
		"language": func(x *api.InboxSettings) { x.Language = "chinese" },
		"project":  func(x *api.InboxSettings) { x.CardProjectId = -1 },
		"model":    func(x *api.InboxSettings) { x.TranscribeModel = strings.Repeat("x", 101) },
	} {
		bad := good
		mutate(&bad)
		if status, _ := f.env.Do(http.MethodPut, "/inbox/settings", bad, nil); status != http.StatusBadRequest {
			t.Errorf("%s: %d", name, status)
		}
	}
}

func TestFileEveryKind(t *testing.T) {
	f := setup(t)
	f.env.Elevate()
	file := func(it api.InboxItem, in map[string]any) (int, api.InboxItem) {
		var out api.InboxItem
		status, raw := f.env.Do(http.MethodPost, fmt.Sprintf("/inbox/%d/file", it.Id), in, &out)
		if status >= 300 {
			t.Logf("file %v: %d %s", in["kind"], status, raw)
		}
		return status, out
	}
	// Reminder: needs a time.
	it := f.add(t, "给医生打电话")
	if status, _ := file(it, map[string]any{"kind": "reminder", "title": "给医生打电话"}); status != http.StatusBadRequest {
		t.Errorf("reminder without time: %d", status)
	}
	status, done := file(it, map[string]any{"kind": "reminder", "title": "给医生打电话", "at": "2026-11-12T15:00:00+08:00"})
	if status != http.StatusOK || done.Status != api.InboxItemStatusFiled || done.FiledKind == nil || *done.FiledKind != api.Reminder || done.FiledRef == nil || *done.FiledRef == "" {
		t.Fatalf("filed reminder: %d %+v", status, done)
	}
	var rems struct{ Items []struct{ Title string } }
	f.env.MustDo(http.MethodGet, "/reminders", nil, &rems)
	found := false
	for _, r := range rems.Items {
		found = found || r.Title == "给医生打电话"
	}
	if !found {
		t.Errorf("reminder not created: %+v", rems)
	}
	// Filing again is a conflict and creates nothing.
	if status, _ := file(it, map[string]any{"kind": "reminder", "title": "x", "at": "2026-11-12T15:00:00+08:00"}); status != http.StatusConflict {
		t.Errorf("file twice: %d", status)
	}
	// Note.
	note := f.add(t, "SQLite 的 WAL 模式读写互不阻塞")
	if status, out := file(note, map[string]any{"kind": "note", "title": "WAL", "body": note.Text}); status != http.StatusOK || *out.FiledRef == "" {
		t.Errorf("note: %d %+v", status, out)
	}
	if status, _ := file(f.add(t, "空"), map[string]any{"kind": "note"}); status != http.StatusBadRequest {
		t.Errorf("empty note: %d", status)
	}
	// Card: needs a project.
	var project struct{ Id int64 }
	f.env.MustDo(http.MethodPost, "/projects", map[string]any{"key": "INB", "name": "收集"}, &project)
	card := f.add(t, "重构登录页")
	if status, _ := file(card, map[string]any{"kind": "card", "title": "重构登录页"}); status != http.StatusBadRequest {
		t.Errorf("card without project: %d", status)
	}
	if status, out := file(card, map[string]any{"kind": "card", "title": "重构登录页", "projectId": project.Id}); status != http.StatusOK || !strings.HasPrefix(*out.FiledRef, "INB-") {
		t.Errorf("card: %d %+v", status, out)
	}
	// The default project of the settings is used when none is given.
	f.env.MustDo(http.MethodPut, "/inbox/settings", api.InboxSettings{Suggest: true, ReviewReminder: true, ReviewTime: "21:00", CardProjectId: project.Id}, nil)
	if status, out := file(f.add(t, "再来一张"), map[string]any{"kind": "card", "title": "再来一张"}); status != http.StatusOK || !strings.HasPrefix(*out.FiledRef, "INB-") {
		t.Errorf("default project: %d %+v", status, out)
	}
	// Read later: needs an address.
	link := f.add(t, "稍后读这篇 https://example.com/post 很长的话")
	for _, bad := range []string{"", "ftp://x", "example.com"} {
		if status, _ := file(link, map[string]any{"kind": "readlater", "url": bad}); status != http.StatusBadRequest {
			t.Errorf("readlater %q: %d", bad, status)
		}
	}
	if status, out := file(link, map[string]any{"kind": "readlater", "url": "https://example.com/post"}); status != http.StatusOK || *out.FiledKind != api.Readlater {
		t.Errorf("readlater: %d %+v", status, out)
	}
	// None is not a place.
	if status, _ := file(f.add(t, "拿不准"), map[string]any{"kind": "none"}); status != http.StatusBadRequest {
		t.Errorf("none: %d", status)
	}
	if status, _ := f.env.Do(http.MethodPost, "/inbox/9999/file", map[string]any{"kind": "note", "title": "x"}, nil); status != http.StatusNotFound {
		t.Errorf("unknown: %d", status)
	}
}

func TestEditDismissDeleteAndLists(t *testing.T) {
	f := setup(t)
	a := f.add(t, "第一条")
	b := f.add(t, "第二条")
	var edited api.InboxItem
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/inbox/%d", a.Id), map[string]any{"text": "改过的第一条"}, &edited)
	inbox.WaitSuggestions(f.m)
	if edited.Text != "改过的第一条" || f.get(t, a.Id).SuggestState != api.Ready {
		t.Errorf("edited: %+v", edited)
	}
	if f.ai.callCount() != 3 {
		t.Errorf("a new suggestion should be made after an edit: %d calls", f.ai.callCount())
	}
	var out api.InboxItem
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/inbox/%d/dismiss", b.Id), nil, &out)
	if out.Status != api.InboxItemStatusDismissed || out.ClosedAt == nil {
		t.Errorf("dismissed: %+v", out)
	}
	if status, _ := f.env.Do(http.MethodPost, fmt.Sprintf("/inbox/%d/dismiss", b.Id), nil, nil); status != http.StatusConflict {
		t.Errorf("dismiss twice: %d", status)
	}
	if status, _ := f.env.Do(http.MethodPatch, fmt.Sprintf("/inbox/%d", b.Id), map[string]any{"text": "x"}, nil); status != http.StatusConflict {
		t.Errorf("edit a closed item: %d", status)
	}
	var open struct {
		Items     []api.InboxItem
		OpenCount int
	}
	f.env.MustDo(http.MethodGet, "/inbox", nil, &open)
	if len(open.Items) != 1 || open.OpenCount != 1 || open.Items[0].Id != a.Id {
		t.Errorf("open: %+v", open)
	}
	var closed struct{ Items []api.InboxItem }
	f.env.MustDo(http.MethodGet, "/inbox?status=closed", nil, &closed)
	if len(closed.Items) != 1 || closed.Items[0].Id != b.Id {
		t.Errorf("closed: %+v", closed)
	}
	f.env.MustDo(http.MethodDelete, fmt.Sprintf("/inbox/%d", a.Id), nil, nil)
	if status, _ := f.env.Do(http.MethodDelete, fmt.Sprintf("/inbox/%d", a.Id), nil, nil); status != http.StatusNotFound {
		t.Errorf("delete twice: %d", status)
	}
}

func (f *fixture) voice(body []byte, contentType, query string) (int, api.InboxItem) {
	req, _ := http.NewRequest(http.MethodPost, f.env.URL("/inbox/voice"+query), bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Requested-With", "x-console")
	resp, err := f.env.Client.Do(req)
	if err != nil {
		f.env.T.Fatal(err)
	}
	defer resp.Body.Close()
	var it api.InboxItem
	_ = json.NewDecoder(resp.Body).Decode(&it)
	return resp.StatusCode, it
}

func TestVoice(t *testing.T) {
	f := setup(t)
	status, it := f.voice([]byte("fake-audio"), "audio/webm;codecs=opus", "?language=zh")
	inbox.WaitSuggestions(f.m)
	if status != http.StatusCreated || it.Text != "买牛奶" || it.Source != api.Voice {
		t.Fatalf("voice: %d %+v", status, it)
	}
	if len(f.ai.audio) != 1 || string(f.ai.audio[0]) != "fake-audio" {
		t.Errorf("audio: %q", f.ai.audio)
	}
	if status, _ := f.voice([]byte("x"), "audio/webm", "?language=chinese"); status != http.StatusBadRequest {
		t.Errorf("bad language: %d", status)
	}
	if status, _ := f.voice(bytes.Repeat([]byte("a"), 10<<20+1), "audio/webm", ""); status != http.StatusRequestEntityTooLarge {
		t.Errorf("too large: %d", status)
	}
	f.ai.speech = "  "
	if status, _ := f.voice([]byte("x"), "audio/webm", ""); status != http.StatusBadRequest {
		t.Errorf("silence: %d", status)
	}
	f.ai.speech, f.ai.speechErr = "", errors.New("provider down")
	if status, _ := f.voice([]byte("x"), "audio/webm", ""); status != http.StatusBadGateway {
		t.Errorf("provider error: %d", status)
	}
	f.ai.speechErr, f.ai.available = nil, false
	if status, _ := f.voice([]byte("x"), "audio/webm", ""); status != http.StatusConflict {
		t.Errorf("no provider: %d", status)
	}
}

func TestTelegramMessages(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tg, ok := module.Lookup[contracts.TelegramInbox](f.env.App.Deps.Registry, contracts.TelegramInboxFallbackKey)
	if !ok {
		t.Fatal("not registered as the fallback inbox")
	}
	reply, handled := tg.HandleTelegramMessage(ctx, contracts.TelegramMessage{Text: "记得买牛奶"})
	inbox.WaitSuggestions(f.m)
	if !handled || !strings.Contains(reply, "已收进收集箱") || !strings.Contains(reply, "记得买牛奶") {
		t.Fatalf("text: %q %v", reply, handled)
	}
	var open struct{ Items []api.InboxItem }
	f.env.MustDo(http.MethodGet, "/inbox", nil, &open)
	if len(open.Items) != 1 || open.Items[0].Source != api.Telegram {
		t.Fatalf("items: %+v", open.Items)
	}
	// The suggestion comes back as a notification with buttons.
	if n := f.notifications(t, "inbox.suggest"); n != 1 {
		t.Errorf("suggestion notifications: %d", n)
	}
	// Commands and empty messages are not for the inbox.
	for _, text := range []string{"/start", "  ", ""} {
		if _, handled := tg.HandleTelegramMessage(ctx, contracts.TelegramMessage{Text: text}); handled {
			t.Errorf("%q should not be handled", text)
		}
	}
	// Voice.
	voice := contracts.TelegramMessage{Voice: &contracts.TelegramVoice{Seconds: 5, Mime: "audio/ogg"},
		Download: func(context.Context) ([]byte, error) { return []byte("ogg-bytes"), nil }}
	reply, handled = tg.HandleTelegramMessage(ctx, voice)
	inbox.WaitSuggestions(f.m)
	if !handled || !strings.Contains(reply, "已收进收集箱") {
		t.Fatalf("voice: %q %v", reply, handled)
	}
	f.env.MustDo(http.MethodGet, "/inbox", nil, &open)
	if len(open.Items) != 2 || open.Items[1].Source != api.TelegramVoice {
		t.Errorf("voice item: %+v", open.Items)
	}
	long := voice
	long.Voice = &contracts.TelegramVoice{Seconds: 301}
	if reply, handled := tg.HandleTelegramMessage(ctx, long); !handled || !strings.Contains(reply, "太长") {
		t.Errorf("long voice: %q %v", reply, handled)
	}
	broken := voice
	broken.Download = func(context.Context) ([]byte, error) { return nil, errors.New("网络断了") }
	if reply, _ := tg.HandleTelegramMessage(ctx, broken); !strings.Contains(reply, "网络断了") {
		t.Errorf("download error: %q", reply)
	}
	f.ai.available = false
	if reply, handled := tg.HandleTelegramMessage(ctx, voice); !handled || !strings.Contains(reply, "AI 供应商") {
		t.Errorf("no provider: %q", reply)
	}
}

func TestSuggestionButtons(t *testing.T) {
	f := setup(t)
	f.env.Elevate()
	ctx := context.Background()
	press := func(id string) error { return f.env.App.Deps.Notify.HandleAction(ctx, id) }
	// A reminder suggestion with a time files with one tap.
	f.reply("reminder", "给医生打电话", "", "2026-11-12T15:00:00+08:00", "")
	a := f.add(t, "明天下午三点给医生打电话")
	if err := press(fmt.Sprintf("inbox.file:%d", a.Id)); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, a.Id); got.Status != api.InboxItemStatusFiled || *got.FiledKind != api.Reminder {
		t.Errorf("filed: %+v", got)
	}
	// Without a time it asks for the panel.
	f.reply("reminder", "打电话", "", "", "")
	b := f.add(t, "打电话")
	if err := press(fmt.Sprintf("inbox.file:%d", b.Id)); err == nil || !strings.Contains(err.Error(), "时间") {
		t.Errorf("reminder without time: %v", err)
	}
	// A card needs a default project.
	f.reply("card", "重构", "", "", "")
	c := f.add(t, "重构")
	if err := press(fmt.Sprintf("inbox.file:%d", c.Id)); err == nil || !strings.Contains(err.Error(), "项目") {
		t.Errorf("card without project: %v", err)
	}
	// Nothing to file without a suggestion.
	f.ai.available = false
	d := f.add(t, "没有建议")
	if err := press(fmt.Sprintf("inbox.file:%d", d.Id)); err == nil {
		t.Error("filing without a suggestion should fail")
	}
	if err := press(fmt.Sprintf("inbox.dismiss:%d", d.Id)); err != nil {
		t.Fatal(err)
	}
	if f.get(t, d.Id).Status != api.InboxItemStatusDismissed {
		t.Error("not dismissed")
	}
	for _, bad := range []string{"inbox.file:x", "inbox.file", "inbox.other:1", "inbox.file:9999"} {
		if err := press(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}

func TestEveningReminder(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tick := func(h, m int) {
		f.now = time.Date(2026, 11, 11, h, m, 0, 0, inbox.Loc(f.m))
		if err := inbox.ReviewTick(f.m, ctx); err != nil {
			t.Fatal(err)
		}
	}
	tick(21, 5) // nothing waiting
	if n := f.notifications(t, "inbox.review"); n != 0 {
		t.Fatalf("empty inbox: %d", n)
	}
	f.add(t, "一条")
	f.add(t, "两条")
	tick(20, 59) // too early
	if n := f.notifications(t, "inbox.review"); n != 0 {
		t.Fatalf("early: %d", n)
	}
	tick(21, 0)
	tick(21, 30) // the same day: no second reminder
	if n := f.notifications(t, "inbox.review"); n != 1 {
		t.Fatalf("reminders: %d", n)
	}
	f.now = f.now.AddDate(0, 0, 1)
	if err := inbox.ReviewTick(f.m, ctx); err != nil {
		t.Fatal(err)
	}
	if n := f.notifications(t, "inbox.review"); n != 2 {
		t.Fatalf("next day: %d", n)
	}
	// Switched off.
	f.env.MustDo(http.MethodPut, "/inbox/settings", api.InboxSettings{Suggest: true, ReviewReminder: false, ReviewTime: "21:00"}, nil)
	f.now = f.now.AddDate(0, 0, 1)
	_ = inbox.ReviewTick(f.m, ctx)
	if n := f.notifications(t, "inbox.review"); n != 2 {
		t.Fatalf("switched off: %d", n)
	}
}

func TestLimitAndActions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.ai.available = false
	res, err := f.env.App.Deps.Actions.Run(ctx, "inbox.add", []byte(`{"text":"AI 放进来的"}`))
	if err != nil || res == nil {
		t.Fatalf("inbox.add: %v %v", res, err)
	}
	if _, err := f.env.App.Deps.Actions.Run(ctx, "inbox.add", []byte(`{"text":""}`)); err == nil {
		t.Fatal("empty text should fail")
	}
	list, err := f.env.App.Deps.Actions.Run(ctx, "inbox.list", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(list)
	if !strings.Contains(string(b), "AI 放进来的") || !strings.Contains(string(b), `"source":"action"`) {
		t.Errorf("list: %s", b)
	}
	// 500 open items is the limit.
	for i := 1; i < 500; i++ {
		if _, err := f.env.App.Deps.DB.ExecContext(ctx, `INSERT INTO inbox_items (text, source, status, suggest_state, created_at, updated_at) VALUES (?, 'web', 'open', 'off', ?, ?)`, fmt.Sprint("条", i), f.now, f.now); err != nil {
			t.Fatal(err)
		}
	}
	if status, _ := f.env.Do(http.MethodPost, "/inbox", map[string]any{"text": "第 501 条"}, nil); status != http.StatusBadRequest {
		t.Errorf("over the limit: %d", status)
	}
}
