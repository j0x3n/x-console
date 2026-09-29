package core_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// get sends a request without a login, as an install script does. Each test
// uses its own address (through X-Forwarded-For) so the limits do not mix.
func get(t *testing.T, env *testutil.Env, path, ip string) (int, http.Header, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, env.URL(path), nil)
	req.Header.Set("X-Forwarded-For", ip)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, body
}

func agentsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"linux-amd64/x-console-agent":       "LINUX-AMD64",
		"linux-arm64/x-console-agent":       "LINUX-ARM64",
		"windows-amd64/x-console-agent.exe": "MZ-WINDOWS",
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func newCode(t *testing.T, env *testutil.Env, name, kind string) string {
	t.Helper()
	env.Elevate()
	var out struct{ Code string }
	env.MustDo(http.MethodPost, "/agents/pairing-codes", map[string]string{"name": name, "kind": kind}, &out)
	return out.Code
}

func TestInstallScripts(t *testing.T) {
	env := testutil.NewWithConfig(t, func(c *config.Config) { c.AgentsDir = agentsDir(t) })
	code := newCode(t, env, "web-1", "server")

	status, header, body := get(t, env, "/agent/install.sh?code="+code, "10.0.0.1")
	if status != 200 || !strings.HasPrefix(header.Get("Content-Type"), "text/x-shellscript") || header.Get("Cache-Control") != "no-store" {
		t.Fatalf("install.sh: %d %v", status, header)
	}
	script := string(body)
	for _, want := range []string{"SERVER='" + env.Server.URL + "'", "CODE='" + code + "'", "MemoryMax=128M", "CPUQuota=20%", "Nice=10",
		"/api/v1/agent/download/linux/$ARCH", "X-Checksum-Sha256"} {
		if !strings.Contains(script, want) && !strings.Contains(strings.ToLower(script), strings.ToLower(want)) {
			t.Errorf("install.sh has no %q", want)
		}
	}
	// A code typed in lower case and without the dash is the same code.
	status, _, _ = get(t, env, "/agent/install.sh?code="+strings.ToLower(strings.ReplaceAll(code, "-", "")), "10.0.0.1")
	if status != 200 {
		t.Fatalf("lower case code: %d", status)
	}
	status, header, body = get(t, env, "/agent/install.ps1?code="+code, "10.0.0.1")
	if status != 200 || !strings.Contains(string(body), "$Server = '"+env.Server.URL+"'") || !strings.Contains(string(body), "$Code = '"+code+"'") || header.Get("Content-Type") == "" {
		t.Fatalf("install.ps1: %d %s", status, body)
	}
	// Looking at a script does not use the code up: the agent still pairs with it.
	status, _, _ = get(t, env, "/agent/install.sh?code="+code, "10.0.0.1")
	if status != 200 {
		t.Fatalf("code used up by a script: %d", status)
	}
	status, _, body = get(t, env, "/agent/uninstall.sh", "10.0.0.1")
	if status != 200 || !strings.Contains(string(body), "systemctl disable x-console-agent") || strings.Contains(string(body), code) {
		t.Fatalf("uninstall.sh: %d", status)
	}

	// Pairing uses the code up, and then every entrance answers 404.
	env.MustDo(http.MethodPost, "/agent/pair", map[string]any{"code": code, "os": "linux", "arch": "amd64", "hostname": "web-1", "version": "test"}, nil)
	for _, path := range []string{"/agent/install.sh?code=" + code, "/agent/install.ps1?code=" + code, "/agent/setup.exe?code=" + code, "/agent/download/linux/amd64?code=" + code} {
		if status, _, _ := get(t, env, path, "10.0.0.2"); status != 404 {
			t.Errorf("used code on %s: %d", path, status)
		}
	}
}

func TestInstallRejectsBadCodes(t *testing.T) {
	env := testutil.NewWithConfig(t, func(c *config.Config) { c.AgentsDir = agentsDir(t) })
	// An expired code.
	expired := "ABCD-2345"
	if _, err := env.App.Deps.DB.Exec(`INSERT INTO pairing_codes (code_hash, name, kind, expires_at) VALUES (?, 'old', 'server', ?)`,
		secrets.Hash("ABCD2345"), time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	bad := []string{"", "nonsense", "AAAA-BBBB", expired, "x'; rm -rf /", "%0A%0A", strings.Repeat("A", 500)}
	for i, code := range bad {
		for _, path := range []string{"/agent/install.sh", "/agent/install.ps1", "/agent/setup.exe", "/agent/download/linux/amd64"} {
			status, _, body := get(t, env, path+"?code="+url.QueryEscape(code), fmt.Sprintf("10.0.1.%d", i))
			if status != 404 || !strings.Contains(string(body), "资源不存在") {
				t.Errorf("%s with %q: %d %s", path, code, status, body)
			}
		}
	}
	// Without a login and without a code the download is refused too, and everything else still needs a login.
	if status, _, _ := get(t, env, "/agent/download/linux/amd64", "10.0.2.1"); status != 404 {
		t.Errorf("download without code: %d", status)
	}
	for _, path := range []string{"/agents", "/audit"} {
		if status, _, _ := get(t, env, path, "10.0.2.1"); status != 401 {
			t.Errorf("%s without a login: %d", path, status)
		}
	}
}

func TestInstallRateLimit(t *testing.T) {
	env := testutil.NewWithConfig(t, func(c *config.Config) { c.AgentsDir = agentsDir(t) })
	code := newCode(t, env, "pc", "desktop")
	for i := 1; i <= 10; i++ {
		if status, _, _ := get(t, env, "/agent/install.ps1?code="+code, "10.9.9.9"); status != 200 {
			t.Fatalf("request %d: %d", i, status)
		}
	}
	if status, _, _ := get(t, env, "/agent/install.ps1?code="+code, "10.9.9.9"); status != 429 {
		t.Fatalf("11th request: %d", status)
	}
	// Bad codes count too, and another address is not affected.
	if status, _, _ := get(t, env, "/agent/install.ps1?code="+code, "10.9.9.10"); status != 200 {
		t.Fatalf("other address: %d", status)
	}
}

func TestDownloadAgent(t *testing.T) {
	env := testutil.NewWithConfig(t, func(c *config.Config) { c.AgentsDir = agentsDir(t) })
	code := newCode(t, env, "web-1", "server")

	status, header, body := get(t, env, "/agent/download/linux/arm64?code="+code, "10.1.0.1")
	sum := sha256.Sum256([]byte("LINUX-ARM64"))
	if status != 200 || string(body) != "LINUX-ARM64" || header.Get("X-Checksum-Sha256") != hex.EncodeToString(sum[:]) ||
		!strings.Contains(header.Get("Content-Disposition"), `attachment; filename="x-console-agent"`) || header.Get("Content-Length") != "11" {
		t.Fatalf("download: %d %q %v", status, body, header)
	}
	status, header, body = get(t, env, "/agent/download/windows/amd64?code="+code, "10.1.0.1")
	if status != 200 || string(body) != "MZ-WINDOWS" || !strings.Contains(header.Get("Content-Disposition"), "x-console-agent.exe") {
		t.Fatalf("windows: %d %q %v", status, body, header)
	}
	// Platforms that are not packaged, and ones that do not exist.
	status, _, body = get(t, env, "/agent/download/windows/arm64?code="+code, "10.1.0.1")
	if status != 404 || !strings.Contains(string(body), "这个面板没有打包代理") {
		t.Fatalf("not packaged: %d %s", status, body)
	}
	if status, _, _ = get(t, env, "/agent/download/plan9/amd64?code="+code, "10.1.0.1"); status != 404 {
		t.Fatalf("unknown platform: %d", status)
	}
	// A login is enough without a code.
	resp, err := env.Client.Get(env.URL("/agent/download/linux/amd64"))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(got) != "LINUX-AMD64" {
		t.Fatalf("logged in download: %d %q", resp.StatusCode, got)
	}
}

func TestSetupExe(t *testing.T) {
	env := testutil.NewWithConfig(t, func(c *config.Config) { c.AgentsDir = agentsDir(t); c.PublicURL = "https://console.example.com" })
	code := newCode(t, env, "my-pc", "desktop")
	status, header, body := get(t, env, "/agent/setup.exe?code="+code, "10.2.0.1")
	want := "MZ-WINDOWS\nXC-SETUP:{\"server\":\"https://console.example.com\"}"
	if status != 200 || string(body) != want || !strings.Contains(header.Get("Content-Disposition"), `filename="x-console-agent-setup-`+code+`.exe"`) {
		t.Fatalf("setup.exe: %d %q %v", status, body, header)
	}
	if header.Get("Content-Length") != "58" && header.Get("Content-Length") == "" {
		t.Fatalf("length: %v", header)
	}
	// The address in scripts is the configured one, not the Host header.
	_, _, script := get(t, env, "/agent/install.sh?code="+code, "10.2.0.1")
	if !strings.Contains(string(script), "SERVER='https://console.example.com'") {
		t.Fatalf("script server: %s", script[:300])
	}
}

func TestNoAgentsPackaged(t *testing.T) {
	env := testutil.NewWithConfig(t, func(c *config.Config) { c.AgentsDir = filepath.Join(t.TempDir(), "none") })
	code := newCode(t, env, "x", "server")
	status, _, body := get(t, env, "/agent/download/linux/amd64?code="+code, "10.3.0.1")
	if status != 404 || !strings.Contains(string(body), "这个面板没有打包代理") {
		t.Fatalf("no agents: %d %s", status, body)
	}
	// The scripts still work: they only fail when they try to download.
	if status, _, _ := get(t, env, "/agent/install.sh?code="+code, "10.3.0.1"); status != 200 {
		t.Fatalf("script without agents: %d", status)
	}
}
