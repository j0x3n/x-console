// Package ws serves the browser event stream (GET /api/v1/events).
// Every events.Bus event is forwarded as {"topic","data","at"}; the frontend
// uses the topic to invalidate TanStack Query caches.
package ws

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/j0x3n/x-console/backend/internal/server/events"
)

// Events returns the handler. Authentication is done by the auth middleware;
// websocket.Accept rejects cross-origin requests.
func Events(bus *events.Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		ctx := c.CloseRead(context.WithoutCancel(r.Context()))
		ch, cancel := bus.Subscribe("", 256)
		defer cancel()
		ping := time.NewTicker(30 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-ctx.Done():
				c.Close(websocket.StatusNormalClosure, "")
				return
			case ev := <-ch:
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
}
