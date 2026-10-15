package browser_test

import (
	"bufio"
	"encoding/json"
	"fmt"
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

// probeSite is a page that reads what a site can see of the browser and
// posts it to /report, plus pages to click and type in. It also remembers the
// request headers of /probe.
type probeSite struct {
	*httptest.Server
	mu      sync.Mutex
	headers http.Header
	reports chan json.RawMessage
}

const probePage = `<html><body><iframe id="f" src="/blank" width="50" height="50"></iframe><script>
(async () => {
  const r = {};
  r.webdriver = navigator.webdriver;
  r.ua = navigator.userAgent;
  r.plugins = navigator.plugins.length;
  r.mimeTypes = navigator.mimeTypes.length;
  r.pluginsTag = Object.prototype.toString.call(navigator.plugins);
  r.pluginsIs = navigator.plugins instanceof PluginArray;
  r.pluginIs = navigator.plugins[0] instanceof Plugin;
  r.mimeIs = navigator.mimeTypes[0] instanceof MimeType;
  r.pluginNames = Array.from(navigator.plugins).map((p) => p.name);
  r.pluginFile = navigator.plugins[0].filename;
  r.pluginLen = navigator.plugins[0].length;
  r.named = navigator.plugins.namedItem('Chrome PDF Viewer') === navigator.plugins[1] && navigator.plugins['PDF Viewer'] === navigator.plugins[0];
  r.mimeType = navigator.mimeTypes['application/pdf'].type;
  r.enabled = navigator.mimeTypes[0].enabledPlugin === navigator.plugins[0];
  r.languages = navigator.languages;
  r.language = navigator.language;
  r.hasChrome = typeof window.chrome === 'object' && window.chrome !== null;
  r.chromeKeys = ['runtime', 'app', 'csi', 'loadTimes'].filter((k) => k in window.chrome);
  r.csi = typeof window.chrome.csi().onloadT;
  r.notification = Notification.permission;
  r.permission = (await navigator.permissions.query({ name: 'notifications' })).state;
  r.otherPermission = (await navigator.permissions.query({ name: 'geolocation' })).state;
  r.outerH = outerHeight; r.innerH = innerHeight; r.outerW = outerWidth; r.innerW = innerWidth;
  r.screenW = screen.width; r.screenH = screen.height;
  try {
    const c = document.createElement('canvas');
    const gl = c.getContext('webgl');
    if (gl) {
      const ext = gl.getExtension('WEBGL_debug_renderer_info');
      r.webglVendor = gl.getParameter(ext.UNMASKED_VENDOR_WEBGL);
      r.webglRenderer = gl.getParameter(ext.UNMASKED_RENDERER_WEBGL);
      r.webglOther = gl.getParameter(gl.VERSION);
    }
    const gl2 = document.createElement('canvas').getContext('webgl2');
    if (gl2) {
      const ext = gl2.getExtension('WEBGL_debug_renderer_info');
      r.webgl2Renderer = gl2.getParameter(ext.UNMASKED_RENDERER_WEBGL);
    }
  } catch (e) { r.webglError = String(e); }
  const ts = (f) => Function.prototype.toString.call(f);
  r.toStringGetter = ts(Object.getOwnPropertyDescriptor(Navigator.prototype, 'webdriver').get);
  r.toStringSelf = ts(Function.prototype.toString);
  r.toStringQuery = ts(navigator.permissions.query);
  r.toStringPlain = ts(function plain() { return 1; });
  r.toStringOriginal = ts(Math.max);
  r.getterNames = ['webdriver', 'languages', 'plugins'].map((k) => Object.getOwnPropertyDescriptor(Navigator.prototype, k).get.name);
  r.uadBrands = navigator.userAgentData.brands.map((b) => b.brand + '/' + b.version);
  r.uadMobile = navigator.userAgentData.mobile;
  r.uadPlatform = navigator.userAgentData.platform;
  const he = await navigator.userAgentData.getHighEntropyValues(['fullVersionList', 'architecture', 'bitness', 'platformVersion']);
  r.uadFull = he.fullVersionList.map((b) => b.brand + '/' + b.version);
  r.uadArch = he.architecture + '/' + he.bitness;
  const f = document.getElementById('f').contentWindow;
  r.frameWebdriver = f.navigator.webdriver;
  r.framePlugins = f.navigator.plugins.length;
  await fetch('/report', { method: 'POST', body: JSON.stringify(r) });
})();
</script></body></html>`

// clickPage has a button at a known place that records mouse events.
const clickPage = `<html><body style="margin:0"><button id="b" style="position:absolute;left:600px;top:400px;width:200px;height:40px">点这里</button><script>
const log = [];
for (const t of ['mousemove', 'mousedown', 'mouseup', 'click']) {
  document.addEventListener(t, (e) => log.push({ t, x: e.clientX, y: e.clientY, at: e.timeStamp, trusted: e.isTrusted }), true);
}
document.getElementById('b').addEventListener('click', () => {
  const r = document.getElementById('b').getBoundingClientRect();
  fetch('/report', { method: 'POST', body: JSON.stringify({ log: log.splice(0), rect: { x: r.x, y: r.y, w: r.width, h: r.height } }) });
});
</script></body></html>`

// typePage has an input and a textarea that record keyboard events and
// report them 300 milliseconds after the last one.
const typePage = `<html><body>
<input id="i" placeholder="输入框"><textarea id="t" placeholder="多行"></textarea>
<script>
const log = [];
let timer;
for (const t of ['keydown', 'keypress', 'input', 'keyup']) {
  document.addEventListener(t, (e) => {
    log.push({ t, key: e.key, at: e.timeStamp });
    clearTimeout(timer);
    timer = setTimeout(() => {
      fetch('/report', { method: 'POST', body: JSON.stringify({ log: log.splice(0), input: document.getElementById('i').value, area: document.getElementById('t').value }) });
    }, 300);
  }, true);
}
</script></body></html>`

func newProbeSite(t *testing.T) *probeSite {
	t.Helper()
	p := &probeSite{reports: make(chan json.RawMessage, 16)}
	mux := http.NewServeMux()
	mux.HandleFunc("/probe", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.headers = r.Header.Clone()
		p.mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Ask for the hints that Chrome does not send on its own.
		w.Header().Set("Accept-CH", "Sec-CH-UA-Full-Version-List")
		fmt.Fprint(w, probePage)
	})
	mux.HandleFunc("/blank", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<html><body>x</body></html>") })
	mux.HandleFunc("/click", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, clickPage)
	})
	mux.HandleFunc("/type", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, typePage)
	})
	mux.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		p.reports <- b
	})
	p.Server = httptest.NewServer(mux)
	t.Cleanup(p.Close)
	return p
}

// report waits for the next post to /report.
func (p *probeSite) report(t *testing.T, v any) {
	t.Helper()
	select {
	case b := <-p.reports:
		if err := json.Unmarshal(b, v); err != nil {
			t.Fatalf("report %s: %v", b, err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the page did not report")
	}
}

func (p *probeSite) header(name string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.headers.Get(name)
}

func TestLooksLikeNormalChrome(t *testing.T) {
	t.Parallel()
	s := browsertest.Service(t)
	site := newProbeSite(t)
	c := ctx(t)
	x, err := s.Open(c, browser.Options{Locale: "zh-CN", Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if err := x.Navigate(c, site.URL+"/probe"); err != nil {
		t.Fatal(err)
	}
	var r struct {
		Webdriver                                                          *bool
		UA                                                                 string
		Plugins, MimeTypes, PluginLen                                      int
		PluginsTag                                                         string
		PluginsIs, PluginIs, MimeIs, Named, Enabled                        bool
		PluginNames                                                        []string
		PluginFile, MimeType                                               string
		Languages                                                          []string
		Language                                                           string
		HasChrome                                                          bool
		ChromeKeys                                                         []string
		CSI                                                                string
		Notification, Permission, OtherPermission                          string
		OuterH, InnerH, OuterW, InnerW, ScreenW, ScreenH                   int
		WebglVendor, WebglRenderer, WebglOther, Webgl2Renderer, WebglError string
		ToStringGetter, ToStringSelf, ToStringQuery, ToStringPlain         string
		ToStringOriginal                                                   string
		GetterNames                                                        []string
		UadBrands, UadFull                                                 []string
		UadMobile                                                          bool
		UadPlatform, UadArch                                               string
		FrameWebdriver                                                     *bool
		FramePlugins                                                       int
	}
	site.report(t, &r)

	if r.Webdriver == nil || *r.Webdriver {
		t.Errorf("navigator.webdriver = %v, want false", r.Webdriver)
	}
	if r.FrameWebdriver == nil || *r.FrameWebdriver || r.FramePlugins != 5 {
		t.Errorf("frames are not covered: webdriver %v plugins %d", r.FrameWebdriver, r.FramePlugins)
	}
	if strings.Contains(r.UA, "Headless") {
		t.Errorf("user agent: %s", r.UA)
	}
	if r.Plugins != 5 || r.MimeTypes != 2 || r.PluginLen != 2 {
		t.Errorf("plugins %d mimeTypes %d plugin.length %d", r.Plugins, r.MimeTypes, r.PluginLen)
	}
	if r.PluginsTag != "[object PluginArray]" || !r.PluginsIs || !r.PluginIs || !r.MimeIs {
		t.Errorf("plugin types: %s %v %v %v", r.PluginsTag, r.PluginsIs, r.PluginIs, r.MimeIs)
	}
	if strings.Join(r.PluginNames, ",") != "PDF Viewer,Chrome PDF Viewer,Chromium PDF Viewer,Microsoft Edge PDF Viewer,WebKit built-in PDF" ||
		r.PluginFile != "internal-pdf-viewer" || !r.Named || !r.Enabled || r.MimeType != "application/pdf" {
		t.Errorf("plugin details: %+v", r)
	}
	if strings.Join(r.Languages, ",") != "zh-CN,zh" || r.Language != "zh-CN" {
		t.Errorf("languages %v %s", r.Languages, r.Language)
	}
	if !r.HasChrome || strings.Join(r.ChromeKeys, ",") != "runtime,app,csi,loadTimes" || r.CSI != "number" {
		t.Errorf("window.chrome: %v %v %s", r.HasChrome, r.ChromeKeys, r.CSI)
	}
	// Both sides agree. Headless Chrome says denied for one and prompt for
	// the other.
	want := map[string]string{"default": "prompt", "denied": "denied", "granted": "granted"}[r.Notification]
	if r.Permission != want {
		t.Errorf("Notification.permission %q but permissions.query says %q", r.Notification, r.Permission)
	}
	if r.OtherPermission == "" {
		t.Error("other permissions must still work")
	}
	if r.OuterH != r.InnerH+85 || r.OuterH <= r.InnerH || r.OuterW != r.InnerW || r.InnerW != browser.ViewWidth || r.InnerH != browser.ViewHeight {
		t.Errorf("window %dx%d outer %dx%d", r.InnerW, r.InnerH, r.OuterW, r.OuterH)
	}
	if r.ScreenW != 1920 || r.ScreenH != 1080 {
		t.Errorf("screen %dx%d", r.ScreenW, r.ScreenH)
	}
	if r.WebglRenderer == "" && r.WebglError == "" {
		t.Log("no WebGL in this browser, skipping the WebGL check")
	} else {
		if r.WebglError != "" || strings.Contains(r.WebglRenderer, "SwiftShader") || !strings.Contains(r.WebglRenderer, "Intel") || r.WebglVendor != "Google Inc. (Intel)" {
			t.Errorf("webgl %q %q %s", r.WebglVendor, r.WebglRenderer, r.WebglError)
		}
		if r.Webgl2Renderer != "" && strings.Contains(r.Webgl2Renderer, "SwiftShader") {
			t.Errorf("webgl2 %q", r.Webgl2Renderer)
		}
		if !strings.Contains(r.WebglOther, "WebGL") {
			t.Errorf("other parameters must stay: %q", r.WebglOther)
		}
	}
	for name, got := range map[string]string{"getter": r.ToStringGetter, "toString": r.ToStringSelf, "query": r.ToStringQuery, "Math.max": r.ToStringOriginal} {
		if !strings.Contains(got, "[native code]") {
			t.Errorf("%s toString: %s", name, got)
		}
	}
	if r.ToStringGetter != "function get webdriver() { [native code] }" || r.ToStringSelf != "function toString() { [native code] }" {
		t.Errorf("toString text: %q %q", r.ToStringGetter, r.ToStringSelf)
	}
	if strings.Contains(r.ToStringPlain, "[native code]") || !strings.Contains(r.ToStringPlain, "return 1") {
		t.Errorf("a page function must keep its source: %s", r.ToStringPlain)
	}
	if strings.Join(r.GetterNames, ",") != "get webdriver,get languages,get plugins" {
		t.Errorf("getter names %v", r.GetterNames)
	}

	// Client Hints: navigator.userAgentData and the request header.
	for _, list := range [][]string{r.UadBrands, r.UadFull} {
		joined := strings.Join(list, ",")
		if strings.Contains(joined, "Headless") || !strings.Contains(joined, "Chromium/") || !strings.Contains(joined, "Google Chrome/") || !strings.Contains(joined, "Not.A/Brand/") {
			t.Errorf("userAgentData brands: %v", list)
		}
	}
	if r.UadMobile || r.UadPlatform != "Linux" || r.UadArch != "x86/64" {
		t.Errorf("userAgentData: mobile %v platform %s arch %s", r.UadMobile, r.UadPlatform, r.UadArch)
	}
	// Chrome sends the low-entropy hints to localhost, which counts as a
	// secure context.
	ch := site.header("Sec-Ch-Ua")
	if ch == "" {
		t.Skip("Chrome did not send Sec-CH-UA to the local server, so the header cannot be checked")
	}
	if strings.Contains(ch, "Headless") || !strings.Contains(ch, `"Chromium";v=`) || !strings.Contains(ch, `"Google Chrome";v=`) {
		t.Errorf("Sec-CH-UA: %s", ch)
	}
	if site.header("Sec-Ch-Ua-Mobile") != "?0" || site.header("Sec-Ch-Ua-Platform") != `"Linux"` || strings.Contains(site.header("User-Agent"), "Headless") {
		t.Errorf("hint headers: %v", site.headers)
	}
}

func TestLanguageFollowsLocale(t *testing.T) {
	t.Parallel()
	s := browsertest.Service(t)
	site := newProbeSite(t)
	c := ctx(t)
	x, err := s.Open(c, browser.Options{Locale: "en-US"})
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if err := x.Navigate(c, site.URL+"/probe"); err != nil {
		t.Fatal(err)
	}
	var r struct {
		Languages []string
		Language  string
		Plugins   int
	}
	site.report(t, &r)
	if strings.Join(r.Languages, ",") != "en-US,en" || r.Language != "en-US" || r.Plugins != 5 {
		t.Errorf("%+v", r)
	}
	// A second session of the same browser gets the script too.
	y, err := s.Open(c, browser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer y.Close()
	if err := y.Navigate(c, site.URL+"/probe"); err != nil {
		t.Fatal(err)
	}
	site.report(t, &r)
	if strings.Join(r.Languages, ",") != "zh-CN,zh" || r.Plugins != 5 {
		t.Errorf("default locale: %+v", r)
	}
}

func TestLanguagesAndUserAgent(t *testing.T) {
	for in, want := range map[string]string{"": "zh-CN,zh", "zh-CN": "zh-CN,zh", "en-US": "en-US,en", "fr": "fr", "pt-BR": "pt-BR,pt"} {
		if got := strings.Join(browser.LanguagesFor(in), ","); got != want {
			t.Errorf("%q: %s", in, got)
		}
	}
	linux := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/141.0.0.0 Safari/537.36"
	ua, meta, ok := browser.UserAgent(linux, "HeadlessChrome/141.0.7390.37")
	if !ok || strings.Contains(ua, "Headless") || !strings.Contains(ua, "Chrome/141.0.0.0") {
		t.Fatalf("%s %v", ua, ok)
	}
	b, _ := json.Marshal(meta)
	s := string(b)
	for _, want := range []string{`"brand":"Chromium","version":"141"`, `"brand":"Google Chrome","version":"141"`, `"brand":"Not.A/Brand"`,
		`"version":"141.0.7390.37"`, `"platform":"Linux"`, `"mobile":false`, `"architecture":"x86"`, `"bitness":"64"`} {
		if !strings.Contains(s, want) {
			t.Errorf("metadata lacks %s: %s", want, s)
		}
	}
	if strings.Contains(s, "Headless") {
		t.Errorf("metadata: %s", s)
	}
	_, meta, _ = browser.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/140.0.0.0 Safari/537.36", "Chrome/140.1.2.3")
	if meta["platform"] != "macOS" {
		t.Errorf("mac: %v", meta["platform"])
	}
	if _, _, ok := browser.UserAgent("something else", "nothing"); ok {
		t.Error("an unknown user agent has no metadata")
	}
}

type mouseEvent struct {
	T       string
	X, Y    float64
	At      float64
	Trusted bool
}

type clickReport struct {
	Log  []mouseEvent
	Rect struct{ X, Y, W, H float64 }
}

func TestClickMovesTheMouseFirst(t *testing.T) {
	t.Parallel()
	s := browsertest.Service(t)
	site := newProbeSite(t)
	c := ctx(t)
	x, err := s.Open(c, browser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if err := x.Navigate(c, site.URL+"/click"); err != nil {
		t.Fatal(err)
	}
	p, _ := x.Snapshot(c, 0)
	button := ref(t, p, "点这里")

	var offCenter bool
	var last mouseEvent
	for i := 0; i < 3; i++ {
		start := time.Now()
		if err := x.Click(c, button); err != nil {
			t.Fatal(err)
		}
		var r clickReport
		site.report(t, &r)
		t.Logf("click %d took %v with %d events", i, time.Since(start), len(r.Log))
		var moves, down, up int
		var downAt, upAt, prevAt float64
		var seq []string
		for _, e := range r.Log {
			seq = append(seq, e.T)
			switch e.T {
			case "mousemove":
				if down > 0 {
					t.Error("mousemove after mousedown")
				}
				moves++
				if prevAt > 0 && e.At-prevAt < 8 {
					t.Errorf("moves only %.1fms apart", e.At-prevAt)
				}
				prevAt = e.At
				last = e
			case "mousedown":
				down++
				downAt = e.At
				if prevAt > 0 && e.At-prevAt < 8 {
					t.Errorf("mousedown %.1fms after the last move", e.At-prevAt)
				}
				if !e.Trusted || e.X < r.Rect.X || e.X > r.Rect.X+r.Rect.W || e.Y < r.Rect.Y || e.Y > r.Rect.Y+r.Rect.H {
					t.Errorf("mousedown at %v,%v outside %+v", e.X, e.Y, r.Rect)
				}
				cx, cy := r.Rect.X+r.Rect.W/2, r.Rect.Y+r.Rect.H/2
				if e.X != cx || e.Y != cy {
					offCenter = true
				}
				if e.X != last.X || e.Y != last.Y {
					t.Errorf("the last move %v,%v is not where the button went down %v,%v", last.X, last.Y, e.X, e.Y)
				}
			case "mouseup":
				up++
				upAt = e.At
			}
			if e.T == "mouseup" {
				break
			}
		}
		if moves < 3 || down != 1 || up != 1 || seq[0] != "mousemove" {
			t.Fatalf("click %d: %d moves, %d down, %d up: %v", i, moves, down, up, seq)
		}
		if gap := upAt - downAt; gap < 45 || gap > 400 {
			t.Errorf("press and release %.1fms apart", gap)
		}
	}
	if !offCenter {
		t.Error("the click always lands on the exact middle")
	}
}

type keyEvent struct {
	T, Key string
	At     float64
}

type keyReport struct {
	Log         []keyEvent
	Input, Area string
}

func count(log []keyEvent, typ string) int {
	n := 0
	for _, e := range log {
		if e.T == typ {
			n++
		}
	}
	return n
}

func TestTypingIsOneCharacterAtATime(t *testing.T) {
	t.Parallel()
	s := browsertest.Service(t)
	site := newProbeSite(t)
	c := ctx(t)
	x, err := s.Open(c, browser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if err := x.Navigate(c, site.URL+"/type"); err != nil {
		t.Fatal(err)
	}
	p, _ := x.Snapshot(c, 0)
	input, area := ref(t, p, `"输入框"`), ref(t, p, `"多行"`)
	report := func() keyReport {
		t.Helper()
		var r keyReport
		site.report(t, &r)
		return r
	}

	start := time.Now()
	if err := x.Type(c, input, "Hello"); err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	r := report()
	if r.Input != "Hello" || count(r.Log, "keydown") != 5 || count(r.Log, "keyup") != 5 || count(r.Log, "input") < 5 {
		t.Fatalf("typing Hello: %+v", r)
	}
	var downs []keyEvent
	for _, e := range r.Log {
		if e.T == "keydown" {
			downs = append(downs, e)
		}
	}
	for i := 1; i < len(downs); i++ {
		if gap := downs[i].At - downs[i-1].At; gap < 28 {
			t.Errorf("keys %d and %d only %.1fms apart", i-1, i, gap)
		}
	}
	if downs[0].Key != "H" {
		t.Errorf("key %q", downs[0].Key)
	}
	t.Logf("5 characters took %v", took)

	// Typing again replaces the old text.
	if err := x.Type(c, input, "ab 12"); err != nil {
		t.Fatal(err)
	}
	if r := report(); r.Input != "ab 12" || count(r.Log, "keydown") != 5 {
		t.Fatalf("second typing: %+v", r)
	}

	// Chinese characters get keydown too.
	x.SetPace(10)
	if err := x.Type(c, input, "你好吗世界"); err != nil {
		t.Fatal(err)
	}
	r = report()
	t.Logf("Chinese: %d keydown, %d input, value %q", count(r.Log, "keydown"), count(r.Log, "input"), r.Input)
	if r.Input != "你好吗世界" || count(r.Log, "keydown") != 5 {
		t.Fatalf("typing Chinese: %+v", r)
	}

	// A line break goes in as text, not as the Enter key.
	if err := x.Type(c, area, "a\nb"); err != nil {
		t.Fatal(err)
	}
	if r := report(); r.Area != "a\nb" {
		t.Fatalf("textarea: %q", r.Area)
	}

	// Over 200 characters go in at once.
	long := strings.Repeat("字a", 101)
	start = time.Now()
	if err := x.Type(c, input, long); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("long text took %v", time.Since(start))
	}
	if r := report(); r.Input != long || count(r.Log, "keydown") != 0 {
		t.Fatalf("long text: %d keydown, equal %v", count(r.Log, "keydown"), r.Input == long)
	}
	// Exactly 200 is still typed by character.
	exact := strings.Repeat("x", 200)
	x.SetPace(5)
	if err := x.Type(c, input, exact); err != nil {
		t.Fatal(err)
	}
	if r := report(); r.Input != exact || count(r.Log, "keydown") != 200 {
		t.Fatalf("200 characters: %d keydown", count(r.Log, "keydown"))
	}
}

// The user's own input in the takeover view goes in as it comes.
func TestTakeoverInputIsNotSlowed(t *testing.T) {
	t.Parallel()
	s := browsertest.Service(t)
	site := newProbeSite(t)
	c := ctx(t)
	x, err := s.Open(c, browser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if err := x.Navigate(c, site.URL+"/click"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := x.Send(c, browser.Input{Type: "click", X: 700, Y: 420}); err != nil {
		t.Fatal(err)
	}
	var r clickReport
	site.report(t, &r)
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("takeover click took %v", time.Since(start))
	}
	var moves int
	for _, e := range r.Log {
		if e.T == "mousemove" {
			moves++
		}
	}
	if moves != 1 {
		t.Errorf("takeover click should be one move, one press, one release: %d moves", moves)
	}
}

// startChrome starts a Chrome with these extra flags and returns its DevTools
// address.
func startChrome(t *testing.T, flags ...string) string {
	t.Helper()
	path := browsertest.Path()
	if path == "" {
		t.Skip("没有找到 Chrome")
	}
	dir, err := os.MkdirTemp("", "xc-webgl-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	args := append([]string{"--headless=new", "--remote-debugging-port=0", "--user-data-dir=" + dir, "--no-sandbox", "--no-first-run"}, flags...)
	cmd := exec.Command(path, append(args, "about:blank")...)
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
	return "http://" + hostport
}

// The test machine may have no WebGL, so this one starts a Chrome with the
// software renderer, which is the case that gives itself away.
func TestWebGLNamesAreNotSoftware(t *testing.T) {
	t.Parallel()
	s := browser.New(startChrome(t, "--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"), nil)
	t.Cleanup(s.Close)
	site := newProbeSite(t)
	c := ctx(t)
	x, err := s.Open(c, browser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if err := x.Navigate(c, site.URL+"/probe"); err != nil {
		t.Fatal(err)
	}
	var r struct {
		WebglVendor, WebglRenderer, WebglOther, Webgl2Renderer, WebglError string
	}
	site.report(t, &r)
	if r.WebglRenderer == "" && r.WebglError == "" {
		t.Skip("这个 Chrome 没有 WebGL")
	}
	t.Logf("webgl %q %q webgl2 %q", r.WebglVendor, r.WebglRenderer, r.Webgl2Renderer)
	if r.WebglError != "" || strings.Contains(r.WebglRenderer, "SwiftShader") || !strings.Contains(r.WebglRenderer, "Intel") || r.WebglVendor != "Google Inc. (Intel)" {
		t.Errorf("webgl %q %q %s", r.WebglVendor, r.WebglRenderer, r.WebglError)
	}
	if r.Webgl2Renderer != "" && !strings.Contains(r.Webgl2Renderer, "Intel") {
		t.Errorf("webgl2 %q", r.Webgl2Renderer)
	}
	if !strings.Contains(r.WebglOther, "WebGL") {
		t.Errorf("other parameters must stay: %q", r.WebglOther)
	}
}
