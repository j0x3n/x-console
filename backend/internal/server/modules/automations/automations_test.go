package automations_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/automations/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type counterModule struct{}

func (counterModule) Name() string     { return "automation-counter" }
func (counterModule) Mount(chi.Router) {}

var count atomic.Int32

func counter(d *module.Deps) (module.Module, error) {
	d.Actions.Register(actions.Action{Name: "test.count", Title: "计数", Description: "Count runs", Input: actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`), Effect: actions.Write, Run: func(context.Context, json.RawMessage) (any, error) { return count.Add(1), nil }})
	return counterModule{}, nil
}
func waitCount(t *testing.T, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if count.Load() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("runs=%d want=%d", count.Load(), want)
}
func TestMetricCooldownAndWebhookSecret(t *testing.T) {
	count.Store(0)
	env := testutil.New(t, counter)
	input := map[string]any{"name": "CPU 告警", "enabled": true, "trigger": map[string]any{"type": "metric", "metric": "cpu", "op": ">", "value": 90}, "conditions": []any{}, "actions": []any{map[string]any{"action": "test.count", "input": map[string]any{}}}, "cooldownSeconds": 60}
	var rule api.Automation
	env.MustDo("POST", "/automations", input, &rule)
	for i := 0; i < 3; i++ {
		env.App.Deps.Bus.Publish("host.metrics", map[string]any{"hostId": "h1", "sample": map[string]any{"cpu": 95}})
	}
	waitCount(t, 1)
	var runs []api.Run
	env.MustDo("GET", fmt.Sprintf("/automations/%d/runs", rule.Id), nil, &runs)
	if len(runs) != 1 || runs[0].Status != api.Ok {
		t.Fatalf("runs=%+v", runs)
	}
	input["trigger"] = map[string]any{"type": "webhook"}
	input["name"] = "Webhook"
	input["cooldownSeconds"] = 0
	env.MustDo("POST", "/automations", input, &rule)
	if rule.Trigger.WebhookPath == nil || !strings.HasPrefix(*rule.Trigger.WebhookPath, "/hooks/") {
		t.Fatalf("webhook path: %+v", rule.Trigger)
	}
	bad, _ := http.NewRequest("POST", env.URL("/hooks/not-a-token"), strings.NewReader(`{}`))
	res, err := env.Client.Do(bad)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("bad token: %d", res.StatusCode)
	}
	good, _ := http.NewRequest("POST", env.URL(*rule.Trigger.WebhookPath), strings.NewReader(`{"value":1}`))
	good.Header.Set("Content-Type", "application/json")
	res, err = env.Client.Do(good)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 202 {
		t.Fatalf("good token: %d", res.StatusCode)
	}
	waitCount(t, 2)
	// A webhook URL remains stable when the authenticated owner reads the rule.
	var fetched api.Automation
	env.MustDo("GET", fmt.Sprintf("/automations/%d", rule.Id), nil, &fetched)
	if *fetched.Trigger.WebhookPath != *rule.Trigger.WebhookPath {
		t.Fatal("webhook path changed")
	}
	// Editing the rule keeps the URL that outside systems already use.
	input["name"] = "Webhook 改名"
	var edited api.Automation
	env.MustDo("PUT", fmt.Sprintf("/automations/%d", rule.Id), input, &edited)
	if edited.Trigger.WebhookPath == nil || *edited.Trigger.WebhookPath != *rule.Trigger.WebhookPath {
		t.Fatalf("webhook path changed on edit: %v", edited.Trigger.WebhookPath)
	}
	input["trigger"] = map[string]any{"type": "ha_state", "entityId": "switch.test", "to": "on"}
	input["name"] = "智能家居状态"
	env.MustDo("POST", "/automations", input, &rule)
	env.App.Deps.Bus.Publish("ha.state_changed", map[string]any{"entityId": "switch.test", "state": "off"})
	time.Sleep(30 * time.Millisecond)
	if count.Load() != 2 {
		t.Fatal("不匹配的智能家居状态触发了规则")
	}
	env.App.Deps.Bus.Publish("ha.state_changed", map[string]any{"entityId": "switch.test", "state": "on"})
	waitCount(t, 3)
}
