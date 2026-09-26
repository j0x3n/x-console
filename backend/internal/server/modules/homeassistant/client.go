package homeassistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	minBackoff   = time.Second
	maxBackoff   = time.Minute
	pingInterval = 30 * time.Second
)

// Event topics.
const (
	TopicStateChanged      = "ha.state_changed"
	TopicConnectionChanged = "ha.connection_changed"
)

// ConnectionChanged is the payload of "ha.connection_changed".
type ConnectionChanged struct {
	Connected bool `json:"connected"`
}

// client keeps one live connection to HA, reconnecting with backoff, and
// caches every entity state in memory.
type client struct {
	log  *slog.Logger
	bus  *events.Bus
	load func(ctx context.Context) (config, error)
	pick func(cfg config) transport

	reload chan struct{} // config changed: drop the session and the cache
	wake   chan struct{} // retry now (for example the agent came back)

	mu          sync.RWMutex
	sess        *session
	connectedAt time.Time
	lastErr     string
	states      map[string]entry
	loaded      bool              // snapshot of this session applied
	early       map[string]*entry // events seen before the snapshot
	favorites   map[string]bool
	watched     map[string]bool
}

func newClient(log *slog.Logger, bus *events.Bus, load func(context.Context) (config, error), tr func(config) transport) *client {
	return &client{log: log, bus: bus, load: load, pick: tr,
		reload: make(chan struct{}, 1), wake: make(chan struct{}, 1),
		states: map[string]entry{}, favorites: map[string]bool{}, watched: map[string]bool{}}
}

// Reload makes the client reread its config and reconnect.
func (c *client) Reload() { signal(c.reload) }

// Wake skips the current backoff wait.
func (c *client) Wake() { signal(c.wake) }

func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// run connects and reconnects until ctx ends.
func (c *client) run(ctx context.Context) {
	backoff := minBackoff
	for ctx.Err() == nil {
		cfg, err := c.load(ctx)
		if err != nil {
			c.log.Error("home assistant config", "err", err)
		}
		if err != nil || !cfg.configured() {
			c.reset("")
			if _, ok := c.waitReload(ctx, 0); !ok {
				return
			}
			continue
		}

		start := time.Now()
		sctx, cancel := context.WithCancel(ctx)
		var reloaded atomic.Bool
		watcherDone := make(chan struct{})
		go func() {
			defer close(watcherDone)
			select {
			case <-c.reload:
				reloaded.Store(true)
				cancel()
			case <-sctx.Done():
			}
		}()
		err = c.runSession(sctx, cfg)
		cancel()
		<-watcherDone // so it cannot swallow a later reload
		if ctx.Err() != nil {
			c.setDisconnected("")
			return
		}
		if reloaded.Load() {
			c.reset("")
			backoff = minBackoff
			continue
		}
		msg := describe(err)
		c.log.Warn("home assistant disconnected", "err", err, "retry_in", backoff)
		c.setDisconnected(msg)

		var auth *authError
		if errors.As(err, &auth) {
			// HA bans an IP after repeated failed logins, so do not retry a
			// bad token. Wait until the config changes.
			for {
				wasReload, ok := c.waitReload(ctx, 0)
				if !ok {
					return
				}
				if wasReload {
					break
				}
			}
			c.reset("")
			backoff = minBackoff
			continue
		}
		if time.Since(start) > time.Minute {
			backoff = minBackoff
		}
		wasReload, ok := c.waitReload(ctx, backoff)
		if !ok {
			return
		}
		if wasReload {
			c.reset("")
			backoff = minBackoff
			continue
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

// waitReload waits d (forever when d is 0) or until a reload or wake signal.
// It reports whether a reload arrived and false when ctx ended.
func (c *client) waitReload(ctx context.Context, d time.Duration) (bool, bool) {
	var timer <-chan time.Time
	if d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		timer = t.C
	}
	select {
	case <-ctx.Done():
		return false, false
	case <-c.reload:
		return true, true
	case <-c.wake:
		return false, true
	case <-timer:
		return false, true
	}
}

func (c *client) runSession(ctx context.Context, cfg config) error {
	conn, version, err := dialAndAuth(ctx, c.pick(cfg), cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	s := newSession(conn, version)
	c.mu.Lock()
	c.loaded = false
	c.early = map[string]*entry{}
	c.mu.Unlock()

	readErr := make(chan error, 1)
	go func() { readErr <- s.readLoop(ctx, c.handleEvent) }()

	// Subscribe before fetching so no change falls between the two.
	if _, err := s.call(ctx, map[string]any{"type": "subscribe_events", "event_type": "state_changed"}); err != nil {
		return fmt.Errorf("subscribe_events: %w", err)
	}
	res, err := s.call(ctx, map[string]any{"type": "get_states"})
	if err != nil {
		return fmt.Errorf("get_states: %w", err)
	}
	var snapshot []haRawState
	if err := json.Unmarshal(res.Result, &snapshot); err != nil {
		return fmt.Errorf("get_states: %w", err)
	}
	c.applySnapshot(snapshot)
	c.setConnected(s)
	c.log.Info("home assistant connected", "version", version, "entities", len(snapshot))

	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case err := <-readErr:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, err := s.call(pctx, map[string]any{"type": "ping"})
			cancel()
			if err != nil {
				return fmt.Errorf("ping: %w", err)
			}
		}
	}
}

func (c *client) handleEvent(ev *haEvent) {
	if ev.EventType != "state_changed" {
		return
	}
	id := ev.Data.EntityID
	c.mu.Lock()
	var pub *contracts.HAState
	if ev.Data.NewState == nil {
		delete(c.states, id)
	} else {
		e := ev.Data.NewState.entry()
		c.states[id] = e
		if !c.loaded {
			c.early[id] = &e
		}
		if c.favorites[id] || c.watched[id] {
			pub = &e.state
		}
	}
	c.mu.Unlock()
	if pub != nil {
		c.bus.Publish(TopicStateChanged, *pub)
	}
}

// applySnapshot replaces the cache with get_states. Changes seen by events
// while the snapshot was in flight win when they are newer. Forwarded
// entities that changed while disconnected are published.
func (c *client) applySnapshot(snapshot []haRawState) {
	next := make(map[string]entry, len(snapshot))
	for _, rs := range snapshot {
		next[rs.EntityID] = rs.entry()
	}
	c.mu.Lock()
	for id, e := range c.early {
		if cur, ok := next[id]; !ok || e.updated.After(cur.updated) {
			next[id] = *e
		}
	}
	var pubs []contracts.HAState
	for id, n := range next {
		if !c.favorites[id] && !c.watched[id] {
			continue
		}
		if o, had := c.states[id]; had && (o.state.State != n.state.State || !o.state.LastChanged.Equal(n.state.LastChanged)) {
			pubs = append(pubs, n.state)
		}
	}
	c.states = next
	c.early = nil
	c.loaded = true
	c.mu.Unlock()
	for _, p := range pubs {
		c.bus.Publish(TopicStateChanged, p)
	}
}

func (c *client) setConnected(s *session) {
	c.mu.Lock()
	c.sess = s
	c.connectedAt = time.Now().UTC()
	c.lastErr = ""
	c.mu.Unlock()
	c.bus.Publish(TopicConnectionChanged, ConnectionChanged{Connected: true})
}

// setDisconnected keeps the cached states (shown as last known values).
func (c *client) setDisconnected(msg string) {
	c.mu.Lock()
	was := c.sess != nil
	c.sess = nil
	if msg != "" {
		c.lastErr = msg
	}
	changed := was || msg != ""
	c.mu.Unlock()
	if changed {
		c.bus.Publish(TopicConnectionChanged, ConnectionChanged{Connected: false})
	}
}

// reset forgets everything about the previous HA (config changed or removed).
func (c *client) reset(msg string) {
	c.setDisconnected("")
	c.mu.Lock()
	c.states = map[string]entry{}
	c.lastErr = msg
	c.mu.Unlock()
}

func (c *client) session() *session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sess
}

type clientStatus struct {
	connected   bool
	connectedAt time.Time
	version     string
	entityCount int
	lastErr     string
}

func (c *client) status() clientStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	st := clientStatus{entityCount: len(c.states), lastErr: c.lastErr}
	if c.sess != nil {
		st.connected = true
		st.connectedAt = c.connectedAt
		st.version = c.sess.version
	}
	return st
}

// state returns one cached entity.
func (c *client) state(entityID string) (contracts.HAState, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.states[entityID]
	return e.state, ok
}

// list returns cached states filtered by domain and a search text, sorted by id.
func (c *client) list(domain, q string) []contracts.HAState {
	q = strings.ToLower(strings.TrimSpace(q))
	c.mu.RLock()
	out := make([]contracts.HAState, 0, len(c.states))
	for id, e := range c.states {
		if domain != "" && !strings.HasPrefix(id, domain+".") {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(id), q) && !strings.Contains(strings.ToLower(friendlyName(e.state)), q) {
			continue
		}
		out = append(out, e.state)
	}
	c.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].EntityID < out[j].EntityID })
	return out
}

func (c *client) setFavorites(ids []string) {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	c.mu.Lock()
	c.favorites = m
	c.mu.Unlock()
}

func (c *client) watch(entityID string) {
	c.mu.Lock()
	c.watched[entityID] = true
	c.mu.Unlock()
}

// call runs a command on the live session.
func (c *client) call(ctx context.Context, cmd map[string]any) (haMsg, error) {
	s := c.session()
	if s == nil {
		return haMsg{}, errNotConnected
	}
	return s.call(ctx, cmd)
}

var errNotConnected = httpx.NewError(503, "ha_unavailable", "Home Assistant 当前没有连上")

func friendlyName(s contracts.HAState) string {
	if n, ok := s.Attributes["friendly_name"].(string); ok {
		return n
	}
	return ""
}

// describe turns a connection error into a message for the user.
func describe(err error) string {
	if err == nil {
		return ""
	}
	var auth *authError
	var apiErr *httpx.Error
	var pe *protocol.Error
	switch {
	case errors.As(err, &auth):
		return auth.Error()
	case errors.As(err, &apiErr):
		if strings.HasPrefix(apiErr.Code, "agent_") && apiErr.Code != "agent_offline" {
			return "代理转发失败: " + apiErr.Message
		}
		return apiErr.Message
	case errors.As(err, &pe):
		return "代理转发失败: " + pe.Message
	case errors.Is(err, context.DeadlineExceeded):
		return "连接 Home Assistant 超时"
	default:
		return "连不上 Home Assistant: " + err.Error()
	}
}
