// Package exec runs one shell command and waits for it (exec.run).
package exec

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	osexec "os/exec"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	defaultTimeout = 60 * time.Second
	maxTimeout     = 30 * time.Minute
)

// Register adds exec.run.
func Register(c *conn.Client) {
	c.Handle(protocol.MethodExecRun, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.ExecParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return Run(ctx, p)
	})
}

// Timeout returns the effective timeout of a request.
func Timeout(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultTimeout
	}
	return min(time.Duration(seconds)*time.Second, maxTimeout)
}

// Run executes p.Command through the system shell. Output beyond
// protocol.ExecMaxOutput per stream is dropped and Truncated is set.
func Run(ctx context.Context, p protocol.ExecParams) (protocol.ExecResult, error) {
	if strings.TrimSpace(p.Command) == "" {
		return protocol.ExecResult{}, rpcutil.BadParams("command is required")
	}
	if p.Cwd != "" {
		if st, err := os.Stat(p.Cwd); err != nil || !st.IsDir() {
			return protocol.ExecResult{}, rpcutil.BadParams("cwd is not a directory")
		}
	}
	timeout := Timeout(p.TimeoutSeconds)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	name, args := shellCommand(p.Command)
	cmd := osexec.Command(name, args...)
	cmd.Dir = p.Cwd
	stdout, stderr := &capped{max: protocol.ExecMaxOutput}, &capped{max: protocol.ExecMaxOutput}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	// Background jobs ("server &") keep the output pipes open; stop waiting
	// for them shortly after the shell itself exited.
	cmd.WaitDelay = 3 * time.Second
	prepare(cmd)
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return protocol.ExecResult{}, rpcutil.Failed("start: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	timedOut := false
	select {
	case err = <-done:
	case <-runCtx.Done():
		timedOut = errors.Is(runCtx.Err(), context.DeadlineExceeded)
		killTree(cmd)
		select {
		case err = <-done:
		case <-time.After(5 * time.Second):
			err = runCtx.Err()
		}
	}
	res := protocol.ExecResult{
		ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String(), TimedOut: timedOut,
		Truncated: stdout.truncated || stderr.truncated, DurationMs: time.Since(start).Milliseconds(),
	}
	if err != nil {
		res.ExitCode = -1
		var exitErr *osexec.ExitError
		if errors.As(err, &exitErr) && exitErr.Exited() {
			res.ExitCode = exitErr.ExitCode()
		} else if errors.Is(err, osexec.ErrWaitDelay) && cmd.ProcessState != nil {
			res.ExitCode = cmd.ProcessState.ExitCode()
		}
	}
	if ctx.Err() != nil && !timedOut {
		return res, ctx.Err() // the server canceled the request
	}
	return res, nil
}

// capped is a buffer that keeps the first max bytes.
type capped struct {
	mu        sync.Mutex
	buf       []byte
	max       int
	truncated bool
}

func (c *capped) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	room := c.max - len(c.buf)
	if room < len(p) {
		c.truncated = true
		if room > 0 {
			c.buf = append(c.buf, p[:room]...)
		}
		return len(p), nil
	}
	c.buf = append(c.buf, p...)
	return len(p), nil
}

func (c *capped) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.ToValidUTF8(string(c.buf), "�")
}
