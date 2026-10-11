package brief_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief"
)

type fakePlan struct {
	status string
	lines  []string
	err    error
}

func (f fakePlan) Lines(context.Context, string) (string, []string, error) {
	return f.status, f.lines, f.err
}

// B135: the brief lists the day plan; a draft says so; no plan, no section.
func TestBriefPlanSection(t *testing.T) {
	env, m, ws := setup(t)
	s := baseSettings(ws)
	s["sections"] = []string{"plan"}
	putSettings(t, env, s)

	gen := func(p contracts.DayPlanSource) (string, []string) {
		t.Helper()
		reg := module.NewRegistry()
		if p != nil {
			module.Provide[contracts.DayPlanSource](reg, contracts.DayPlanSourceKey, p)
		}
		content, keys, err := brief.Generate(m, context.Background(), reg, at(27, 8, 0))
		if err != nil {
			t.Fatal(err)
		}
		return content, keys
	}
	content, keys := gen(fakePlan{status: "confirmed", lines: []string{"09:00–10:00 写周报", "15:00–15:30 给医生打电话"}})
	if strings.Join(keys, ",") != "plan" || !strings.Contains(content, "## 今天最重要的几件事\n\n1. 09:00–10:00 写周报\n2. 15:00–15:30 给医生打电话") || strings.Contains(content, "草稿") {
		t.Fatalf("confirmed: %v\n%s", keys, content)
	}
	content, _ = gen(fakePlan{status: "draft", lines: []string{"写周报"}})
	if !strings.Contains(content, "（草稿，还没确认）\n1. 写周报") {
		t.Fatalf("draft:\n%s", content)
	}
	for name, p := range map[string]contracts.DayPlanSource{"no plan": fakePlan{}, "error": fakePlan{err: errors.New("boom")}, "no module": nil} {
		if _, keys := gen(p); len(keys) != 0 {
			t.Errorf("%s: %v", name, keys)
		}
	}
}
