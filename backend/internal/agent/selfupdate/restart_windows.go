//go:build windows

package selfupdate

import (
	"os"
	"os/exec"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/nowindow"
)

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// restartProcess starts the new program with the same arguments, detached
// from this process and without a window (the same way setup.Background
// starts the agent), and then ends this process. The new agent connects to
// the panel and takes over; the old file is deleted when it starts.
func restartProcess(exe string) error {
	cmd := exec.Command(exe, os.Args[1:]...)
	nowindow.Hide(cmd)
	cmd.SysProcAttr.CreationFlags |= detachedProcess | createNewProcessGroup
	// No handles are passed on: the new agent opens agent.log itself, the
	// same way as when the task scheduler starts it, so the log still rotates.
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	time.Sleep(300 * time.Millisecond) // let the last frame leave
	os.Exit(0)
	return nil
}
