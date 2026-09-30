package hosts

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	unitsTTL        = 10 * time.Minute
	syslogTimeout   = 30 * time.Second
	syslogFirstWait = 400 * time.Millisecond
)

// unitCache remembers the unit list of each host for 10 minutes.
type unitCache struct {
	mu   sync.Mutex
	byID map[string]cachedUnits
}

type cachedUnits struct {
	at    time.Time
	items []string
}

func (c *unitCache) get(id string, now time.Time) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	hit, ok := c.byID[id]
	return hit.items, ok && now.Sub(hit.at) < unitsTTL
}

func (c *unitCache) put(id string, now time.Time, items []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byID == nil {
		c.byID = map[string]cachedUnits{}
	}
	c.byID[id] = cachedUnits{at: now, items: items}
}

func priorityOK(p *int) error {
	if p != nil && (*p < 0 || *p > 7) {
		return httpx.Invalid("级别要在 0 到 7 之间")
	}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toAPILog(e protocol.SyslogEntry) api.SyslogEntry {
	out := api.SyslogEntry{Time: e.Time, Priority: e.Priority, Unit: e.Unit, Message: e.Message}
	if e.PID > 0 {
		pid := e.PID
		out.Pid = &pid
	}
	return out
}

// GetSyslog is GET /hosts/{hostId}/syslog.
func (m *Module) GetSyslog(w http.ResponseWriter, r *http.Request, hostID api.HostId, params api.GetSyslogParams) {
	ctx, cancel := context.WithTimeout(r.Context(), syslogTimeout)
	defer cancel()
	p := protocol.SyslogQueryParams{Since: params.Since, Until: params.Until, Priority: params.Priority,
		Unit: deref(params.Unit), Grep: deref(params.Grep), Cursor: deref(params.Cursor), Limit: 500}
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > 5000 {
			httpx.Fail(w, r, httpx.Invalid("limit 要在 1 到 5000 之间"))
			return
		}
		p.Limit = *params.Limit
	}
	if err := priorityOK(p.Priority); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var page protocol.SyslogPage
	err := m.call(ctx, hostID, protocol.CapSyslog, protocol.MethodSyslogQuery, p, &page)
	if p.Cursor == "" { // a page further back is the same reading
		detail := map[string]any{"unit": p.Unit, "grep": clip(p.Grep, 100)}
		if p.Priority != nil {
			detail["priority"] = *p.Priority
		}
		m.d.Audit.Record(r.Context(), "host.syslog.read", hostID, detail, err)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := api.SyslogPage{Items: make([]api.SyslogEntry, 0, len(page.Items))}
	for _, e := range page.Items {
		out.Items = append(out.Items, toAPILog(e))
	}
	if page.Cursor != "" {
		out.Cursor = &page.Cursor
	}
	httpx.JSON(w, http.StatusOK, out)
}

// ListSyslogUnits is GET /hosts/{hostId}/syslog/units.
func (m *Module) ListSyslogUnits(w http.ResponseWriter, r *http.Request, hostID api.HostId) {
	if items, ok := m.units.get(hostID, m.now()); ok {
		if _, err := m.agentFor(r.Context(), hostID, protocol.CapSyslog); err == nil {
			httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), syslogTimeout)
	defer cancel()
	var out protocol.SyslogUnits
	if err := m.call(ctx, hostID, protocol.CapSyslog, protocol.MethodSyslogUnits, nil, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if out.Items == nil {
		out.Items = []string{}
	}
	m.units.put(hostID, m.now(), out.Items)
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out.Items})
}

type syslogFrame struct {
	b   []byte
	err error
}

// FollowSyslog is GET /hosts/{hostId}/syslog/follow, a WebSocket. Every text
// frame is a JSON array of new entries. Problems that show up right away, such
// as a missing permission, are answered as plain JSON errors before the upgrade.
func (m *Module) FollowSyslog(w http.ResponseWriter, r *http.Request, hostID api.HostId, params api.FollowSyslogParams) {
	if err := priorityOK(params.Priority); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a, err := m.agentFor(r.Context(), hostID, protocol.CapSyslog)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// The stream lives as long as the socket, not the request bookkeeping.
	sctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()
	s, err := m.d.Agents.Open(sctx, a.ID, protocol.MethodSyslogFollow,
		protocol.SyslogFollowParams{Priority: params.Priority, Unit: deref(params.Unit), Grep: deref(params.Grep)})
	if err != nil {
		httpx.Fail(w, r, agentErr(err))
		return
	}
	defer s.Close(nil)
	frames := make(chan syslogFrame)
	go func() {
		for {
			b, err := s.Recv(sctx)
			select {
			case frames <- syslogFrame{b, err}:
			case <-sctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	var held *syslogFrame
	select {
	case f := <-frames:
		if f.err != nil && !errors.Is(f.err, io.EOF) {
			httpx.Fail(w, r, agentErr(f.err))
			return
		}
		held = &f
	case <-time.After(syslogFirstWait):
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	// The browser only listens; CloseRead ends wsCtx when it goes away.
	wsCtx := ws.CloseRead(sctx)
	for {
		var f syslogFrame
		if held != nil {
			f, held = *held, nil
		} else {
			select {
			case f = <-frames:
			case <-wsCtx.Done():
				return
			}
		}
		if f.err != nil {
			if errors.Is(f.err, io.EOF) {
				_ = ws.Close(websocket.StatusNormalClosure, "log ended")
			} else {
				_ = ws.Close(websocket.StatusInternalError, clip(agentErr(f.err).Error(), 100))
			}
			return
		}
		var entries []protocol.SyslogEntry
		if json.Unmarshal(f.b, &entries) != nil {
			continue
		}
		out := make([]api.SyslogEntry, 0, len(entries))
		for _, e := range entries {
			out = append(out, toAPILog(e))
		}
		raw, _ := json.Marshal(out)
		if err := ws.Write(wsCtx, websocket.MessageText, raw); err != nil {
			return
		}
	}
}
