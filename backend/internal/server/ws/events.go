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
	topics := map[string]bool{}
	paused := false
	defer func() {
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
				next := validTopics(msg.Topics)
				if !paused {
					h.updateDetail(topics, next)
				}
				topics = next
			case "pause":
				if !paused {
					h.updateDetail(topics, nil)
					paused = true
				}
			case "resume":
				if paused {
					paused = false
					h.updateDetail(nil, topics)
				}
			}
		case ev := <-ch:
			if ev.Topic == "agent.online" && !paused && subscribedAgent(topics, ev.Data) {
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
