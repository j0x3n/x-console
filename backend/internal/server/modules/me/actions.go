package me

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "me.state",
		Title: "查看我的状态",
		Description: "What the user is doing now: working (a work computer is in use), away (stepped away from the computer), " +
			"sleeping, out (left home), home or unknown. Returns the state, since when, whether it was guessed from the sleep schedule " +
			"or set by hand, and what it was judged from (pc, phone, ha, manual, schedule).",
		Input:  actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
			return m.fresh(ctx), nil
		},
	})
}
