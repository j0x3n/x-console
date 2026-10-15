package watchers

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/api"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:        "watchers.list",
		Title:       "查看网站自动化",
		Description: "List the website automations (web and X): name, address, goal (what the AI should do, may be empty), the value read last time, status and when it was last checked.",
		Input:       actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		Effect:      actions.Read,
		Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
			rows, err := m.q.ListWatchers(ctx)
			if err != nil {
				return nil, err
			}
			items := make([]api.Watcher, 0, len(rows))
			for _, r := range rows {
				items = append(items, m.toAPI(ctx, r))
			}
			return map[string]any{"items": items}, nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:        "watchers.check",
		Title:       "立即检查网站自动化",
		Description: "Check one website automation now. The check runs in the background; read watchers.list later for the value.",
		Input:       actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"],"additionalProperties":false}`),
		Effect:      actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, httpx.Invalid("参数不正确")
			}
			if _, err := m.get(ctx, in.ID); err != nil {
				return nil, err
			}
			if !m.startRun(in.ID, "check") {
				return map[string]any{"started": false, "reason": "任务正在跑"}, nil
			}
			m.d.Audit.Record(ctx, "watcher.run", "", map[string]any{"id": in.ID, "kind": "check"}, nil)
			return map[string]any{"started": true}, nil
		},
	})
}
