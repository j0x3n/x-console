//go:build linux

package pty

import (
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	cpty "github.com/creack/pty"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func available() bool { return true }

// DefaultShell is $SHELL, then bash, then sh.
func DefaultShell() (string, error) {
	for _, s := range []string{os.Getenv("SHELL"), "/bin/bash", "/bin/sh"} {
		if s == "" {
			continue
		}
		if p, err := exec.LookPath(s); err == nil {
			return p, nil
		}
	}
	return "", errNoShell
}

type unixTerminal struct {
	f    *os.File
	cmd  *exec.Cmd
	done chan struct{}
	err  error
	once sync.Once
}

func start(p protocol.PTYOpenParams) (terminal, error) {
	shell := p.Shell
	var err error
	if shell == "" {
		if shell, err = DefaultShell(); err != nil {
			return nil, rpcutil.Failed("%v", err)
		}
	} else if shell, err = exec.LookPath(shell); err != nil {
		return nil, rpcutil.BadParams("shell not found: %v", err)
	}
	cmd := exec.Command(shell, "-l")
	cmd.Dir = p.Cwd
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	// pty.Start makes the shell a session leader with the pty as its
	// controlling terminal, so the whole session can be signaled at once.
	f, err := cpty.StartWithSize(cmd, &cpty.Winsize{Cols: uint16(p.Cols), Rows: uint16(p.Rows)})
	if err != nil {
		return nil, rpcutil.Failed("start shell: %v", err)
	}
	t := &unixTerminal{f: f, cmd: cmd, done: make(chan struct{})}
	go func() {
		t.err = cmd.Wait()
		close(t.done)
	}()
	return t, nil
}

func (t *unixTerminal) Read(b []byte) (int, error)  { return t.f.Read(b) }
func (t *unixTerminal) Write(b []byte) (int, error) { return t.f.Write(b) }

func (t *unixTerminal) Resize(cols, rows int) error {
	return cpty.Setsize(t.f, &cpty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (t *unixTerminal) Wait() error {
	<-t.done
	return t.err
}

// Kill hangs up the session like a closed terminal window would, and uses
// SIGKILL on anything still alive after killGrace.
func (t *unixTerminal) Kill() {
	t.once.Do(func() {
		pgid := t.cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGHUP)
		_ = syscall.Kill(-pgid, syscall.SIGCONT)
		select {
		case <-t.done:
		case <-time.After(killGrace):
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			<-t.done
		}
		_ = t.f.Close()
	})
}
