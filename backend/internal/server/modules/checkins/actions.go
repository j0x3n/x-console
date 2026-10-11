package checkins

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "checkins.answer",
		Title: "记一次状态打卡",
		Description: "Record a 1 to 5 score for a check-in question (for example energy or stress). `questionId` comes from checkins.series. " +
			"Without `slot` it is a free answer for today. The same question, date and slot overwrites the earlier answer.",
		Input:  actions.Schema(`{"type":"object","properties":{"questionId":{"type":"integer"},"value":{"type":"integer","minimum":1,"maximum":5},"slot":{"type":"string"},"date":{"type":"string"}},"required":["questionId","value"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				QuestionID int64  `json:"questionId"`
				Value      int    `json:"value"`
				Slot       string `json:"slot"`
				Date       string `json:"date"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, httpx.Invalid("参数不正确")
			}
			row, err := m.record(ctx, in.QuestionID, in.Value, in.Slot, in.Date, "action")
			if err != nil {
				return nil, err
			}
			return toAnswer(row), nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "checkins.series",
		Title: "查看状态打卡",
		Description: "Daily average of each check-in question (energy, stress and the user's own) for the last `days` days (default 30, at most 365). " +
			"Returns questions with id, label, overall average, count and one point per day that has answers.",
		Input:  actions.Schema(`{"type":"object","properties":{"days":{"type":"integer","minimum":1,"maximum":365}},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Days int `json:"days"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, httpx.Invalid("参数不正确")
				}
			}
			days := 30
			if in.Days > 0 {
				days = min(in.Days, 365)
			}
			to := m.now().In(m.loc())
			items, err := m.series(ctx, to.AddDate(0, 0, -(days-1)), to)
			if err != nil {
				return nil, err
			}
			return map[string]any{"items": items}, nil
		},
	})
}
