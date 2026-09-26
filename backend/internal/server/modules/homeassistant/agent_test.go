package homeassistant_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/netproxy"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/homeassistant"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func proxyAgent(env *testutil.Env) string {
	return env.Agent("server", []string{protocol.CapProxy}, func(c *conn.Client) {
		c.Handle(protocol.MethodHTTPProxy, netproxy.HTTP)
		c.HandleStream(protocol.MethodWSProxy, netproxy.WS)
	})
}

// TestViaAgent reaches the fake HA only through the agent's http.proxy and
// ws.proxy, including a get_states answer larger than one proxy frame.
func TestViaAgent(t *testing.T) {
	env := testutil.New(t)
	ha := newFakeHA(t)
	big := strings.Repeat("x", protocol.WSProxyChunk+1000)
	ha.set("sensor.big", "1", map[string]any{"blob": big})
	agentID := proxyAgent(env)

	env.MustDo(http.MethodPut, "/ha/config", map[string]any{"url": ha.URL(), "token": goodToken, "mode": "agent", "agentId": agentID}, nil)
	connected(t, env)
	st := status(env)
	if st.Mode != "agent" || st.EntityCount != 5 {
		t.Fatalf("status: %+v", st)
	}
	var one haState
	env.MustDo(http.MethodGet, "/ha/states/sensor.big", nil, &one)
	if one.Attributes["blob"] != big {
		t.Fatalf("big attribute lost: %d bytes", len(one.Attributes["blob"].(string)))
	}

	var res struct {
		Ok      bool
		Message string
		Version string
	}
	env.MustDo(http.MethodPost, "/ha/test", nil, &res)
	if !res.Ok || res.Version != haVersion {
		t.Fatalf("test via agent: %+v", res)
	}

	env.MustDo(http.MethodPut, "/ha/favorites", map[string]any{"items": []map[string]any{{"entityId": "light.kitchen"}}}, nil)
	ch := subscribe(t, env)
	env.MustDo(http.MethodPost, "/ha/services/light/turn_on", map[string]any{"entityId": "light.kitchen"}, nil)
	if got := next(t, ch, "ha.state_changed").Data.(contracts.HAState); got.State != "on" {
		t.Fatalf("event via agent: %+v", got)
	}

	// HA restarts: the agent stream ends and the module reconnects.
	ha.dropAll()
	if next(t, ch, "ha.connection_changed").Data.(homeassistant.ConnectionChanged).Connected {
		t.Fatal("expected disconnect")
	}
	connected(t, env)

	// The bad token message also comes through the agent.
	env.MustDo(http.MethodPost, "/ha/test", map[string]any{"url": ha.URL(), "token": "nope-nope-nope", "mode": "agent", "agentId": agentID}, &res)
	if res.Ok || !strings.Contains(res.Message, "令牌无效") {
		t.Fatalf("bad token via agent: %+v", res)
	}
}

func TestAgentModeValidation(t *testing.T) {
	env := testutil.New(t)
	ha := newFakeHA(t)
	plain := env.Agent("server", nil, nil) // no proxy capability
	cases := []map[string]any{
		{"url": ha.URL(), "token": goodToken, "mode": "agent"},
		{"url": ha.URL(), "token": goodToken, "mode": "agent", "agentId": "missing"},
		{"url": ha.URL(), "token": goodToken, "mode": "agent", "agentId": plain},
	}
	for _, body := range cases {
		code, raw := env.Do(http.MethodPut, "/ha/config", body, nil)
		if code != http.StatusBadRequest {
			t.Fatalf("%v: %d %s", body, code, raw)
		}
	}
}

// The agent refuses targets outside private networks.
func TestAgentRefusesPublicTarget(t *testing.T) {
	env := testutil.New(t)
	agentID := proxyAgent(env)
	var res protocol.HTTPProxyResult
	err := env.App.Deps.Agents.Call(context.Background(), agentID, protocol.MethodHTTPProxy,
		protocol.HTTPProxyParams{URL: "http://8.8.8.8/api/"}, &res)
	if err == nil || !strings.Contains(err.Error(), "forbidden_target") {
		t.Fatalf("public target: %v", err)
	}
	env.MustDo(http.MethodPut, "/ha/config", map[string]any{"url": "http://8.8.8.8:8123", "token": goodToken, "mode": "agent", "agentId": agentID}, nil)
	var out struct {
		Ok      bool
		Message string
	}
	env.MustDo(http.MethodPost, "/ha/test", nil, &out)
	if out.Ok || !strings.Contains(out.Message, "内网") {
		raw, _ := json.Marshal(out)
		t.Fatalf("test: %s", raw)
	}
}
