package hosts

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/coder/websocket"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// termSession is a running terminal on an agent or over SSH.
type termSession interface {
	// Input writes keystrokes.
	Input(ctx context.Context, b []byte) error
	// Resize changes the window size.
	Resize(ctx context.Context, cols, rows int) error
	// Output returns the next chunk of terminal output, io.EOF at the end.
	Output(ctx context.Context) ([]byte, error)
	// Close ends the session; the shell is killed.
	Close()
}

// controlMessage is a text frame from the browser.
type controlMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func clampSize(cols, rows *int) (int, int) {
	c, r := 80, 24
	if cols != nil && *cols > 0 {
		c = min(*cols, 1000)
	}
	if rows != nil && *rows > 0 {
		r = min(*rows, 500)
	}
	return c, r
}

// OpenTerminal is GET /hosts/{hostId}/terminal, a WebSocket. Elevation and
// the host are checked before the upgrade so failures are plain JSON errors.
func (m *Module) OpenTerminal(w http.ResponseWriter, r *http.Request, hostID string, params api.OpenTerminalParams) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cols, rows := clampSize(params.Cols, params.Rows)
	h, err := m.host(ctx, hostID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// The session outlives the elevation window and the handler's request
	// context bookkeeping, so it gets its own context.
	sessCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	var term termSession
	if h.ssh != nil {
		term, err = m.sshTerminal(sessCtx, *h.ssh, cols, rows)
	} else {
		term, err = m.agentTerminal(sessCtx, hostID, cols, rows)
	}
	m.d.Audit.Record(ctx, "host.terminal", hostID, map[string]any{"cols": cols, "rows": rows}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer term.Close()
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(1 << 20)
	bridge(sessCtx, ws, term)
}

// bridge copies between the browser and the terminal until either ends.
func bridge(ctx context.Context, ws *websocket.Conn, term termSession) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 2)
	go func() {
		for {
			b, err := term.Output(ctx)
			if err != nil {
				done <- err
				return
			}
			if err := ws.Write(ctx, websocket.MessageBinary, b); err != nil {
				done <- nil
				return
			}
		}
	}()
	go func() {
		for {
			typ, data, err := ws.Read(ctx)
			if err != nil {
				done <- nil
				return
			}
			if typ == websocket.MessageBinary {
				err = term.Input(ctx, data)
			} else {
				var msg controlMessage
				if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" {
					c, r := clampSize(&msg.Cols, &msg.Rows)
					err = term.Resize(ctx, c, r)
				}
			}
			if err != nil {
				done <- err
				return
			}
		}
	}()
	err := <-done
	term.Close()
	if err == nil || errors.Is(err, io.EOF) {
		_ = ws.Close(websocket.StatusNormalClosure, "terminal closed")
		return
	}
	reason := err.Error()
	if len(reason) > 120 {
		reason = reason[:120]
	}
	_ = ws.Close(websocket.StatusInternalError, reason)
}

// agentTerm speaks the pty.open stream framing.
type agentTerm struct{ s *rpc.Stream }

func (m *Module) agentTerminal(ctx context.Context, hostID string, cols, rows int) (termSession, error) {
	a, err := m.agentFor(ctx, hostID, protocol.CapPTY)
	if err != nil {
		return nil, err
	}
	s, err := m.d.Agents.Open(ctx, a.ID, protocol.MethodPTYOpen, protocol.PTYOpenParams{Cols: cols, Rows: rows})
	if err != nil {
		return nil, agentErr(err)
	}
	return &agentTerm{s: s}, nil
}

func (t *agentTerm) Input(ctx context.Context, b []byte) error {
	return t.s.Send(ctx, append([]byte{protocol.PTYFrameData}, b...))
}

func (t *agentTerm) Resize(ctx context.Context, cols, rows int) error {
	raw, _ := json.Marshal(protocol.PTYResize{Cols: cols, Rows: rows})
	return t.s.Send(ctx, append([]byte{protocol.PTYFrameResize}, raw...))
}

func (t *agentTerm) Output(ctx context.Context) ([]byte, error) {
	for {
		b, err := t.s.Recv(ctx)
		if err != nil {
			return nil, err
		}
		if len(b) > 1 && b[0] == protocol.PTYFrameData {
			return b[1:], nil
		}
	}
}

func (t *agentTerm) Close() { t.s.Close(nil) }
