package contacts

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "contacts.list",
		Title: "查看联系人和重要日期",
		Description: "List the user's contacts with their yearly dates (birthdays, anniversaries) and when they were last in touch. " +
			"Each has name, group (family, friend, colleague, other), events (label, date, nextOn, nextIn days, years), lastContactOn (YYYY-MM-DD, empty if never recorded), " +
			"sinceContact (days), contactEveryDays, contactDueIn (days, negative when overdue) and status soon, overdue, ok or none. " +
			"Use `q` to find a person by name. Use `days` to list only the people with a date within that many days, for example 30 for birthdays this month.",
		Input:  actions.Schema(`{"type":"object","properties":{"q":{"type":"string"},"days":{"type":"integer","minimum":0,"maximum":366}},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Q    string `json:"q"`
				Days *int   `json:"days"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, err
				}
			}
			items, sum, err := m.list(ctx, nil, in.Q, false, in.Days)
			if err != nil {
				return nil, err
			}
			return map[string]any{"items": items, "summary": sum}, nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "contacts.touch",
		Title: "记一次联系",
		Description: "Record that the user was in touch with a contact: set the last contact date (YYYY-MM-DD, default today, not in the future). " +
			"Give the contact's exact `name` or its `id`. If the name matches no one or more than one contact, it is refused.",
		Input:  actions.Schema(`{"type":"object","properties":{"name":{"type":"string"},"id":{"type":"integer"},"date":{"type":"string"}},"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Name string `json:"name"`
				ID   int64  `json:"id"`
				Date string `json:"date"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
			id := in.ID
			if id == 0 {
				name := strings.TrimSpace(in.Name)
				if name == "" {
					return nil, httpx.Invalid("要给联系人的名称或 id")
				}
				items, _, err := m.list(ctx, nil, "", false, nil)
				if err != nil {
					return nil, err
				}
				for _, c := range items {
					if strings.EqualFold(c.Name, name) {
						if id != 0 {
							return nil, httpx.Invalid("有不止一个叫 " + name + " 的联系人，请用 id")
						}
						id = c.Id
					}
				}
				if id == 0 {
					return nil, httpx.Invalid("没有叫 " + name + " 的联系人")
				}
			}
			v, err := m.touch(ctx, id, in.Date)
			m.d.Audit.Record(ctx, "contact.touch", "", map[string]any{"id": id, "via": "ai"}, err)
			if err != nil {
				return nil, err
			}
			m.d.Bus.Publish("contact.updated", v)
			return v, nil
		},
	})
}
