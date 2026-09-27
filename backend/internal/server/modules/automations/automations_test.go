package automations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/automations/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type testModule struct{}

func (*testModule) Name() string     { return "automation_test_action" }
func (*testModule) Mount(chi.Router) {}

func setup(t *testing.T, effect actions.Effect) (*testutil.Env, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	env := testutil.New(t, func(d *module.Deps) (module.Module, error) {
		d.Actions.Register(actions.Action{
			Name: "test.mark", Title: "测试", Description: "record a test call",
			Input: actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`), Effect: effect,
			Run: func(context.Context, json.RawMessage) (any, error) {
				calls.Add(1)
				return map[string]any{"ok": true}, nil
			},
		})
		return &testModule{}, nil
	})
	return env, calls
}

func TestMetricCooldownAndWebhook(t *testing.T) {
	env, calls := setup(t, actions.Write)
	var rule api.Automation
	env.MustDo(http.MethodPost, "/automations", map[string]any{
		"name": "CPU 告警", "enabled": true,
		"trigger":    map[string]any{"type": "metric", "topic": "host.metrics", "field": "data.cpu", "op": "gt", "value": 90},
		"conditions": []any{}, "actions": []any{map[string]any{"action": "test.mark", "input": map[string]any{}}},
		"cooldownSeconds": 3600,
	}, &rule)
	env.App.Deps.Bus.Publish("host.metrics", map[string]any{"cpu": 95})
	env.App.Deps.Bus.Publish("host.metrics", map[string]any{"cpu": 96})
	deadline := time.After(3 * time.Second)
	for calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("automation did not run")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	time.Sleep(150 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("cooldown allowed %d runs", got)
	}
	var runs []api.AutomationRun
	env.MustDo(http.MethodGet, "/automations/"+rule.Id+"/runs", nil, &runs)
	if len(runs) != 1 || runs[0].Status != "done" {
		t.Fatalf("runs: %+v", runs)
	}

	var hook api.Automation
	env.MustDo(http.MethodPost, "/automations", map[string]any{
		"name": "Webhook", "enabled": true, "trigger": map[string]any{"type": "webhook"},
		"conditions": []any{}, "actions": []any{map[string]any{"action": "test.mark", "input": map[string]any{}}}, "cooldownSeconds": 0,
	}, &hook)
	if hook.WebhookUrl == nil {
		t.Fatal("missing webhook URL")
	}
	resp, err := http.Post(env.Server.URL+"/api/v1/hooks/bad-token", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("bad token status %d", resp.StatusCode)
	}
	resp, err = http.Post(env.Server.URL+*hook.WebhookUrl, "application/json", bytes.NewBufferString(`{"source":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("valid webhook status %d", resp.StatusCode)
	}
}

func TestDangerousRuleNeedsElevation(t *testing.T) {
	env, _ := setup(t, actions.Dangerous)
	body := map[string]any{"name": "Danger", "enabled": true, "trigger": map[string]any{"type": "event", "topic": "test."}, "conditions": []any{}, "actions": []any{map[string]any{"action": "test.mark", "input": map[string]any{}}}, "cooldownSeconds": 0}
	status, _ := env.Do(http.MethodPost, "/automations", body, nil)
	if status != http.StatusForbidden {
		t.Fatalf("without elevation: %d", status)
	}
	env.Elevate()
	env.MustDo(http.MethodPost, "/automations", body, nil)
}
