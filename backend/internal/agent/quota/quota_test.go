package quota

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

func jwt(claims map[string]any) string {
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return enc(map[string]string{"alg": "none"}) + "." + enc(claims) + ".sig"
}

func code(err error) string {
	var pe *protocol.Error
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func TestResolveHome(t *testing.T) {
	t.Setenv("CODEX_HOME", "")
	t.Setenv("HOME", "/home/tester")
	t.Setenv("USERPROFILE", `C:\Users\tester`)
	abs := filepath.Join(t.TempDir(), "acct")
	for _, tc := range []struct {
		name, kind, home string
		bad              bool
	}{
		{"absolute", "codex", abs, false},
		{"empty means default", "codex", "", false},
		{"relative", "codex", "acct", true},
		{"dot dot", "codex", abs + "/../x", true},
		{"dot dot backslash", "grok", `C:\a\..\b`, true},
		{"unknown kind", "gemini", abs, true},
		{"tilde", "claude", "~/work", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveHome(tc.kind, tc.home)
			if tc.bad {
				if code(err) != protocol.CodeBadParams {
					t.Fatalf("want bad_params, got %v", err)
				}
				return
			}
			if err != nil || !filepath.IsAbs(got) {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
	t.Setenv("CODEX_HOME", abs)
	if got, _ := resolveHome("codex", ""); got != abs {
		t.Fatalf("CODEX_HOME not used: %q", got)
	}
}

// ---- Codex ----

type codexFake struct {
	usageHits, tokenHits atomic.Int32
	usageStatus          int
	tokenStatus          int
	gotAccount, gotAuth  atomic.Value
	srv                  *httptest.Server
}

func newCodexFake(t *testing.T) *codexFake {
	f := &codexFake{usageStatus: 200, tokenStatus: 200}
	mux := http.NewServeMux()
	mux.HandleFunc("/backend-api/wham/usage", func(w http.ResponseWriter, r *http.Request) {
		f.usageHits.Add(1)
		f.gotAccount.Store(r.Header.Get("chatgpt-account-id"))
		f.gotAuth.Store(r.Header.Get("Authorization"))
		if f.usageStatus != 200 {
			w.WriteHeader(f.usageStatus)
			return
		}
		reset := time.Now().Add(2 * time.Hour).Unix()
		json.NewEncoder(w).Encode(map[string]any{
			"plan_type": "plus",
			"credits":   map[string]any{"has_credits": true, "unlimited": false, "balance": "12.345"},
			"rate_limit": map[string]any{
				"primary_window":   map[string]any{"used_percent": 37.5, "limit_window_seconds": 18000, "reset_at": reset, "reset_after_seconds": 7200},
				"secondary_window": map[string]any{"used_percent": 8, "limit_window_seconds": 604800, "reset_after_seconds": 86400},
			},
		})
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		f.tokenHits.Add(1)
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["refresh_token"] != "old-refresh" || body["client_id"] == "" {
			w.WriteHeader(400)
			return
		}
		if f.tokenStatus != 200 {
			w.WriteHeader(f.tokenStatus)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"access_token":  jwt(map[string]any{"exp": time.Now().Add(time.Hour).Unix()}),
			"refresh_token": "new-refresh",
		})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	oldBase, oldTok := codexBase, codexTokenURL
	codexBase, codexTokenURL = f.srv.URL+"/backend-api", f.srv.URL+"/oauth/token"
	t.Cleanup(func() { codexBase, codexTokenURL = oldBase, oldTok })
	return f
}

func writeCodexAuth(t *testing.T, access string) string {
	t.Helper()
	home := t.TempDir()
	auth := map[string]any{
		"OPENAI_API_KEY": nil,
		"auth_mode":      "chatgpt",
		"tokens": map[string]any{
			"access_token": access, "refresh_token": "old-refresh",
			"id_token": jwt(map[string]any{"email": "me@example.com", "https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acct-1"}}),
		},
		"last_refresh": "2026-01-01T00:00:00Z",
	}
	b, _ := json.Marshal(auth)
	if err := os.WriteFile(filepath.Join(home, "auth.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestCodexReadsWindows(t *testing.T) {
	f := newCodexFake(t)
	home := writeCodexAuth(t, jwt(map[string]any{"exp": time.Now().Add(time.Hour).Unix()}))
	r, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "codex", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if r.Plan != "plus" || r.User != "me@example.com" || r.Credits != "12.35 积分" {
		t.Fatalf("reading: %+v", r)
	}
	if len(r.Windows) != 2 || r.Windows[0].Name != "5 小时" || r.Windows[0].UsedPercent != 37.5 || r.Windows[0].SpanSecs != 18000 {
		t.Fatalf("windows: %+v", r.Windows)
	}
	if r.Windows[1].Name != "7 天" || r.Windows[1].ResetsAt == nil || time.Until(*r.Windows[1].ResetsAt) < 23*time.Hour {
		t.Fatalf("second window: %+v", r.Windows[1])
	}
	if f.gotAccount.Load() != "acct-1" || !strings.HasPrefix(f.gotAuth.Load().(string), "Bearer ") {
		t.Fatalf("headers: %v %v", f.gotAccount.Load(), f.gotAuth.Load())
	}
	if f.tokenHits.Load() != 0 {
		t.Fatal("a good token must not be refreshed")
	}
}

func TestCodexRefreshWritesBackAndKeepsTheRest(t *testing.T) {
	f := newCodexFake(t)
	home := writeCodexAuth(t, jwt(map[string]any{"exp": time.Now().Add(time.Minute).Unix()}))
	if _, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "codex", Home: home}); err != nil {
		t.Fatal(err)
	}
	if f.tokenHits.Load() != 1 {
		t.Fatalf("token hits = %d", f.tokenHits.Load())
	}
	path := filepath.Join(home, "auth.json")
	var got map[string]any
	b, _ := os.ReadFile(path)
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	toks := got["tokens"].(map[string]any)
	if toks["refresh_token"] != "new-refresh" || toks["access_token"] == "" {
		t.Fatalf("tokens not renewed: %v", toks)
	}
	if toks["id_token"] == "" {
		t.Fatal("id_token lost")
	}
	if got["auth_mode"] != "chatgpt" {
		t.Fatalf("other fields must stay: %v", got)
	}
	if _, ok := got["OPENAI_API_KEY"]; !ok {
		t.Fatal("null field dropped")
	}
	if got["last_refresh"] == "2026-01-01T00:00:00Z" {
		t.Fatal("last_refresh not updated")
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(path)
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", st.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 1 {
		t.Fatalf("temporary file left behind: %v", entries)
	}
	// the renewed token is used next time, no second refresh
	if _, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "codex", Home: home}); err != nil || f.tokenHits.Load() != 1 {
		t.Fatalf("second read: %v, hits %d", err, f.tokenHits.Load())
	}
}

func TestCodexRefreshFailureLeavesTheFile(t *testing.T) {
	f := newCodexFake(t)
	f.tokenStatus = 400
	home := writeCodexAuth(t, jwt(map[string]any{"exp": time.Now().Add(time.Minute).Unix()}))
	path := filepath.Join(home, "auth.json")
	before, _ := os.ReadFile(path)
	_, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "codex", Home: home})
	if code(err) != protocol.CodeQuotaSignedOut || !strings.Contains(err.Error(), "codex login") {
		t.Fatalf("want signed out, got %v", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("auth.json changed after a failed refresh")
	}
	if f.usageHits.Load() != 0 {
		t.Fatal("must not ask usage without a token")
	}
}

func TestCodexTakesATokenTheCLIRenewedMeanwhile(t *testing.T) {
	f := newCodexFake(t)
	f.tokenStatus = 400
	home := writeCodexAuth(t, jwt(map[string]any{"exp": time.Now().Add(time.Minute).Unix()}))
	path := filepath.Join(home, "auth.json")
	// the CLI renews right after the agent reads: the refresh then fails with
	// the old refresh token, but the file already holds a good one
	good := jwt(map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	httpClientOld := httpClient
	t.Cleanup(func() { httpClient = httpClientOld })
	httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/oauth/token") {
			b, _ := json.Marshal(map[string]any{"tokens": map[string]any{"access_token": good, "refresh_token": "cli-refresh"}})
			os.WriteFile(path, b, 0o600)
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	r, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "codex", Home: home})
	if err != nil || len(r.Windows) != 2 {
		t.Fatalf("read: %+v, %v", r, err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "cli-refresh") {
		t.Fatal("the CLI's file must stay as it wrote it")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCodexSignedOutAndRefused(t *testing.T) {
	f := newCodexFake(t)
	empty := t.TempDir()
	if _, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "codex", Home: empty}); code(err) != protocol.CodeQuotaSignedOut {
		t.Fatalf("no auth.json: %v", err)
	}
	home := writeCodexAuth(t, jwt(map[string]any{"exp": time.Now().Add(time.Hour).Unix()}))
	f.usageStatus = 401
	if _, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "codex", Home: home}); code(err) != protocol.CodeQuotaSignedOut {
		t.Fatalf("401: %v", err)
	}
	f.usageStatus = 500
	_, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "codex", Home: home})
	if code(err) != protocol.CodeQuotaUnavailable || !strings.Contains(err.Error(), "500") {
		t.Fatalf("500: %v", err)
	}
	if strings.Contains(err.Error(), "http://") || strings.Contains(err.Error(), "Bearer") {
		t.Fatalf("error leaks request details: %v", err)
	}
}

// ---- Grok ----

func writeGrokAuth(t *testing.T, key string, exp time.Time) string {
	t.Helper()
	home := t.TempDir()
	b, _ := json.Marshal(map[string]any{
		"b-entry": map[string]any{"key": "other-key", "email": "b@example.com"},
		"a-entry": map[string]any{"key": key, "email": "a@example.com", "expires_at": exp.UTC().Format(time.RFC3339)},
	})
	if err := os.WriteFile(filepath.Join(home, "auth.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func newGrokFake(t *testing.T, status int, body string) (*httptest.Server, *atomic.Value) {
	var gotAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		if r.URL.Path != "/v1/billing" || r.URL.Query().Get("format") != "credits" {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	old := grokBase
	grokBase = srv.URL + "/v1"
	t.Cleanup(func() { grokBase = old })
	return srv, &gotAuth
}

func TestGrokReadsPeriodAndOnDemand(t *testing.T) {
	end := time.Now().Add(3 * 24 * time.Hour).UTC().Truncate(time.Second)
	_, auth := newGrokFake(t, 200, `{"config":{"creditUsagePercent":42,"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","end":"`+end.Format(time.RFC3339)+`"},"onDemandCap":{"val":50},"onDemandUsed":{"val":10}}}`)
	home := writeGrokAuth(t, "good-key", time.Now().Add(time.Hour))
	r, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "grok", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if auth.Load() != "Bearer good-key" || r.User != "a@example.com" {
		t.Fatalf("auth %v, user %q", auth.Load(), r.User)
	}
	if len(r.Windows) != 2 || r.Windows[0].Name != "7 天" || r.Windows[0].UsedPercent != 42 || !r.Windows[0].ResetsAt.Equal(end) {
		t.Fatalf("windows: %+v", r.Windows)
	}
	if w := r.Windows[1]; w.Name != "按量付费" || w.UsedPercent != 20 || !w.Aside {
		t.Fatalf("on-demand: %+v", w)
	}
}

func TestGrokFallsBackToBillingPeriodEnd(t *testing.T) {
	newGrokFake(t, 200, `{"config":{"creditUsagePercent":5,"billingPeriodEnd":"2026-11-01T00:00:00Z"}}`)
	home := writeGrokAuth(t, "k", time.Now().Add(time.Hour))
	r, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "grok", Home: home})
	if err != nil || len(r.Windows) != 1 || r.Windows[0].Name != "额度" || r.Windows[0].ResetsAt == nil {
		t.Fatalf("%+v, %v", r, err)
	}
}

func TestGrokRefreshesThroughTheCLIAndNeverWritesTheFile(t *testing.T) {
	_, auth := newGrokFake(t, 200, `{"config":{"creditUsagePercent":1}}`)
	home := writeGrokAuth(t, "stale-key", time.Now().Add(time.Minute))
	var ran atomic.Int32
	old := grokRefresh
	t.Cleanup(func() { grokRefresh = old })
	grokRefresh = func(ctx context.Context, h string) {
		ran.Add(1)
		// the CLI renews its own file
		b, _ := json.Marshal(map[string]any{"a-entry": map[string]any{"key": "fresh-key", "email": "a@example.com", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}})
		os.WriteFile(filepath.Join(h, "auth.json"), b, 0o600)
	}
	if _, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "grok", Home: home}); err != nil {
		t.Fatal(err)
	}
	if ran.Load() != 1 || auth.Load() != "Bearer fresh-key" {
		t.Fatalf("refresh ran %d times, auth %v", ran.Load(), auth.Load())
	}
}

func TestGrokExpiredAndRefused(t *testing.T) {
	old := grokRefresh
	t.Cleanup(func() { grokRefresh = old })
	grokRefresh = func(context.Context, string) {}
	newGrokFake(t, 200, `{}`)
	home := writeGrokAuth(t, "k", time.Now().Add(-time.Hour))
	_, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "grok", Home: home})
	if code(err) != protocol.CodeQuotaSignedOut || !strings.Contains(err.Error(), "grok login") {
		t.Fatalf("expired: %v", err)
	}
	if _, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "grok", Home: t.TempDir()}); code(err) != protocol.CodeQuotaSignedOut {
		t.Fatalf("no file: %v", err)
	}
	newGrokFake(t, 403, ``)
	home = writeGrokAuth(t, "k", time.Now().Add(time.Hour))
	if _, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "grok", Home: home}); code(err) != protocol.CodeQuotaSignedOut {
		t.Fatalf("403: %v", err)
	}
	newGrokFake(t, 502, ``)
	if _, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "grok", Home: home}); code(err) != protocol.CodeQuotaUnavailable {
		t.Fatalf("502: %v", err)
	}
}

// ---- Claude ----

const claudeSample = "\x1b[1mCurrent session: 13% used · resets Oct 1 at 3:30pm (Asia/Shanghai)\x1b[0m\n" +
	"Current week (all models): 4% used · resets Oct 3 at 2pm (Asia/Shanghai)\n" +
	"Current week (Fable 5.1): 0% used\n" +
	"Current week (all models): 9% used\n"

func TestParseClaudeUsage(t *testing.T) {
	sh, _ := time.LoadLocation("Asia/Shanghai")
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, sh)
	ws, err := parseClaudeUsage(claudeSample, at)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 3 {
		t.Fatalf("want 3 windows (duplicate dropped), got %+v", ws)
	}
	if ws[0].Name != "5 小时" || ws[0].UsedPercent != 13 || ws[0].SpanSecs != 18000 ||
		!ws[0].ResetsAt.Equal(time.Date(2026, 10, 1, 15, 30, 0, 0, sh)) {
		t.Fatalf("session: %+v", ws[0])
	}
	if ws[1].Name != "7 天" || !ws[1].ResetsAt.Equal(time.Date(2026, 10, 3, 14, 0, 0, 0, sh)) {
		t.Fatalf("week: %+v", ws[1])
	}
	if ws[2].Name != "7 天 · Fable 5.1" || ws[2].Model != "fable-5-1" || ws[2].ResetsAt != nil {
		t.Fatalf("model week: %+v", ws[2])
	}
}

func TestParseClaudeUsageErrors(t *testing.T) {
	for _, text := range []string{
		"Not logged in · Please run /login",
		"Error: 401 Unauthorized",
		"Current session: 3% used\nOAuth token has expired: sign-in expired",
	} {
		if _, err := parseClaudeUsage(text, time.Now()); code(err) != protocol.CodeQuotaSignedOut {
			t.Fatalf("%q: want signed out, got %v", text, err)
		}
	}
	_, err := parseClaudeUsage("You are on the Max plan.", time.Now())
	if code(err) != protocol.CodeQuotaUnavailable || !strings.Contains(err.Error(), "Max plan") {
		t.Fatalf("no windows: %v", err)
	}
}

func TestClaudeResetTime(t *testing.T) {
	sh, _ := time.LoadLocation("Asia/Shanghai")
	ny, _ := time.LoadLocation("America/New_York")
	at := time.Date(2026, 12, 30, 20, 0, 0, 0, sh)
	for _, tc := range []struct {
		in   string
		want time.Time
	}{
		{"Dec 31 at 3:30pm (Asia/Shanghai)", time.Date(2026, 12, 31, 15, 30, 0, 0, sh)},
		{"Jan 2 at 2pm (Asia/Shanghai)", time.Date(2027, 1, 2, 14, 0, 0, 0, sh)}, // next year
		{"Dec 31 at 3:30pm (America/New_York)", time.Date(2026, 12, 31, 15, 30, 0, 0, ny)},
		{"Dec 31 at 3:30\u202fPM (Asia/Shanghai)", time.Date(2026, 12, 31, 15, 30, 0, 0, sh)},
		{"11pm (Asia/Shanghai)", time.Date(2026, 12, 30, 23, 0, 0, 0, sh)},
		{"7:15am (Asia/Shanghai)", time.Date(2026, 12, 31, 7, 15, 0, 0, sh)}, // already past today
		{"Jan 3, 2027 at 1pm (Asia/Shanghai)", time.Date(2027, 1, 3, 13, 0, 0, 0, sh)},
	} {
		got, ok := claudeResetTime(tc.in, at)
		if !ok || !got.Equal(tc.want) {
			t.Errorf("%q: got %v ok=%v, want %v", tc.in, got, ok, tc.want)
		}
	}
	for _, bad := range []string{"", "soon", "Dec 31 at 3pm (Nowhere/Land)"} {
		if _, ok := claudeResetTime(bad, at); ok {
			t.Errorf("%q should not parse", bad)
		}
	}
}

func TestReadClaudeRunsUsageInTheAccountDirectory(t *testing.T) {
	old := claudeUsage
	t.Cleanup(func() { claudeUsage = old })
	var gotHome string
	var gotDefault bool
	claudeUsage = func(ctx context.Context, isDefault bool, home string) (string, error) {
		gotHome, gotDefault = home, isDefault
		return claudeSample, nil
	}
	dir := t.TempDir()
	r, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "claude", Home: dir})
	if err != nil || len(r.Windows) != 3 || gotHome != dir || gotDefault {
		t.Fatalf("%+v %v home=%q default=%v", r, err, gotHome, gotDefault)
	}
	if _, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "claude", Home: filepath.Join(dir, "missing")}); code(err) != protocol.CodeQuotaSignedOut {
		t.Fatalf("missing dir: %v", err)
	}
	claudeUsage = func(context.Context, bool, string) (string, error) { return "", unavailable("找不到 claude") }
	if _, err := Read(context.Background(), protocol.QuotaReadParams{Kind: "claude", Home: dir}); code(err) != protocol.CodeQuotaUnavailable {
		t.Fatalf("command failed: %v", err)
	}
}

func TestClaudeResult(t *testing.T) {
	if r, ok := claudeResult([]byte(`{"type":"result","is_error":false,"result":"Current session: 1% used"}`)); !ok || r.Result == "" {
		t.Fatal("object form")
	}
	if r, ok := claudeResult([]byte(`[{"type":"system"},{"type":"result","is_error":true,"result":"Not logged in"}]`)); !ok || !r.IsError || r.Result != "Not logged in" {
		t.Fatalf("array form: %+v", r)
	}
	if _, ok := claudeResult([]byte(`not json`)); ok {
		t.Fatal("garbage")
	}
	if err := classifyClaude("Not logged in · Please run /login"); code(err) != protocol.CodeQuotaSignedOut {
		t.Fatal(err)
	}
}

// ---- Register ----

type fakeRegistrar map[string]rpc.Handler

func (f fakeRegistrar) Handle(m string, h rpc.Handler) { f[m] = h }

func TestRegisterHandlesQuotaRead(t *testing.T) {
	reg := fakeRegistrar{}
	Register(reg)
	h := reg[protocol.MethodQuotaRead]
	if h == nil {
		t.Fatal("quota.read not registered")
	}
	_, err := h(context.Background(), json.RawMessage(`{"kind":"codex","home":"relative"}`))
	if code(err) != protocol.CodeBadParams {
		t.Fatalf("relative home: %v", err)
	}
	if _, err := h(context.Background(), json.RawMessage(`{`)); code(err) != protocol.CodeBadParams {
		t.Fatalf("bad json: %v", err)
	}
}

func TestAvailable(t *testing.T) {
	oldLook := lookPath
	t.Cleanup(func() { lookPath = oldLook })
	lookPath = func(string) (string, error) { return "", errors.New("none") }
	empty := t.TempDir()
	t.Setenv("HOME", empty)
	t.Setenv("USERPROFILE", empty)
	for _, e := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "GROK_HOME"} {
		t.Setenv(e, "")
	}
	if Available() {
		t.Fatal("nothing installed")
	}
	os.Mkdir(filepath.Join(empty, ".codex"), 0o755)
	if !Available() {
		t.Fatal("a codex directory counts")
	}
	os.Remove(filepath.Join(empty, ".codex"))
	lookPath = func(n string) (string, error) {
		if n == "grok" {
			return "/usr/bin/grok", nil
		}
		return "", errors.New("none")
	}
	if !Available() {
		t.Fatal("grok on PATH counts")
	}
}
