package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func pair(t *testing.T) (server, agent *Peer, ctx context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	a, b := Pipe()
	server = NewPeer(a, "s")
	agent = NewPeer(b, "a")
	return server, agent, ctx
}

func TestCallAndError(t *testing.T) {
	server, agent, ctx := pair(t)
	agent.Handle("echo", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in map[string]string
		_ = json.Unmarshal(p, &in)
		return in, nil
	})
	agent.Handle("fail", func(ctx context.Context, p json.RawMessage) (any, error) {
		return nil, errors.New("boom")
	})
	go server.Run(ctx)
	go agent.Run(ctx)

	var out map[string]string
	if err := server.Call(ctx, "echo", map[string]string{"a": "b"}, &out); err != nil || out["a"] != "b" {
		t.Fatalf("echo: %v %v", out, err)
	}
	var pe *protocol.Error
	if err := server.Call(ctx, "fail", nil, nil); !errors.As(err, &pe) || pe.Code != protocol.CodeFailed {
		t.Fatalf("fail: %v", err)
	}
	if err := server.Call(ctx, "missing", nil, nil); !errors.As(err, &pe) || pe.Code != protocol.CodeUnknownMethod {
		t.Fatalf("missing: %v", err)
	}
}

func TestCancelReachesHandler(t *testing.T) {
	server, agent, ctx := pair(t)
	canceled := make(chan struct{})
	agent.Handle("wait", func(ctx context.Context, p json.RawMessage) (any, error) {
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	})
	go server.Run(ctx)
	go agent.Run(ctx)
	callCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if err := server.Call(callCtx, "wait", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline, got %v", err)
	}
	select {
	case <-canceled:
	case <-ctx.Done():
		t.Fatal("handler was not canceled")
	}
}

func TestStreamEcho(t *testing.T) {
	server, agent, ctx := pair(t)
	agent.HandleStream("upper", func(ctx context.Context, p json.RawMessage, s *Stream) error {
		for {
			b, err := s.Recv(ctx)
			if err != nil {
				return nil
			}
			if string(b) == "quit" {
				return errors.New("bye")
			}
			if err := s.Send(ctx, append([]byte("got:"), b...)); err != nil {
				return err
			}
		}
	})
	go server.Run(ctx)
	go agent.Run(ctx)

	s, err := server.Open(ctx, "upper", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Send(ctx, []byte("hi"))
	b, err := s.Recv(ctx)
	if err != nil || string(b) != "got:hi" {
		t.Fatalf("recv: %q %v", b, err)
	}
	_ = s.Send(ctx, []byte("quit"))
	_, err = s.Recv(ctx)
	var pe *protocol.Error
	if !errors.As(err, &pe) || pe.Message != "bye" {
		t.Fatalf("want remote error, got %v", err)
	}
}

func TestStreamLocalCloseAndShutdown(t *testing.T) {
	server, agent, ctx := pair(t)
	ended := make(chan struct{})
	agent.HandleStream("hold", func(ctx context.Context, p json.RawMessage, s *Stream) error {
		_, err := s.Recv(ctx)
		if err == io.EOF {
			close(ended)
		}
		return nil
	})
	go server.Run(ctx)
	go agent.Run(ctx)
	s, _ := server.Open(ctx, "hold", nil)
	s.Close(nil)
	select {
	case <-ended:
	case <-ctx.Done():
		t.Fatal("agent did not see end")
	}

	s2, _ := server.Open(ctx, "hold", nil)
	_ = agent.conn.Close()
	if _, err := s2.Recv(ctx); err == nil {
		t.Fatal("want error after shutdown")
	}
	if err := server.Call(ctx, "x", nil, nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("want ErrClosed, got %v", err)
	}
}

func TestEvents(t *testing.T) {
	server, agent, ctx := pair(t)
	got := make(chan string, 1)
	server.OnEvent(func(method string, p json.RawMessage) { got <- method + string(p) })
	go server.Run(ctx)
	go agent.Run(ctx)
	_ = agent.Emit(ctx, "metrics", map[string]int{"cpu": 5})
	select {
	case v := <-got:
		if v != `metrics{"cpu":5}` {
			t.Fatal(v)
		}
	case <-ctx.Done():
		t.Fatal("no event")
	}
}

// fragileConn closes itself when a write starts with an ended context, like
// the websocket transport does.
type fragileConn struct {
	Conn
	broken chan struct{}
}

func (f *fragileConn) Write(ctx context.Context, env protocol.Envelope) error {
	if ctx.Err() != nil {
		close(f.broken)
		_ = f.Conn.Close()
		return ctx.Err()
	}
	return f.Conn.Write(ctx, env)
}

func TestCanceledWriterKeepsConnection(t *testing.T) {
	a, b := Pipe()
	fa := &fragileConn{Conn: a, broken: make(chan struct{})}
	server, agent := NewPeer(fa, "s"), NewPeer(b, "a")
	agent.Handle("ping", func(ctx context.Context, _ json.RawMessage) (any, error) { return "pong", nil })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go server.Run(ctx)
	go agent.Run(ctx)

	dead, stop := context.WithCancel(ctx)
	stop()
	if err := server.Emit(dead, "x", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("emit with canceled ctx: %v", err)
	}
	var out string
	if err := server.Call(ctx, "ping", nil, &out); err != nil || out != "pong" {
		t.Fatalf("connection lost: %v", err)
	}
	select {
	case <-fa.broken:
		t.Fatal("transport saw a canceled write context")
	default:
	}
}
