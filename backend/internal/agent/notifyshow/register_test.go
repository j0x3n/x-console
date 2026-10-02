package notifyshow

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

func TestRegisterNotificationRPC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connected := make(chan *rpc.Peer, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		transport := rpc.WebSocket(ws)
		if _, err := transport.Read(ctx); err != nil {
			return
		}
		if err := transport.Write(ctx, protocol.Envelope{Kind: protocol.KindWelcome, Params: protocol.Marshal(protocol.Welcome{ProtocolVersion: protocol.Version, AgentID: "test"})}); err != nil {
			return
		}
		peer := rpc.NewPeer(transport, "s")
		connected <- peer
		_ = peer.Run(ctx)
	}))
	defer srv.Close()
	client := conn.New(srv.URL, "test", protocol.Hello{})
	shown := make(chan Input, 1)
	register(client, func(_ context.Context, input Input) error { shown <- input; return nil })
	done := make(chan struct{})
	go func() { defer close(done); _ = client.Run(ctx) }()
	defer func() { cancel(); <-done }()
	var peer *rpc.Peer
	select {
	case peer = <-connected:
	case <-ctx.Done():
		t.Fatal("not connected")
	}
	input := Input{Title: "护眼", Body: "看向 6 米外，休息 20 秒。"}
	var out struct{ Shown bool }
	if err := peer.Call(ctx, "notify.show", input, &out); err != nil || !out.Shown {
		t.Fatalf("notification RPC: %+v %v", out, err)
	}
	select {
	case got := <-shown:
		if got != input {
			t.Fatalf("shown: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("not shown")
	}
	var remote *protocol.Error
	if err := peer.Call(ctx, "notify.show", Input{}, nil); !errors.As(err, &remote) || remote.Code != protocol.CodeBadParams {
		t.Fatalf("invalid notification: %v", err)
	}
}
