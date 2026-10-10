//go:build windows

// Package nowindow keeps child programs from opening a console window.
// The Windows agent is a GUI program with no console of its own, so every
// console program it starts (taskkill, wevtutil, powershell, git...) gets a
// new visible window unless it is started with CREATE_NO_WINDOW.
package nowindow

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// Hide makes cmd start without a console window. Other attributes already
// set on cmd are kept.
func Hide(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
