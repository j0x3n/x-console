//go:build !windows

// Package nowindow keeps child programs from opening a console window.
// Only Windows needs it; elsewhere Hide does nothing.
package nowindow

import "os/exec"

// Hide does nothing outside Windows.
func Hide(*exec.Cmd) {}
