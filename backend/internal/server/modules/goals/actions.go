package goals

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals/api"
)

type goalFilter struct {
	Year    *int    `json:"year"`
	Quarter *int    `json:"quarter"`
	Status  *string `json:"status"`
	GoalID  *int64  `json:"goalId"`
}

func (m *Module) filtered(ctx context.Context, raw json.RawMessage) ([]api.Goal, error) {
	var in goalFilter
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
	}
	if (in.Year == nil) != (in.Quarter == nil) {
		return nil, httpx.Invalid("year 和 quarter 要一起给")
	}
	if in.Quarter != nil && (*in.Quarter < 1 || *in.Quarter > 4) {
		return nil, httpx.Invalid("quarter 要在 1 到 4 之间")
	}
	status := api.GoalStatusActive
	if in.Status != nil {
		status = api.GoalStatus(*in.Status)
		if !status.Valid() {
			return nil, httpx.Invalid("不认识的状态")
		}
	}
	if in.GoalID != nil {
		g, err := m.get(ctx, m.q, *in.GoalID)
		if err != nil {
			return nil, err
		}
		return []api.Goal{g}, nil
	}
	return m.list(ctx, in.Year, in.Quarter, &status)
}

func (m *Module) registerActions() {
	filterSchema := `{"type":"object","properties":{` +
		`"year":{"type":"integer"},"quarter":{"type":"integer","minimum":1,"maximum":4},` +
		`"status":{"type":"string","enum":["active","done","dropped"]},"goalId":{"type":"integer"}},"additionalProperties":false}`
	m.d.Actions.Register(actions.Action{
		Name:  "goals.list",
		Title: "查看目标",
		Description: "List the user's quarterly goals with their key results. By default only active goals of all quarters; " +
			"`year` and `quarter` (together) narrow it to one quarter, `status` is active, done or dropped, `goalId` returns one goal. " +
			"Each key result has id, name, target, unit and source (manual, habit_checkins, workouts, focus_minutes, screen_minutes, github_commits, github_merged_prs).",
		Input:  actions.Schema(filterSchema),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			goals, err := m.filtered(ctx, raw)
			if err != nil {
				return nil, err
			}
			type result struct {
				ID     int64   `json:"id"`
				Name   string  `json:"name"`
				Target float64 `json:"target"`
				Unit   string  `json:"unit"`
				Source string  `json:"source"`
			}
			type goal struct {
				ID      int64    `json:"id"`
				Title   string   `json:"title"`
				Year    int      `json:"year"`
				Quarter int      `json:"quarter"`
				Status  string   `json:"status"`
				Note    string   `json:"note"`
				Results []result `json:"results"`
			}
			out := make([]goal, 0, len(goals))
			for _, g := range goals {
				item := goal{ID: g.Id, Title: g.Title, Year: g.Year, Quarter: g.Quarter, Status: string(g.Status), Note: g.Note, Results: []result{}}
				for _, r := range g.Results {
					item.Results = append(item.Results, result{ID: r.Id, Name: r.Name, Target: r.Target, Unit: r.Unit, Source: string(r.Source)})
				}
				out = append(out, item)
			}
			return map[string]any{"goals": out}, nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "goals.progress",
		Title: "查看目标进度",
		Description: "Show how far the user's goals have come. Same filters as goals.list (active goals by default). " +
			"For each goal: progress (percent, null when nothing can be measured), expected (percent of the quarter that has passed) and state " +
			"(done, on_track, behind, upcoming, missed). Each key result has value, target, unit, progress, state and `unavailable` " +
			"(empty, or hidden, missing, error when its number cannot be read). Progress of non-manual key results is measured from the user's other data.",
		Input:  actions.Schema(filterSchema),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			goals, err := m.filtered(ctx, raw)
			if err != nil {
				return nil, err
			}
			return map[string]any{"goals": goals}, nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "goals.update_progress",
		Title: "更新目标进度",
		Description: "Set the current value of a key result that is filled in by hand (source manual). `resultId` comes from goals.list, `value` is the new current value in the key result's unit. " +
			"Key results measured from habits, workouts, focus, computer time or GitHub cannot be set. Returns the key result with its new progress.",
		Input:  actions.Schema(`{"type":"object","properties":{"resultId":{"type":"integer"},"value":{"type":"number"}},"required":["resultId","value"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				ResultID int64    `json:"resultId"`
				Value    *float64 `json:"value"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
			if in.ResultID == 0 || in.Value == nil {
				return nil, httpx.Invalid("要给 resultId 和 value")
			}
			res, err := m.setValue(ctx, in.ResultID, *in.Value)
			m.d.Audit.Record(ctx, "goal.progress", strconv.FormatInt(in.ResultID, 10), map[string]any{"value": *in.Value, "via": "action"}, err)
			if err != nil {
				return nil, err
			}
			return res, nil
		},
	})
}
