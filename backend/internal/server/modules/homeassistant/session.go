package homeassistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// This file speaks the Home Assistant WebSocket API (/api/websocket):
//
//	HA:     {"type":"auth_required","ha_version":"2026.9.1"}
//	client: {"type":"auth","access_token":"..."}
//	HA:     {"type":"auth_ok","ha_version":"2026.9.1"} or {"type":"auth_invalid","message":"..."}
//	client: {"id":1,"type":"subscribe_events","event_type":"state_changed"}
//	HA:     {"id":1,"type":"result","success":true,"result":null}
//	HA:     {"id":1,"type":"event","event":{"event_type":"state_changed","data":{"entity_id":..,"new_state":{..}}}}
//	client: {"id":2,"type":"get_states"} / {"id":3,"type":"call_service",...} / {"id":4,"type":"ping"}

const handshakeTimeout = 15 * time.Second

// msgConn carries whole WebSocket text messages, directly or through an agent.
type msgConn interface {
	Read(ctx context.Context) ([]byte, error)
	Write(ctx context.Context, msg []byte) error
	Close() error
}

type haMsg struct {
	ID        int64           `json:"id,omitempty"`
	Type      string          `json:"type"`
	HAVersion string          `json:"ha_version,omitempty"`
	Message   string          `json:"message,omitempty"`
	Success   bool            `json:"success,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *haError        `json:"error,omitempty"`
	Event     *haEvent        `json:"event,omitempty"`
}

type haError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *haError) Error() string { return e.Code + ": " + e.Message }

type haEvent struct {
	EventType string `json:"event_type"`
	Data      struct {
		EntityID string      `json:"entity_id"`
		NewState *haRawState `json:"new_state"`
	} `json:"data"`
}

// haRawState is a state object as HA sends it.
type haRawState struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged time.Time      `json:"last_changed"`
	LastUpdated time.Time      `json:"last_updated"`
}

// entry is a cached state plus when HA last touched it (attributes included).
type entry struct {
	state   contracts.HAState
	updated time.Time
}

func (r haRawState) entry() entry {
	attrs := r.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	updated := r.LastUpdated
	if updated.IsZero() {
		updated = r.LastChanged
	}
	return entry{state: contracts.HAState{EntityID: r.EntityID, State: r.State, Attributes: attrs, LastChanged: r.LastChanged}, updated: updated}
}

// authError means HA refused the token.
type authError struct{ haMessage string }

func (e *authError) Error() string {
	return "访问令牌无效。请在 Home Assistant 的个人资料页重新生成长期访问令牌"
}

// errDisconnected is returned for calls on a session that ended.
var errDisconnected = errors.New("home assistant connection closed")

// websocketURL turns the configured base URL into the /api/websocket URL.
func websocketURL(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("地址必须以 http:// 或 https:// 开头")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/websocket"
	return u.String(), nil
}

// dialAndAuth opens the WebSocket and authenticates. It returns the HA version.
func dialAndAuth(ctx context.Context, tr transport, cfg config) (msgConn, string, error) {
	u, err := websocketURL(cfg.URL)
	if err != nil {
		return nil, "", err
	}
	hctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	conn, err := tr.dial(hctx, u)
	if err != nil {
		return nil, "", err
	}
	version, err := authenticate(hctx, conn, cfg.Token)
	if err != nil {
		_ = conn.Close()
		return nil, "", err
	}
	return conn, version, nil
}

func authenticate(ctx context.Context, conn msgConn, token string) (string, error) {
	first, err := readMsg(ctx, conn)
	if err != nil {
		return "", err
	}
	if first.Type != "auth_required" {
		return "", fmt.Errorf("这个地址不是 Home Assistant 的 WebSocket 接口（收到 %q）", first.Type)
	}
	raw, _ := json.Marshal(map[string]string{"type": "auth", "access_token": token})
	if err := conn.Write(ctx, raw); err != nil {
		return "", err
	}
	res, err := readMsg(ctx, conn)
	if err != nil {
		return "", err
	}
	switch res.Type {
	case "auth_ok":
		if res.HAVersion == "" {
			res.HAVersion = first.HAVersion
		}
		return res.HAVersion, nil
	case "auth_invalid":
		return "", &authError{haMessage: res.Message}
	default:
		return "", fmt.Errorf("认证时收到意外的消息 %q", res.Type)
	}
}

func readMsg(ctx context.Context, conn msgConn) (haMsg, error) {
	raw, err := conn.Read(ctx)
	if err != nil {
		return haMsg{}, err
	}
	var msg haMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return haMsg{}, fmt.Errorf("无法解析 Home Assistant 的消息: %w", err)
	}
	return msg, nil
}

// session is one authenticated connection. Commands are matched to results
// by id; events go to the callback given to readLoop.
type session struct {
	conn    msgConn
	version string
	seq     atomic.Int64
	wmu     sync.Mutex

	mu      sync.Mutex
	pending map[int64]chan haMsg
	done    chan struct{}
}

func newSession(conn msgConn, version string) *session {
	return &session{conn: conn, version: version, pending: map[int64]chan haMsg{}, done: make(chan struct{})}
}

// call sends a command and waits for its result (or pong).
func (s *session) call(ctx context.Context, cmd map[string]any) (haMsg, error) {
	id := s.seq.Add(1)
	ch := make(chan haMsg, 1)
	s.mu.Lock()
	select {
	case <-s.done:
		s.mu.Unlock()
		return haMsg{}, errDisconnected
	default:
	}
	s.pending[id] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
	}()
	cmd["id"] = id
	raw, err := json.Marshal(cmd)
	if err != nil {
		return haMsg{}, err
	}
	s.wmu.Lock()
	err = s.conn.Write(ctx, raw)
	s.wmu.Unlock()
	if err != nil {
		return haMsg{}, err
	}
	select {
	case msg := <-ch:
		if msg.Type == "result" && !msg.Success {
			if msg.Error == nil {
				msg.Error = &haError{Code: "unknown_error", Message: "unknown error"}
			}
			return msg, msg.Error
		}
		return msg, nil
	case <-ctx.Done():
		return haMsg{}, ctx.Err()
	case <-s.done:
		return haMsg{}, errDisconnected
	}
}

// readLoop dispatches messages until the connection fails. onEvent runs on
// the read goroutine and must return quickly.
func (s *session) readLoop(ctx context.Context, onEvent func(*haEvent)) error {
	defer func() {
		s.mu.Lock()
		close(s.done)
		s.mu.Unlock()
	}()
	for {
		raw, err := s.conn.Read(ctx)
		if err != nil {
			return err
		}
		var msg haMsg
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		switch msg.Type {
		case "event":
			if msg.Event != nil {
				onEvent(msg.Event)
			}
		case "result", "pong":
			s.mu.Lock()
			ch := s.pending[msg.ID]
			s.mu.Unlock()
			if ch != nil {
				select {
				case ch <- msg:
				default:
				}
			}
		}
	}
}
