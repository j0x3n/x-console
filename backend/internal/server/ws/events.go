// Package ws serves the browser event stream (GET /api/v1/events).
// Browser connections subscribe to high-volume events. Small events are
// forwarded by default so alarms and notifications remain real-time.
package ws

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// ServeHTTP handles one authenticated browser connection.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")
	c.SetReadLimit(4096)
	ctx, stop := context.WithCancel(context.WithoutCancel(r.Context()))
	defer stop()
	ch, cancel := h.bus.Subscribe("", 256)
	defer cancel()
	commands := make(chan command, 8)
	go func() {
		defer stop()
		for {
			var msg command
			if wsjson.Read(ctx, c, &msg) != nil {
				return
			}
			select {
			case commands <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	connID := h.newConn()
	topics := map[string]bool{}
	asked := map[string]int{} // host -> interval this browser asked for
	paused := false
	// publishWants tells the handler which intervals this connection wants
	// right now: only for hosts it watches, and not while it is paused.
	publishWants := func() {
		next := map[string]int{}
		if !paused {
			for host, ms := range asked {
				if topics["host.metrics:"+host] {
					next[host] = ms
				}
			}
		}
		h.setWants(connID, next)
	}
	defer func() {
		h.setWants(connID, nil)
		if !paused {
			h.updateDetail(topics, nil)
		}
	}()
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-commands:
			switch msg.Type {
			case "subscribe":
				prev := topics
				next := validTopics(msg.Topics)
				topics = next
				// The interval goes in first, so the agent's first call has it.
				publishWants()
				if !paused {
					h.updateDetail(prev, next)
				}
			case "interval":
				if msg.HostID == "" || len(msg.HostID) > 128 {
					break
				}
				if validInterval(msg.Ms) {
					if len(asked) < 64 || asked[msg.HostID] > 0 {
						asked[msg.HostID] = msg.Ms
					}
				} else {
					delete(asked, msg.HostID) // 0 and unknown values mean "no longer asking"
				}
				publishWants()
			case "pause":
				if !paused {
					paused = true
					publishWants()
					h.updateDetail(topics, nil)
				}
			case "resume":
				if paused {
					paused = false
					publishWants()
					h.updateDetail(nil, topics)
				}
			}
		case ev := <-ch:
			if ev.Topic == "agent.online" && !paused && subscribedAgent(topics, ev.Data) {
				// The agent lost its state when it reconnected: tell it again.
				h.forget(agentID(ev.Data))
				h.setDetail(agentID(ev.Data), true)
			}
			if !wanted(ev, topics, paused) {
				continue
			}
			wctx, done := context.WithTimeout(ctx, 10*time.Second)
			err := wsjson.Write(wctx, c, ev)
			done()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, done := context.WithTimeout(ctx, 10*time.Second)
			err := c.Ping(pctx)
			done()
			if err != nil {
				return
			}
		}
	}
}
