//go:build windows

package setup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/j0x3n/x-console/backend/internal/agent/nowindow"
)

const taskName = "X Console Agent"

// IsSetup reports whether this process is the setup program.
func IsSetup() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	_, ok := ParseName(exe)
	return ok
}

// Run installs the agent for the current user: copies the program to
// %LOCALAPPDATA%\x-console-agent, pairs, registers a task that starts it at
// logon and starts it now. It shows a message when it is done or fails.
func Run(opts Options) error {
	err := install(opts)
	if err != nil {
		messageBox("X Console 代理", "安装失败：\n"+err.Error())
		return err
	}
	messageBox("X Console 代理", "已连接到面板。几秒后面板的设备列表里会出现这台电脑。")
	return nil
}

func install(opts Options) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	code, _ := ParseName(self)
	server := ""
	if t, ok, _ := ReadTrailer(self); ok {
		server = t.Server
	}
	_, paired := os.Stat(opts.ConfigPath)
	needPair := paired != nil
	if needPair {
		if server == "" {
			if server = strings.TrimSpace(inputBox("面板地址", "面板地址，例如 https://console.example.com", "")); server == "" {
				return errors.New("没有填面板地址")
			}
		}
		if code == "" {
			if code = strings.ToUpper(strings.TrimSpace(inputBox("配对码", "配对码，在面板的设置里生成", ""))); code == "" {
				return errors.New("没有填配对码")
			}
		}
	}

	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return errors.New("找不到 LOCALAPPDATA")
	}
	dst := filepath.Join(base, "x-console-agent", "x-console-agent.exe")
	if !strings.EqualFold(dst, self) {
		// A running agent has to stop before its file can be replaced.
		_ = run("schtasks", "/End", "/TN", taskName)
		_ = run("taskkill", "/F", "/IM", "x-console-agent.exe")
		if err := CopyProgram(self, dst); err != nil {
			return fmt.Errorf("复制程序失败：%w", err)
		}
	}
	if needPair {
		if err := opts.Pair(server, code); err != nil {
			return fmt.Errorf("配对失败：%w", err)
		}
	}
	if _, err := Background(opts.ConfigPath); err != nil {
		return err
	}
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	nowindow.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// runLine runs a program with a command line written out in full, for
// arguments that carry quotes.
func runLine(name, cmdline string) error {
	cmd := exec.Command(name)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: cmdline}
	nowindow.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// inputBox asks for a line of text with a small window.
func inputBox(title, prompt, def string) string {
	esc := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	script := fmt.Sprintf("Add-Type -AssemblyName Microsoft.VisualBasic; [Microsoft.VisualBasic.Interaction]::InputBox('%s','%s','%s')",
		esc(prompt), esc(title), esc(def))
	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	nowindow.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func messageBox(title, text string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	proc := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	_, _, _ = proc.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0)
}
