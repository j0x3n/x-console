package codingroi

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
)

// actionTasks is how many tasks the action returns. The totals and groups
// always cover every task of the week.
const actionTasks = 30

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "codingroi.weekly",
		Title: "查看 AI 编码的投入产出",
		Description: "Cost report of the user's AI coding tasks (Claude Code and Codex) for one week: per group (repository, tool or agent) and in total, " +
			"the number of tasks, how many were merged, reverted, canceled or failed, the cost in USD, tokens, how long the AI ran, " +
			"the user's review time (an estimate from computer time), rework rounds and the ratios per merge. " +
			"A value that is null has no record. It is not zero. Codex does not report a cost, so its cost is null. " +
			"Optional `date` (YYYY-MM-DD, any day in the week; default today) and `groupBy` (repo, tool or agent; default repo). Returns at most 30 tasks.",
		Input:  actions.Schema(`{"type":"object","properties":{"date":{"type":"string"},"groupBy":{"type":"string","enum":["repo","tool","agent"]}},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Date    string `json:"date"`
				GroupBy string `json:"groupBy"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, err
				}
			}
			q, err := parseQuery(m.location(), "week", in.Date, in.GroupBy, m.now())
			if err != nil {
				return nil, err
			}
			rep, err := m.report(ctx, q)
			if err != nil {
				return nil, err
			}
			if len(rep.Tasks) > actionTasks {
				rep.Tasks = rep.Tasks[:actionTasks]
			}
			return rep, nil
		},
	})
}
