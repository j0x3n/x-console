package hosts

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

type presenceStore struct {
	mu     sync.Mutex
	states map[string]contracts.HostPresence
	subs   map[chan contracts.HostPresence]struct{}
}

func newPresenceStore() *presenceStore {
	return &presenceStore{states: map[string]contracts.HostPresence{}, subs: map[chan contracts.HostPresence]struct{}{}}
}

func (m *Module) Subscribe() (<-chan contracts.HostPresence, func()) {
	ch := make(chan contracts.HostPresence, 64)
	m.presence.mu.Lock()
	m.presence.subs[ch] = struct{}{}
	m.presence.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() { m.presence.mu.Lock(); delete(m.presence.subs, ch); close(ch); m.presence.mu.Unlock() })
	}
}

func (m *Module) State(id string) contracts.HostPresence {
	ref, err := m.lookupHost(context.Background(), id)
	if err != nil {
		return contracts.HostPresence{HostID: id, State: "offline"}
	}
	m.presence.mu.Lock()
	defer m.presence.mu.Unlock()
	p := m.presence.states[id]
	p.HostID, p.Name = id, ref.Name
	p.Online = ref.agent != nil && m.d.Agents.Online(id)
	now := m.now()
	if !p.Online {
		p.State, p.Known, p.IdleSeconds, p.Locked, p.DisplayOff = "offline", false, nil, nil, nil
	} else if p.UpdatedAt.IsZero() || now.Sub(p.UpdatedAt) > 90*time.Second {
		p.State, p.Known, p.IdleSeconds, p.Locked, p.DisplayOff = "unknown", false, nil, nil, nil
	} else {
		if p.Known && p.IdleSeconds != nil {
			seconds := *p.IdleSeconds + max(0, int64(now.Sub(p.UpdatedAt)/time.Second))
			p.IdleSeconds = &seconds
		}
		p = classifyPresence(p, 5)
	}
	previous := m.presence.states[id]
	if p.Since.IsZero() || previous.State != p.State {
		p.Since = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}
	if previous.State != p.State || previous.Since.IsZero() {
		previous.HostID, previous.Name, previous.Online, previous.State, previous.Since = p.HostID, p.Name, p.Online, p.State, p.Since
		m.presence.states[id] = previous
	}
	return p
}

func classifyPresence(p contracts.HostPresence, minutes int) contracts.HostPresence {
	switch {
	case !p.Online:
		p.State = "offline"
	case p.Locked != nil && *p.Locked:
		p.State = "locked"
	case p.DisplayOff != nil && *p.DisplayOff:
		p.State = "idle"
	case !p.Known || p.IdleSeconds == nil:
		p.State = "unknown"
	case *p.IdleSeconds >= int64(minutes*60):
		p.State = "idle"
	default:
		p.State = "active"
	}
	return p
}

func (m *Module) onPresence(id string, raw json.RawMessage) {
	var sample protocol.PresenceSample
	if json.Unmarshal(raw, &sample) != nil || sample.IdleSeconds < 0 || sample.IdleSeconds > 365*24*60*60 {
		return
	}
	if !m.d.Agents.Online(id) {
		return
	}
	a, err := m.d.Agents.Get(context.Background(), id)
	if err != nil || !a.Has(protocol.CapPresence) {
		return
	}
	p := contracts.HostPresence{HostID: id, Name: a.Name, Online: true, Known: sample.Known, Locked: sample.Locked, DisplayOff: sample.DisplayOff, UpdatedAt: m.now()}
	if sample.Known {
		p.IdleSeconds = new(sample.IdleSeconds)
	}
	m.storePresence(classifyPresence(p, 5))
}

func (m *Module) storePresence(p contracts.HostPresence) {
	m.presence.mu.Lock()
	old := m.presence.states[p.HostID]
	changed := old.State != p.State || old.Online != p.Online || old.Known != p.Known
	p.Since = old.Since
	if changed || p.Since.IsZero() {
		p.Since = p.UpdatedAt
	}
	m.presence.states[p.HostID] = p
	for ch := range m.presence.subs {
		select {
		case ch <- p:
		default:
		}
	}
	m.presence.mu.Unlock()
	if changed {
		m.d.Bus.Publish("host.presence", p)
	}
}

func (m *Module) refreshPresence(ctx context.Context, id string) {
	a, err := m.d.Agents.Get(ctx, id)
	if err != nil {
		return
	}
	p := contracts.HostPresence{HostID: id, Name: a.Name, Online: a.Online, State: "offline", UpdatedAt: m.now()}
	if a.Online {
		p.State = "unknown"
		m.storePresence(p)
		if a.Has(protocol.CapPresence) {
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			var sample protocol.PresenceSample
			if m.d.Agents.Call(callCtx, id, protocol.MethodPresenceGet, nil, &sample) == nil {
				raw, _ := json.Marshal(sample)
				m.onPresence(id, raw)
			}
		}
	} else {
		m.storePresence(p)
	}
}

func (m *Module) followPresence(ctx context.Context) {
	ch, cancel := m.d.Bus.Subscribe("agent.", 64)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				if ev.Topic != "agent.online" && ev.Topic != "agent.offline" && ev.Topic != "agent.revoked" {
					continue
				}
				raw, _ := json.Marshal(ev.Data)
				var data struct {
					ID string `json:"agentId"`
				}
				if json.Unmarshal(raw, &data) != nil || data.ID == "" {
					continue
				}
				if ev.Topic == "agent.revoked" {
					m.presence.mu.Lock()
					delete(m.presence.states, data.ID)
					m.presence.mu.Unlock()
					m.storePresence(contracts.HostPresence{HostID: data.ID, State: "offline", UpdatedAt: m.now()})
				} else {
					m.refreshPresence(ctx, data.ID)
				}
			}
		}
	}()
}
