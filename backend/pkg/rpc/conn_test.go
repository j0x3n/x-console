package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// A canceled context on one write must not close the shared connection.
func TestWebSocketCanceledWriteKeepsConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	serverPeer := make(chan *Peer, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		p := NewPeer(WebSocket(c), "s")
		p.Handle("echo", func(ctx context.Context, raw json.RawMessage) (any, error) { return string(raw), nil })
		serverPeer <- p
		_ = p.Run(context.WithoutCancel(r.Context()))
	}))
	defer srv.Close()
	c, _, err := websocket.Dial(ctx, strings.Replace(srv.URL, "http://", "ws://", 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	client := NewPeer(WebSocket(c), "a")
	go client.Run(ctx)
	<-serverPeer

	canceled, stop := context.WithCancel(ctx)
	stop()
	if err := client.Emit(canceled, "x", nil); err == nil {
		t.Fatal("want error for canceled context")
	}
	var out string
	if err := client.Call(ctx, "echo", 1, &out); err != nil || out != "1" {
		t.Fatalf("connection unusable after canceled write: %q %v", out, err)
	}
}
