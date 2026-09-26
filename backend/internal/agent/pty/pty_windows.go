//go:build windows

package pty

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"sync"

	"github.com/UserExistsError/conpty"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func available() bool { return conpty.IsConPtyAvailable() }

type winTerminal struct {
	c    *conpty.ConPty
	done chan struct{}
	err  error
	once sync.Once
}

func start(p protocol.PTYOpenParams) (terminal, error) {
	if !conpty.IsConPtyAvailable() {
		return nil, rpcutil.Unsupported("ConPTY (Windows 10 1809 or newer)")
	}
	opts := []conpty.ConPtyOption{conpty.ConPtyDimensions(p.Cols, p.Rows)}
	if p.Cwd != "" {
		opts = append(opts, conpty.ConPtyWorkDir(p.Cwd))
	}
	c, err := conpty.Start(WindowsCommandLine(p.Shell), opts...)
	if err != nil {
		return nil, rpcutil.Failed("start shell: %v", err)
	}
	t := &winTerminal{c: c, done: make(chan struct{})}
	go func() {
		_, t.err = c.Wait(context.Background())
		close(t.done)
	}()
	return t, nil
}

func (t *winTerminal) Read(b []byte) (int, error)  { return t.c.Read(b) }
func (t *winTerminal) Write(b []byte) (int, error) { return t.c.Write(b) }
func (t *winTerminal) Resize(cols, rows int) error { return t.c.Resize(cols, rows) }

func (t *winTerminal) Wait() error {
	<-t.done
	return t.err
}

// Kill ends the shell's process tree, then closes the pseudo console.
func (t *winTerminal) Kill() {
	t.once.Do(func() {
		select {
		case <-t.done:
		default:
			pid := t.c.Pid()
			_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
			if p, err := os.FindProcess(pid); err == nil {
				_ = p.Kill()
			}
		}
		_ = t.c.Close()
	})
}
