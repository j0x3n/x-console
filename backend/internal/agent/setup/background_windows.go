//go:build windows

package setup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/config"
	"github.com/j0x3n/x-console/backend/internal/agent/nowindow"
)

// ExitRevoked is the exit code of "run" when the panel rejected the token. It
// is also what Background returns in ErrRevoked's place, so a script can tell
// "pair again" from other failures.
const ExitRevoked = 3

// ErrRevoked means the agent started and stopped at once because the panel
// does not accept its token any more (the device was removed in the panel).
var ErrRevoked = errors.New("the panel rejected the agent token; pair again")

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
	startupCheck          = 5 * time.Second
	runKey                = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
)

// Background makes the agent run without a terminal: it puts the program in
// %LOCALAPPDATA%\x-console-agent, makes it start at logon, stops an agent that
// is already running and starts a new one detached from the terminal that ran
// this command. It waits a few seconds and fails when the new agent stops at
// once, with the end of the log in the message. configPath is passed on to
// "run" when it is not the default.
func Background(configPath string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", errors.New("找不到 LOCALAPPDATA")
	}
	dir := filepath.Join(base, "x-console-agent")
	dst := filepath.Join(dir, "x-console-agent.exe")

	// A running agent has to stop before its file can be replaced. This
	// process keeps running, so it is left out.
	_ = run("schtasks", "/End", "/TN", taskName)
	_ = run("taskkill", "/F", "/FI", "IMAGENAME eq x-console-agent.exe", "/FI", "PID ne "+strconv.Itoa(os.Getpid()))
	time.Sleep(500 * time.Millisecond)
	if !strings.EqualFold(dst, self) {
		if err := CopyProgram(self, dst); err != nil {
			return "", fmt.Errorf("复制程序失败：%w", err)
		}
	}

	runArgs := []string{"run"}
	cmdTail := `run`
	if configPath != "" && configPath != config.DefaultPath() {
		runArgs = append(runArgs, "--config", configPath)
		cmdTail += ` --config \"` + configPath + `\"`
	}
	how, err := registerAutostart(dst, cmdTail)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(dst, runArgs...)
	cmd.Dir = dir
	nowindow.Hide(cmd)
	cmd.SysProcAttr.CreationFlags |= detachedProcess | createNewProcessGroup
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动代理失败：%w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case werr := <-done:
		var ee *exec.ExitError
		if errors.As(werr, &ee) && ee.ExitCode() == ExitRevoked {
			return "", ErrRevoked
		}
		return "", fmt.Errorf("代理启动后马上退出了（%v）。日志在 %s\n%s", werr, filepath.Join(dir, "agent.log"), logTail(filepath.Join(dir, "agent.log")))
	case <-time.After(startupCheck):
		_ = cmd.Process.Release()
	}
	return "代理已在后台运行，" + how + "。关掉这个窗口也不会停。", nil
}

// registerAutostart starts the agent at logon. The task scheduler comes first;
// where a normal user may not create tasks, the Run key of the current user
// does the same without an administrator.
func registerAutostart(dst, cmdTail string) (string, error) {
	cmdline := fmt.Sprintf(`schtasks /Create /F /TN "%s" /TR "\"%s\" %s" /SC ONLOGON /RL LIMITED`, taskName, dst, cmdTail)
	terr := runLine("schtasks", cmdline)
	if terr == nil {
		// The Run key of an earlier install would start a second agent.
		_ = run("reg", "delete", runKey, "/v", taskName, "/f")
		return "下次登录会自动启动", nil
	}
	rline := fmt.Sprintf(`reg add "%s" /v "%s" /t REG_SZ /d "\"%s\" %s" /f`, runKey, taskName, dst, cmdTail)
	if rerr := runLine("reg", rline); rerr != nil {
		return "", fmt.Errorf("注册开机启动失败：任务计划 %v；启动项 %v", terr, rerr)
	}
	return "下次登录会自动启动（用启动项）", nil
}

func logTail(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\r\n"), "\n")
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	return strings.Join(lines, "\n")
}
