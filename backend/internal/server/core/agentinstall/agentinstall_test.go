package agentinstall_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/agent/setup"
	"github.com/j0x3n/x-console/backend/internal/server/core/agentinstall"
)

func TestScriptsCarryServerAndCode(t *testing.T) {
	sh, err := agentinstall.Script("https://console.example.com", "K7QM-3XHP")
	if err != nil || !strings.Contains(string(sh), "SERVER='https://console.example.com'") || !strings.Contains(string(sh), "CODE='K7QM-3XHP'") ||
		strings.Contains(string(sh), "{{") {
		t.Fatalf("install.sh: %v", err)
	}
	ps, err := agentinstall.PowerShell("http://10.0.0.5:8080", "K7QM-3XHP")
	if err != nil || !strings.Contains(string(ps), "$Server = 'http://10.0.0.5:8080'") || strings.Contains(string(ps), "{{") {
		t.Fatalf("install.ps1: %v", err)
	}
	// 代理在 Windows 上是图形程序，"&" 不会等它结束，配对必须用 Start-Process -Wait。
	if !strings.Contains(string(ps), "Start-Process -FilePath $exe -ArgumentList @('pair'") || !strings.Contains(string(ps), "-Wait") || strings.Contains(string(ps), "& $exe") {
		t.Fatal("install.ps1 must wait for pair with Start-Process -Wait")
	}
	// 关掉终端代理也要继续跑：用 install 放到后台，令牌失效（退出码 3）时用新配对码重配。
	for _, want := range []string{"'--no-start'", "-ArgumentList 'install'", "$p.ExitCode -eq 3"} {
		if !strings.Contains(string(ps), want) {
			t.Fatalf("install.ps1 lacks %q", want)
		}
	}
	if !strings.HasPrefix(string(sh), "#!/bin/sh\n") || !strings.HasPrefix(string(agentinstall.Uninstall()), "#!/bin/sh\n") {
		t.Fatal("script shape")
	}
}

// Values that reach a script are checked first: a panel address or code that
// could end the quotes must never be rendered.
func TestScriptsRefuseUnsafeValues(t *testing.T) {
	for _, server := range []string{"", "console.example.com", "https://x.com'; rm -rf /; '", "https://x.com/ y", "https://x.com\nrm", "ftp://x.com", "https://a.com`id`", "https://a.com$(id)",
		"https://" + strings.Repeat("a", 300)} {
		if _, err := agentinstall.Script(server, "K7QM-3XHP"); err == nil {
			t.Errorf("server %q accepted", server)
		}
		if _, err := agentinstall.PowerShell(server, "K7QM-3XHP"); err == nil {
			t.Errorf("server %q accepted by the PowerShell script", server)
		}
	}
	for _, code := range []string{"", "k7qm-3xhp", "K7QM3XHP", "K7QM-3XH'", "K7QM-3XHP; id", "K7QM-3XHP\n"} {
		if _, err := agentinstall.Script("https://console.example.com", code); err == nil {
			t.Errorf("code %q accepted", code)
		}
	}
	for _, server := range []string{"https://console.example.com", "http://10.0.0.5:8080", "https://console.example.com/x-console", "http://[::1]:8080", "https://a-b.example.com:443"} {
		if _, err := agentinstall.Script(server, "K7QM-3XHP"); err != nil {
			t.Errorf("server %q refused: %v", server, err)
		}
	}
}

// The setup program in the agent reads what AppendSetup writes.
func TestAppendSetupRoundTrip(t *testing.T) {
	if setup.Marker != agentinstall.SetupMarker {
		t.Fatalf("marker differs: agent %q, server %q", setup.Marker, agentinstall.SetupMarker)
	}
	exe := []byte("MZ" + strings.Repeat("program", 1000))
	out, err := agentinstall.AppendSetup(exe, "https://console.example.com")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "x-console-agent-setup-K7QM-3XHP.exe")
	if err := os.WriteFile(path, out, 0o755); err != nil {
		t.Fatal(err)
	}
	tr, ok, err := setup.ReadTrailer(path)
	if err != nil || !ok || tr.Server != "https://console.example.com" || tr.Size != int64(len(exe)) {
		t.Fatalf("trailer: %+v %v %v", tr, ok, err)
	}
	if _, err := agentinstall.AppendSetup(exe, "javascript:alert(1)"); err == nil {
		t.Fatal("bad server accepted")
	}
}
