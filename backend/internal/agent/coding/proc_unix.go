//go:build !windows

package coding

import (
	"os/exec"
	"syscall"
)

// procGroup controls an executor and everything it starts. On Unix the
// executor leads its own process group.
type procGroup struct{ cmd *exec.Cmd }

// prepareGroup must be called before cmd.Start.
func prepareGroup(cmd *exec.Cmd) *procGroup {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return &procGroup{cmd: cmd}
}

// started is called right after cmd.Start.
func (g *procGroup) started() error { return nil }

// interrupt asks the whole group to stop (SIGINT, like Ctrl+C).
func (g *procGroup) interrupt() {
	if g.cmd.Process != nil {
		_ = syscall.Kill(-g.cmd.Process.Pid, syscall.SIGINT)
	}
}

// kill ends the whole group.
func (g *procGroup) kill() {
	if g.cmd.Process != nil {
		_ = syscall.Kill(-g.cmd.Process.Pid, syscall.SIGKILL)
	}
}

// close releases resources after the executor exited.
func (g *procGroup) close() {}

func hideWindow(*exec.Cmd) {}
