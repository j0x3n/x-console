package skills

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/skills/api"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "skills.log",
		Title: "记一次技能练习",
		Description: "Log one practice session of a skill the user already created (it does not create skills). skill is the skill's name or its numeric id. " +
			"minutes is 1 to 1440. date is YYYY-MM-DD, today or up to a year back (default today). content is what was practiced and stuck is where it got stuck (both optional). Returns the session.",
		Input: actions.Schema(`{"type":"object","properties":{"skill":{"type":"string"},"minutes":{"type":"integer","minimum":1,"maximum":1440},"date":{"type":"string"},` +
			`"content":{"type":"string"},"stuck":{"type":"string"}},"required":["skill","minutes"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Skill   string `json:"skill"`
				Minutes int    `json:"minutes"`
				Date    string `json:"date"`
				Content string `json:"content"`
				Stuck   string `json:"stuck"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, httpx.Invalid("参数不正确")
			}
			id, err := m.findSkill(ctx, in.Skill)
			if err != nil {
				return nil, err
			}
			return m.logSession(ctx, id, api.SkillSessionInput{Minutes: in.Minutes, PracticedOn: &in.Date, Content: &in.Content, Stuck: &in.Stuck})
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:        "skills.list",
		Title:       "查看技能和每周投入",
		Description: "List the skills that are not archived with this week's minutes, the weekly target, the last practice date and the minutes of the last 8 weeks (oldest first, the last one is this week).",
		Input:       actions.Schema(`{"type":"object","additionalProperties":false}`),
		Effect:      actions.Read,
		Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
			items, err := m.list(ctx, false)
			if err != nil {
				return nil, err
			}
			return map[string]any{"items": items, "today": m.today()}, nil
		},
	})
}

// findSkill finds a skill by name (ignoring case) or by numeric id.
func (m *Module) findSkill(ctx context.Context, ref string) (int64, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return 0, httpx.Invalid("请写技能名称")
	}
	row, err := m.q.GetSkillByName(ctx, ref)
	if err == nil {
		return row.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if id, perr := parseID(ref); perr == nil {
		if sk, err := m.get(ctx, id); err == nil {
			return sk.ID, nil
		}
	}
	return 0, httpx.Invalid("没有叫这个名字的技能，先在技能页建一个")
}
