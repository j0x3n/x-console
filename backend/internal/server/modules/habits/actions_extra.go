package habits

import (
	"context"
	"encoding/json"
	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/db"
	"time"
)

// habitFieldsSchema lists the habit fields the AI may set (B96).
const habitFieldsSchema = `"name":{"type":"string"},` +
	`"kind":{"type":"string","enum":["count","workout"]},` +
	`"icon":{"type":"string","description":"an emoji, e.g. 💧"},` +
	`"unit":{"type":"string","description":"e.g. 杯, 次"},` +
	`"dailyTarget":{"type":"number"},` +
	`"template":{"type":"string","enum":["water","eyes","move","medicine"]},` +
	`"remindMode":{"type":"string","enum":["none","interval","times"]},` +
	`"remindIntervalMinutes":{"type":"integer","minimum":5,"maximum":1440},` +
	`"remindWindow":{"type":"string","description":"HH:MM-HH:MM, e.g. 09:00-21:00"},` +
	`"remindTimes":{"type":"array","items":{"type":"string","description":"HH:MM"}},` +
	`"remindWhen":{"type":"array","items":{"type":"string","enum":["window","awake","work","active"]}},` +
	`"activeHostIds":{"type":"array","items":{"type":"string"}},` +
	`"remindOnHost":{"type":"boolean"},` +
	`"haEntityId":{"type":"string"}`

// habitFieldsHelp explains the reminder fields to the AI.
const habitFieldsHelp = "Reminders: remindMode none (no reminder), interval (every remindIntervalMinutes) or times (at each remindTimes HH:MM). " +
	"For interval, remindWhen chooses when it runs: window (inside remindWindow), awake (between wake and sleep time), " +
	"work (work hours on work days), active (only while someone uses one of activeHostIds; get the ids from hosts.list). " +
	"remindOnHost also shows the reminder on those computers. Reminders stop once the daily target is reached."

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
	// B96: the AI can set every reminder field, and a template is used once.
	m.d.Actions.Register(actions.Action{Name: "habits.create", Title: "新建习惯", Description: "Create a count or workout habit. " +
		"With a template (water, eyes, move, medicine) the missing fields take the template's defaults; " +
		"if an active habit already uses that template, nothing is created and that habit is returned with existing=true. " + habitFieldsHelp,
		Input: actions.Schema(`{"type":"object","properties":{` + habitFieldsSchema + `},"required":["name"]}`), Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in api.HabitInput
			if err := decodeStrict(raw, &in); err != nil {
				return nil, err
			}
			if in.Template != nil {
				if h, ok, err := m.byTemplate(ctx, string(*in.Template)); err != nil {
					return nil, err
				} else if ok {
					out, err := m.habitAPI(ctx, h, time.Now())
					return map[string]any{"existing": true, "habit": out}, err
				}
			}
			row, err := m.create(ctx, in)
			if err != nil {
				return nil, err
			}
			return m.habitAPI(ctx, row, time.Now())
		}})
	m.d.Actions.Register(actions.Action{Name: "habits.update", Title: "修改习惯", Description: "Change a habit by id. Only the fields given change. " + habitFieldsHelp,
		Input: actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"},` + habitFieldsSchema + `,"archived":{"type":"boolean"}},"required":["id"]}`), Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var head struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(raw, &head); err != nil || head.ID == 0 {
				return nil, httpx.Invalid("要传习惯的 id")
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				return nil, httpx.Invalid("请求体格式不正确")
			}
			delete(fields, "id")
			rest, _ := json.Marshal(fields)
			var p api.HabitPatch
			if err := decodeStrict(rest, &p); err != nil {
				return nil, err
			}
			row, err := m.update(ctx, head.ID, p)
			if err != nil {
				return nil, err
			}
			return m.habitAPI(ctx, row, time.Now())
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

// byTemplate finds the active (not archived) habit made from a template (B96).
func (m *Module) byTemplate(ctx context.Context, template string) (db.Habit, bool, error) {
	rows, err := m.q.ListHabits(ctx, 0)
	if err != nil {
		return db.Habit{}, false, err
	}
	for _, h := range rows {
		if h.Template == template {
			return h, true, nil
		}
	}
	return db.Habit{}, false, nil
}
