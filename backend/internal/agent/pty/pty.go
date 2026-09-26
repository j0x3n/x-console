// Package pty serves interactive terminals (pty.open). Linux uses
// creack/pty, Windows uses ConPTY. Every stream chunk starts with a type
// byte, see protocol.PTYFrameData and protocol.PTYFrameResize.
package pty

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// Available reports whether terminals work on this system.
func Available() bool { return available() }

// Register adds the pty.open stream.
func Register(c *conn.Client) {
	c.HandleStream(protocol.MethodPTYOpen, Serve)
}

// terminal is a running shell attached to a pseudo terminal.
type terminal interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Resize(cols, rows int) error
	// Wait blocks until the shell exits.
	Wait() error
	// Kill ends the shell and everything it started, then releases the pty.
	Kill()
}

// killGrace is how long a shell gets to exit after the hangup signal.
var killGrace = 2 * time.Second

// Serve runs one terminal for the lifetime of the stream. When the stream
// ends (the browser closed the page) the shell is killed; when the shell
// exits the stream ends.
func Serve(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
	var p protocol.PTYOpenParams
	if err := rpcutil.Decode(raw, &p); err != nil {
		return err
	}
	p.Cols, p.Rows = ClampSize(p.Cols, p.Rows)
	if p.Cwd != "" {
		if st, err := os.Stat(p.Cwd); err != nil || !st.IsDir() {
			return rpcutil.BadParams("cwd is not a directory")
		}
	} else if home, err := os.UserHomeDir(); err == nil {
		p.Cwd = home
	}
	t, err := start(p)
	if err != nil {
		return err
	}
	var once sync.Once
	kill := func() { once.Do(t.Kill) }
	defer kill()

	exited := make(chan struct{})
	go func() {
		defer close(exited)
		buf := make([]byte, 32<<10)
		for {
			n, err := t.Read(buf)
			if n > 0 {
				frame := make([]byte, n+1)
				frame[0] = protocol.PTYFrameData
				copy(frame[1:], buf[:n])
				if s.Send(ctx, frame) != nil {
					kill()
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		for {
			chunk, err := s.Recv(ctx)
			if err != nil {
				kill()
				return
			}
			kind, data, size, ok := ParseFrame(chunk)
			switch {
			case !ok:
			case kind == protocol.PTYFrameData:
				if _, err := t.Write(data); err != nil {
					kill()
					return
				}
			case kind == protocol.PTYFrameResize:
				c, r := ClampSize(size.Cols, size.Rows)
				if err := t.Resize(c, r); err != nil {
					slog.Debug("pty resize failed", "err", err)
				}
			}
		}
	}()

	select {
	case <-exited:
		_ = t.Wait()
		return nil
	case <-ctx.Done():
		kill()
		return nil
	}
}

// ParseFrame splits a stream chunk. ok is false for empty or unknown frames.
func ParseFrame(chunk []byte) (kind byte, data []byte, size protocol.PTYResize, ok bool) {
	if len(chunk) == 0 {
		return 0, nil, size, false
	}
	switch chunk[0] {
	case protocol.PTYFrameData:
		return chunk[0], chunk[1:], size, true
	case protocol.PTYFrameResize:
		if err := json.Unmarshal(chunk[1:], &size); err != nil {
			return 0, nil, size, false
		}
		return chunk[0], nil, size, true
	}
	return 0, nil, size, false
}

// ResizeFrame builds a PTYFrameResize chunk.
func ResizeFrame(cols, rows int) []byte {
	raw, _ := json.Marshal(protocol.PTYResize{Cols: cols, Rows: rows})
	return append([]byte{protocol.PTYFrameResize}, raw...)
}

// DataFrame builds a PTYFrameData chunk.
func DataFrame(b []byte) []byte {
	return append([]byte{protocol.PTYFrameData}, b...)
}

// ClampSize keeps the window size in a sane range; zero means 80x24.
func ClampSize(cols, rows int) (int, int) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return min(cols, 1000), min(rows, 500)
}

// WindowsCommandLine is the ConPTY command line for shell. Empty means
// PowerShell. Names without a path or extension get ".exe".
func WindowsCommandLine(shell string) string {
	shell = strings.TrimSpace(shell)
	switch strings.ToLower(shell) {
	case "", "powershell", "powershell.exe":
		return "powershell.exe -NoLogo"
	case "pwsh", "pwsh.exe":
		return "pwsh.exe -NoLogo"
	case "cmd", "cmd.exe":
		return "cmd.exe"
	}
	if strings.ContainsAny(shell, " \t") && !strings.HasPrefix(shell, `"`) {
		return `"` + shell + `"`
	}
	return shell
}

var errNoShell = errors.New("no shell found")
