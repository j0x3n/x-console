package me

// Home Assistant as a second source of the same events. Both entities are
// optional. A change of the "at home" entity is home.arrive or home.leave; a
// change of the sleep focus sensor is sleep.start or sleep.end. The events go
// through the same store as the phone reports, so the 2 minute rule drops a
// repeat of what the phone just said.

import (
	"context"
	"encoding/json"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

func ignoredHAState(s string) bool { return s == "" || s == "unavailable" || s == "unknown" }

func toHAState(data any) (contracts.HAState, bool) {
	switch v := data.(type) {
	case contracts.HAState:
		return v, v.EntityID != ""
	case *contracts.HAState:
		if v == nil {
			return contracts.HAState{}, false
		}
		return *v, v.EntityID != ""
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return contracts.HAState{}, false
	}
	var st contracts.HAState
	if json.Unmarshal(raw, &st) != nil {
		return contracts.HAState{}, false
	}
	return st, st.EntityID != ""
}

// haEvent is the report a state of one of the two entities means. ok is
// false when the state says nothing (unavailable, or an entity not chosen).
func haEvent(cfg config, entity, state string) (string, bool) {
	if ignoredHAState(state) {
		return "", false
	}
	switch {
	case entity != "" && entity == cfg.HAHome:
		if state == "home" {
			return evHomeArrive, true
		}
		return evHomeLeave, true
	case entity != "" && entity == cfg.HASleep:
		switch state {
		case "on":
			return evSleepStart, true
		case "off":
			return evSleepEnd, true
		}
	}
	return "", false
}

func (m *Module) followHA(ctx context.Context) {
	ch, cancel := m.d.Bus.Subscribe("ha.state_changed", 256)
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
				if st, ok := toHAState(ev.Data); ok {
					m.onHAState(ctx, st)
				}
			}
		}
	}()
	m.mu.Lock()
	err := m.ensureLoaded(ctx)
	cfg := m.cfg
	m.mu.Unlock()
	if err == nil {
		go m.watchHA(ctx, cfg)
	}
}

// onHAState turns a change of a chosen entity into a report.
func (m *Module) onHAState(ctx context.Context, st contracts.HAState) {
	m.mu.Lock()
	cfg := m.cfg
	m.mu.Unlock()
	event, ok := haEvent(cfg, st.EntityID, st.State)
	if !ok {
		return
	}
	m.haMu.Lock()
	same := m.haLast[st.EntityID] == st.State
	m.haLast[st.EntityID] = st.State
	m.haMu.Unlock()
	if same {
		return // an attribute changed, not the state
	}
	if _, err := m.ingest(ctx, event, srcHA, time.Time{}); err != nil {
		m.d.Log.Warn("me: report from Home Assistant", "entity", st.EntityID, "err", err)
	}
}

// watchHA asks Home Assistant to publish the chosen entities, and takes their
// current state as a report when it is newer than what was reported before,
// so a restart or a new setting does not leave the state empty.
func (m *Module) watchHA(ctx context.Context, cfg config) {
	ha, ok := module.Lookup[contracts.HomeAssistant](m.d.Registry, contracts.HomeAssistantKey)
	if !ok {
		return
	}
	for _, entity := range []string{cfg.HAHome, cfg.HASleep} {
		if entity == "" {
			continue
		}
		ha.WatchEntity(entity)
		callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		st, err := ha.State(callCtx, entity)
		cancel()
		if err != nil || ignoredHAState(st.State) {
			continue
		}
		m.haMu.Lock()
		m.haLast[entity] = st.State
		m.haMu.Unlock()
		event, ok := haEvent(cfg, entity, st.State)
		if !ok {
			continue
		}
		at := st.LastChanged
		m.mu.Lock()
		now := m.nowLocked()
		var latest report
		var err2 error
		if event == evHomeArrive || event == evHomeLeave {
			latest, err2 = m.latestPlace(ctx)
		} else {
			latest, err2 = m.latestSleep(ctx)
		}
		m.mu.Unlock()
		if err2 != nil || at.IsZero() || at.After(now) || !at.After(latest.At) {
			continue
		}
		if _, err := m.ingest(ctx, event, srcHA, at); err != nil {
			m.d.Log.Warn("me: read Home Assistant", "entity", entity, "err", err)
		}
	}
}
