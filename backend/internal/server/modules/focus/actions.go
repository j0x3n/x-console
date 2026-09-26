package focus

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/focus/api"
)

func decodeInput(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return httpx.Invalid("参数格式不正确: " + err.Error())
	}
	return nil
}

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "focus.start",
		Title: "开始番茄钟",
		Description: "Start a focus session (pomodoro). `minutes` defaults to 25 (1-180). `issueKey` links an issue such as XC-12. " +
			"Fails if another session is still running. Returns the session with its planned end `endsAt`.",
		Input:  actions.Schema(`{"type":"object","properties":{"minutes":{"type":"integer","minimum":1,"maximum":180},"issueKey":{"type":"string"},"note":{"type":"string"}},"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in api.FocusStart
			if err := decodeInput(raw, &in); err != nil {
				return nil, err
			}
			row, err := m.start(ctx, in, time.Now())
			if err != nil {
				return nil, err
			}
			return m.toAPI(ctx, row), nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:        "focus.stop",
		Title:       "结束番茄钟",
		Description: "Stop the running focus session, or the one with `id`. `completed` defaults to whether the planned time was reached.",
		Input:       actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"},"completed":{"type":"boolean"}},"additionalProperties":false}`),
		Effect:      actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				ID        int64 `json:"id"`
				Completed *bool `json:"completed"`
			}
			if err := decodeInput(raw, &in); err != nil {
				return nil, err
			}
			if in.ID == 0 {
				cur, err := m.current(ctx)
				if err != nil {
					return nil, err
				}
				if cur == nil {
					return nil, httpx.Invalid("现在没有进行中的番茄钟")
				}
				in.ID = cur.Id
			}
			row, err := m.stop(ctx, in.ID, in.Completed, nil, time.Now())
			if err != nil {
				return nil, err
			}
			return m.toAPI(ctx, row), nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:        "focus.current",
		Title:       "查看番茄钟",
		Description: "Return the running focus session, or null when none is running.",
		Input:       actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		Effect:      actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct{}
			if err := decodeInput(raw, &in); err != nil {
				return nil, err
			}
			return m.current(ctx)
		},
	})
}
