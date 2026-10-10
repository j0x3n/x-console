//go:build windows

package nowindow

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestHideKeepsExistingFlags(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x200} // CREATE_NEW_PROCESS_GROUP
	Hide(cmd)
	if !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags != 0x200|createNoWindow {
		t.Fatalf("unexpected attributes: %+v", cmd.SysProcAttr)
	}
}
