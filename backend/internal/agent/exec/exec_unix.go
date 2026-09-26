//go:build !windows

package exec

import (
	osexec "os/exec"
	"syscall"
)

func shellCommand(command string) (string, []string) { return "/bin/sh", []string{"-c", command} }

// prepare puts the command in its own process group so a timeout kills
// everything it started.
func prepare(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killTree(cmd *osexec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
