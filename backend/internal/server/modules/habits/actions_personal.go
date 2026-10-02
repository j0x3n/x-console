package habits

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
)

func (m *Module) registerPersonalActions() {
	m.d.Actions.Register(actions.Action{Name: "habits.library", Title: "个人计划资料", Description: "List exercise names/IDs, habit templates, strength sessions and run levels. With exerciseId return complete exercise instructions. With articleId return the article (food, English or daily routines).", Input: actions.Schema(`{"type":"object","properties":{"exerciseId":{"type":"string"},"articleId":{"type":"string"}},"additionalProperties":false}`), Effect: actions.Read, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			ExerciseID string `json:"exerciseId"`
			ArticleID  string `json:"articleId"`
		}
		if err := decodeStrict(raw, &in); err != nil {
			return nil, err
		}
		if in.ExerciseID != "" {
			for _, e := range personalLibrary.Exercises {
				if e.Id == in.ExerciseID {
					return e, nil
				}
			}
			return nil, httpx.Invalid("动作不存在")
		}
		if in.ArticleID != "" {
			for _, a := range personalLibrary.Articles {
				if a.Id == in.ArticleID {
					return a, nil
				}
			}
			return nil, httpx.Invalid("资料不存在")
		}
		exercises := make([]map[string]string, 0, len(personalLibrary.Exercises))
		for _, e := range personalLibrary.Exercises {
			exercises = append(exercises, map[string]string{"id": e.Id, "name": e.Name, "group": e.Group, "dose": e.Dose})
		}
		articles := make([]map[string]string, 0, len(personalLibrary.Articles))
		for _, a := range personalLibrary.Articles {
			articles = append(articles, map[string]string{"id": a.Id, "title": a.Title, "category": string(a.Category)})
		}
		return map[string]any{"exercises": exercises, "articles": articles, "habits": personalLibrary.Habits, "sessions": personalLibrary.Sessions, "phases": personalLibrary.Phases, "runLevels": personalLibrary.RunLevels}, nil
	}})
	m.d.Actions.Register(actions.Action{Name: "habits.personal_profile", Title: "个人计划设置", Description: "Read personal plan settings and the native habit IDs of selected templates.", Input: actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`), Effect: actions.Read, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct{}
		if err := decodeStrict(raw, &in); err != nil {
			return nil, err
		}
		p, err := m.personalProfile(ctx, m.d.DB)
		return p.PersonalProfile, err
	}})
	m.d.Actions.Register(actions.Action{Name: "habits.personal_day", Title: "个人计划日记录", Description: "Read body metrics, notes, completed sets and habit checks for a YYYY-MM-DD date.", Input: actions.Schema(`{"type":"object","properties":{"date":{"type":"string"}},"required":["date"],"additionalProperties":false}`), Effect: actions.Read, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			Date string `json:"date"`
		}
		if err := decodeStrict(raw, &in); err != nil {
			return nil, err
		}
		d, err := m.personalDay(ctx, m.d.DB, in.Date)
		return d.PersonalDay, err
	}})
	m.d.Actions.Register(actions.Action{Name: "habits.personal_activate", Title: "加入个人计划习惯", Description: "Select habit template IDs from habits.library and add them to native habits. Repeated selection is idempotent; archived habits stay archived.", Input: actions.Schema(`{"type":"object","properties":{"ids":{"type":"array","items":{"type":"string"}}},"required":["ids"],"additionalProperties":false}`), Effect: actions.Write, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in api.ActivatePersonalHabitsJSONBody
		if err := decodeStrict(raw, &in); err != nil {
			return nil, err
		}
		return m.personalTx(ctx, "habit.personal.activate", func(tx *sql.Tx, p *personalProfileState) error { return m.personalActivate(ctx, tx, p, in.Ids) })
	}})
	m.d.Actions.Register(actions.Action{Name: "habits.personal_check", Title: "个人计划打卡", Description: "Check or uncheck a selected template at a YYYY-MM-DD date. Unchecking deletes only the record created by this personal plan.", Input: actions.Schema(`{"type":"object","properties":{"date":{"type":"string"},"id":{"type":"string"},"done":{"type":"boolean"}},"required":["date","id","done"],"additionalProperties":false}`), Effect: actions.Write, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			Date string `json:"date"`
			ID   string `json:"id"`
			Done bool   `json:"done"`
		}
		if err := decodeStrict(raw, &in); err != nil {
			return nil, err
		}
		rules, err := m.loadSchedule(ctx)
		if err != nil {
			return nil, err
		}
		var d personalDayState
		wasDone := false
		_, err = m.personalTx(ctx, "habit.personal.check", func(tx *sql.Tx, p *personalProfileState) error {
			var err error
			d, err = m.personalDay(ctx, tx, in.Date)
			if err != nil {
				return err
			}
			wasDone = d.Checks[in.ID]
			if err = m.personalCheck(ctx, tx, p, &d, in.ID, in.Done, rules.location()); err != nil {
				return err
			}
			return personalPut(ctx, tx, personalDayPrefix+in.Date, d)
		})
		if err == nil && in.Done && !wasDone {
			m.personalPublishCheckin(ctx, d.HabitLogIDs[in.ID])
		}
		return d.PersonalDay, err
	}})
}
