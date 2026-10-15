package router_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// B227: the install script is downloaded with one command.

type installToken struct {
	Token, ReportUrl, Script, InstallUrl, Command string
	InstallExpiresAt                              time.Time
}

// download gets a link the way wget does: no session.
func download(t *testing.T, url string) (int, string, string) {
	t.Helper()
	resp, err := (&http.Client{}).Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw), resp.Header.Get("Content-Type")
}

func newInstallToken(t *testing.T, env *testutil.Env) installToken {
	t.Helper()
	var tok installToken
	env.MustDo(http.MethodPost, "/router/push/token", nil, &tok)
	return tok
}

func TestInstallDownloadOnce(t *testing.T) {
	env, _, c, _ := setupPush(t)
	tok := newInstallToken(t, env)
	if !strings.HasPrefix(tok.InstallUrl, env.URL("/router/install/")) || len(strings.TrimPrefix(tok.InstallUrl, env.URL("/router/install/"))) != 64 {
		t.Fatalf("install url: %s", tok.InstallUrl)
	}
	if want := "wget -qO /tmp/xc-install.sh '" + tok.InstallUrl + "' && sh /tmp/xc-install.sh"; tok.Command != want {
		t.Fatalf("command: %q", tok.Command)
	}
	if !tok.InstallExpiresAt.Equal(c.t.Add(15 * time.Minute)) {
		t.Fatalf("expires: %v", tok.InstallExpiresAt)
	}

	// Without a session, the script has the token filled in.
	code, body, ctype := download(t, tok.InstallUrl)
	if code != http.StatusOK || !strings.HasPrefix(ctype, "text/plain") || body != tok.Script {
		t.Fatalf("download: %d %q", code, ctype)
	}
	if !strings.Contains(body, "TOKEN='"+tok.Token+"'") {
		t.Fatal("token is not in the downloaded script")
	}
	// The second download finds nothing, and says what to do.
	code, body, _ = download(t, tok.InstallUrl)
	if code != http.StatusNotFound || !strings.Contains(body, "安装链接失效了，回面板重新生成") {
		t.Fatalf("second download: %d %q", code, body)
	}
	// The code opens nothing else.
	if code, _, _ := download(t, tok.InstallUrl+"x"); code != http.StatusNotFound {
		t.Fatalf("wrong code: %d", code)
	}
	if n := countRows(t, env, `SELECT COUNT(*) FROM audit_log WHERE action = 'router.install.download'`); n != 1 {
		t.Fatalf("audit entries: %d", n)
	}
	var detail string
	if err := env.App.Deps.DB.QueryRow(`SELECT detail FROM audit_log WHERE action = 'router.install.download'`).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, `"ip":"127.0.0.1"`) || strings.Contains(detail, tok.Token) {
		t.Fatalf("audit detail: %s", detail)
	}
}

func TestInstallCodeExpires(t *testing.T) {
	env, _, c, _ := setupPush(t)
	tok := newInstallToken(t, env)
	c.t = c.t.Add(15*time.Minute + time.Second)
	if code, _, _ := download(t, tok.InstallUrl); code != http.StatusNotFound {
		t.Fatalf("after 15 minutes: %d", code)
	}
	// A new token gives a new, working link.
	next := newInstallToken(t, env)
	if code, _, _ := download(t, next.InstallUrl); code != http.StatusOK {
		t.Fatalf("new link: %d", code)
	}
}

func TestInstallNewTokenEndsOldLink(t *testing.T) {
	env, _, c, _ := setupPush(t)
	old := newInstallToken(t, env)
	next := newInstallToken(t, env)
	if old.InstallUrl == next.InstallUrl {
		t.Fatal("same link twice")
	}
	if code, _, _ := download(t, old.InstallUrl); code != http.StatusNotFound {
		t.Fatalf("old link: %d", code)
	}
	// The old script reports with the old token, which no longer works.
	if code := post(t, env, old.Token, reportBody(1, 1, true, c.t.Unix())); code != http.StatusUnauthorized {
		t.Fatalf("old token: %d", code)
	}
	code, body, _ := download(t, next.InstallUrl)
	if code != http.StatusOK || !strings.Contains(body, "TOKEN='"+next.Token+"'") {
		t.Fatalf("new link: %d", code)
	}
	if code := post(t, env, next.Token, reportBody(1, 1, true, c.t.Unix())); code != http.StatusOK {
		t.Fatalf("new token: %d", code)
	}
}

func TestInstallNeedsElevationToCreate(t *testing.T) {
	env := testutil.New(t)
	if code, _ := env.Do(http.MethodPost, "/router/push/token", nil, nil); code != http.StatusForbidden {
		t.Fatalf("token without elevation: %d", code)
	}
	if code, _, _ := download(t, env.URL("/router/install/nothing")); code != http.StatusNotFound {
		t.Fatalf("unknown code: %d", code)
	}
}

func countRows(t *testing.T, env *testutil.Env, query string, args ...any) int {
	t.Helper()
	var n int
	if err := env.App.Deps.DB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
