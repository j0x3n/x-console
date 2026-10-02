package aiagents_test

import (
	"fmt"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestBuiltinRunLogsAndCancellation(t *testing.T) {
	env := testutil.New(t)
	runner := &fakeRunner{release: make(chan struct{})}
	module.Provide[contracts.ToolRunner](env.App.Deps.Registry, contracts.ToolRunnerKey, runner)
	c := newCard(t, env)
	var a agent
	env.MustDo("POST", "/ai-agents", map[string]any{"name": "整理员", "kind": "builtin", "model": "1:model"}, &a)
	env.MustDo("POST", fmt.Sprintf("/ai-agents/%d/assign", a.ID), map[string]any{"issueKey": c.Key}, nil)
	var runs []api.AiAgentRun
	waitFor(t, func() bool {
		env.MustDo("GET", "/ai-agents/runs?issueKey="+c.Key, nil, &runs)
		return len(runs) == 1 && runs[0].Status == api.Running
	})
	var events struct {
		Items   []api.AiAgentRunEvent
		LastSeq int64
	}
	path := fmt.Sprintf("/ai-agents/runs/%d", runs[0].Id)
	waitFor(t, func() bool { env.MustDo("GET", path+"/events", nil, &events); return len(events.Items) >= 2 })
	for i, e := range events.Items {
		if i > 0 && e.Seq <= events.Items[i-1].Seq {
			t.Fatal("unordered events")
		}
	}
	env.MustDo("GET", fmt.Sprintf("%s/events?after=%d", path, events.LastSeq), nil, &events)
	if len(events.Items) != 0 {
		t.Fatal("after filter")
	}
	env.Elevate()
	env.MustDo("POST", path+"/cancel", nil, nil)
	waitFor(t, func() bool { env.MustDo("GET", "/ai-agents/runs", nil, &runs); return runs[0].Status == api.Canceled })
	if s, _ := env.Do("GET", "/ai-agents/runs/999/events", nil, nil); s != 404 {
		t.Fatal(s)
	}
	if s, _ := env.Do("POST", path+"/cancel", nil, nil); s != 409 {
		t.Fatal(s)
	}
}
