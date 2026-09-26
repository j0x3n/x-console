package homeassistant_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/homeassistant"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// The module is registered in app/modules.go, so testutil.New(t) already
// includes it (passing homeassistant.New again would register it twice).

type haStatus struct {
	Configured  bool
	Connected   bool
	Mode        string
	Version     string
	EntityCount int
	Error       string
}

type haState struct {
	EntityID    string         `json:"entityId"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged time.Time      `json:"lastChanged"`
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func status(env *testutil.Env) haStatus {
	var st haStatus
	env.MustDo(http.MethodGet, "/ha/status", nil, &st)
	return st
}

// setup starts a server, a fake HA and configures the module to use it.
func setup(t *testing.T, token string) (*testutil.Env, *fakeHA) {
	env := testutil.New(t)
	ha := newFakeHA(t)
	env.Elevate()
	env.MustDo(http.MethodPut, "/ha/config", map[string]any{"url": ha.URL() + "/", "token": token, "mode": "direct"}, nil)
	return env, ha
}

func connected(t *testing.T, env *testutil.Env) {
	t.Helper()
	waitFor(t, "connected", func() bool { return status(env).Connected })
}

func subscribe(t *testing.T, env *testutil.Env) <-chan events.Event {
	ch, cancel := env.App.Deps.Bus.Subscribe("ha.", 64)
	t.Cleanup(cancel)
	return ch
}

// next returns the next event with topic, skipping others.
func next(t *testing.T, ch <-chan events.Event, topic string) events.Event {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Topic == topic {
				return ev
			}
		case <-timeout:
			t.Fatalf("no %s event", topic)
		}
	}
}

func errCode(raw []byte) string {
	var e struct{ Code string }
	_ = json.Unmarshal(raw, &e)
	return e.Code
}

func TestNotConfigured(t *testing.T) {
	env := testutil.New(t)
	var cfg struct {
		URL      string `json:"url"`
		HasToken bool   `json:"hasToken"`
		Mode     string `json:"mode"`
	}
	env.MustDo(http.MethodGet, "/ha/config", nil, &cfg)
	if cfg.URL != "" || cfg.HasToken || cfg.Mode != "direct" {
		t.Fatalf("config: %+v", cfg)
	}
	if st := status(env); st.Configured || st.Connected {
		t.Fatalf("status: %+v", st)
	}
	for _, path := range []string{"/ha/states", "/ha/states/light.kitchen"} {
		code, raw := env.Do(http.MethodGet, path, nil, nil)
		if code != http.StatusPreconditionFailed || errCode(raw) != "integration_not_configured" {
			t.Fatalf("%s: %d %s", path, code, raw)
		}
	}
	code, raw := env.Do(http.MethodPost, "/ha/test", nil, nil)
	if code != http.StatusPreconditionFailed {
		t.Fatalf("test: %d %s", code, raw)
	}
	var favs []any
	env.MustDo(http.MethodGet, "/ha/favorites", nil, &favs)
	if len(favs) != 0 {
		t.Fatalf("favorites: %v", favs)
	}
}

func TestConfig(t *testing.T) {
	env := testutil.New(t)
	ha := newFakeHA(t)
	body := map[string]any{"url": ha.URL(), "token": goodToken, "mode": "direct"}
	code, raw := env.Do(http.MethodPut, "/ha/config", body, nil)
	if code != http.StatusForbidden || errCode(raw) != "elevation_required" {
		t.Fatalf("unelevated put: %d %s", code, raw)
	}
	env.Elevate()
	for _, bad := range []map[string]any{
		{"url": "ftp://x", "token": "t", "mode": "direct"},
		{"url": ha.URL(), "mode": "direct"}, // no token stored yet
		{"url": ha.URL(), "token": "t", "mode": "nope"},
	} {
		if code, raw := env.Do(http.MethodPut, "/ha/config", bad, nil); code != http.StatusBadRequest {
			t.Fatalf("bad %v: %d %s", bad, code, raw)
		}
	}
	var cfg struct {
		URL      string `json:"url"`
		Token    string `json:"token"`
		HasToken bool   `json:"hasToken"`
	}
	env.MustDo(http.MethodPut, "/ha/config", body, &cfg)
	if cfg.URL != ha.URL() || !cfg.HasToken || strings.Contains(cfg.Token, goodToken[:10]) || !strings.HasSuffix(cfg.Token, "1234") {
		t.Fatalf("saved config: %+v", cfg)
	}
	var encrypted int
	if err := env.App.Deps.DB.QueryRow(`SELECT encrypted FROM settings WHERE key = 'ha.token'`).Scan(&encrypted); err != nil || encrypted != 1 {
		t.Fatalf("token not encrypted: %d %v", encrypted, err)
	}
	// Saving without a token keeps the old one.
	env.MustDo(http.MethodPut, "/ha/config", map[string]any{"url": ha.URL(), "mode": "direct"}, &cfg)
	if !cfg.HasToken {
		t.Fatal("token lost")
	}
	connected(t, env)

	var res struct {
		Ok           bool
		Message      string
		Version      string
		LocationName string
	}
	env.MustDo(http.MethodPost, "/ha/test", nil, &res)
	if !res.Ok || res.Version != haVersion || res.LocationName != "Home" {
		t.Fatalf("test: %+v", res)
	}
	// Testing form values with the stored token.
	env.MustDo(http.MethodPost, "/ha/test", map[string]any{"url": ha.URL(), "mode": "direct"}, &res)
	if !res.Ok {
		t.Fatalf("test form: %+v", res)
	}

	// An empty address removes the integration.
	env.MustDo(http.MethodPut, "/ha/config", map[string]any{"url": "", "mode": "direct"}, &cfg)
	waitFor(t, "disconnect", func() bool { st := status(env); return !st.Configured && !st.Connected })
}

func TestStatesServicesAndEvents(t *testing.T) {
	env, ha := setup(t, goodToken)
	connected(t, env)
	st := status(env)
	if st.Version != haVersion || st.EntityCount != 4 || st.Mode != "direct" {
		t.Fatalf("status: %+v", st)
	}

	var list []haState
	env.MustDo(http.MethodGet, "/ha/states", nil, &list)
	if len(list) != 4 || list[0].EntityID != "light.kitchen" {
		t.Fatalf("states: %+v", list)
	}
	env.MustDo(http.MethodGet, "/ha/states?domain=sensor", nil, &list)
	if len(list) != 1 || list[0].Attributes["unit_of_measurement"] != "°C" {
		t.Fatalf("sensor states: %+v", list)
	}
	env.MustDo(http.MethodGet, "/ha/states?q=living", nil, &list)
	if len(list) != 1 || list[0].EntityID != "sensor.temperature" {
		t.Fatalf("search: %+v", list)
	}
	var one haState
	env.MustDo(http.MethodGet, "/ha/states/switch.fan", nil, &one)
	if one.State != "on" || one.LastChanged.IsZero() {
		t.Fatalf("state: %+v", one)
	}
	if code, _ := env.Do(http.MethodGet, "/ha/states/light.nope", nil, nil); code != http.StatusNotFound {
		t.Fatalf("missing entity: %d", code)
	}
	if code, _ := env.Do(http.MethodGet, "/ha/states/not-an-id", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("bad entity id: %d", code)
	}

	// Favorites.
	if code, _ := env.Do(http.MethodPut, "/ha/favorites", map[string]any{"items": []map[string]any{{"entityId": "bad"}}}, nil); code != http.StatusBadRequest {
		t.Fatalf("bad favorite: %d", code)
	}
	var favs []struct {
		EntityID  string   `json:"entityId"`
		Alias     string   `json:"alias"`
		SortOrder int      `json:"sortOrder"`
		State     *haState `json:"state"`
	}
	env.MustDo(http.MethodPut, "/ha/favorites", map[string]any{"items": []map[string]any{
		{"entityId": "light.kitchen", "alias": "厨房"}, {"entityId": "sensor.temperature"}}}, &favs)
	if len(favs) != 2 || favs[0].EntityID != "light.kitchen" || favs[0].Alias != "厨房" || favs[0].State == nil || favs[1].SortOrder != 1 {
		t.Fatalf("favorites: %+v", favs)
	}

	ch := subscribe(t, env)
	env.MustDo(http.MethodPost, "/ha/services/light/turn_on", map[string]any{"entityId": "light.kitchen", "data": map[string]any{"brightness_pct": 40}}, nil)
	call := ha.lastCall()
	if call["domain"] != "light" || call["service"] != "turn_on" ||
		call["target"].(map[string]any)["entity_id"] != "light.kitchen" || call["service_data"].(map[string]any)["brightness_pct"] != float64(40) {
		t.Fatalf("call: %v", call)
	}
	ev := next(t, ch, "ha.state_changed")
	got := ev.Data.(contracts.HAState)
	if got.EntityID != "light.kitchen" || got.State != "on" {
		t.Fatalf("event: %+v", got)
	}
	waitFor(t, "cache update", func() bool {
		env.MustDo(http.MethodGet, "/ha/states/light.kitchen", nil, &one)
		return one.State == "on"
	})

	// A non-favorite change is cached but not published.
	ha.push("switch.fan", "off")
	ha.push("sensor.temperature", "22.0")
	ev = next(t, ch, "ha.state_changed")
	if got := ev.Data.(contracts.HAState); got.EntityID != "sensor.temperature" || got.State != "22.0" {
		t.Fatalf("expected only the favorite, got %+v", got)
	}
	env.MustDo(http.MethodGet, "/ha/states/switch.fan", nil, &one)
	if one.State != "off" {
		t.Fatalf("non-favorite not cached: %+v", one)
	}

	// Errors from HA.
	code, raw := env.Do(http.MethodPost, "/ha/services/light/missing_service", map[string]any{"entityId": "light.kitchen"}, nil)
	if code != http.StatusNotFound || errCode(raw) != "ha_not_found" {
		t.Fatalf("missing service: %d %s", code, raw)
	}
	if code, _ := env.Do(http.MethodPost, "/ha/services/Light/turn_on", map[string]any{}, nil); code != http.StatusBadRequest {
		t.Fatalf("bad domain: %d", code)
	}
}

func TestDangerousServiceNeedsElevation(t *testing.T) {
	env, ha := setup(t, goodToken)
	connected(t, env)
	// A fresh login drops the elevation from setup.
	env.MustDo(http.MethodPost, "/auth/logout", nil, nil)
	env.MustDo(http.MethodPost, "/auth/login", map[string]string{"username": testutil.Username, "password": testutil.Password, "code": env.Code()}, nil)

	for _, c := range []struct{ path, entity string }{
		{"/ha/services/lock/unlock", "lock.front_door"},
		{"/ha/services/cover/open_cover", "cover.garage"},
		{"/ha/services/homeassistant/turn_on", "cover.garage"},
	} {
		code, raw := env.Do(http.MethodPost, c.path, map[string]any{"entityId": c.entity}, nil)
		if code != http.StatusForbidden || errCode(raw) != "elevation_required" {
			t.Fatalf("%s: %d %s", c.path, code, raw)
		}
	}
	// Harmless services do not need elevation.
	env.MustDo(http.MethodPost, "/ha/services/cover/close_cover", map[string]any{"entityId": "cover.garage"}, nil)
	env.Elevate()
	env.MustDo(http.MethodPost, "/ha/services/lock/unlock", map[string]any{"entityId": "lock.front_door"}, nil)
	if call := ha.lastCall(); call["service"] != "unlock" {
		t.Fatalf("call: %v", call)
	}
}

func TestContractAndWatchEntity(t *testing.T) {
	env, ha := setup(t, goodToken)
	connected(t, env)
	svc, ok := module.Lookup[contracts.HomeAssistant](env.App.Deps.Registry, contracts.HomeAssistantKey)
	if !ok {
		t.Fatal("contracts.HomeAssistant not provided")
	}
	ctx := context.Background()
	st, err := svc.State(ctx, "sensor.temperature")
	if err != nil || st.State != "21.5" {
		t.Fatalf("State: %+v %v", st, err)
	}
	ch := subscribe(t, env)
	svc.WatchEntity("switch.fan")
	ha.push("light.kitchen", "on") // not watched, not a favorite
	ha.push("switch.fan", "off")
	ev := next(t, ch, "ha.state_changed")
	if got := ev.Data.(contracts.HAState); got.EntityID != "switch.fan" || got.State != "off" {
		t.Fatalf("watched event: %+v", got)
	}
	if err := svc.CallService(ctx, "switch", "turn_on", map[string]any{"entity_id": "switch.fan"}); err != nil {
		t.Fatal(err)
	}
	ev = next(t, ch, "ha.state_changed")
	if got := ev.Data.(contracts.HAState); got.EntityID != "switch.fan" || got.State != "on" {
		t.Fatalf("event after CallService: %+v", got)
	}
}

func TestReconnect(t *testing.T) {
	env, ha := setup(t, goodToken)
	connected(t, env)
	env.MustDo(http.MethodPut, "/ha/favorites", map[string]any{"items": []map[string]any{{"entityId": "light.kitchen"}}}, nil)
	ch := subscribe(t, env)

	ha.dropAll()
	ev := next(t, ch, "ha.connection_changed")
	if ev.Data.(homeassistant.ConnectionChanged).Connected {
		t.Fatalf("expected disconnect, got %+v", ev.Data)
	}
	// While disconnected the favorite changes; after reconnecting the
	// snapshot updates the cache and the change is published.
	ha.set("light.kitchen", "on", nil)
	ev = next(t, ch, "ha.state_changed")
	if got := ev.Data.(contracts.HAState); got.EntityID != "light.kitchen" || got.State != "on" {
		t.Fatalf("missed change: %+v", got)
	}
	connected(t, env)
	if ok, _ := ha.counts(); ok != 2 {
		t.Fatalf("auth count %d", ok)
	}
	// The new session still receives events.
	ha.push("light.kitchen", "off")
	ev = next(t, ch, "ha.state_changed")
	if got := ev.Data.(contracts.HAState); got.State != "off" {
		t.Fatalf("after reconnect: %+v", got)
	}
}

func TestBadToken(t *testing.T) {
	env, ha := setup(t, "wrong-token-abcdefgh")
	waitFor(t, "auth error", func() bool { return status(env).Error != "" })
	st := status(env)
	if st.Connected || !strings.Contains(st.Error, "令牌无效") {
		t.Fatalf("status: %+v", st)
	}
	var res struct {
		Ok      bool
		Message string
	}
	env.MustDo(http.MethodPost, "/ha/test", nil, &res)
	if res.Ok || !strings.Contains(res.Message, "令牌无效") {
		t.Fatalf("test: %+v", res)
	}
	// A rejected token is not retried (HA bans IPs after failed logins).
	time.Sleep(1500 * time.Millisecond)
	if _, fail := ha.counts(); fail != 1 {
		t.Fatalf("auth retried %d times", fail)
	}
	// Fixing the token reconnects right away.
	env.MustDo(http.MethodPut, "/ha/config", map[string]any{"url": ha.URL(), "token": goodToken, "mode": "direct"}, nil)
	connected(t, env)
}

func TestUnreachable(t *testing.T) {
	env := testutil.New(t)
	env.Elevate()
	env.MustDo(http.MethodPut, "/ha/config", map[string]any{"url": "http://127.0.0.1:1", "token": goodToken, "mode": "direct"}, nil)
	waitFor(t, "error", func() bool { return status(env).Error != "" })
	if st := status(env); !strings.Contains(st.Error, "连不上") {
		t.Fatalf("status: %+v", st)
	}
	var res struct {
		Ok      bool
		Message string
	}
	env.MustDo(http.MethodPost, "/ha/test", nil, &res)
	if res.Ok || res.Message == "" {
		t.Fatalf("test: %+v", res)
	}
	code, raw := env.Do(http.MethodPost, "/ha/services/light/turn_on", map[string]any{"entityId": "light.kitchen"}, nil)
	if code != http.StatusServiceUnavailable || errCode(raw) != "ha_unavailable" {
		t.Fatalf("call while offline: %d %s", code, raw)
	}
}

func TestActions(t *testing.T) {
	env, ha := setup(t, goodToken)
	connected(t, env)
	reg := env.App.Deps.Actions
	ctx := context.Background()
	for name, effect := range map[string]string{"ha.list_entities": "read", "ha.get_state": "read", "ha.call_service": "write", "ha.call_dangerous_service": "dangerous"} {
		a, ok := reg.Get(name)
		if !ok || string(a.Effect) != effect {
			t.Fatalf("action %s: %v %s", name, ok, a.Effect)
		}
	}
	out, err := reg.Run(ctx, "ha.list_entities", json.RawMessage(`{"domain":"sensor"}`))
	raw, _ := json.Marshal(out)
	if err != nil || !strings.Contains(string(raw), `"unit":"°C"`) || !strings.Contains(string(raw), "Living room temperature") {
		t.Fatalf("list: %s %v", raw, err)
	}
	out, err = reg.Run(ctx, "ha.get_state", json.RawMessage(`{"entityId":"switch.fan"}`))
	if st, ok := out.(contracts.HAState); err != nil || !ok || st.State != "on" {
		t.Fatalf("get_state: %+v %v", out, err)
	}
	if _, err := reg.Run(ctx, "ha.call_service", json.RawMessage(`{"domain":"switch","service":"turn_off","entityId":"switch.fan"}`)); err != nil {
		t.Fatal(err)
	}
	if call := ha.lastCall(); call["service"] != "turn_off" {
		t.Fatalf("call: %v", call)
	}
	if _, err := reg.Run(ctx, "ha.call_service", json.RawMessage(`{"domain":"lock","service":"unlock","entityId":"lock.front_door"}`)); err == nil || !strings.Contains(err.Error(), "dangerous_service") {
		t.Fatalf("dangerous via write action: %v", err)
	}
	if _, err := reg.Run(ctx, "ha.call_dangerous_service", json.RawMessage(`{"domain":"lock","service":"unlock","entityId":"lock.front_door"}`)); err != nil {
		t.Fatal(err)
	}
	if call := ha.lastCall(); call["service"] != "unlock" {
		t.Fatalf("call: %v", call)
	}
}
