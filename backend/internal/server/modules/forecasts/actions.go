package forecasts

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/forecasts/api"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "forecasts.add",
		Title: "记一条预测或决策",
		Description: "Record a prediction or a decision. kind is prediction or decision. probability is the confidence that it comes true, an integer from 1 to 99 (optional). " +
			"dueDate is YYYY-MM-DD, today or later, at most 5 years ahead. On the due date the user is reminded to fill in the outcome. Returns the record.",
		Input: actions.Schema(`{"type":"object","properties":{"kind":{"type":"string","enum":["prediction","decision"]},"statement":{"type":"string"},` +
			`"probability":{"type":"integer","minimum":1,"maximum":99},"dueDate":{"type":"string"},"reason":{"type":"string"}},"required":["kind","statement","dueDate"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in api.ForecastInput
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, httpx.Invalid("参数不正确")
			}
			return m.create(ctx, in)
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:        "forecasts.list",
		Title:       "查看预测和决策",
		Description: "List predictions and decisions. status is open (no outcome yet, due first), resolved or all (default). Returns {items, today}.",
		Input:       actions.Schema(`{"type":"object","properties":{"status":{"type":"string","enum":["open","resolved","all"]}},"additionalProperties":false}`),
		Effect:      actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Status api.ListForecastsParamsStatus `json:"status"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, httpx.Invalid("参数不正确")
			}
			if in.Status == "" {
				in.Status = api.All
			}
			if !in.Status.Valid() {
				return nil, httpx.Invalid("status 只能是 open、resolved 或 all")
			}
			items, err := m.list(ctx, in.Status)
			if err != nil {
				return nil, err
			}
			return map[string]any{"items": items, "today": m.today()}, nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:        "forecasts.resolve",
		Title:       "填预测结果",
		Description: "Fill in the outcome of a record: yes (came true, decision was right), no (did not, decision was wrong) or void (unclear, left out of the stats). Optional note. Returns the record.",
		Input: actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"},"outcome":{"type":"string","enum":["yes","no","void"]},"note":{"type":"string"}},` +
			`"required":["id","outcome"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				ID      int64               `json:"id"`
				Outcome api.ForecastOutcome `json:"outcome"`
				Note    *string             `json:"note"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, httpx.Invalid("参数不正确")
			}
			return m.resolve(ctx, in.ID, in.Outcome, in.Note)
		},
	})
}
