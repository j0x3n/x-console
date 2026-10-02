package notifyshow

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func Available() bool {
	var session uint32
	return windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session) == nil && session != 0
}

func prepareCommand(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
