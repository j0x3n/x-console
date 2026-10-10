package screentime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

func at(min, sec int) time.Time {
	return time.Date(2026, 10, 10, 9, min, sec, 0, time.UTC)
}

func active(app, title string) Reading {
	return Reading{Active: true, OK: true, Window: Window{App: app, Title: title}}
}

func TestAggregatorPicksTheProgramSeenMost(t *testing.T) {
	var a Aggregator
	for i, r := range []Reading{active("Code.exe", "a.go"), active("chrome.exe", "Docs"), active("chrome.exe", "Docs 2"), active("chrome.exe", "Docs 3")} {
		if _, ok := a.Add(at(0, i*15), r); ok {
			t.Fatalf("reading %d finished a minute too early", i)
		}
	}
	s, ok := a.Add(at(1, 0), active("Code.exe", "b.go"))
	if !ok {
		t.Fatal("the minute should be finished")
	}
	if s.App != "chrome.exe" || s.Title != "Docs 3" || s.Minute != at(0, 0).Unix()/60 {
		t.Fatalf("sample: %+v", s)
	}
}

func TestAggregatorTieGoesToTheFirstProgram(t *testing.T) {
	var a Aggregator
	a.Add(at(0, 0), active("Code.exe", ""))
	a.Add(at(0, 15), active("chrome.exe", ""))
	s, ok := a.Add(at(1, 0), Reading{})
	if !ok || s.App != "Code.exe" {
		t.Fatalf("tie: %+v %v", s, ok)
	}
}

func TestAggregatorSkipsMinutesWithoutAnActiveUser(t *testing.T) {
	var a Aggregator
	// 锁屏、走开、取不到窗口都不产生记录
	for i, r := range []Reading{{}, {Active: true}, active("", "x")} {
		if _, ok := a.Add(at(0, i*15), r); ok {
			t.Fatal("nothing to finish yet")
		}
	}
	if s, ok := a.Add(at(1, 0), active("Code.exe", "")); ok {
		t.Fatalf("a minute without an active user must not report: %+v", s)
	}
	s, ok := a.Add(at(2, 0), Reading{})
	if !ok || s.App != "Code.exe" || s.Minute != at(1, 0).Unix()/60 {
		t.Fatalf("next minute: %+v %v", s, ok)
	}
	if _, ok := a.Add(at(3, 0), Reading{}); ok {
		t.Fatal("an empty minute reports nothing")
	}
}

func TestAggregatorClipsLongText(t *testing.T) {
	var a Aggregator
	a.Add(at(0, 0), active(strings.Repeat("a", 500)+".exe", strings.Repeat("长", 1000)))
	s, _ := a.Add(at(1, 0), Reading{})
	if len([]rune(s.App)) != maxApp || len([]rune(s.Title)) != maxTitle {
		t.Fatalf("lengths: %d %d", len([]rune(s.App)), len([]rune(s.Title)))
	}
}

func TestRegisterEmitsOneSamplePerMinute(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	samples := make(chan protocol.ScreenSample, 4)
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
			var s protocol.ScreenSample
			if method == protocol.EventScreenSample && json.Unmarshal(raw, &s) == nil {
				samples <- s
			}
		})
		_ = peer.Run(ctx)
	}))
	defer srv.Close()

	// 每次读取让时钟走 15 秒，所以每 4 次读取过一分钟
	clock := at(0, 0)
	read := func(context.Context) Reading { return active("Code.exe", "main.go") }
	now := func() time.Time { t := clock; clock = clock.Add(15 * time.Second); return t }
	client := conn.New(srv.URL, "test", protocol.Hello{})
	register(client, read, now, 5*time.Millisecond)
	done := make(chan struct{})
	go func() { defer close(done); _ = client.Run(ctx) }()
	defer func() { cancel(); <-done }()

	select {
	case s := <-samples:
		if s.App != "Code.exe" || s.Title != "main.go" {
			t.Fatalf("sample: %+v", s)
		}
	case <-ctx.Done():
		t.Fatal("no sample")
	}
}
