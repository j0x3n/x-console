package screentime

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "screentime.summary",
		Title: "查看电脑时间去向",
		Description: "How the user's computer time was spent in a day, week or month: total minutes, minutes per category " +
			"(coding, ai, chat, web, entertainment, office, other) and the most used programs. " +
			"Optional `range` (day, week, month; default day) and `date` (YYYY-MM-DD, any day inside the range; default today).",
		Input:  actions.Schema(`{"type":"object","properties":{"range":{"type":"string","enum":["day","week","month"]},"date":{"type":"string"}},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Range string `json:"range"`
				Date  string `json:"date"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, err
				}
			}
			return m.summary(ctx, in.Range, in.Date, "")
		},
	})
}
