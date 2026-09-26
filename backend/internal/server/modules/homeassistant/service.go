package homeassistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/homeassistant/api"
)

var (
	namePattern   = regexp.MustCompile(`^[a-z0-9_]+$`)
	entityPattern = regexp.MustCompile(`^[a-z0-9_]+\.[a-z0-9_]+$`)
)

func validEntityID(id string) error {
	if !entityPattern.MatchString(id) {
		return httpx.Invalid("实体 id 格式不对，应该像 light.living_room")
	}
	return nil
}

// State implements contracts.HomeAssistant.
func (m *Module) State(ctx context.Context, entityID string) (contracts.HAState, error) {
	if _, err := m.requireConfigured(ctx); err != nil {
		return contracts.HAState{}, err
	}
	if err := validEntityID(entityID); err != nil {
		return contracts.HAState{}, err
	}
	if st, ok := m.c.state(entityID); ok {
		return st, nil
	}
	if m.c.session() == nil {
		return contracts.HAState{}, errNotConnected
	}
	return contracts.HAState{}, httpx.ErrNotFound
}

// CallService implements contracts.HomeAssistant. Put the target in
// data["entity_id"]. Callers check confirmation and elevation themselves.
func (m *Module) CallService(ctx context.Context, domain, service string, data map[string]any) error {
	return m.callService(ctx, domain, service, "", data)
}

// WatchEntity implements contracts.HomeAssistant.
func (m *Module) WatchEntity(entityID string) { m.c.watch(entityID) }

// callService validates, calls HA and writes the audit log.
func (m *Module) callService(ctx context.Context, domain, service, entityID string, data map[string]any) (err error) {
	if !namePattern.MatchString(domain) || !namePattern.MatchString(service) {
		return httpx.Invalid("服务名格式不对，应该像 light.turn_on")
	}
	if entityID != "" {
		if err := validEntityID(entityID); err != nil {
			return err
		}
	}
	if _, err := m.requireConfigured(ctx); err != nil {
		return err
	}
	defer func() {
		detail := map[string]any{"entityId": entityID}
		if len(data) > 0 {
			detail["data"] = data
		}
		m.d.Audit.Record(ctx, "ha.call_service", domain+"."+service, detail, err)
	}()
	cmd := map[string]any{"type": "call_service", "domain": domain, "service": service}
	if len(data) > 0 {
		cmd["service_data"] = data
	}
	if entityID != "" {
		cmd["target"] = map[string]any{"entity_id": entityID}
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err = m.c.call(cctx, cmd)
	return haToAPIError(err)
}

func haToAPIError(err error) error {
	var he *haError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &he):
		switch he.Code {
		case "not_found":
			return httpx.NewError(http.StatusNotFound, "ha_not_found", "Home Assistant 找不到: "+he.Message)
		case "invalid_format", "invalid_info":
			return httpx.NewError(http.StatusBadRequest, "ha_invalid", "Home Assistant 不接受这些参数: "+he.Message)
		default:
			return httpx.NewError(http.StatusBadGateway, "ha_error", "Home Assistant 返回错误: "+he.Message)
		}
	case errors.Is(err, errDisconnected):
		return errNotConnected
	case errors.Is(err, context.DeadlineExceeded):
		return httpx.NewError(http.StatusGatewayTimeout, "ha_timeout", "Home Assistant 响应超时")
	default:
		return err
	}
}

// Dangerous services unlock doors, disarm alarms or open covers (garage
// doors, gates). They need elevation in the UI and the dangerous action.
var dangerousCover = map[string]bool{
	"open_cover": true, "open_cover_tilt": true, "set_cover_position": true,
	"set_cover_tilt_position": true, "toggle": true, "toggle_cover_tilt": true,
}

func isDangerous(domain, service, entityID string, data map[string]any) bool {
	switch domain {
	case "lock", "alarm_control_panel":
		return true
	case "cover":
		return dangerousCover[service]
	}
	// Generic services such as homeassistant.turn_on act on whatever they target.
	if domain == "homeassistant" {
		for _, k := range []string{"area_id", "device_id", "floor_id", "label_id"} {
			if _, ok := data[k]; ok {
				return true
			}
		}
	}
	for _, id := range targetEntities(entityID, data) {
		d, _, _ := strings.Cut(id, ".")
		if d == "lock" || d == "alarm_control_panel" || (d == "cover" && (service == "turn_on" || service == "toggle" || dangerousCover[service])) {
			return true
		}
	}
	return false
}

func targetEntities(entityID string, data map[string]any) []string {
	var out []string
	if entityID != "" {
		out = append(out, entityID)
	}
	switch v := data["entity_id"].(type) {
	case string:
		for _, s := range strings.Split(v, ",") {
			out = append(out, strings.TrimSpace(s))
		}
	case []any:
		for _, s := range v {
			if str, ok := s.(string); ok {
				out = append(out, str)
			}
		}
	}
	return out
}

// test checks REST (/api/config) and the WebSocket login for cfg.
func (m *Module) test(ctx context.Context, cfg config) api.HATestResult {
	fail := func(msg string) api.HATestResult { return api.HATestResult{Ok: false, Message: &msg} }
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	tr := m.transport(cfg)
	status, body, err := tr.do(ctx, http.MethodGet, cfg.URL+"/api/config",
		http.Header{"Authorization": {"Bearer " + cfg.Token}, "Accept": {"application/json"}}, nil)
	if err != nil {
		return fail(describe(err))
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fail((&authError{}).Error())
	case status != http.StatusOK:
		return fail(fmt.Sprintf("Home Assistant 返回 HTTP %d。请检查地址是否正确", status))
	}
	var info struct {
		Version      string `json:"version"`
		LocationName string `json:"location_name"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return fail("这个地址不像 Home Assistant，返回的内容无法识别")
	}
	conn, version, err := dialAndAuth(ctx, tr, cfg)
	if err != nil {
		var auth *authError
		if errors.As(err, &auth) {
			return fail(auth.Error())
		}
		return fail("REST 接口正常，但 WebSocket 连不上。如果用了反向代理，请确认它转发 WebSocket。" + describe(err))
	}
	_ = conn.Close()
	if version == "" {
		version = info.Version
	}
	return api.HATestResult{Ok: true, Version: &version, LocationName: &info.LocationName}
}
