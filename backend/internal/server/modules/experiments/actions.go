package experiments

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "experiments.list",
		Title: "查看自我实验",
		Description: "List the user's self-experiments: title, what changes, baseline start, change start, end date, phase " +
			"(planned, baseline, change, finished, stopped) and the metrics. No analysis; use experiments.result for that.",
		Input:  actions.Schema(`{"type":"object","additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
			rows, err := m.q.ListExperiments(ctx)
			if err != nil {
				return nil, err
			}
			all, err := m.q.ListAllExperimentMetrics(ctx)
			if err != nil {
				return nil, err
			}
			items := make([]any, 0, len(rows))
			for _, e := range rows {
				var ms = all[:0:0]
				for _, x := range all {
					if x.ExperimentID == e.ID {
						ms = append(ms, x)
					}
				}
				items = append(items, m.toExperiment(e, ms))
			}
			return map[string]any{"items": items}, nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "experiments.result",
		Title: "查看自我实验结果",
		Description: "Compare the baseline and the change phase of one experiment (`id`). For each metric: days with data, mean and standard deviation of both phases, " +
			"the difference, its 95% interval and a verdict: no_data, too_few (fewer than 5 days with data in a phase, numbers only), " +
			"inconclusive (the interval contains 0, so chance is a plausible explanation), higher or lower. Report the verdict before the numbers. " +
			"Notes warn about self-rated metrics, a running change phase, sparse data and before/after comparisons without random assignment.",
		Input:  actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"],"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(raw, &in); err != nil || in.ID <= 0 {
				return nil, httpx.Invalid("参数不正确")
			}
			return m.detail(ctx, in.ID)
		},
	})
}
