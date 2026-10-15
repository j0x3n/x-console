package browser_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/browser"
	"github.com/j0x3n/x-console/backend/internal/server/browser/browsertest"
)

// site is a small fake website with a login, a status page and a captcha.
func site(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			if r.Form.Get("user") == "alice" && r.Form.Get("pass") == "s3cret" {
				http.SetCookie(w, &http.Cookie{Name: "sid", Value: "ok", Path: "/"})
				http.Redirect(w, r, "/status", http.StatusSeeOther)
				return
			}
		}
		fmt.Fprint(w, `<html><body><form method="post" action="/login">
<input name="user" placeholder="用户名"><input name="pass" type="password" placeholder="密码">
<button type="submit">登录</button></form></body></html>`)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("sid"); err != nil || c.Value != "ok" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		fmt.Fprint(w, `<html><body><h1>我的办件</h1><table><tr><td>办理状态</td><td id="st">审核中</td></tr></table>
<script>localStorage.setItem('token','t1')</script></body></html>`)
	})
	mux.HandleFunc("/captcha", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><input name="code" placeholder="请输入验证码"><img src="/captcha.png" width="80" height="30"></body></html>`)
	})
	mux.HandleFunc("/frame", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><p>外层</p><iframe src="/inner" width="400" height="200"></iframe></body></html>`)
	})
	mux.HandleFunc("/inner", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><button onclick="document.getElementById('o').textContent='点过了'">框里的按钮</button><span id="o">没点</span></body></html>`)
	})
	mux.HandleFunc("/away", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><body><a href="%s">去别的站</a></body></html>`, r.URL.Query().Get("to"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return c
}

// ref finds the ref of the first snapshot line that contains want.
func ref(t *testing.T, p browser.Page, want string) string {
	t.Helper()
	for _, line := range strings.Split(p.Text, "\n") {
		if strings.Contains(line, want) && strings.HasPrefix(line, "[") {
			return line[1:strings.Index(line, "]")]
		}
	}
	t.Fatalf("snapshot has no %q:\n%s", want, p.Text)
	return ""
}

func TestLoginStateAndLocators(t *testing.T) {
	s := browsertest.Service(t)
	srv := site(t)
	c := ctx(t)

	x, err := s.Open(c, browser.Options{Key: "w1", AllowedHosts: []string{"127.0.0.1"}, Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Live("w1") != x {
		t.Fatal("live session not found")
	}
	if _, err := s.Open(c, browser.Options{Key: "w1"}); !errors.Is(err, browser.ErrBusy) {
		t.Fatalf("second open with the same key: %v", err)
	}
	if err := x.Navigate(c, srv.URL+"/status"); err != nil {
		t.Fatal(err)
	}
	st, err := x.Check(c)
	if err != nil || !st.Password || !strings.HasSuffix(st.URL, "/login") {
		t.Fatalf("login page not detected: %+v %v", st, err)
	}
	p, err := x.Snapshot(c, 0)
	if err != nil {
		t.Fatal(err)
	}
	user, pass := ref(t, p, `"用户名"`), ref(t, p, "type=password")
	if err := x.Type(c, user, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := x.Type(c, pass, "s3cret"); err != nil {
		t.Fatal(err)
	}
	p, _ = x.Snapshot(c, 0)
	if !strings.Contains(p.Text, `值="已填"`) || strings.Contains(p.Text, "s3cret") {
		t.Fatalf("password must show as filled and never as text:\n%s", p.Text)
	}
	if err := x.Click(c, ref(t, p, `按钮 "登录"`)); err != nil {
		t.Fatal(err)
	}
	p, _ = x.Snapshot(c, 0)
	value := ref(t, p, "审核中")
	loc, err := x.Describe(c, value)
	if err != nil || loc.ID != "st" || loc.Anchor != "办理状态" {
		t.Fatalf("locator: %+v %v", loc, err)
	}
	loc.Region = true
	shot, err := x.Screenshot(c, 60)
	if err != nil || len(shot) < 100 || shot[0] != 0xFF {
		t.Fatalf("screenshot: %d bytes, %v", len(shot), err)
	}
	state, err := x.SaveState(c)
	if err != nil || !strings.Contains(string(state), `"sid"`) || !strings.Contains(string(state), `"token":"t1"`) {
		t.Fatalf("state: %s %v", state, err)
	}
	x.Close()
	if s.Live("w1") != nil {
		t.Fatal("closed session still live")
	}

	// A new session with the saved state is still logged in, and the
	// locator finds the value again even without its id.
	y, err := s.Open(c, browser.Options{Key: "w1", State: state, AllowedHosts: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer y.Close()
	if err := y.Navigate(c, srv.URL+"/status"); err != nil {
		t.Fatal(err)
	}
	if u, _ := y.URL(c); !strings.HasSuffix(u, "/status") {
		t.Fatalf("not logged in after restore: %s", u)
	}
	loc.ID = ""
	r, err := y.Locate(c, loc)
	if err != nil {
		t.Fatal(err)
	}
	if txt, err := y.Text(c, r); err != nil || txt != "审核中" {
		t.Fatalf("text %q %v", txt, err)
	}
	loc.CSS = "#nothing"
	if r, err = y.Locate(c, loc); err != nil {
		t.Fatalf("anchor fallback: %v", err)
	}
	if txt, _ := y.Text(c, r); txt != "审核中" {
		t.Fatalf("anchor found %q", txt)
	}
}

func TestCaptchaFramesAndHosts(t *testing.T) {
	s := browsertest.Service(t)
	srv := site(t)
	c := ctx(t)
	x, err := s.Open(c, browser.Options{AllowedHosts: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	if err := x.Navigate(c, srv.URL+"/captcha"); err != nil {
		t.Fatal(err)
	}
	if st, _ := x.Check(c); st.Captcha == "" {
		t.Fatalf("captcha not detected: %+v", st)
	}
	// B214: the image and its field are found, and the image can be shot alone.
	parts, ok, err := x.Captcha(c)
	if err != nil || !ok || parts.Image == "" || parts.Input == "" {
		t.Fatalf("captcha parts: %+v %v %v", parts, ok, err)
	}
	png, err := x.ElementShot(c, parts.Image)
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err := pngSize(png); err != nil || cfg[0] < 70 || cfg[0] > 90 || cfg[1] < 25 || cfg[1] > 35 {
		t.Fatalf("element shot size %v %v", cfg, err)
	}

	if err := x.Navigate(c, srv.URL+"/frame"); err != nil {
		t.Fatal(err)
	}
	p, _ := x.Snapshot(c, 0)
	if err := x.Click(c, ref(t, p, "框里的按钮")); err != nil {
		t.Fatal(err)
	}
	p, _ = x.Snapshot(c, 0)
	if !strings.Contains(p.Text, "点过了") {
		t.Fatalf("click inside a frame did nothing:\n%s", p.Text)
	}
	loc, err := x.Describe(c, ref(t, p, "框里的按钮"))
	if err != nil || len(loc.Frame) != 1 {
		t.Fatalf("frame locator %+v %v", loc, err)
	}
	if _, err := x.Locate(c, loc); err != nil {
		t.Fatal(err)
	}

	other := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
	if err := x.Navigate(c, other+"/status"); !errors.Is(err, browser.ErrHostNotAllowed) {
		t.Fatalf("navigate to another host: %v", err)
	}
	if err := x.Navigate(c, srv.URL+"/away?to="+other+"/login"); err != nil {
		t.Fatal(err)
	}
	p, _ = x.Snapshot(c, 0)
	if err := x.Click(c, ref(t, p, "去别的站")); !errors.Is(err, browser.ErrHostNotAllowed) {
		t.Fatalf("click to another host: %v", err)
	}
}

func TestSlotsAndTakeover(t *testing.T) {
	s := browsertest.Service(t)
	s.SetLimits(1, 2*time.Second)
	srv := site(t)
	c := ctx(t)
	x, err := s.Open(c, browser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	// The only slot is taken, so a second open waits.
	short, cancel := context.WithTimeout(c, 300*time.Millisecond)
	defer cancel()
	if _, err := s.Open(short, browser.Options{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("open without a slot: %v", err)
	}
	if err := x.Navigate(c, srv.URL+"/login"); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var frames []browser.Frame
	sc, stop := context.WithCancel(c)
	go func() {
		_ = x.Screencast(sc, func(f browser.Frame) {
			mu.Lock()
			frames = append(frames, f)
			mu.Unlock()
		})
	}()
	p, _ := x.Snapshot(c, 0)
	r := ref(t, p, `"用户名"`)
	if err := x.Type(c, r, ""); err != nil { // focus the field
		t.Fatal(err)
	}
	if err := x.Send(c, browser.Input{Type: "text", Text: "bob"}); err != nil {
		t.Fatal(err)
	}
	p, _ = x.Snapshot(c, 0)
	if !strings.Contains(p.Text, `值="bob"`) {
		t.Fatalf("typed text missing:\n%s", p.Text)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(frames)
		mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	stop()
	mu.Lock()
	if len(frames) == 0 || frames[0].Width == 0 || len(frames[0].JPEG) == 0 {
		t.Fatalf("no screencast frame: %d", len(frames))
	}
	mu.Unlock()

	// The session closes itself after the maximum use time and frees the slot.
	select {
	case <-x.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not close after max use")
	}
	y, err := s.Open(c, browser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	y.Close()
}

func TestHostAllowed(t *testing.T) {
	hosts := []string{"https://gov.cn/x", "*.example.com"}
	for u, want := range map[string]bool{
		"https://www.gov.cn/a":     true,
		"https://gov.cn":           true,
		"https://evilgov.cn":       false,
		"http://a.b.example.com/":  true,
		"file:///etc/passwd":       false,
		"about:blank":              true,
		"https://example.org/path": false,
	} {
		if got := browser.HostAllowed(hosts, u); got != want {
			t.Errorf("%s: got %v", u, got)
		}
	}
	if !browser.HostAllowed(nil, "https://any.where") {
		t.Error("no hosts allows everything")
	}
}

func TestUnavailable(t *testing.T) {
	s := browser.New("", nil)
	if _, err := s.Open(context.Background(), browser.Options{}); !errors.Is(err, browser.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	s = browser.New("http://127.0.0.1:1", nil)
	if _, err := s.Check(context.Background()); !errors.Is(err, browser.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}

// TestRemoteAddress connects the way the server does in production: to the
// DevTools HTTP address of a browser that runs elsewhere.
func TestRemoteAddress(t *testing.T) {
	path := browsertest.Path()
	if path == "" {
		t.Skip("没有找到 Chrome")
	}
	// Not t.TempDir: Chrome's helper processes may still write there after
	// the main process is gone, and the test would fail on cleanup.
	dir, err := os.MkdirTemp("", "xc-remote-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cmd := exec.Command(path, "--headless=new", "--remote-debugging-port=0", "--user-data-dir="+dir, "--no-sandbox", "about:blank")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	re := regexp.MustCompile(`DevTools listening on ws://([^/]+)/`)
	sc := bufio.NewScanner(stderr)
	var hostport string
	for sc.Scan() {
		if m := re.FindStringSubmatch(sc.Text()); m != nil {
			hostport = m[1]
			break
		}
	}
	go func() { _, _ = io.Copy(io.Discard, stderr) }()
	if hostport == "" {
		t.Fatal("browser did not start")
	}
	s := browser.New("http://"+hostport, nil)
	t.Cleanup(s.Close)
	v, err := s.Check(ctx(t))
	if err != nil || !strings.Contains(v, "Chrome") {
		t.Fatalf("version %q %v", v, err)
	}
	x, err := s.Open(ctx(t), browser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	x.Close()
}

func pngSize(b []byte) ([2]int, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(b))
	return [2]int{cfg.Width, cfg.Height}, err
}
