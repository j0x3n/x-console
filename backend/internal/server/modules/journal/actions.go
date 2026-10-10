package journal

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "journal.day",
		Title: "查看某一天的时间线",
		Description: "Show what happened on one day: finished cards, coding agent tasks, commits and pull requests, focus sessions, habit check-ins, calendar events, server alerts, " +
			"and the user's own diary for that day. `date` is YYYY-MM-DD in the server time zone; default today. " +
			"Returns items (at, kind, title, detail), counts per kind and the diary text.",
		Input:  actions.Schema(`{"type":"object","properties":{"date":{"type":"string","pattern":"^\\d{4}-\\d{2}-\\d{2}$"}},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Date string `json:"date"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, err
				}
			}
			if in.Date == "" {
				in.Date = m.dayOf(m.now())
			}
			day, err := m.day(ctx, in.Date)
			if err != nil {
				return nil, err
			}
			for i := range day.Items {
				day.Items[i].Link = ""
			}
			return day, nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "journal.search",
		Title: "搜索日记和时间线",
		Description: "Search the user's diary and the timeline (titles and details of past cards, tasks, commits, alerts and so on) for a word. " +
			"Returns up to 100 hits, newest first, each with day (YYYY-MM-DD), kind (diary or an item kind), title and snippet.",
		Input:  actions.Schema(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Q string `json:"q"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
			return m.search(ctx, in.Q)
		},
	})
}
