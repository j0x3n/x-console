//go:build !windows

package quota

import "os/exec"

// hide keeps a console window from opening on Windows; nothing to do here.
func hide(cmd *exec.Cmd) {}
