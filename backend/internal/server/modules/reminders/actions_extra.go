package reminders

import (
	"context"
	"encoding/json"
	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"time"
)

func (m *Module) registerExtraActions() {
	m.d.Actions.Register(actions.Action{Name: "reminders.update", Title: "修改提醒", Description: "Update a reminder by id.", Input: actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"},"title":{"type":"string"},"at":{"type":"string","format":"date-time"},"body":{"type":"string"},"rrule":{"type":"string"}},"required":["id"]}`), Effect: actions.Write, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			ID    int64      `json:"id"`
			Title *string    `json:"title"`
			At    *time.Time `json:"at"`
			Body  *string    `json:"body"`
			Rrule *string    `json:"rrule"`
		}
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		row, err := m.update(ctx, in.ID, api.ReminderPatch{Title: in.Title, At: in.At, Body: in.Body, Rrule: in.Rrule}, time.Now())
		if err != nil {
			return nil, err
		}
		return toAPI(row), nil
	}})
	m.d.Actions.Register(actions.Action{Name: "reminders.done", Title: "完成提醒", Description: "Complete a reminder by id.", Input: actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}`), Effect: actions.Write, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			ID int64 `json:"id"`
		}
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		row, err := m.complete(ctx, in.ID, time.Now())
		if err != nil {
			return nil, err
		}
		return toAPI(row), nil
	}})
	m.d.Actions.Register(actions.Action{Name: "reminders.delete", Title: "删除提醒", Description: "Delete a reminder by id.", Input: actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}`), Effect: actions.Write, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			ID int64 `json:"id"`
		}
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		err := m.remove(ctx, in.ID)
		return map[string]any{"deleted": err == nil}, err
	}})
}
