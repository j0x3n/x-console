package inbox

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "inbox.add",
		Title: "记进收集箱",
		Description: "Drop a thought into the user's capture inbox (up to 5000 characters). Nothing is created from it: the user confirms where it belongs " +
			"(reminder, board card, note, read later) later. Use this when you are unsure which module a note belongs to. Returns the item.",
		Input:  actions.Schema(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, httpx.Invalid("参数不正确")
			}
			row, err := m.capture(ctx, in.Text, "action", false)
			if err != nil {
				return nil, err
			}
			return toItem(row), nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:        "inbox.list",
		Title:       "查看收集箱",
		Description: "List what is waiting in the capture inbox (not yet filed or dismissed), oldest first, with the AI suggestion when there is one.",
		Input:       actions.Schema(`{"type":"object","additionalProperties":false}`),
		Effect:      actions.Read,
		Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
			rows, err := m.q.ListOpenInboxItems(ctx)
			if err != nil {
				return nil, err
			}
			items := make([]any, 0, len(rows))
			for _, r := range rows {
				items = append(items, toItem(r))
			}
			return map[string]any{"items": items}, nil
		},
	})
}
