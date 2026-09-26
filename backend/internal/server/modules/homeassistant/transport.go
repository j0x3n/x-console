package homeassistant

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// maxMessage bounds one WebSocket message or HTTP body from HA. get_states on
// a large installation is a few MB.
const maxMessage = 64 << 20

// transport reaches HA: directly from the server, or through a paired agent
// on the home network (agent.go).
type transport interface {
	dial(ctx context.Context, wsURL string) (msgConn, error)
	do(ctx context.Context, method, url string, header http.Header, body []byte) (int, []byte, error)
}

// direct talks to HA from the server process.
type direct struct{ client *http.Client }

func newDirect() *direct { return &direct{client: &http.Client{Timeout: 15 * time.Second}} }

func (d *direct) dial(ctx context.Context, u string) (msgConn, error) {
	c, _, err := websocket.Dial(ctx, u, nil)
	if err != nil {
		return nil, err
	}
	c.SetReadLimit(maxMessage)
	return &wsConn{c: c}, nil
}

func (d *direct) do(ctx context.Context, method, u string, header http.Header, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxMessage+1))
	if err != nil {
		return 0, nil, err
	}
	if len(raw) > maxMessage {
		return 0, nil, fmt.Errorf("response too large")
	}
	return resp.StatusCode, raw, nil
}

type wsConn struct{ c *websocket.Conn }

func (w *wsConn) Read(ctx context.Context) ([]byte, error) {
	_, b, err := w.c.Read(ctx)
	return b, err
}

func (w *wsConn) Write(ctx context.Context, msg []byte) error {
	return w.c.Write(ctx, websocket.MessageText, msg)
}

func (w *wsConn) Close() error { return w.c.Close(websocket.StatusNormalClosure, "") }
