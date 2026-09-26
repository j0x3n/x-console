package homeassistant

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// An action has one fixed Effect, so service calls are split in two:
// ha.call_service (write) refuses dangerous services and
// ha.call_dangerous_service (dangerous) accepts any service.
func (m *Module) registerActions() {
	callSchema := actions.Schema(`{"type":"object","properties":{
		"domain":{"type":"string","description":"Service domain, e.g. light"},
		"service":{"type":"string","description":"Service name, e.g. turn_on"},
		"entityId":{"type":"string","description":"Target entity, e.g. light.living_room"},
		"data":{"type":"object","description":"Extra service data, e.g. {\"brightness_pct\":40}"}},
		"required":["domain","service"],"additionalProperties":false}`)

	m.d.Actions.Register(actions.Action{
		Name:        "ha.list_entities",
		Title:       "列出智能家居设备",
		Description: "List Home Assistant entities with their current state. Filter by domain (light, switch, sensor, climate, ...) and/or a search text matched against the entity id and friendly name.",
		Input:       actions.Schema(`{"type":"object","properties":{"domain":{"type":"string"},"q":{"type":"string"}},"additionalProperties":false}`),
		Effect:      actions.Read,
		Run:         m.actionList,
	})
	m.d.Actions.Register(actions.Action{
		Name:        "ha.get_state",
		Title:       "查看设备状态",
		Description: "Get the current state and attributes of one Home Assistant entity.",
		Input:       actions.Schema(`{"type":"object","properties":{"entityId":{"type":"string"}},"required":["entityId"],"additionalProperties":false}`),
		Effect:      actions.Read,
		Run:         m.actionGetState,
	})
	m.d.Actions.Register(actions.Action{
		Name:  "ha.call_service",
		Title: "控制智能家居设备",
		Description: "Call a Home Assistant service, e.g. light.turn_on, switch.toggle, scene.turn_on, climate.set_temperature. " +
			"Locks, alarm panels and opening covers are refused here; use ha.call_dangerous_service for those.",
		Input:  callSchema,
		Effect: actions.Write,
		Run:    func(ctx context.Context, in json.RawMessage) (any, error) { return m.actionCall(ctx, in, false) },
	})
	m.d.Actions.Register(actions.Action{
		Name:        "ha.call_dangerous_service",
		Title:       "控制门锁、安防或车库门",
		Description: "Call any Home Assistant service, including unlocking locks, disarming alarm panels and opening covers such as garage doors.",
		Input:       callSchema,
		Effect:      actions.Dangerous,
		Run:         func(ctx context.Context, in json.RawMessage) (any, error) { return m.actionCall(ctx, in, true) },
	})
}

// entitySummary is the compact view returned to the assistant.
type entitySummary struct {
	EntityID string `json:"entityId"`
	Name     string `json:"name,omitempty"`
	State    string `json:"state"`
	Unit     string `json:"unit,omitempty"`
}

func (m *Module) actionList(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct{ Domain, Q string }
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, httpx.Invalid(err.Error())
	}
	if _, err := m.requireConfigured(ctx); err != nil {
		return nil, err
	}
	states := m.c.list(in.Domain, in.Q)
	out := make([]entitySummary, 0, len(states))
	for _, st := range states {
		unit, _ := st.Attributes["unit_of_measurement"].(string)
		out = append(out, entitySummary{EntityID: st.EntityID, Name: friendlyName(st), State: st.State, Unit: unit})
	}
	return out, nil
}

func (m *Module) actionGetState(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct{ EntityID string }
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, httpx.Invalid(err.Error())
	}
	return m.State(ctx, in.EntityID)
}

func (m *Module) actionCall(ctx context.Context, raw json.RawMessage, allowDangerous bool) (any, error) {
	var in struct {
		Domain   string         `json:"domain"`
		Service  string         `json:"service"`
		EntityID string         `json:"entityId"`
		Data     map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, httpx.Invalid(err.Error())
	}
	if !allowDangerous && isDangerous(in.Domain, in.Service, in.EntityID, in.Data) {
		return nil, httpx.NewError(http.StatusForbidden, "dangerous_service", "门锁、安防和开门类操作要用 ha.call_dangerous_service")
	}
	if err := m.callService(ctx, in.Domain, in.Service, in.EntityID, in.Data); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}
