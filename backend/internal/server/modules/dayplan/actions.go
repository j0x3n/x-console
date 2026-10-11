package dayplan

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "dayplan.today",
		Title: "查看今天的计划",
		Description: "Today's plan (or `date`, YYYY-MM-DD): status (draft or confirmed), source (ai, rule, manual) and the items with start and end times, " +
			"reason and whether each is done. Returns plan null when the day has none.",
		Input:  actions.Schema(`{"type":"object","properties":{"date":{"type":"string"}},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Date string `json:"date"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, httpx.Invalid("参数不正确")
				}
			}
			date := in.Date
			if date == "" {
				date = m.today()
			}
			if _, _, err := m.dayBounds(date); err != nil {
				return nil, errBadDate
			}
			plan, err := m.load(ctx, date)
			if err == httpx.ErrNotFound {
				return map[string]any{"date": date, "plan": nil}, nil
			}
			if err != nil {
				return nil, err
			}
			return map[string]any{"date": date, "plan": plan}, nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "dayplan.generate",
		Title: "排今天的计划",
		Description: "Plan the day: picks up to three things from the calendar's free time, due issues, reminders and yesterday's leftovers, and saves a DRAFT for the user to confirm. " +
			"Fails with 409 when the day's plan is already confirmed. `date` defaults to today.",
		Input:  actions.Schema(`{"type":"object","properties":{"date":{"type":"string"}},"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Date string `json:"date"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, httpx.Invalid("参数不正确")
				}
			}
			date := in.Date
			if date == "" {
				date = m.today()
			}
			return m.generate(ctx, date, false)
		},
	})
}
