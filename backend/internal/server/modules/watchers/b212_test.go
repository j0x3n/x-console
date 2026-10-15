package watchers_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/api"
)

// B212: the models chosen in the watcher settings reach the AI calls.
// Exploring uses the page model, judging a change uses the judge model.
func TestChosenModelsReachTheAI(t *testing.T) {
	f := setup(t, true)
	var set api.WatchersSettings
	f.env.MustDo(http.MethodPut, "/watchers/settings", map[string]any{"budgetUsd": 1, "dangerWords": []string{"提交"},
		"agentModel": " 3:vision-model ", "agentEffort": "high", "judgeModel": "4:cheap", "judgeEffort": "off"}, &set)
	if *set.AgentModel != "3:vision-model" || *set.AgentEffort != api.High || *set.JudgeModel != "4:cheap" || *set.JudgeEffort != api.Off {
		t.Fatalf("saved %+v", set)
	}
	// Leaving the fields out keeps them. An empty model follows Settings → AI.
	f.env.MustDo(http.MethodPut, "/watchers/settings", map[string]any{"budgetUsd": 1, "dangerWords": []string{"提交"}, "judgeModel": "", "judgeEffort": ""}, &set)
	if *set.AgentModel != "3:vision-model" || *set.AgentEffort != api.High || *set.JudgeModel != "" || *set.JudgeEffort != api.Empty {
		t.Fatalf("partial save %+v", set)
	}
	for _, bad := range []map[string]any{{"agentModel": "vision-model"}, {"agentModel": "0:x"}, {"judgeEffort": "max"}} {
		bad["budgetUsd"], bad["dangerWords"] = 1, []string{}
		if status, _ := f.env.Do(http.MethodPut, "/watchers/settings", bad, nil); status != http.StatusBadRequest {
			t.Fatalf("%v: %d", bad, status)
		}
	}
	f.env.MustDo(http.MethodPut, "/watchers/settings", map[string]any{"budgetUsd": 1, "dangerWords": []string{"提交"}, "judgeModel": "4:cheap", "judgeEffort": "low"}, nil)

	f.chat.script(append(loginMoves(), finish("审核中"))...)
	w := f.create(t, map[string]any{"name": "签证", "url": f.site.URL + "/status", "goal": "盯办理状态", "username": "alice", "password": "s3cret"})
	f.until(t, w.Id, api.WatcherStatusOk)
	watchers.WaitRuns(f.m)
	f.site.set(func(s *site) { s.value = "已受理" })
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
	watchers.WaitRuns(f.m)

	page := contracts.AIModel{Model: "3:vision-model", Effort: "high"}
	judge := contracts.AIModel{Model: "4:cheap", Effort: "low"}
	if len(f.chat.models) == 0 || len(f.llm.models) != 1 {
		t.Fatalf("calls: chat %d, judge %d", len(f.chat.models), len(f.llm.models))
	}
	for _, got := range f.chat.models {
		if got != page {
			t.Fatalf("explore used %+v", got)
		}
	}
	if f.llm.models[0] != judge {
		t.Fatalf("judge used %+v", f.llm.models[0])
	}
}
