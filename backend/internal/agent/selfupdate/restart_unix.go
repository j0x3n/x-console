//go:build !windows

package selfupdate

import (
	"os"
	"syscall"
)

// restartProcess replaces this process with the new program. The process id
// stays, so systemd does not see an exit. Arguments and environment are the
// same as before.
func restartProcess(exe string) error {
	return syscall.Exec(exe, os.Args, os.Environ())
}
