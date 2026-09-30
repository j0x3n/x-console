package automations_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/automations/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type echoModule struct{}

func (echoModule) Name() string     { return "automation-echo" }
func (echoModule) Mount(chi.Router) {}

var echoes atomic.Int32

// echo registers an action that publishes the event its rule listens to.
func echo(d *module.Deps) (module.Module, error) {
	d.Actions.Register(actions.Action{Name: "test.echo", Title: "回声", Description: "Publish test.loop", Input: actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`), Effect: actions.Write, Run: func(context.Context, json.RawMessage) (any, error) {
		echoes.Add(1)
		d.Bus.Publish("test.loop", map[string]any{})
		return nil, nil
	}})
	return echoModule{}, nil
}

func TestLoopingRuleIsStopped(t *testing.T) {
	echoes.Store(0)
	env := testutil.New(t, echo)
	var rule api.Automation
	env.MustDo("POST", "/automations", map[string]any{"name": "自己触发自己", "enabled": true,
		"trigger": map[string]any{"type": "event", "topic": "test.loop"}, "conditions": []any{},
		"actions": []any{map[string]any{"action": "test.echo", "input": map[string]any{}}}, "cooldownSeconds": 0}, &rule)
	env.App.Deps.Bus.Publish("test.loop", map[string]any{})
	deadline := time.Now().Add(3 * time.Second)
	for {
		var got api.Automation
		env.MustDo("GET", fmt.Sprintf("/automations/%d", rule.Id), nil, &got)
		if !got.Enabled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("looping rule still enabled after %d runs", echoes.Load())
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	settled := echoes.Load()
	if settled > 25 {
		t.Fatalf("rule ran %d times before it was stopped", settled)
	}
	env.App.Deps.Bus.Publish("test.loop", map[string]any{})
	time.Sleep(100 * time.Millisecond)
	if echoes.Load() != settled {
		t.Fatal("a stopped rule still runs")
	}
	var notified int
	if err := env.App.Deps.DB.QueryRow(`SELECT count(*) FROM notifications WHERE kind='automation.loop'`).Scan(&notified); err != nil || notified != 1 {
		t.Fatalf("loop notification: %d %v", notified, err)
	}
}

func TestEventRulesFollowToggle(t *testing.T) {
	count.Store(0)
	env := testutil.New(t, counter)
	var rule api.Automation
	env.MustDo("POST", "/automations", map[string]any{"name": "事件", "enabled": true,
		"trigger": map[string]any{"type": "event", "topic": "test.toggle"}, "conditions": []any{},
		"actions": []any{map[string]any{"action": "test.count", "input": map[string]any{}}}, "cooldownSeconds": 0}, &rule)
	env.App.Deps.Bus.Publish("test.toggle", map[string]any{})
	waitCount(t, 1)
	env.MustDo("PATCH", fmt.Sprintf("/automations/%d", rule.Id), map[string]any{"enabled": false}, nil)
	env.App.Deps.Bus.Publish("test.toggle", map[string]any{})
	time.Sleep(50 * time.Millisecond)
	if count.Load() != 1 {
		t.Fatal("a disabled rule still runs")
	}
	env.MustDo("PATCH", fmt.Sprintf("/automations/%d", rule.Id), map[string]any{"enabled": true}, nil)
	env.App.Deps.Bus.Publish("test.toggle", map[string]any{})
	waitCount(t, 2)
}

func TestMetricRuleFiresOnCrossing(t *testing.T) {
	count.Store(0)
	env := testutil.New(t, counter)
	env.MustDo("POST", "/automations", map[string]any{"name": "CPU 高", "enabled": true,
		"trigger": map[string]any{"type": "metric", "metric": "cpu", "op": ">", "value": 90}, "conditions": []any{},
		"actions": []any{map[string]any{"action": "test.count", "input": map[string]any{}}}, "cooldownSeconds": 0}, nil)
	sample := func(host string, cpu float64) {
		env.App.Deps.Bus.Publish("host.metrics", map[string]any{"hostId": host, "sample": map[string]any{"cpu": cpu}})
		time.Sleep(20 * time.Millisecond)
	}
	for i := 0; i < 5; i++ {
		sample("h1", 95) // stays high: one run, not five
	}
	waitCount(t, 1)
	sample("h2", 95) // another host crosses on its own
	waitCount(t, 2)
	sample("h1", 50)
	sample("h1", 96) // back down and up again
	waitCount(t, 3)
	time.Sleep(50 * time.Millisecond)
	if count.Load() != 3 {
		t.Fatalf("runs=%d", count.Load())
	}
}
