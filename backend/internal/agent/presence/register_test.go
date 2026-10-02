package presence

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

func TestRegisterRPCAndStateReports(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	connected := make(chan *rpc.Peer, 1)
	reports := make(chan Sample, 8)
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
		peer.OnEvent(func(method string, raw json.RawMessage) {
			if method != "presence.update" {
				return
			}
			var sample Sample
			if json.Unmarshal(raw, &sample) == nil {
				reports <- sample
			}
		})
		connected <- peer
		_ = peer.Run(ctx)
	}))
	defer srv.Close()
	var locked atomic.Bool
	client := conn.New(srv.URL, "test", protocol.Hello{})
	register(client, func(context.Context) Sample { return Sample{Known: true, IdleSeconds: 0, Locked: new(locked.Load())} })
	done := make(chan struct{})
	go func() { defer close(done); _ = client.Run(ctx) }()
	defer func() { cancel(); <-done }()
	var peer *rpc.Peer
	select {
	case peer = <-connected:
	case <-ctx.Done():
		t.Fatal("not connected")
	}
	var sample Sample
	if err := peer.Call(ctx, "presence.get", nil, &sample); err != nil || !sample.Known || sample.Locked == nil || *sample.Locked {
		t.Fatalf("rpc sample: %+v %v", sample, err)
	}
	select {
	case sample = <-reports:
		if !sample.Known || sample.Locked == nil || *sample.Locked {
			t.Fatalf("initial report: %+v", sample)
		}
	case <-ctx.Done():
		t.Fatal("no initial report")
	}
	locked.Store(true)
	select {
	case sample = <-reports:
		if sample.Locked == nil || !*sample.Locked {
			t.Fatalf("lock report: %+v", sample)
		}
	case <-ctx.Done():
		t.Fatal("no changed report")
	}
}
