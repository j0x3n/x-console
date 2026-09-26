package habits

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:        "habits.today",
		Title:       "今天的习惯",
		Description: "Today's progress of every active habit (in the user's time zone): done amount, daily target, unit, streak and today's check-ins.",
		Input:       actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		Effect:      actions.Read,
		Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
			return m.today(ctx, time.Now())
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "habits.checkin",
		Title: "习惯打卡",
		Description: "Check in a habit. Identify it by `habitId` or by exact `name`. `amount` defaults to 1 (in the habit's unit, " +
			"for example cups or minutes). Returns today's progress of that habit.",
		Input:  actions.Schema(`{"type":"object","properties":{"habitId":{"type":"integer"},"name":{"type":"string"},"amount":{"type":"number","exclusiveMinimum":0},"note":{"type":"string"}},"additionalProperties":false}`),
		Effect: actions.Write,
		Run:    m.actionCheckin,
	})
	m.d.Actions.Register(actions.Action{
		Name:  "workouts.log",
		Title: "记录训练",
		Description: "Log a workout. `date` is YYYY-MM-DD (default today). With `planId` and no `items` the plan's exercises are copied. " +
			"Items are {name, sets, reps, weight (kg), note}. Returns the log.",
		Input:  actions.Schema(`{"type":"object","properties":{"date":{"type":"string"},"planId":{"type":"integer"},"durationMinutes":{"type":"integer","minimum":0},"note":{"type":"string"},"items":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"sets":{"type":"integer"},"reps":{"type":"integer"},"weight":{"type":"number"},"note":{"type":"string"}},"required":["name"]}}},"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in api.WorkoutLogInput
			if err := decodeStrict(raw, &in); err != nil {
				return nil, err
			}
			row, err := m.logWorkout(ctx, in, time.Now())
			if err != nil {
				return nil, err
			}
			return workoutLogToAPI(row), nil
		},
	})
}

func (m *Module) actionCheckin(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		HabitID int64    `json:"habitId"`
		Name    string   `json:"name"`
		Amount  *float64 `json:"amount"`
		Note    string   `json:"note"`
	}
	if err := decodeStrict(raw, &in); err != nil {
		return nil, err
	}
	if in.HabitID == 0 {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			return nil, httpx.Invalid("要给出 habitId 或 name")
		}
		habits, err := m.q.ListHabits(ctx, 0)
		if err != nil {
			return nil, err
		}
		for _, h := range habits {
			if strings.EqualFold(h.Name, name) {
				in.HabitID = h.ID
				break
			}
		}
		if in.HabitID == 0 {
			return nil, httpx.NewError(404, "not_found", "没有叫这个名字的习惯: "+name)
		}
	}
	amount := 1.0
	if in.Amount != nil {
		amount = *in.Amount
	}
	source := "ai"
	if strings.HasPrefix(audit.Actor(ctx), "automation") {
		source = "automation"
	}
	_, today, err := m.checkin(ctx, in.HabitID, amount, in.Note, source, time.Now())
	if err != nil {
		return nil, err
	}
	return today, nil
}
