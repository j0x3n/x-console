package quotas

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "quotas.list",
		Title: "查看 AI 额度",
		Description: "List the AI quota accounts (Claude, Codex, Grok subscriptions and DeepSeek balances) with their last reading. " +
			"Each account has `windows` (usedPercent of an allowance window and when it resetsAt) or `balances`, and `status` ok, error or pending.",
		Input:  actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
			items, err := m.views(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]any{"items": items}, nil
		},
	})
}
