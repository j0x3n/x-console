package notifyshow

import (
	"os/exec"

	"golang.org/x/sys/windows"

	"github.com/j0x3n/x-console/backend/internal/agent/nowindow"
)

func Available() bool {
	var session uint32
	return windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session) == nil && session != 0
}

func prepareCommand(cmd *exec.Cmd) { nowindow.Hide(cmd) }
