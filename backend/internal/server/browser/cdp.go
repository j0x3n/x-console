package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
)

// conn is a Chrome DevTools Protocol connection to the browser endpoint. It
// uses flat sessions: a call to a page carries that page's sessionId.
type conn struct {
	ws     *websocket.Conn
	nextID atomic.Int64

	mu      sync.Mutex
	pending map[int64]chan reply
	subs    map[string][]chan event // key: sessionID + "|" + method
	closed  chan struct{}
	err     error
}

type reply struct {
	Result json.RawMessage
	Err    *cdpError
}

type event struct {
	Method string
	Params json.RawMessage
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *cdpError) Error() string {
	return fmt.Sprintf("浏览器返回错误 %d：%s", e.Code, e.Message)
}

// errClosed is returned by calls on a closed connection.
var errClosed = errors.New("浏览器连接已断开")

func dial(ctx context.Context, wsURL string) (*conn, error) {
	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	// Screenshots and screencast frames come as base64 in one message.
	ws.SetReadLimit(64 << 20)
	c := &conn{ws: ws, pending: map[int64]chan reply{}, subs: map[string][]chan event{}, closed: make(chan struct{})}
	go c.readLoop()
	return c, nil
}

func (c *conn) readLoop() {
	var err error
	defer func() { c.shutdown(err) }()
	for {
		var data []byte
		// Never a cancellable context here: coder/websocket closes the whole
		// connection when the read context ends.
		_, data, err = c.ws.Read(context.Background())
		if err != nil {
			return
		}
		var msg struct {
			ID        int64           `json:"id"`
			Method    string          `json:"method"`
			Params    json.RawMessage `json:"params"`
			Result    json.RawMessage `json:"result"`
			Error     *cdpError       `json:"error"`
			SessionID string          `json:"sessionId"`
		}
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		if msg.ID != 0 {
			c.mu.Lock()
			ch := c.pending[msg.ID]
			delete(c.pending, msg.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- reply{Result: msg.Result, Err: msg.Error}
			}
			continue
		}
		c.mu.Lock()
		for _, ch := range c.subs[msg.SessionID+"|"+msg.Method] {
			select {
			case ch <- event{Method: msg.Method, Params: msg.Params}:
			default: // a slow reader loses events, it never blocks the loop
			}
		}
		for _, ch := range c.subs[msg.SessionID+"|*"] {
			select {
			case ch <- event{Method: msg.Method, Params: msg.Params}:
			default:
			}
		}
		c.mu.Unlock()
	}
}

func (c *conn) shutdown(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.closed:
		return
	default:
	}
	if err == nil {
		err = errClosed
	}
	c.err = err
	close(c.closed)
	for id, ch := range c.pending {
		ch <- reply{Err: &cdpError{Code: -1, Message: errClosed.Error()}}
		delete(c.pending, id)
	}
}

func (c *conn) close() {
	_ = c.ws.Close(websocket.StatusNormalClosure, "")
	c.shutdown(errClosed)
}

func (c *conn) alive() bool {
	select {
	case <-c.closed:
		return false
	default:
		return true
	}
}

// call sends method to the browser (sessionID "") or to a page session and
// decodes the result into out when out is not nil.
func (c *conn) call(ctx context.Context, sessionID, method string, params, out any) error {
	if !c.alive() {
		return errClosed
	}
	id := c.nextID.Add(1)
	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if sessionID != "" {
		msg["sessionId"] = sessionID
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	ch := make(chan reply, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	// The write must not use a context that may be cancelled mid-write, or
	// coder/websocket closes the connection. Calls are short, so the
	// cancellation is checked while waiting for the reply instead.
	if err := c.ws.Write(context.Background(), websocket.MessageText, data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return err
	}
	select {
	case r := <-ch:
		if r.Err != nil {
			return r.Err
		}
		if out != nil && len(r.Result) > 0 {
			return json.Unmarshal(r.Result, out)
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	}
}

// subscribe returns a channel of events with this method ("*" for all) from
// sessionID, and a func that ends the subscription.
func (c *conn) subscribe(sessionID, method string, size int) (<-chan event, func()) {
	ch := make(chan event, size)
	key := sessionID + "|" + method
	c.mu.Lock()
	c.subs[key] = append(c.subs[key], ch)
	c.mu.Unlock()
	return ch, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		list := c.subs[key]
		for i, x := range list {
			if x == ch {
				c.subs[key] = append(list[:i:i], list[i+1:]...)
				break
			}
		}
		if len(c.subs[key]) == 0 {
			delete(c.subs, key)
		}
	}
}
