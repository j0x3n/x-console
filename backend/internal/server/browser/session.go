package browser

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed page.js
var pageJS string

// Options for Open.
type Options struct {
	// Key names the session, for example "watcher:3". Only one session per
	// key is open at a time; Live finds it.
	Key string
	// Proxy is the outgoing proxy, http://, https:// or socks5://, with an
	// optional user:password.
	Proxy string
	// State is what SaveState returned last time.
	State []byte
	// AllowedHosts limits where the page may go. A host matches itself and
	// its subdomains. Empty allows everything.
	AllowedHosts []string
	// Timezone and Locale of the page, for example "Asia/Shanghai" and "zh-CN".
	Timezone, Locale string
}

// Viewport size of every page.
const (
	ViewWidth  = 1280
	ViewHeight = 900
)

// Session is one page in its own browser context. Its methods are safe for
// concurrent use; actions run one at a time.
type Session struct {
	s       *Service
	c       *conn
	key     string
	ctxID   string
	target  string
	sid     string
	allowed []string

	ops      sync.Mutex // one action at a time
	mu       sync.Mutex
	origins  map[string]map[string]string
	deadline *time.Timer
	done     chan struct{}
	closed   bool
	release  func()

	mouseMu  sync.Mutex // the last position of the mouse
	mouseX   float64
	mouseY   float64
	mouseSet bool

	paceScale atomic.Int64 // percent of the human delays to wait, 0 is all

	inflight  map[string]bool
	lastNet   time.Time
	netMu     sync.Mutex
	unsubNet  func()
	unsubAuth func()
}

// ErrBusy means another session with the same key is open.
var ErrBusy = errors.New("这个任务的浏览器正在使用")

// ErrHostNotAllowed is returned when a page tries to leave the allowed hosts.
var ErrHostNotAllowed = errors.New("不允许访问这个域名")

// Open starts a session. It waits for a free slot until ctx ends.
func (s *Service) Open(ctx context.Context, o Options) (*Session, error) {
	if o.Key != "" && s.Live(o.Key) != nil {
		return nil, ErrBusy
	}
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	release := sync.OnceFunc(func() { <-s.slots })
	x, err := s.open(ctx, o)
	if err != nil {
		release()
		return nil, err
	}
	x.release = release
	s.mu.Lock()
	if o.Key != "" {
		if s.sessions[o.Key] != nil {
			s.mu.Unlock()
			x.Close()
			return nil, ErrBusy
		}
		s.sessions[o.Key] = x
	}
	s.mu.Unlock()
	x.deadline = time.AfterFunc(s.maxUse, x.Close)
	return x, nil
}

type savedState struct {
	Cookies []map[string]any             `json:"cookies"`
	Origins map[string]map[string]string `json:"origins"`
}

func (s *Service) open(ctx context.Context, o Options) (*Session, error) {
	c, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	params := map[string]any{"disposeOnDetach": true}
	var proxyUser, proxyPass string
	if o.Proxy != "" {
		u, err := url.Parse(o.Proxy)
		if err != nil || u.Host == "" {
			return nil, errors.New("代理地址不对，应该像 http://主机:端口 或 socks5://主机:端口")
		}
		if u.User != nil {
			proxyUser = u.User.Username()
			proxyPass, _ = u.User.Password()
		}
		params["proxyServer"] = u.Scheme + "://" + u.Host
	}
	var bc struct {
		ID string `json:"browserContextId"`
	}
	if err := c.call(ctx, "", "Target.createBrowserContext", params, &bc); err != nil {
		return nil, err
	}
	x := &Session{s: s, c: c, key: o.Key, ctxID: bc.ID, allowed: normalizeHosts(o.AllowedHosts),
		origins: map[string]map[string]string{}, done: make(chan struct{}), inflight: map[string]bool{}}
	fail := func(err error) (*Session, error) {
		_ = c.call(context.Background(), "", "Target.disposeBrowserContext", map[string]any{"browserContextId": bc.ID}, nil)
		return nil, err
	}
	var st savedState
	if len(o.State) > 0 {
		if err := json.Unmarshal(o.State, &st); err != nil {
			st = savedState{}
		}
	}
	if len(st.Cookies) > 0 {
		if err := c.call(ctx, "", "Storage.setCookies", map[string]any{"cookies": cookieParams(st.Cookies), "browserContextId": bc.ID}, nil); err != nil {
			s.log.Warn("browser restore cookies", "err", err)
		}
	}
	for k, v := range st.Origins {
		x.origins[k] = v
	}
	_ = c.call(ctx, "", "Browser.setDownloadBehavior", map[string]any{"behavior": "deny", "browserContextId": bc.ID}, nil)
	var t struct {
		ID string `json:"targetId"`
	}
	if err := c.call(ctx, "", "Target.createTarget", map[string]any{"url": "about:blank", "browserContextId": bc.ID}, &t); err != nil {
		return fail(err)
	}
	x.target = t.ID
	var at struct {
		SID string `json:"sessionId"`
	}
	if err := c.call(ctx, "", "Target.attachToTarget", map[string]any{"targetId": t.ID, "flatten": true}, &at); err != nil {
		return fail(err)
	}
	x.sid = at.SID
	x.watchNetwork()
	if proxyUser != "" {
		x.handleProxyAuth(proxyUser, proxyPass)
	}
	setup := []struct {
		method string
		params any
	}{
		{"Page.enable", nil},
		{"Runtime.enable", nil},
		{"Network.enable", nil},
		{"Emulation.setDeviceMetricsOverride", map[string]any{"width": ViewWidth, "height": ViewHeight, "deviceScaleFactor": 1, "mobile": false,
			"screenWidth": screenWidth, "screenHeight": screenHeight}},
	}
	if o.Timezone != "" {
		setup = append(setup, struct {
			method string
			params any
		}{"Emulation.setTimezoneOverride", map[string]any{"timezoneId": o.Timezone}})
	}
	for _, step := range setup {
		if err := c.call(ctx, x.sid, step.method, step.params, nil); err != nil {
			x.unsubscribe()
			return fail(fmt.Errorf("%s: %w", step.method, err))
		}
	}
	// Headless Chrome says so in its user agent and in the Client Hints that
	// go with it (Sec-CH-UA, navigator.userAgentData), and some sites refuse it.
	var ver struct {
		UA      string `json:"userAgent"`
		Product string `json:"product"`
	}
	if c.call(ctx, "", "Browser.getVersion", nil, &ver) == nil && ver.UA != "" {
		lang := o.Locale
		if lang == "" {
			lang = "zh-CN"
		}
		ua, meta, ok := userAgent(ver.UA, ver.Product)
		p := map[string]any{"userAgent": ua, "acceptLanguage": lang + "," + strings.SplitN(lang, "-", 2)[0] + ";q=0.9"}
		if ok {
			p["userAgentMetadata"] = meta
		}
		_ = c.call(ctx, x.sid, "Network.setUserAgentOverride", p, nil)
	}
	// Runs in every document and frame of the session, before the scripts of
	// the page. Not part of the storage script below: that one exists only
	// when there is saved state.
	if err := c.call(ctx, x.sid, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": stealthSource(o.Locale)}, nil); err != nil {
		s.log.Warn("browser stealth script", "err", err)
	}
	if len(x.origins) > 0 {
		raw, _ := json.Marshal(x.origins)
		src := `(function(){try{var all=` + string(raw) + `;var m=all[location.origin];if(!m)return;for(var k in m){if(localStorage.getItem(k)===null)localStorage.setItem(k,m[k]);}}catch(e){}})();`
		_ = c.call(ctx, x.sid, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": src}, nil)
	}
	return x, nil
}

func cookieParams(list []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, ck := range list {
		p := map[string]any{}
		for _, k := range []string{"name", "value", "domain", "path", "secure", "httpOnly", "sameSite", "priority", "sameParty", "sourceScheme", "partitionKey"} {
			if v, ok := ck[k]; ok {
				p[k] = v
			}
		}
		if exp, ok := ck["expires"].(float64); ok && exp > 0 {
			p["expires"] = exp
		}
		out = append(out, p)
	}
	return out
}

func normalizeHosts(list []string) []string {
	var out []string
	for _, h := range list {
		h = strings.ToLower(strings.TrimSpace(h))
		h = strings.TrimPrefix(strings.TrimPrefix(h, "https://"), "http://")
		h = strings.TrimPrefix(h, "*.")
		h = strings.TrimSuffix(strings.SplitN(h, "/", 2)[0], ".")
		if h != "" {
			out = append(out, h)
		}
	}
	return out
}

// HostAllowed reports whether rawURL may be opened under hosts (see
// Options.AllowedHosts).
func HostAllowed(hosts []string, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme == "about" || u.Scheme == "data" || u.Scheme == "blob" {
		return true
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	hosts = normalizeHosts(hosts)
	if len(hosts) == 0 {
		return true
	}
	h := strings.ToLower(u.Hostname())
	for _, a := range hosts {
		if h == a || strings.HasSuffix(h, "."+a) {
			return true
		}
	}
	return false
}

func (x *Session) watchNetwork() {
	ch, cancel := x.c.subscribe(x.sid, "*", 512)
	x.unsubNet = cancel
	go func() {
		for {
			select {
			case <-x.done:
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				var p struct {
					RequestID string `json:"requestId"`
				}
				switch ev.Method {
				case "Network.requestWillBeSent":
					_ = json.Unmarshal(ev.Params, &p)
					x.netMu.Lock()
					x.inflight[p.RequestID] = true
					x.lastNet = time.Now()
					x.netMu.Unlock()
				case "Network.loadingFinished", "Network.loadingFailed":
					_ = json.Unmarshal(ev.Params, &p)
					x.netMu.Lock()
					delete(x.inflight, p.RequestID)
					x.lastNet = time.Now()
					x.netMu.Unlock()
				}
			}
		}
	}()
}

func (x *Session) handleProxyAuth(user, pass string) {
	ch, cancel := x.c.subscribe(x.sid, "*", 512)
	x.unsubAuth = cancel
	_ = x.c.call(context.Background(), x.sid, "Fetch.enable", map[string]any{"handleAuthRequests": true}, nil)
	go func() {
		for {
			select {
			case <-x.done:
				return
			case ev := <-ch:
				var p struct {
					RequestID string `json:"requestId"`
				}
				switch ev.Method {
				case "Fetch.requestPaused":
					_ = json.Unmarshal(ev.Params, &p)
					go func() {
						_ = x.c.call(context.Background(), x.sid, "Fetch.continueRequest", map[string]any{"requestId": p.RequestID}, nil)
					}()
				case "Fetch.authRequired":
					_ = json.Unmarshal(ev.Params, &p)
					go func() {
						_ = x.c.call(context.Background(), x.sid, "Fetch.continueWithAuth", map[string]any{"requestId": p.RequestID,
							"authChallengeResponse": map[string]any{"response": "ProvideCredentials", "username": user, "password": pass}}, nil)
					}()
				}
			}
		}
	}()
}

func (x *Session) unsubscribe() {
	if x.unsubNet != nil {
		x.unsubNet()
	}
	if x.unsubAuth != nil {
		x.unsubAuth()
	}
}

// Done is closed when the session ends.
func (x *Session) Done() <-chan struct{} { return x.done }

// Hold keeps the session open for d from now, for example while the user
// solves a captcha.
func (x *Session) Hold(d time.Duration) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if !x.closed && x.deadline != nil {
		x.deadline.Reset(d)
	}
}

// Close ends the session and frees its slot. It is safe to call twice.
func (x *Session) Close() {
	x.mu.Lock()
	if x.closed {
		x.mu.Unlock()
		return
	}
	x.closed = true
	close(x.done)
	if x.deadline != nil {
		x.deadline.Stop()
	}
	x.mu.Unlock()
	x.unsubscribe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = x.c.call(ctx, "", "Target.disposeBrowserContext", map[string]any{"browserContextId": x.ctxID}, nil)
	x.s.mu.Lock()
	if x.key != "" && x.s.sessions[x.key] == x {
		delete(x.s.sessions, x.key)
	}
	x.s.mu.Unlock()
	if x.release != nil {
		x.release()
	}
}

func (x *Session) alive() error {
	select {
	case <-x.done:
		return errors.New("浏览器会话已经结束")
	default:
		return nil
	}
}

// eval runs page.js with op and arg in the page and decodes the result.
func (x *Session) eval(ctx context.Context, op string, arg, out any) error {
	if err := x.alive(); err != nil {
		return err
	}
	a, err := json.Marshal(arg)
	if err != nil {
		return err
	}
	expr := "(function(){" + pageJS + "\nreturn __xc(" + mustJSON(op) + "," + string(a) + ");})()"
	var res struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		Exception *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := x.c.call(ctx, x.sid, "Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true, "awaitPromise": false}, &res); err != nil {
		return err
	}
	if res.Exception != nil {
		return fmt.Errorf("页面脚本出错：%s %s", res.Exception.Text, res.Exception.Exception.Description)
	}
	if out == nil || len(res.Result.Value) == 0 {
		return nil
	}
	return json.Unmarshal(res.Result.Value, out)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// settle waits until the page has loaded and the network has been quiet for
// half a second, at most for max.
func (x *Session) settle(ctx context.Context, max time.Duration) {
	end := time.Now().Add(max)
	for time.Now().Before(end) {
		if ctx.Err() != nil || x.alive() != nil {
			return
		}
		var ready string
		var res struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		if x.c.call(ctx, x.sid, "Runtime.evaluate", map[string]any{"expression": "document.readyState", "returnByValue": true}, &res) == nil {
			ready = res.Result.Value
		}
		x.netMu.Lock()
		quiet := len(x.inflight) == 0 && time.Since(x.lastNet) > 500*time.Millisecond
		// Long polling and analytics keep a request open forever; after a
		// while a quiet period with only a couple open is good enough.
		if !quiet && len(x.inflight) <= 2 && time.Since(x.lastNet) > 2*time.Second {
			quiet = true
		}
		x.netMu.Unlock()
		if ready == "complete" && quiet {
			return
		}
		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}
}

// Page is what Snapshot returns.
type Page struct {
	URL       string `json:"url"`
	Title     string `json:"title"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

// Navigate opens rawURL and waits for the page.
func (x *Session) Navigate(ctx context.Context, rawURL string) error {
	x.ops.Lock()
	defer x.ops.Unlock()
	if !HostAllowed(x.allowed, rawURL) {
		return fmt.Errorf("%w：%s", ErrHostNotAllowed, rawURL)
	}
	if err := x.alive(); err != nil {
		return err
	}
	var res struct {
		ErrorText string `json:"errorText"`
	}
	if err := x.c.call(ctx, x.sid, "Page.navigate", map[string]any{"url": rawURL}, &res); err != nil {
		return err
	}
	if res.ErrorText != "" {
		return fmt.Errorf("打不开网页：%s", res.ErrorText)
	}
	x.settle(ctx, 20*time.Second)
	return x.afterAction(ctx)
}

// afterAction records local storage and goes back when the page left the
// allowed hosts.
func (x *Session) afterAction(ctx context.Context) error {
	cur, err := x.url(ctx)
	if err != nil {
		return err
	}
	if !HostAllowed(x.allowed, cur) {
		_ = x.c.call(ctx, x.sid, "Page.navigate", map[string]any{"url": "about:blank"}, nil)
		return fmt.Errorf("%w：页面跳到了 %s，已经停下", ErrHostNotAllowed, cur)
	}
	_ = x.keepStorage(ctx)
	return nil
}

func (x *Session) url(ctx context.Context) (string, error) {
	var res struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := x.c.call(ctx, x.sid, "Runtime.evaluate", map[string]any{"expression": "location.href", "returnByValue": true}, &res); err != nil {
		return "", err
	}
	return res.Result.Value, nil
}

// URL returns the address of the page.
func (x *Session) URL(ctx context.Context) (string, error) {
	x.ops.Lock()
	defer x.ops.Unlock()
	return x.url(ctx)
}

// Snapshot lists the visible text and controls of the page, each with a ref.
func (x *Session) Snapshot(ctx context.Context, maxChars int) (Page, error) {
	x.ops.Lock()
	defer x.ops.Unlock()
	var p Page
	err := x.eval(ctx, "snapshot", maxChars, &p)
	return p, err
}

// Locator finds an element again on a later visit.
type Locator struct {
	Tag         string `json:"tag"`
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Aria        string `json:"aria,omitempty"`
	Text        string `json:"text,omitempty"`
	CSS         string `json:"css,omitempty"`
	Anchor      string `json:"anchor,omitempty"`
	Frame       []int  `json:"frame,omitempty"`
	// Region is set for an area whose text is read. Its text changes, so it
	// is found by position and by the label next to it, not by text.
	Region bool `json:"region,omitempty"`
}

// ErrNoElement means a ref or locator matched nothing.
var ErrNoElement = errors.New("页面上找不到这个元素")

// Describe returns a locator for the element with ref.
func (x *Session) Describe(ctx context.Context, ref string) (Locator, error) {
	x.ops.Lock()
	defer x.ops.Unlock()
	var l *Locator
	if err := x.eval(ctx, "describe", ref, &l); err != nil {
		return Locator{}, err
	}
	if l == nil {
		return Locator{}, ErrNoElement
	}
	return *l, nil
}

// Locate finds the element of l and returns its ref.
func (x *Session) Locate(ctx context.Context, l Locator) (string, error) {
	x.ops.Lock()
	defer x.ops.Unlock()
	var ref *string
	if err := x.eval(ctx, "locate", l, &ref); err != nil {
		return "", err
	}
	if ref == nil {
		return "", ErrNoElement
	}
	return *ref, nil
}

// Element is a short description of an element, used before clicking.
type Element struct {
	Tag  string `json:"tag"`
	Type string `json:"type"`
	Text string `json:"text"`
}

// Info describes the element with ref.
func (x *Session) Info(ctx context.Context, ref string) (Element, error) {
	x.ops.Lock()
	defer x.ops.Unlock()
	var e *Element
	if err := x.eval(ctx, "info", ref, &e); err != nil {
		return Element{}, err
	}
	if e == nil {
		return Element{}, ErrNoElement
	}
	return *e, nil
}

// Common returns the ref of the smallest element that holds all of refs.
func (x *Session) Common(ctx context.Context, refs []string) (string, error) {
	x.ops.Lock()
	defer x.ops.Unlock()
	var ref *string
	if err := x.eval(ctx, "common", refs, &ref); err != nil {
		return "", err
	}
	if ref == nil {
		return "", ErrNoElement
	}
	return *ref, nil
}

// Shape says how a list area splits into records and where each field sits
// in a record.
type Shape struct {
	// Row is the tag names from the area down to one record.
	Row []string `json:"row"`
	// Class is the classes of the first record. Another record shares one.
	Class []string `json:"class,omitempty"`
	// Paths are the CSS paths from a record to each field.
	Paths []string `json:"paths"`
}

// Shape works out the shape of the area region from the fields of its
// first record.
func (x *Session) Shape(ctx context.Context, region string, fields []string) (Shape, error) {
	x.ops.Lock()
	defer x.ops.Unlock()
	var s *Shape
	if err := x.eval(ctx, "shape", map[string]any{"region": region, "fields": fields}, &s); err != nil {
		return Shape{}, err
	}
	if s == nil {
		return Shape{}, ErrNoElement
	}
	return *s, nil
}

// Rows reads the fields of every record in the area region. A record that
// misses a field, such as a header row, is left out.
func (x *Session) Rows(ctx context.Context, region string, s Shape) ([][]string, error) {
	x.ops.Lock()
	defer x.ops.Unlock()
	var rows [][]string
	arg := map[string]any{"region": region, "row": s.Row, "class": s.Class, "paths": s.Paths}
	if err := x.eval(ctx, "rows", arg, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// Text returns the visible text of the element with ref.
func (x *Session) Text(ctx context.Context, ref string) (string, error) {
	x.ops.Lock()
	defer x.ops.Unlock()
	var s *string
	if err := x.eval(ctx, "text", ref, &s); err != nil {
		return "", err
	}
	if s == nil {
		return "", ErrNoElement
	}
	return *s, nil
}

// Status says whether the page shows a login form or a captcha.
type Status struct {
	URL      string `json:"url"`
	Password bool   `json:"password"`
	Captcha  string `json:"captcha"`
}

// Check looks for a password field and a captcha on the page.
func (x *Session) Check(ctx context.Context) (Status, error) {
	x.ops.Lock()
	defer x.ops.Unlock()
	var st Status
	err := x.eval(ctx, "check", nil, &st)
	return st, err
}

// target is where an element is on the page: its middle and its size.
type target struct{ X, Y, W, H float64 }

func (x *Session) point(ctx context.Context, ref string) (target, error) {
	var pt *struct {
		target
		Visible bool
	}
	if err := x.eval(ctx, "point", ref, &pt); err != nil {
		return target{}, err
	}
	if pt == nil {
		return target{}, ErrNoElement
	}
	if !pt.Visible {
		return target{}, errors.New("这个元素现在看不见")
	}
	return pt.target, nil
}

// Click clicks the element with ref like a mouse does and waits for the page.
func (x *Session) Click(ctx context.Context, ref string) error {
	x.ops.Lock()
	defer x.ops.Unlock()
	pt, err := x.point(ctx, ref)
	if err != nil {
		return err
	}
	if err := x.humanClick(ctx, pt.X, pt.Y, pt.W, pt.H); err != nil {
		return err
	}
	// A click may start a navigation; give it a moment to begin.
	select {
	case <-time.After(300 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	x.settle(ctx, 15*time.Second)
	return x.afterAction(ctx)
}

// Type replaces the content of the field with ref by text.
func (x *Session) Type(ctx context.Context, ref, text string) error {
	x.ops.Lock()
	defer x.ops.Unlock()
	pt, err := x.point(ctx, ref)
	if err != nil {
		return err
	}
	if err := x.humanClick(ctx, pt.X, pt.Y, pt.W, pt.H); err != nil {
		return err
	}
	var ok bool
	if err := x.eval(ctx, "clear", ref, &ok); err != nil {
		return err
	}
	if !ok {
		return ErrNoElement
	}
	return x.typeText(ctx, text)
}

// Fill types a secret, such as a stored user name, into the field with ref.
// Snapshot then shows the field as filled without its value, and Text
// does not read it.
func (x *Session) Fill(ctx context.Context, ref, text string) error {
	if err := x.Type(ctx, ref, text); err != nil {
		return err
	}
	x.ops.Lock()
	defer x.ops.Unlock()
	var ok bool
	if err := x.eval(ctx, "secret", ref, &ok); err != nil {
		return err
	}
	if !ok {
		return ErrNoElement
	}
	return nil
}

// Select picks the option with this text or value in a drop-down.
func (x *Session) Select(ctx context.Context, ref, value string) error {
	x.ops.Lock()
	defer x.ops.Unlock()
	var ok bool
	if err := x.eval(ctx, "select", map[string]string{"ref": ref, "value": value}, &ok); err != nil {
		return err
	}
	if !ok {
		return errors.New("下拉框里没有这个选项")
	}
	x.settle(ctx, 10*time.Second)
	return nil
}

// Keys a user or the AI can press.
var keys = map[string]struct {
	Code string
	VK   int
	Text string
}{
	"Enter":     {"Enter", 13, "\r"},
	"Tab":       {"Tab", 9, ""},
	"Backspace": {"Backspace", 8, ""},
	"Escape":    {"Escape", 27, ""},
	"ArrowDown": {"ArrowDown", 40, ""},
	"ArrowUp":   {"ArrowUp", 38, ""},
}

// Press presses a key such as "Enter" in the focused element.
func (x *Session) Press(ctx context.Context, key string) error {
	x.ops.Lock()
	defer x.ops.Unlock()
	if err := x.press(ctx, key); err != nil {
		return err
	}
	if key == "Enter" {
		select {
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
		x.settle(ctx, 15*time.Second)
		return x.afterAction(ctx)
	}
	return nil
}

func (x *Session) press(ctx context.Context, key string) error {
	k, ok := keys[key]
	if !ok {
		return fmt.Errorf("不支持的按键 %s", key)
	}
	down := map[string]any{"type": "keyDown", "key": k.Code, "code": k.Code, "windowsVirtualKeyCode": k.VK}
	if k.Text != "" {
		down["text"] = k.Text
	}
	if err := x.c.call(ctx, x.sid, "Input.dispatchKeyEvent", down, nil); err != nil {
		return err
	}
	return x.c.call(ctx, x.sid, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": k.Code, "code": k.Code, "windowsVirtualKeyCode": k.VK}, nil)
}

// Wait waits for d and then for the page to settle.
func (x *Session) Wait(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
	case <-ctx.Done():
		return ctx.Err()
	case <-x.done:
		return x.alive()
	}
	x.ops.Lock()
	defer x.ops.Unlock()
	x.settle(ctx, 10*time.Second)
	return nil
}

// Screenshot returns a JPEG of the visible part of the page.
func (x *Session) Screenshot(ctx context.Context, quality int) ([]byte, error) {
	x.ops.Lock()
	defer x.ops.Unlock()
	if err := x.alive(); err != nil {
		return nil, err
	}
	if quality <= 0 || quality > 100 {
		quality = 70
	}
	var res struct {
		Data string `json:"data"`
	}
	if err := x.c.call(ctx, x.sid, "Page.captureScreenshot", map[string]any{"format": "jpeg", "quality": quality}, &res); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(res.Data)
}

// SaveState returns the cookies of the session and the local storage of the
// sites it visited, for Options.State next time. It holds login tokens, so
// store it encrypted.
func (x *Session) SaveState(ctx context.Context) ([]byte, error) {
	x.ops.Lock()
	defer x.ops.Unlock()
	if err := x.alive(); err != nil {
		return nil, err
	}
	_ = x.keepStorage(ctx)
	var res struct {
		Cookies []map[string]any `json:"cookies"`
	}
	if err := x.c.call(ctx, "", "Storage.getCookies", map[string]any{"browserContextId": x.ctxID}, &res); err != nil {
		return nil, err
	}
	x.mu.Lock()
	st := savedState{Cookies: res.Cookies, Origins: x.origins}
	b, err := json.Marshal(st)
	x.mu.Unlock()
	return b, err
}

// keepStorage remembers the local storage of the page's site.
func (x *Session) keepStorage(ctx context.Context) error {
	var st struct {
		Origin string            `json:"origin"`
		Items  map[string]string `json:"items"`
	}
	if err := x.eval(ctx, "storage", nil, &st); err != nil {
		return err
	}
	if st.Origin != "" && st.Origin != "null" {
		x.mu.Lock()
		x.origins[st.Origin] = st.Items
		x.mu.Unlock()
	}
	return nil
}
