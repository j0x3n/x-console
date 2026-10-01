package agentinstall_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/core/agentinstall"
)

// fakeAgent stands in for the real program: pair writes the config, run waits
// a little and exits with 3 (revoked) so the boot script stops by itself.
const fakeAgent = `#!/bin/sh
cmd=$1
shift
while [ $# -gt 0 ]; do
    [ "$1" = --config ] && cfg=$2
    shift
done
case "$cmd" in
pair) echo '{"token":"t"}' > "$cfg" ;;
run) sleep 3; exit 3 ;;
esac
`

// TestUnraidInstall runs install.sh in a temporary root that looks like Unraid
// (B64): the program and config go to the USB stick, /boot/config/go gets one
// block however many times the script runs, and uninstall.sh takes it out.
func TestUnraidInstall(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("shell scripts run on Linux")
	}
	for _, tool := range []string{"sh", "pgrep", "pkill", "sed", "install", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("no %s", tool)
		}
	}
	root := t.TempDir()
	bin := t.TempDir()
	t.Cleanup(func() { _ = exec.Command("pkill", "-f", root).Run() })
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	sum := sha256.Sum256([]byte(fakeAgent))
	must(os.WriteFile(filepath.Join(bin, "agent"), []byte(fakeAgent), 0o755))
	// curl -fsSL -D headers -o file url: copy the fake agent and send its checksum.
	must(os.WriteFile(filepath.Join(bin, "curl"), []byte(`#!/bin/sh
while [ $# -gt 0 ]; do
    case "$1" in
    -D) headers=$2; shift ;;
    -o) out=$2; shift ;;
    esac
    shift
done
cp '`+filepath.Join(bin, "agent")+`' "$out"
printf 'HTTP/1.1 200 OK\r\nX-Checksum-Sha256: `+hex.EncodeToString(sum[:])+`\r\n\r\n' > "$headers"
`), 0o755))
	must(os.WriteFile(filepath.Join(bin, "id"), []byte("#!/bin/sh\necho 0\n"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "etc"), 0o755))
	must(os.WriteFile(filepath.Join(root, "etc/unraid-version"), []byte("version=\"7.1.4\"\n"), 0o644))
	must(os.MkdirAll(filepath.Join(root, "boot/config"), 0o755))
	must(os.WriteFile(filepath.Join(root, "boot/config/go"), []byte("#!/bin/bash\n/usr/local/sbin/emhttp &\n"), 0o644))

	script, err := agentinstall.Script("https://console.example.com", "K7QM-3XHP")
	must(err)
	run := func(body []byte) string {
		t.Helper()
		cmd := exec.Command("sh", "-s")
		cmd.Stdin = strings.NewReader(string(body))
		cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "XC_UNRAID_ROOT="+root)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return string(out)
	}

	out := run(script)
	if !strings.Contains(out, "Unraid") || !strings.Contains(out, "代理已启动") {
		t.Fatalf("first install: %s", out)
	}
	usb := filepath.Join(root, "boot/config/plugins/x-console-agent")
	for _, name := range []string{"x-console-agent", "agent.json", "run.sh"} {
		if _, err := os.Stat(filepath.Join(usb, name)); err != nil {
			t.Fatalf("missing %s on the USB stick: %v", name, err)
		}
	}
	out = run(script)
	if !strings.Contains(out, "只升级程序") {
		t.Fatalf("second install should keep the pairing: %s", out)
	}
	goFile, err := os.ReadFile(filepath.Join(root, "boot/config/go"))
	must(err)
	if n := strings.Count(string(goFile), "# >>> x-console-agent >>>"); n != 1 {
		t.Fatalf("blocks in go: %d\n%s", n, goFile)
	}
	if !strings.HasPrefix(string(goFile), "#!/bin/bash\n/usr/local/sbin/emhttp &\n") || !strings.Contains(string(goFile), filepath.Join(usb, "run.sh")) {
		t.Fatalf("go file: %s", goFile)
	}
	boot, err := os.ReadFile(filepath.Join(usb, "run.sh"))
	must(err)
	if !strings.Contains(string(boot), "run --config '"+filepath.Join(usb, "agent.json")+"'") || !strings.Contains(string(boot), "-eq 3 ] && break") {
		t.Fatalf("run.sh: %s", boot)
	}

	run(agentinstall.Uninstall())
	goFile, err = os.ReadFile(filepath.Join(root, "boot/config/go"))
	must(err)
	if string(goFile) != "#!/bin/bash\n/usr/local/sbin/emhttp &\n" {
		t.Fatalf("go after uninstall: %q", goFile)
	}
	if _, err := os.Stat(usb); !os.IsNotExist(err) {
		t.Fatalf("USB folder left: %v", err)
	}
}
