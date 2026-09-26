//go:build windows

package exec

import (
	osexec "os/exec"
	"strconv"
	"syscall"
)

func shellCommand(command string) (string, []string) { return PowerShellCommand(command) }

// prepare hides the console window PowerShell would otherwise open.
func prepare(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

func killTree(cmd *osexec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = osexec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	_ = cmd.Process.Kill()
}
