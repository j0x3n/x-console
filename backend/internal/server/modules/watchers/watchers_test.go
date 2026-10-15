package watchers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/j0x3n/x-console/backend/internal/server/browser"
	"github.com/j0x3n/x-console/backend/internal/server/browser/browsertest"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// ---- a fake government site ----

type site struct {
	*httptest.Server
	mu       sync.Mutex
	value    string
	captcha  bool
	sessions map[string]bool
	revoked  int
	logins   int
	// business is the rows of /business: type, name, status. flat shows
	// them as one paragraph, as if the site was redesigned.
	business [][3]string
	flat     bool
}

func newSite(t *testing.T) *site {
	s := &site{value: "审核中", sessions: map[string]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			ok := r.Form.Get("user") == "alice" && r.Form.Get("pass") == "s3cret"
			if s.captcha && r.Form.Get("code") != "1234" {
				ok = false
			}
			if ok {
				s.logins++
				token := "t" + strconv.Itoa(s.logins)
				s.sessions[token] = true
				http.SetCookie(w, &http.Cookie{Name: "sid", Value: token, Path: "/"})
				http.Redirect(w, r, "/status", http.StatusSeeOther)
				return
			}
		}
		code := ""
		if s.captcha {
			code = `<input name="code" placeholder="请输入验证码"><img src="/captcha.png" alt="验证码" width="80" height="30">`
		}
		fmt.Fprintf(w, `<html><body><h2>统一身份认证</h2><form method="post" action="/login">
<input name="user" placeholder="用户名"><input name="pass" type="password" placeholder="密码">%s
<button type="submit">登录</button></form></body></html>`, code)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		c, err := r.Cookie("sid")
		if err != nil || !s.sessions[c.Value] {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		fmt.Fprintf(w, `<html><body><h1>我的办件</h1><p>访问时间 %d</p>
<table><tr><td>办理状态</td><td id="st">%s</td></tr></table>
<form method="post" action="/revoke"><button type="submit">撤销申请</button></form></body></html>`, time.Now().UnixNano(), s.value)
	})
	mux.HandleFunc("/business", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var b strings.Builder
		b.WriteString(`<html><body><h2>我的业务</h2><div class="list"><div class="head"><span>业务</span><span>名称</span><span><b>状态</b></span></div>`)
		for _, row := range s.business {
			if s.flat {
				fmt.Fprintf(&b, `<p>%s %s %s</p>`, row[0], row[1], row[2])
				continue
			}
			fmt.Fprintf(&b, `<div class="row"><span>%s</span><span>%s</span><span><b>%s</b></span></div>`, row[0], row[1], row[2])
		}
		b.WriteString(`</div></body></html>`)
		fmt.Fprint(w, b.String())
	})
	mux.HandleFunc("/revoke", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.revoked++
		s.mu.Unlock()
		fmt.Fprint(w, `<html><body>已撤销</body></html>`)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func (s *site) set(f func(*site)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

// ---- a scripted AI ----

// move picks the next tool call from the page the model last saw.
type move func(page string) (string, map[string]any)

type fakeChat struct {
	t     *testing.T
	mu    sync.Mutex
	moves []move
	next  int
	calls int
	seen  []string
	spent float64
	// manual turns off the automatic confirm of finish.
	manual bool
	// models is the B212 model choice each call carried.
	models []contracts.AIModel
	// codes are the answers to captcha reads, in order (B214). reads
	// counts the reads that came with a PNG image.
	codes []string
	reads int
}

func (f *fakeChat) autoConfirm() bool { return !f.manual }

func (f *fakeChat) Available(context.Context, string) bool { return true }

func (f *fakeChat) Spent(context.Context, string, string, time.Time) (float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.spent, nil
}

func (f *fakeChat) Chat(ctx context.Context, _, system string, msgs []contracts.ChatMessage, _ []contracts.ChatTool) (contracts.ChatReply, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	c, _ := contracts.AIModelFrom(ctx)
	f.models = append(f.models, c)
	if strings.Contains(system, "图形验证码") && len(msgs) == 1 && strings.HasPrefix(system, "你在读") {
		if len(msgs[0].Images) == 1 && msgs[0].Images[0].MIME == "image/png" {
			f.reads++
		}
		answer := "看不清"
		if len(f.codes) > 0 {
			answer, f.codes = f.codes[0], f.codes[1:]
		}
		return contracts.ChatReply{Text: answer, InputTokens: 50, OutputTokens: 2}, nil
	}
	f.seen = append(f.seen, system)
	for _, m := range msgs {
		f.seen = append(f.seen, m.Content)
		for _, c := range m.ToolCalls {
			f.seen = append(f.seen, string(c.Arguments))
		}
	}
	page := ""
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "assistant" {
			page = msgs[i].Content
			break
		}
	}
	// finish asks to confirm what it read. A scripted finish confirms by
	// itself unless the script says otherwise.
	if n := len(msgs); n >= 2 && f.autoConfirm() && (strings.HasPrefix(msgs[n-1].Content, "这块区域读到的文字是") || strings.HasPrefix(msgs[n-1].Content, "按格式读到")) {
		if prev := msgs[n-2].ToolCalls; len(prev) == 1 && prev[0].Name == "finish" {
			return contracts.ChatReply{ToolCalls: []contracts.ChatToolCall{{ID: "c" + strconv.Itoa(f.calls), Name: "finish", Arguments: prev[0].Arguments}},
				InputTokens: 100, OutputTokens: 10}, nil
		}
	}
	name, args := "give_up", map[string]any{"reason": "脚本走完了"}
	if f.next < len(f.moves) {
		name, args = f.moves[f.next](page)
		f.next++
	}
	raw, _ := json.Marshal(args)
	return contracts.ChatReply{ToolCalls: []contracts.ChatToolCall{{ID: "c" + strconv.Itoa(f.calls), Name: name, Arguments: raw}},
		InputTokens: 100, OutputTokens: 10}, nil
}

func (f *fakeChat) script(moves ...move) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.moves, f.next = moves, 0
}

func (f *fakeChat) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeChat) saw(s string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.seen {
		if strings.Contains(x, s) {
			return true
		}
	}
	return false
}

// ref finds the element number of the first line of page that has want.
func ref(page, want string) string {
	for _, line := range strings.Split(page, "\n") {
		if strings.HasPrefix(line, "[") && strings.Contains(line, want) {
			return line[1:strings.Index(line, "]")]
		}
	}
	return "missing:" + want
}

func do(name string, args func(page string) map[string]any) move {
	return func(page string) (string, map[string]any) { return name, args(page) }
}

func click(want string) move {
	return do("click", func(p string) map[string]any { return map[string]any{"ref": ref(p, want)} })
}

func fill(want, field string) move {
	return do("fill_login", func(p string) map[string]any { return map[string]any{"ref": ref(p, want), "field": field} })
}

func finish(want string) move {
	return do("finish", func(p string) map[string]any { return map[string]any{"ref": ref(p, want), "note": "办理状态"} })
}

type fakeLLM struct {
	calls  int
	models []contracts.AIModel
}

func (f *fakeLLM) Available(context.Context) bool { return true }
func (f *fakeLLM) CompleteText(context.Context, string, string, string) (string, error) {
	return "", nil
}
func (f *fakeLLM) CompleteJSON(ctx context.Context, _, system, user string, _ json.RawMessage, out any) error {
	f.calls++
	c, _ := contracts.AIModelFrom(ctx)
	f.models = append(f.models, c)
	if strings.Contains(system, "推文") {
		// keep only posts about 降息
		keep := []int{}
		for _, line := range strings.Split(user, "\n") {
			var i int
			if _, err := fmt.Sscanf(line, "[%d]", &i); err == nil && strings.Contains(line, "降息") {
				keep = append(keep, i)
			}
		}
		raw, _ := json.Marshal(map[string]any{"keep": keep, "summary": "有降息的消息"})
		return json.Unmarshal(raw, out)
	}
	return json.Unmarshal([]byte(`{"important":true,"summary":"办理状态变成已受理"}`), out)
}

// ---- fixture ----

type fixture struct {
	env  *testutil.Env
	m    *watchers.Module
	chat *fakeChat
	llm  *fakeLLM
	site *site
}

func setup(t *testing.T, withBrowser bool) *fixture {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*watchers.Module](env.App.Deps.Registry, watchers.ServiceKey)
	if !ok {
		t.Fatal("watchers module not registered")
	}
	f := &fixture{env: env, m: m, chat: &fakeChat{t: t}, llm: &fakeLLM{}, site: newSite(t)}
	if withBrowser {
		watchers.SetBrowser(m, browsertest.Service(t))
	} else {
		watchers.SetBrowser(m, browser.New("", nil))
	}
	watchers.SetChat(m, f.chat)
	watchers.SetLLM(m, f.llm)
	watchers.SetWait(m, 20*time.Second)
	t.Cleanup(func() { watchers.WaitRuns(m) })
	return f
}

func (f *fixture) get(t *testing.T, id int64) api.WatcherDetail {
	t.Helper()
	var out api.WatcherDetail
	f.env.MustDo(http.MethodGet, fmt.Sprintf("/watchers/%d", id), nil, &out)
	return out
}

// until waits for the watcher to reach a status.
func (f *fixture) until(t *testing.T, id int64, status api.WatcherStatus) api.WatcherDetail {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for {
		w := f.get(t, id)
		if w.Status == status {
			return w
		}
		if time.Now().After(deadline) {
			t.Fatalf("watcher %d stayed %s (%s), want %s; runs: %+v", id, w.Status, w.StatusDetail, status, w.Runs)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (f *fixture) create(t *testing.T, body map[string]any) api.Watcher {
	t.Helper()
	f.env.Elevate()
	var w api.Watcher
	f.env.MustDo(http.MethodPost, "/watchers", body, &w)
	return w
}

func (f *fixture) notified(t *testing.T, kind string) int {
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

func loginMoves() []move {
	return []move{fill(`"用户名"`, "username"), fill("type=password", "password"), click(`按钮 "登录"`)}
}

func TestExploreReplayAndChange(t *testing.T) {
	f := setup(t, true)
	f.chat.script(append(loginMoves(), finish("审核中"))...)
	w := f.create(t, map[string]any{"name": "签证", "url": f.site.URL + "/status", "goal": "盯办理状态",
		"username": "alice", "password": "s3cret"})
	if !w.HasLogin || w.Status != api.WatcherStatusNew || len(w.AllowedHosts) != 1 || w.AllowedHosts[0] != "127.0.0.1" {
		t.Fatalf("created %+v", w)
	}
	got := f.until(t, w.Id, api.WatcherStatusOk)
	watchers.WaitRuns(f.m)
	if got.CurrentValue != "审核中" || !got.HasRecipe || got.TargetNote == nil || len(got.Recipe) < 4 {
		t.Fatalf("after explore: %+v", got)
	}
	if got.Recipe[1].Text != "填用户名" || got.Recipe[2].Text != "填密码" {
		t.Fatalf("recipe %+v", got.Recipe)
	}
	if f.chat.saw("s3cret") || f.chat.saw("alice") {
		t.Fatal("the login details reached the model")
	}
	if len(got.Runs) != 1 || got.Runs[0].Outcome != api.WatcherRunOutcomeOk || got.Runs[0].Tokens == 0 {
		t.Fatalf("runs %+v", got.Runs)
	}

	// The value changes. The check replays the steps without AI, the saved
	// login skips the form, and the change is judged and notified.
	f.site.set(func(s *site) { s.value = "已受理" })
	calls := f.chat.count()
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
	watchers.WaitRuns(f.m)
	got = f.get(t, w.Id)
	if got.Status != api.WatcherStatusOk || got.CurrentValue != "已受理" || f.chat.count() != calls {
		t.Fatalf("after check: %s %q %s, chat calls %d -> %d\n%s", got.Status, got.CurrentValue, got.StatusDetail, calls, f.chat.count(), runText(got.Runs[0]))
	}
	if len(got.Changes) != 1 || got.Changes[0].OldValue != "审核中" || got.Changes[0].Summary != "办理状态变成已受理" || got.Changes[0].ShotId == nil {
		t.Fatalf("changes %+v", got.Changes)
	}
	if f.notified(t, "watcher.changed") != 1 {
		t.Fatal("no change notification")
	}
	f.site.mu.Lock()
	logins := f.site.logins
	f.site.mu.Unlock()
	if logins != 1 {
		t.Fatalf("saved login was not used: %d logins", logins)
	}
	status, _ := f.env.Do(http.MethodGet, fmt.Sprintf("/watchers/shots/%d", *got.Changes[0].ShotId), nil, nil)
	if status != http.StatusOK {
		t.Fatalf("shot: %d", status)
	}

	// The site drops the login. The replay fills the form again by itself.
	f.site.set(func(s *site) { s.sessions = map[string]bool{} })
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
	watchers.WaitRuns(f.m)
	got = f.get(t, w.Id)
	if got.Status != api.WatcherStatusOk || got.CurrentValue != "已受理" || len(got.Changes) != 1 || f.chat.count() != calls {
		t.Fatalf("after relogin: %s %q %s", got.Status, got.CurrentValue, got.StatusDetail)
	}

	// A wrong password ends as "needs login" and does not ask AI to repair.
	f.env.Elevate()
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/watchers/%d", w.Id), map[string]any{"password": "wrong"}, nil)
	f.site.set(func(s *site) { s.sessions = map[string]bool{} })
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
	watchers.WaitRuns(f.m)
	got = f.get(t, w.Id)
	if got.Status != api.WatcherStatusLogin || !strings.Contains(got.StatusDetail, "密码") || f.chat.count() != calls {
		t.Fatalf("wrong password: %s %s", got.Status, got.StatusDetail)
	}
}

func TestCaptchaIsHandedToTheUser(t *testing.T) {
	f := setup(t, true)
	f.site.set(func(s *site) { s.captcha = true })
	f.chat.script(fill(`"用户名"`, "username"), fill("type=password", "password"),
		do("ask_user", func(string) map[string]any {
			return map[string]any{"kind": "captcha", "message": "请输入图形验证码"}
		}),
		click(`按钮 "登录"`), finish("审核中"))
	w := f.create(t, map[string]any{"name": "社保", "url": f.site.URL + "/status", "goal": "盯办理状态",
		"username": "alice", "password": "s3cret"})
	got := f.until(t, w.Id, api.WatcherStatusWaiting)
	if got.Pending == nil || got.Pending.Kind != api.WatcherPendingKindCaptcha || got.Pending.Live == nil || !*got.Pending.Live {
		t.Fatalf("pending %+v", got.Pending)
	}
	if f.notified(t, "watcher.ask") != 1 {
		t.Fatal("no ask notification")
	}

	// The takeover view needs elevation, shows the page and takes input.
	f.env.Elevate()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, f.env.WSURL(fmt.Sprintf("/watchers/%d/live", w.Id)), &websocket.DialOptions{HTTPClient: f.env.Client})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	ws.SetReadLimit(8 << 20)
	typ, frame, err := ws.Read(ctx)
	if err != nil || typ != websocket.MessageBinary || len(frame) < 100 {
		t.Fatalf("first frame: %v %d %v", typ, len(frame), err)
	}
	// The user types the captcha: focus the field the way a click would,
	// then send the text through the takeover connection.
	sess := watchers.Live(f.m, w.Id)
	p, err := sess.Snapshot(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Type(ctx, ref(p.Text, "验证码\""), ""); err != nil {
		t.Fatal(err)
	}
	in, _ := json.Marshal(browser.Input{Type: "text", Text: "1234"})
	if err := ws.Write(ctx, websocket.MessageText, in); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		p, _ = sess.Snapshot(ctx, 0)
		if strings.Contains(p.Text, `值="1234"`) || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(p.Text, `值="1234"`) {
		t.Fatalf("typed captcha missing:\n%s", p.Text)
	}
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/answer", w.Id), map[string]string{"answer": "done"}, nil)
	got = f.until(t, w.Id, api.WatcherStatusOk)
	watchers.WaitRuns(f.m)
	if got.CurrentValue != "审核中" {
		t.Fatalf("value %q", got.CurrentValue)
	}
	found := false
	for _, s := range got.Recipe {
		if strings.Contains(s.Text, "验证码") {
			found = true
		}
	}
	if !found {
		t.Fatalf("recipe has no captcha step: %+v", got.Recipe)
	}
	status, _ := f.env.Do(http.MethodPost, fmt.Sprintf("/watchers/%d/answer", w.Id), map[string]string{"answer": "done"}, nil)
	if status != http.StatusConflict {
		t.Fatalf("answer while not waiting: %d", status)
	}
}

func TestDangerousClickNeedsApproval(t *testing.T) {
	f := setup(t, true)
	f.chat.script(append(loginMoves(), click("撤销申请"), finish("审核中"))...)
	w := f.create(t, map[string]any{"name": "公积金", "url": f.site.URL + "/status", "goal": "盯办理状态",
		"username": "alice", "password": "s3cret"})
	got := f.until(t, w.Id, api.WatcherStatusWaiting)
	if got.Pending == nil || got.Pending.Kind != api.WatcherPendingKindApprove || !strings.Contains(got.Pending.Message, "撤销申请") {
		t.Fatalf("pending %+v", got.Pending)
	}
	// The deny button of the notification answers the run.
	if f.notified(t, "watcher.ask") != 1 {
		t.Fatal("no ask notification")
	}
	action := fmt.Sprintf("watcher.deny:%d", w.Id)
	if err := f.env.App.Deps.Notify.HandleAction(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	got = f.until(t, w.Id, api.WatcherStatusOk)
	f.site.mu.Lock()
	revoked := f.site.revoked
	f.site.mu.Unlock()
	if revoked != 0 || got.CurrentValue != "审核中" {
		t.Fatalf("revoked %d, value %q", revoked, got.CurrentValue)
	}
	if !f.chat.saw("用户不让点这个按钮") {
		t.Fatal("the model was not told about the denial")
	}
}

func TestBrokenStepsAreRepairedByAI(t *testing.T) {
	f := setup(t, true)
	f.chat.script(append(loginMoves(), finish("审核中"))...)
	w := f.create(t, map[string]any{"name": "护照", "url": f.site.URL + "/status", "goal": "盯办理状态",
		"username": "alice", "password": "s3cret", "intervalMinutes": 30})
	f.until(t, w.Id, api.WatcherStatusOk)
	watchers.WaitRuns(f.m)
	// The page changes so the value can no longer be found; the check hands
	// over to AI, which finds the new place.
	f.site.set(func(s *site) { s.value = "" })
	f.chat.script(finish("办理状态"))
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
	watchers.WaitRuns(f.m)
	got := f.get(t, w.Id)
	if got.Status != api.WatcherStatusOk || got.CurrentValue != "办理状态" {
		t.Fatalf("after repair: %s %q %s", got.Status, got.CurrentValue, got.StatusDetail)
	}
	if got.Runs[0].Kind != api.WatcherRunKindCheck || !strings.Contains(runText(got.Runs[0]), "让 AI 接着找") {
		t.Fatalf("run %+v", got.Runs[0])
	}
}

func runText(r api.WatcherRun) string {
	var b strings.Builder
	for _, s := range r.Steps {
		b.WriteString(s.Text + "\n")
	}
	return b.String()
}

func TestValidationPermissionsAndSettings(t *testing.T) {
	f := setup(t, false)
	body := map[string]any{"name": "x", "url": "https://www.example.gov.cn/a", "goal": "盯", "password": "p"}
	if status, _ := f.env.Do(http.MethodPost, "/watchers", body, nil); status != http.StatusForbidden {
		t.Fatalf("login without elevation: %d", status)
	}
	for _, bad := range []map[string]any{
		{"name": "x", "url": "ftp://a", "goal": "g"},
		{"name": "", "url": "https://a.cn", "goal": "g"},
		{"name": "x", "url": "https://a.cn", "goal": "g", "intervalMinutes": 5},
		{"name": "x", "url": "https://a.cn", "goal": "g", "allowedHosts": []string{"b.cn"}},
	} {
		if status, _ := f.env.Do(http.MethodPost, "/watchers", bad, nil); status != http.StatusBadRequest {
			t.Fatalf("%v: %d", bad, status)
		}
	}
	if status, _ := f.env.Do(http.MethodGet, "/watchers/999", nil, nil); status != http.StatusNotFound {
		t.Fatalf("missing: %d", status)
	}
	if got := watchers.BaseDomain("www.example.gov.cn"); got != "example.gov.cn" {
		t.Fatalf("base domain %s", got)
	}

	// Without a browser the first exploration fails with a clear reason.
	w := f.create(t, map[string]any{"name": "x", "url": "https://www.example.gov.cn/a", "goal": "盯状态", "proxy": "socks5://u:p@10.0.0.1:1080"})
	if w.AllowedHosts[0] != "example.gov.cn" || !w.HasProxy {
		t.Fatalf("created %+v", w)
	}
	got := f.until(t, w.Id, api.WatcherStatusError)
	if !strings.Contains(got.StatusDetail, "浏览器没有启动") {
		t.Fatalf("detail %q", got.StatusDetail)
	}
	watchers.WaitRuns(f.m)

	var st api.WatchersStatus
	f.env.MustDo(http.MethodGet, "/watchers/status", nil, &st)
	if st.BrowserConfigured || !st.Ai || st.BudgetUsd != 1 {
		t.Fatalf("status %+v", st)
	}
	var set api.WatchersSettings
	f.env.MustDo(http.MethodPut, "/watchers/settings", map[string]any{"budgetUsd": 0, "dangerWords": []string{" 提交 ", ""}}, &set)
	if set.BudgetUsd != 0 || len(set.DangerWords) != 1 || set.DangerWords[0] != "提交" {
		t.Fatalf("settings %+v", set)
	}
	if status, _ := f.env.Do(http.MethodPut, "/watchers/settings", map[string]any{"budgetUsd": -1, "dangerWords": []string{}}, nil); status != http.StatusBadRequest {
		t.Fatalf("negative budget: %d", status)
	}

	// Pausing keeps the watcher out of the schedule; resuming makes it due.
	var p api.Watcher
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/watchers/%d", w.Id), map[string]any{"enabled": false}, &p)
	if p.Status != api.WatcherStatusPaused {
		t.Fatalf("paused %+v", p)
	}
	if err := watchers.Tick(f.m, context.Background()); err != nil {
		t.Fatal(err)
	}
	watchers.WaitRuns(f.m)
	if len(f.get(t, w.Id).Runs) != 1 {
		t.Fatal("a paused watcher ran")
	}

	// Actions.
	out, err := f.env.App.Deps.Actions.Run(context.Background(), "watchers.list", json.RawMessage(`{}`))
	if err != nil || !strings.Contains(mustJSON(out), `"name":"x"`) {
		t.Fatalf("watchers.list %v %v", out, err)
	}
	if _, err := f.env.App.Deps.Actions.Run(context.Background(), "watchers.check", json.RawMessage(fmt.Sprintf(`{"id":%d}`, w.Id))); err != nil {
		t.Fatal(err)
	}
	watchers.WaitRuns(f.m)
	if err := watchers.Cleanup(f.m, context.Background()); err != nil {
		t.Fatal(err)
	}

	if status, _ := f.env.Do(http.MethodGet, fmt.Sprintf("/watchers/%d/live", w.Id), nil, nil); status != http.StatusConflict {
		t.Fatalf("live without a browser: %d", status)
	}
	f.env.MustDo(http.MethodDelete, fmt.Sprintf("/watchers/%d", w.Id), nil, nil)
	if status, _ := f.env.Do(http.MethodGet, fmt.Sprintf("/watchers/%d", w.Id), nil, nil); status != http.StatusNotFound {
		t.Fatalf("after delete: %d", status)
	}
}

func TestBudgetStopsTheAgent(t *testing.T) {
	f := setup(t, true)
	f.env.MustDo(http.MethodPut, "/watchers/settings", map[string]any{"budgetUsd": 0.5, "dangerWords": []string{}}, nil)
	f.chat.spent = 0.6
	f.chat.script(append(loginMoves(), finish("审核中"))...)
	w := f.create(t, map[string]any{"name": "x", "url": f.site.URL + "/status", "goal": "盯办理状态"})
	got := f.until(t, w.Id, api.WatcherStatusError)
	if !strings.Contains(got.StatusDetail, "上限") || f.chat.count() != 0 {
		t.Fatalf("budget: %q, %d calls", got.StatusDetail, f.chat.count())
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// The model only picks a heading at first. finish shows it what it read, it
// switches to the whole row, and the row is kept after it confirms.
func TestFinishConfirmsWhatItRead(t *testing.T) {
	f := setup(t, true)
	f.chat.manual = true
	var row []string
	pick := do("finish", func(p string) map[string]any {
		row = []string{ref(p, "] 办理状态"), ref(p, "审核中")}
		return map[string]any{"refs": []string{ref(p, "] 办理状态")}, "note": "办理状态"}
	})
	widen := do("finish", func(string) map[string]any { return map[string]any{"refs": row, "note": "办理状态那一行"} })
	f.chat.script(append(loginMoves(), pick, widen, widen)...)
	w := f.create(t, map[string]any{"name": "签证", "url": f.site.URL + "/status", "goal": "盯办理状态",
		"username": "alice", "password": "s3cret"})
	got := f.until(t, w.Id, api.WatcherStatusOk)
	watchers.WaitRuns(f.m)
	if got.CurrentValue != "办理状态 审核中" {
		t.Fatalf("value %q", got.CurrentValue)
	}
	if !f.chat.saw("这块区域读到的文字是：\n办理状态\n") {
		t.Fatal("the model was not shown what it read")
	}
	// A later check finds the same row without AI.
	f.site.set(func(s *site) { s.value = "已受理" })
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
	watchers.WaitRuns(f.m)
	if got := f.get(t, w.Id); got.CurrentValue != "办理状态 已受理" {
		t.Fatalf("after check: %q %s", got.CurrentValue, got.StatusDetail)
	}
}
