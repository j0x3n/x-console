package rpc

import (
	"context"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// WebSocket adapts a websocket connection.
func WebSocket(c *websocket.Conn) Conn { return &wsConn{c: c} }

type wsConn struct{ c *websocket.Conn }

func (w *wsConn) Read(ctx context.Context) (protocol.Envelope, error) {
	var env protocol.Envelope
	err := wsjson.Read(ctx, w.c, &env)
	return env, err
}

// writeTimeout bounds one frame write. A write that takes longer means the
// connection is stuck, and closing it is the right outcome.
const writeTimeout = 30 * time.Second

// Write sends one frame. coder/websocket closes the whole connection when the
// context of an in-flight write is canceled, so a canceled request or a
// finished stream must never cancel the write itself: we check ctx first and
// then write with a context that only carries the timeout.
func (w *wsConn) Write(ctx context.Context, env protocol.Envelope) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()
	return wsjson.Write(wctx, w.c, env)
}

func (w *wsConn) Close() error { return w.c.Close(websocket.StatusNormalClosure, "") }

// Pipe returns two connected in-memory Conns for tests.
func Pipe() (Conn, Conn) {
	ab := make(chan protocol.Envelope, 1024)
	ba := make(chan protocol.Envelope, 1024)
	done := make(chan struct{})
	var once sync.Once
	closeFn := func() error { once.Do(func() { close(done) }); return nil }
	return &pipeConn{in: ba, out: ab, done: done, close: closeFn},
		&pipeConn{in: ab, out: ba, done: done, close: closeFn}
}

type pipeConn struct {
	in, out chan protocol.Envelope
	done    chan struct{}
	close   func() error
}

func (p *pipeConn) Read(ctx context.Context) (protocol.Envelope, error) {
	select {
	case env := <-p.in:
		return env, nil
	case <-p.done:
		return protocol.Envelope{}, ErrClosed
	case <-ctx.Done():
		return protocol.Envelope{}, ctx.Err()
	}
}

func (p *pipeConn) Write(ctx context.Context, env protocol.Envelope) error {
	select {
	case p.out <- env:
		return nil
	case <-p.done:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *pipeConn) Close() error { return p.close() }
