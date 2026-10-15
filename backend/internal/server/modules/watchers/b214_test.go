package watchers_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/api"
)

func (f *fixture) captchaReads() int {
	f.chat.mu.Lock()
	defer f.chat.mu.Unlock()
	return f.chat.reads
}

func (f *fixture) setCodes(codes ...string) {
	f.chat.mu.Lock()
	defer f.chat.mu.Unlock()
	f.chat.codes, f.chat.reads = codes, 0
}

// B214: the AI reads the captcha while exploring, and the replay reads it
// again by itself. A wrong read loads a new image; after 2 retries the
// user takes over.
func TestCaptchaIsReadByAI(t *testing.T) {
	f := setup(t, true)
	f.site.set(func(s *site) { s.captcha = true })
	f.setCodes("12 34")
	f.chat.script(fill(`"用户名"`, "username"), fill("type=password", "password"),
		do("solve_captcha", func(string) map[string]any { return map[string]any{} }),
		click(`按钮 "登录"`), finish("审核中"))
	w := f.create(t, map[string]any{"name": "社保", "url": f.site.URL + "/status", "goal": "盯办理状态",
		"username": "alice", "password": "s3cret"})
	got := f.until(t, w.Id, api.WatcherStatusOk)
	watchers.WaitRuns(f.m)
	if got.CurrentValue != "审核中" || f.captchaReads() != 1 || f.notified(t, "watcher.ask") != 0 {
		t.Fatalf("explore: %q, %d reads, detail %s", got.CurrentValue, f.captchaReads(), got.StatusDetail)
	}
	step := false
	for _, s := range got.Recipe {
		step = step || strings.Contains(s.Text, "AI 读了填上")
	}
	if !step {
		t.Fatalf("no captcha step: %+v", got.Recipe)
	}

	check := func() api.WatcherDetail {
		t.Helper()
		f.site.set(func(s *site) { s.sessions = map[string]bool{} })
		f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
		watchers.WaitRuns(f.m)
		return f.get(t, w.Id)
	}
	// Read wrong once, right the second time.
	f.setCodes("0000", "1234")
	if got = check(); got.Status != api.WatcherStatusOk || f.captchaReads() != 2 || !strings.Contains(runText(got.Runs[0]), "换一张再试") {
		t.Fatalf("retry: %s %s, %d reads\n%s", got.Status, got.StatusDetail, f.captchaReads(), runText(got.Runs[0]))
	}

	// Wrong three times: the user is asked, with no fourth read.
	f.setCodes("0000", "0000", "0000", "1234")
	f.site.set(func(s *site) { s.sessions = map[string]bool{} })
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
	got = f.until(t, w.Id, api.WatcherStatusWaiting)
	if got.Pending == nil || got.Pending.Kind != api.WatcherPendingKindCaptcha || f.captchaReads() != 3 {
		t.Fatalf("hand over: %+v, %d reads", got.Pending, f.captchaReads())
	}
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/answer", w.Id), map[string]string{"answer": "cancel"}, nil)
	watchers.WaitRuns(f.m)

	// Over the daily AI limit the captcha goes straight to the user.
	f.env.MustDo(http.MethodPut, "/watchers/settings", map[string]any{"budgetUsd": 0.5, "dangerWords": []string{}}, nil)
	f.chat.mu.Lock()
	f.chat.spent = 0.6
	f.chat.mu.Unlock()
	f.setCodes("1234")
	f.site.set(func(s *site) { s.sessions = map[string]bool{} })
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
	got = f.until(t, w.Id, api.WatcherStatusWaiting)
	if f.captchaReads() != 0 {
		t.Fatalf("read over the limit: %d", f.captchaReads())
	}
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/answer", w.Id), map[string]string{"answer": "cancel"}, nil)
}

func TestCaptchaCode(t *testing.T) {
	for in, want := range map[string]string{"12 34": "1234", "答案：aB3d。": "aB3d", "看不清": "", "12": "", "123456789": ""} {
		if got := watchers.CaptchaCode(in); got != want {
			t.Fatalf("%q: %q, want %q", in, got, want)
		}
	}
}
