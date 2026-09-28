// Package conn keeps the agent connected to the server and dispatches
// server requests to registered handlers. It reconnects with backoff.
package conn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// Client is the agent side of the connection.
type Client struct {
	Server string
	Token  string
	Hello  protocol.Hello

	mu             sync.Mutex
	handlers       map[string]rpc.Handler
	streamHandlers map[string]rpc.StreamHandler
	peer           *rpc.Peer
	onConnect      []func(ctx context.Context)
}

// New builds a client. hello.ProtocolVersion is filled in automatically.
func New(server, token string, hello protocol.Hello) *Client {
	hello.ProtocolVersion = protocol.Version
	return &Client{Server: strings.TrimRight(server, "/"), Token: token, Hello: hello,
		handlers: map[string]rpc.Handler{}, streamHandlers: map[string]rpc.StreamHandler{}}
}

// Handle registers a request handler for method.
func (c *Client) Handle(method string, h rpc.Handler) {
	c.mu.Lock()
	c.handlers[method] = h
	c.mu.Unlock()
}

// HandleStream registers a stream handler for method.
func (c *Client) HandleStream(method string, h rpc.StreamHandler) {
	c.mu.Lock()
	c.streamHandlers[method] = h
	c.mu.Unlock()
}

// OnConnect runs fn in a goroutine after every successful connection, with a
// context canceled on disconnect. Use it for periodic pushes such as metrics.
func (c *Client) OnConnect(fn func(ctx context.Context)) {
	c.mu.Lock()
	c.onConnect = append(c.onConnect, fn)
	c.mu.Unlock()
}

// Emit sends an event if connected. Events while offline are dropped.
func (c *Client) Emit(ctx context.Context, method string, params any) error {
	c.mu.Lock()
	p := c.peer
	c.mu.Unlock()
	if p == nil {
		return rpc.ErrClosed
	}
	return p.Emit(ctx, method, params)
}

// Run connects and reconnects until ctx ends.
func (c *Client) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		start := time.Now()
		err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var fatal *fatalError
		if errors.As(err, &fatal) {
			return fatal.err
		}
		if time.Since(start) > time.Minute {
			backoff = time.Second
		}
		slog.Warn("disconnected from server", "err", err, "retry_in", backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}
		backoff = min(backoff*2, time.Minute)
	}
}

type fatalError struct{ err error }

func (f *fatalError) Error() string { return f.err.Error() }
func (f *fatalError) Unwrap() error { return f.err }

// ErrRevoked means the server has rejected this agent token permanently.
var ErrRevoked = errors.New("server rejected the agent token; pair again")

func (c *Client) runOnce(ctx context.Context) error {
	u, err := wsURL(c.Server)
	if err != nil {
		return &fatalError{err}
	}
	dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	ws, resp, err := websocket.Dial(dialCtx, u, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + c.Token}},
	})
	cancel()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return &fatalError{ErrRevoked}
		}
		return err
	}
	ws.SetReadLimit(16 << 20)
	transport := rpc.WebSocket(ws)
	if err := transport.Write(ctx, protocol.Envelope{Kind: protocol.KindHello, Params: protocol.Marshal(c.Hello)}); err != nil {
		return err
	}
	welcomeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	env, err := transport.Read(welcomeCtx)
	cancel()
	if err != nil {
		var ce websocket.CloseError
		if errors.As(err, &ce) && ce.Code == websocket.StatusPolicyViolation {
			return &fatalError{fmt.Errorf("server refused connection: %s", ce.Reason)}
		}
		return err
	}
	if env.Kind != protocol.KindWelcome {
		return fmt.Errorf("expected welcome, got %q", env.Kind)
	}
	var welcome protocol.Welcome
	_ = json.Unmarshal(env.Params, &welcome)
	slog.Info("connected", "server", c.Server, "agent", welcome.AgentID, "name", welcome.Name)

	peer := rpc.NewPeer(transport, "a")
	c.mu.Lock()
	for m, h := range c.handlers {
		peer.Handle(m, h)
	}
	for m, h := range c.streamHandlers {
		peer.HandleStream(m, h)
	}
	c.peer = peer
	hooks := append([]func(context.Context){}, c.onConnect...)
	c.mu.Unlock()

	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	for _, fn := range hooks {
		go fn(runCtx)
	}
	err = peer.Run(runCtx)
	var closed websocket.CloseError
	if errors.As(err, &closed) && closed.Code == websocket.StatusPolicyViolation && closed.Reason == "revoked" {
		err = &fatalError{ErrRevoked}
	}
	c.mu.Lock()
	c.peer = nil
	c.mu.Unlock()
	return err
}

// Pair exchanges a pairing code for credentials.
func Pair(ctx context.Context, server, code string, hello protocol.Hello) (agentID, token string, err error) {
	body, _ := json.Marshal(map[string]any{
		"code": code, "hostname": hello.Hostname, "os": hello.OS, "arch": hello.Arch,
		"version": hello.AgentVersion, "capabilities": hello.Capabilities,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(server, "/")+"/api/v1/agent/pair", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var out struct {
		AgentID string `json:"agentId"`
		Token   string `json:"token"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("pairing failed (%d): %s", resp.StatusCode, out.Message)
	}
	return out.AgentID, out.Token, nil
}

func wsURL(server string) (string, error) {
	u, err := url.Parse(server)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return "", fmt.Errorf("server URL must start with http:// or https://")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/agent/connect"
	return u.String(), nil
}
