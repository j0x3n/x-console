package exec

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestTimeout(t *testing.T) {
	if Timeout(0) != time.Minute || Timeout(10) != 10*time.Second || Timeout(99999) != 30*time.Minute {
		t.Fatal("timeouts")
	}
}

func TestCapped(t *testing.T) {
	c := &capped{max: 5}
	_, _ = c.Write([]byte("abc"))
	_, _ = c.Write([]byte("defg"))
	if c.String() != "abcde" || !c.truncated {
		t.Fatalf("%q %v", c.String(), c.truncated)
	}
}

func TestPowerShellCommand(t *testing.T) {
	name, args := PowerShellCommand("Get-Date")
	if name != "powershell.exe" || !strings.HasSuffix(args[len(args)-1], "Get-Date") || args[len(args)-2] != "-Command" {
		t.Fatal(name, args)
	}
}

func TestRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell")
	}
	ctx := context.Background()
	res, err := Run(ctx, protocol.ExecParams{Command: "echo out; echo err >&2; exit 3", Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 3 || res.Stdout != "out\n" || res.Stderr != "err\n" || res.TimedOut {
		t.Fatalf("%+v", res)
	}
	start := time.Now()
	res, err = Run(ctx, protocol.ExecParams{Command: "sleep 20 & sleep 20", TimeoutSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !res.TimedOut || res.ExitCode != -1 || time.Since(start) > 8*time.Second {
		t.Fatalf("timeout: %+v after %v", res, time.Since(start))
	}
	res, _ = Run(ctx, protocol.ExecParams{Command: "head -c 2000000 /dev/zero"})
	if len(res.Stdout) != protocol.ExecMaxOutput || !res.Truncated {
		t.Fatalf("cap: %d %v", len(res.Stdout), res.Truncated)
	}
	start = time.Now()
	res, _ = Run(ctx, protocol.ExecParams{Command: "sleep 30 & echo started"})
	if res.ExitCode != 0 || res.Stdout != "started\n" || time.Since(start) > 10*time.Second {
		t.Fatalf("background job: %+v after %v", res, time.Since(start))
	}
	if _, err := Run(ctx, protocol.ExecParams{Command: "  "}); err == nil {
		t.Fatal("empty command accepted")
	}
}
