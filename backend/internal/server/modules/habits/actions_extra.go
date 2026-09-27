package habits

import (
	"context"
	"encoding/json"
	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
	"time"
)

func (m *Module) registerExtraActions() {
	m.d.Actions.Register(actions.Action{Name: "habits.stats", Title: "习惯统计", Description: "Get habit stats for up to 366 days.", Input: actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"},"days":{"type":"integer","minimum":1,"maximum":366}},"required":["id"]}`), Effect: actions.Read, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			ID   int64 `json:"id"`
			Days int   `json:"days"`
		}
		if err := decodeStrict(raw, &in); err != nil {
			return nil, err
		}
		if in.Days == 0 {
			in.Days = 30
		}
		return m.stats(ctx, in.ID, in.Days, time.Now())
	}})
	m.d.Actions.Register(actions.Action{Name: "habits.create", Title: "新建习惯", Description: "Create a habit with name and optional daily target and unit.", Input: actions.Schema(`{"type":"object","properties":{"name":{"type":"string"},"dailyTarget":{"type":"number"},"unit":{"type":"string"}},"required":["name"]}`), Effect: actions.Write, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in api.HabitInput
		if err := decodeStrict(raw, &in); err != nil {
			return nil, err
		}
		row, err := m.create(ctx, in)
		if err != nil {
			return nil, err
		}
		return toAPI(row), nil
	}})
	m.d.Actions.Register(actions.Action{Name: "habits.delete", Title: "删除习惯", Description: "Delete a habit by id.", Input: actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}`), Effect: actions.Write, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			ID int64 `json:"id"`
		}
		if err := decodeStrict(raw, &in); err != nil {
			return nil, err
		}
		err := m.remove(ctx, in.ID)
		return map[string]any{"deleted": err == nil}, err
	}})
}
