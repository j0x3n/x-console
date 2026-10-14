package watchers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/browser"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/api"
)

// OpenWatcherLive implements api.ServerInterface: the takeover view (B205).
// The user sees the page and clicks and types into it, for example to solve
// a captcha. Frames go out as binary messages, each followed by a text
// message with the page size; input comes in as text messages.
func (m *Module) OpenWatcherLive(w http.ResponseWriter, r *http.Request, id api.WatcherId) {
	if fail(w, r, auth.RequireElevated(r.Context())) {
		return
	}
	if _, err := m.get(r.Context(), id); fail(w, r, err) {
		return
	}
	sess := m.browser.Live(sessionKey(id))
	if sess == nil {
		fail(w, r, httpx.NewError(http.StatusConflict, "not_open", "这个任务现在没有打开的浏览器"))
		return
	}
	m.d.Audit.Record(r.Context(), "watcher.live", strconv.FormatInt(id, 10), nil, nil)
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(64 << 10)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	// Writes use their own context: coder/websocket closes the connection
	// when a write context is cancelled.
	write := func(typ websocket.MessageType, b []byte) error {
		wctx, wcancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer wcancel()
		return ws.Write(wctx, typ, b)
	}
	frames := make(chan browser.Frame, 2)
	go func() {
		_ = sess.Screencast(ctx, func(f browser.Frame) {
			select {
			case frames <- f:
			default: // the viewer is slow; drop this picture
			}
		})
		cancel()
	}()
	go func() {
		defer cancel()
		for {
			_, data, err := ws.Read(ctx)
			if err != nil {
				return
			}
			var in browser.Input
			if json.Unmarshal(data, &in) != nil {
				continue
			}
			if err := sess.Send(ctx, in); err != nil {
				m.d.Log.Debug("watcher live input", "err", err)
			}
		}
	}()
	for {
		select {
		case f := <-frames:
			if write(websocket.MessageBinary, f.JPEG) != nil {
				return
			}
			meta, _ := json.Marshal(map[string]any{"type": "frame", "width": f.Width, "height": f.Height})
			if write(websocket.MessageText, meta) != nil {
				return
			}
		case <-sess.Done():
			_ = write(websocket.MessageText, []byte(`{"type":"closed"}`))
			_ = ws.Close(websocket.StatusNormalClosure, "")
			return
		case <-ctx.Done():
			_ = ws.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}
