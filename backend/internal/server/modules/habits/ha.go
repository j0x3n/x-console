package habits

import (
	"context"
	"encoding/json"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

// toHAState reads the payload of an "ha.state_changed" event. M9 publishes
// contracts.HAState; anything JSON-shaped the same way works too.
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
	if err := json.Unmarshal(raw, &st); err != nil {
		return contracts.HAState{}, false
	}
	return st, st.EntityID != ""
}

// ignoredState are states that are not a real change of the device.
func ignoredState(s string) bool { return s == "" || s == "unavailable" || s == "unknown" }

// refreshWatches reloads the set of linked entities and asks Home Assistant
// to publish their changes.
func (m *Module) refreshWatches(ctx context.Context) error {
	habits, err := m.q.ListHabits(ctx, 0)
	if err != nil {
		return err
	}
	set := map[string]bool{}
	for _, h := range habits {
		if h.HaEntityID != "" {
			set[h.HaEntityID] = true
		}
	}
	m.haMu.Lock()
	m.watched = set
	m.haMu.Unlock()
	for id := range set {
		m.watch(ctx, id)
	}
	return nil
}

// watch registers entityID with Home Assistant when M9 is available and
// remembers its current state, so the first event is compared correctly.
func (m *Module) watch(ctx context.Context, entityID string) {
	if entityID == "" {
		return
	}
	m.haMu.Lock()
	m.watched[entityID] = true
	_, known := m.haStates[entityID]
	m.haMu.Unlock()
	ha, ok := module.Lookup[contracts.HomeAssistant](m.d.Registry, contracts.HomeAssistantKey)
	if !ok {
		return
	}
	ha.WatchEntity(entityID)
	if known {
		return
	}
	go func() {
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		st, err := ha.State(sctx, entityID)
		if err != nil || ignoredState(st.State) {
			return
		}
		m.haMu.Lock()
		if _, seen := m.haStates[entityID]; !seen {
			m.haStates[entityID] = st.State
		}
		m.haMu.Unlock()
	}()
}

// onHAState checks in every habit linked to the entity when its state
// really changed.
func (m *Module) onHAState(ctx context.Context, st contracts.HAState) error {
	if ignoredState(st.State) {
		return nil
	}
	m.haMu.Lock()
	if !m.watched[st.EntityID] {
		m.haMu.Unlock()
		return nil
	}
	prev, known := m.haStates[st.EntityID]
	m.haStates[st.EntityID] = st.State
	m.haMu.Unlock()
	if known && prev == st.State {
		return nil
	}
	habits, err := m.q.ListHabitsByEntity(ctx, st.EntityID)
	if err != nil {
		return err
	}
	actx := audit.WithActor(ctx, "ha:"+st.EntityID)
	for _, h := range habits {
		if _, _, err := m.checkin(actx, h.ID, 1, st.EntityID+" → "+st.State, "ha", time.Now()); err != nil {
			return err
		}
	}
	return nil
}
