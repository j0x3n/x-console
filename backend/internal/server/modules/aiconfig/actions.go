package aiconfig

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "aiconfig.status",
		Title: "查看 AI 编码工具配置是否一致",
		Description: "Check whether the Claude Code and Codex configuration the user keeps in the panel (global rules, permissions, MCP servers) " +
			"matches each machine it is delivered to. Read only: nothing is written. " +
			"Each machine has a state ok, drift (something is missing, extra or different), conflict (the machine already has a server of the same name that the panel did not write, or a file that can not be parsed), " +
			"absent (neither tool is installed), offline, unsupported (old agent) or error, and the state of each item (rules, permissions, mcp) per tool.",
		Input:  actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
			s, err := m.load(ctx)
			if err != nil {
				return nil, err
			}
			res, err := m.status(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]any{"selectedHosts": len(s.HostIDs), "hosts": res}, nil
		},
	})
}
