//go:build windows

package focusblock

import (
	"os/exec"
	"syscall"
)

// flushDNS empties the Windows name cache, so the change of the hosts file
// counts at once. It is best effort: the result does not matter.
func flushDNS() {
	cmd := exec.Command("ipconfig", "/flushdns")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()
}
