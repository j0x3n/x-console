// Package agenthub keeps the WebSocket connections of paired agents and lets
// modules call methods on them. See docs/04-agent-protocol.md.
package agenthub

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"net/netip"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/core/db"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

const (
	pairingTTL     = 10 * time.Minute
	defaultTimeout = 30 * time.Second
	pingInterval   = 30 * time.Second
)

// Agent is the stored agent plus its live state.
type Agent struct {
	db.Agent
	Online       bool
	Capabilities []string
}

// Has reports whether the agent announced capability c.
func (a Agent) Has(c string) bool {
	for _, v := range a.Capabilities {
		if v == c {
			return true
		}
	}
	return false
}

// EventFunc receives an event emitted by an agent.
type EventFunc func(agentID string, params json.RawMessage)

// Hub tracks connected agents.
type Hub struct {
	q              *db.Queries
	db             *sql.DB
	pairingInfo    contracts.HostPairingInfo
	trustedProxies []netip.Prefix
	sourceIPs      map[string]string
	bus            *events.Bus
	audit          *audit.Log
	now            func() time.Time

	mu       sync.RWMutex
	conns    map[string]*conn
	handlers map[string][]EventFunc
}

type conn struct {
	id    string
	hello protocol.Hello
	peer  *rpc.Peer
	ws    *websocket.Conn
}

// New builds a Hub.
func New(dbConn *sql.DB, bus *events.Bus, log *audit.Log) *Hub {
	return &Hub{db: dbConn, sourceIPs: map[string]string{}, q: db.New(dbConn), bus: bus, audit: log, now: func() time.Time { return time.Now().UTC() },
		conns: map[string]*conn{}, handlers: map[string][]EventFunc{}}
}

// OnEvent registers fn for events named method from any agent.
// Register during module construction, before agents connect.
func (h *Hub) OnEvent(method string, fn EventFunc) {
	h.mu.Lock()
	h.handlers[method] = append(h.handlers[method], fn)
	h.mu.Unlock()
}

// Online reports whether the agent is connected.
func (h *Hub) Online(agentID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.conns[agentID]
	return ok
}

// Hello returns what the connected agent announced.
func (h *Hub) Hello(agentID string) (protocol.Hello, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.conns[agentID]
	if !ok {
		return protocol.Hello{}, false
	}
	return c.hello, true
}

// Call runs method on the agent and decodes the result into out.
// Without a deadline in ctx it times out after 30 seconds.
func (h *Hub) Call(ctx context.Context, agentID, method string, params, out any) error {
	c := h.get(agentID)
	if c == nil {
		return httpx.ErrAgentOffline
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}
	return mapErr(c.peer.Call(ctx, method, params, out))
}

// Open starts a stream (terminal, log follow, coding task output) on the agent.
func (h *Hub) Open(ctx context.Context, agentID, method string, params any) (*rpc.Stream, error) {
	c := h.get(agentID)
	if c == nil {
		return nil, httpx.ErrAgentOffline
	}
	s, err := c.peer.Open(ctx, method, params)
	return s, mapErr(err)
}

// Get returns one agent with live state.
func (h *Hub) Get(ctx context.Context, agentID string) (Agent, error) {
	a, err := h.q.GetAgent(ctx, agentID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && a.RevokedAt != nil) {
		return Agent{}, httpx.ErrNotFound
	}
	if err != nil {
		return Agent{}, err
	}
	return h.view(a), nil
}

// List returns all non-revoked agents.
func (h *Hub) List(ctx context.Context) ([]Agent, error) {
	rows, err := h.q.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(rows))
	for _, a := range rows {
		out = append(out, h.view(a))
	}
	return out, nil
}

// CreatePairingCode returns a one-time code like "K7QM-3XHP".
func (h *Hub) CreatePairingCode(ctx context.Context, name, kind string) (string, time.Time, error) {
	return h.CreatePairingCodeWithInfo(ctx, name, kind, nil)
}

// PairingCodeValid reports whether a code exists, is unused and has not
// expired. It does not use the code up; Pair does that.
func (h *Hub) PairingCodeValid(ctx context.Context, code string) (bool, error) {
	_, err := h.q.PeekPairingCode(ctx, db.PeekPairingCodeParams{CodeHash: secrets.Hash(normalizeCode(code)), ExpiresAt: h.now()})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// Pair exchanges a pairing code for an agent id and a long-lived token.
func (h *Hub) Pair(ctx context.Context, code string, hello protocol.Hello) (string, string, error) {
	return h.pairTransaction(ctx, code, hello)
}

// Revoke disables the agent token and drops its connection.
func (h *Hub) Revoke(ctx context.Context, agentID string) error {
	n, err := h.q.RevokeAgent(ctx, db.RevokeAgentParams{RevokedAt: ptr(h.now()), ID: agentID})
	if err != nil {
		return err
	}
	if n == 0 {
		return httpx.ErrNotFound
	}
	if c := h.get(agentID); c != nil {
		_ = c.ws.Close(websocket.StatusPolicyViolation, "revoked")
	}
	h.audit.Record(ctx, "agent.revoke", agentID, nil, nil)
	h.bus.Publish("agent.revoked", map[string]string{"agentId": agentID})
	return nil
}

// ServeConnect is the agent WebSocket endpoint (GET /api/v1/agent/connect).
// The agent authenticates with "Authorization: Bearer <token>".
func (h *Hub) ServeConnect(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	agent, err := h.q.GetAgentByTokenHash(r.Context(), secrets.Hash(token))
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(16 << 20)
	h.serve(context.WithValue(r.Context(), sourceIPKey{}, h.RequestIP(r)), agent, ws)
}

func (h *Hub) serve(ctx context.Context, agent db.Agent, ws *websocket.Conn) {
	transport := rpc.WebSocket(ws)
	helloCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	first, err := transport.Read(helloCtx)
	cancel()
	var hello protocol.Hello
	if err != nil || first.Kind != protocol.KindHello || json.Unmarshal(first.Params, &hello) != nil {
		_ = ws.Close(websocket.StatusProtocolError, "expected hello")
		return
	}
	if hello.ProtocolVersion != protocol.Version {
		_ = ws.Close(websocket.StatusPolicyViolation, "protocol version mismatch")
		return
	}
	if err := transport.Write(ctx, protocol.Envelope{Kind: protocol.KindWelcome,
		Params: protocol.Marshal(protocol.Welcome{ProtocolVersion: protocol.Version, AgentID: agent.ID, Name: agent.Name})}); err != nil {
		return
	}
	caps, _ := json.Marshal(nonNil(hello.Capabilities))
	_ = h.q.UpdateAgentSeen(ctx, db.UpdateAgentSeenParams{LastSeenAt: ptr(h.now()), Os: hello.OS, Arch: hello.Arch,
		Hostname: hello.Hostname, Version: hello.AgentVersion, Capabilities: string(caps), ID: agent.ID})

	peer := rpc.NewPeer(transport, "s")
	peer.OnEvent(func(method string, params json.RawMessage) { h.dispatchEvent(agent.ID, method, params) })
	c := &conn{id: agent.ID, hello: hello, peer: peer, ws: ws}

	h.mu.Lock()
	if old := h.conns[agent.ID]; old != nil {
		_ = old.ws.Close(websocket.StatusGoingAway, "replaced by a new connection")
	}
	h.conns[agent.ID] = c
	if ip, ok := ctx.Value(sourceIPKey{}).(string); ok {
		h.sourceIPs[agent.ID] = ip
	}
	h.mu.Unlock()
	slog.Info("agent connected", "agent", agent.ID, "name", agent.Name, "hostname", hello.Hostname)
	h.bus.Publish("agent.online", map[string]string{"agentId": agent.ID})

	runCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	go h.keepAlive(runCtx, c)
	err = peer.Run(runCtx)
	stop()

	h.mu.Lock()
	if h.conns[agent.ID] == c {
		delete(h.conns, agent.ID)
	}
	h.mu.Unlock()
	_ = h.q.UpdateAgentSeen(context.Background(), db.UpdateAgentSeenParams{LastSeenAt: ptr(h.now()), Os: hello.OS, Arch: hello.Arch,
		Hostname: hello.Hostname, Version: hello.AgentVersion, Capabilities: string(caps), ID: agent.ID})
	slog.Info("agent disconnected", "agent", agent.ID, "err", err)
	h.bus.Publish("agent.offline", map[string]string{"agentId": agent.ID})
}

func (h *Hub) keepAlive(ctx context.Context, c *conn) {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.ws.Ping(pctx)
			cancel()
			if err != nil {
				_ = c.ws.Close(websocket.StatusGoingAway, "ping timeout")
				return
			}
		}
	}
}

func (h *Hub) dispatchEvent(agentID, method string, params json.RawMessage) {
	h.mu.RLock()
	fns := h.handlers[method]
	h.mu.RUnlock()
	for _, fn := range fns {
		fn(agentID, params)
	}
}

func (h *Hub) get(id string) *conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.conns[id]
}

func (h *Hub) view(a db.Agent) Agent {
	var caps []string
	_ = json.Unmarshal([]byte(a.Capabilities), &caps)
	return Agent{Agent: a, Online: h.Online(a.ID), Capabilities: nonNil(caps)}
}

func mapErr(err error) error {
	if errors.Is(err, rpc.ErrClosed) {
		return httpx.ErrAgentOffline
	}
	var pe *protocol.Error
	if errors.As(err, &pe) {
		status := http.StatusBadGateway
		switch pe.Code {
		case protocol.CodeUnsupported, protocol.CodeUnknownMethod:
			status = http.StatusNotImplemented
		case protocol.CodeBadParams:
			status = http.StatusBadRequest
		case protocol.CodeTimeout:
			status = http.StatusGatewayTimeout
		}
		return &httpx.Error{Status: status, Code: "agent_" + pe.Code, Message: pe.Message}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return httpx.NewError(http.StatusGatewayTimeout, "agent_timeout", "代理响应超时")
	}
	return err
}

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomCode() string {
	var b strings.Builder
	for i := 0; i < 8; i++ {
		if i == 4 {
			b.WriteByte('-')
		}
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		b.WriteByte(codeAlphabet[n.Int64()])
	}
	return b.String()
}

func normalizeCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func ptr[T any](v T) *T { return &v }
