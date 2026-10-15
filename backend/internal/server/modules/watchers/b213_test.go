package watchers_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/api"
)

func TestDetectKind(t *testing.T) {
	for in, want := range map[string]string{
		"@alice":                           "x @alice",
		"  @alice  ":                       "x @alice",
		"https://x.com/alice":              "x https://x.com/alice",
		"https://www.x.com/alice":          "x https://www.x.com/alice",
		"https://mobile.twitter.com/alice": "x https://mobile.twitter.com/alice",
		"HTTPS://Twitter.com/alice":        "x HTTPS://Twitter.com/alice",
		"x.com/alice":                      "x https://x.com/alice",
		"twitter.com/alice/status/1":       "x https://twitter.com/alice/status/1",
		"www.x.com":                        "x https://www.x.com",
		"https://x.com/search?q=a.b":       "x https://x.com/search?q=a.b",
		"https://example.com/a":            "web https://example.com/a",
		"http://127.0.0.1:8080/status":     "web http://127.0.0.1:8080/status",
		"https://x.com.evil.com/a":         "web https://x.com.evil.com/a",
		"https://notx.com/a":               "web https://notx.com/a",
		"降息":                               "x 降息",
		"美联储 降息":                           "x 美联储 降息",
	} {
		kind, addr, err := watchers.DetectKind(in)
		if got := kind + " " + addr; err != nil || got != want {
			t.Errorf("%q: got %q, %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"example.com", "example.com/a", "www.example.com", "ftp://a.com", "a.b 降息", "", "   "} {
		if kind, addr, err := watchers.DetectKind(bad); err == nil {
			t.Errorf("%q passed as %s %s", bad, kind, addr)
		}
	}
	_, _, err := watchers.DetectKind("example.com")
	if err == nil || !strings.Contains(err.Error(), "https://") {
		t.Errorf("hint %v", err)
	}
}

func TestPromptsDependOnGoal(t *testing.T) {
	empty := watchers.AgentFirstMessage("https://a.cn", "", false, "页面")
	if !strings.Contains(empty, "用户没说要做什么") || strings.Contains(empty, "用户要做的事") {
		t.Fatalf("empty goal: %s", empty)
	}
	set := watchers.AgentFirstMessage("https://a.cn", "登录后下载本月账单", true, "页面")
	if !strings.Contains(set, "用户要做的事：登录后下载本月账单") || strings.Contains(set, "用户没说要做什么") || !strings.Contains(set, "fill_login") {
		t.Fatalf("goal: %s", set)
	}
	sys := watchers.AgentSystem()
	for _, want := range []string{"点按钮", "等用户批准", "允许的域名以外", "不是用户说的"} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	if strings.Contains(sys, "只看，不改") {
		t.Error("system prompt still says read only")
	}

	if got := watchers.JudgeUser("", "a", "b"); !strings.Contains(got, "用户没说关心什么") || strings.Contains(got, "用户关心的：") {
		t.Fatalf("judge, empty goal: %s", got)
	}
	if got := watchers.JudgeUser("办理状态", "a", "b"); !strings.Contains(got, "用户关心的：办理状态") {
		t.Fatalf("judge: %s", got)
	}
}

func TestCreateWithoutGoalOrKind(t *testing.T) {
	f := setup(t, true)
	f.chat.script(append(loginMoves(), finish("审核中"))...)
	// No goal and no kind: the address says it is a web page.
	w := f.create(t, map[string]any{"name": "签证", "url": f.site.URL + "/status", "goal": "",
		"username": "alice", "password": "s3cret"})
	if w.Kind != api.Web || w.Goal != "" {
		t.Fatalf("created %+v", w)
	}
	got := f.until(t, w.Id, api.WatcherStatusOk)
	if !strings.Contains(got.CurrentValue, "审核中") {
		t.Fatalf("value %q", got.CurrentValue)
	}
	if !f.chat.saw("用户没说要做什么") {
		t.Fatal("the AI was not told to choose what to watch")
	}
	var found bool
	for _, r := range got.Runs {
		for _, l := range r.Steps {
			if strings.Contains(l.Text, "AI 开始摸索，自己找要盯的内容") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no start line in the record: %+v", got.Runs)
	}

	// The goal can be changed and cleared later too.
	watchers.WaitRuns(f.m)
	f.env.MustDo(http.MethodPatch, "/watchers/"+itoa(w.Id), map[string]any{"goal": "盯办理状态"}, nil)
	f.env.MustDo(http.MethodPatch, "/watchers/"+itoa(w.Id), map[string]any{"goal": ""}, nil)
}

func TestKindFromAddressOnCreate(t *testing.T) {
	f := setup(t, false)
	for in, want := range map[string]api.WatcherKind{
		"@alice":                       api.X,
		"x.com/alice":                  api.X,
		"美联储 降息":                       api.X,
		"https://www.example.gov.cn/a": api.Web,
	} {
		w := f.create(t, map[string]any{"name": "w", "url": in, "goal": ""})
		if w.Kind != want {
			t.Errorf("%q: kind %s, want %s", in, w.Kind, want)
		}
	}
	// A kind that is given wins.
	w := f.create(t, map[string]any{"name": "w", "kind": "web", "url": "https://www.example.gov.cn/b", "goal": ""})
	if w.Kind != api.Web {
		t.Errorf("explicit kind: %s", w.Kind)
	}
	for _, bad := range []string{"example.com/a", "ftp://a.cn", ""} {
		status, _ := f.env.Do(http.MethodPost, "/watchers", map[string]any{"name": "w", "url": bad, "goal": ""}, nil)
		if status != http.StatusBadRequest {
			t.Errorf("%q: status %d", bad, status)
		}
	}
	// The kind does not change on update: the patch has no such field.
	if status, _ := f.env.Do(http.MethodPatch, "/watchers/"+itoa(w.Id), map[string]any{"kind": "x"}, nil); status != http.StatusBadRequest {
		t.Errorf("patch with kind: %d", status)
	}
	var p api.Watcher
	f.env.MustDo(http.MethodPatch, "/watchers/"+itoa(w.Id), map[string]any{"name": "w2"}, &p)
	if p.Kind != api.Web {
		t.Errorf("kind changed to %s", p.Kind)
	}
}

func itoa(id int64) string { return strconv.FormatInt(id, 10) }
