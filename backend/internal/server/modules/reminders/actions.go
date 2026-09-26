package reminders

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

func decodeInput(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return httpx.Invalid("参数格式不正确: " + err.Error())
	}
	return nil
}

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:        "reminders.list",
		Title:       "查看提醒",
		Description: "List reminders. `range` is today (due today or already fired and not done), upcoming, or done. Times are RFC 3339.",
		Input:       actions.Schema(`{"type":"object","properties":{"range":{"type":"string","enum":["today","upcoming","done"]}},"additionalProperties":false}`),
		Effect:      actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Range string `json:"range"`
			}
			if err := decodeInput(raw, &in); err != nil {
				return nil, err
			}
			if in.Range == "" {
				in.Range = rangeToday
			}
			if in.Range != rangeToday && in.Range != rangeUpcoming && in.Range != rangeDone {
				return nil, httpx.Invalid("range 只能是 today、upcoming 或 done")
			}
			rows, err := m.list(ctx, in.Range, time.Now())
			if err != nil {
				return nil, err
			}
			out := make([]api.Reminder, 0, len(rows))
			for _, r := range rows {
				out = append(out, toAPI(r))
			}
			return out, nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "reminders.create",
		Title: "新建提醒",
		Description: "Create a reminder. `at` is RFC 3339 with an offset (the user's time zone is " + m.d.Config.Location.String() +
			"). `rrule` makes it repeat, for example FREQ=DAILY or FREQ=WEEKLY;BYDAY=MO,WE; occurrences keep the time of `at`. Returns the reminder id.",
		Input:  actions.Schema(`{"type":"object","properties":{"title":{"type":"string"},"at":{"type":"string","format":"date-time"},"rrule":{"type":"string"},"body":{"type":"string"},"link":{"type":"string"}},"required":["title","at"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in contracts.CreateReminder
			if err := decodeInput(raw, &in); err != nil {
				return nil, err
			}
			row, err := m.create(ctx, in, time.Now())
			if err != nil {
				return nil, err
			}
			return map[string]any{"id": row.ID, "reminder": toAPI(row)}, nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:        "reminders.complete",
		Title:       "完成提醒",
		Description: "Mark a reminder done. For a repeating reminder this completes the current occurrence. Returns the updated reminder.",
		Input:       actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"],"additionalProperties":false}`),
		Effect:      actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
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
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "notify.send",
		Title: "发送通知",
		Description: "Send a notification to the user. It appears in the app and goes to the external channels chosen by the notification routes. " +
			"`priority` is low, normal, high or urgent (urgent ignores quiet hours). `link` is an in-app path such as /reminders. Returns the notification id.",
		Input:  actions.Schema(`{"type":"object","properties":{"title":{"type":"string"},"body":{"type":"string"},"link":{"type":"string"},"priority":{"type":"string","enum":["low","normal","high","urgent"]},"kind":{"type":"string"}},"required":["title"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run:    m.actionSend,
	})
}

func (m *Module) actionSend(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Link     string `json:"link"`
		Priority string `json:"priority"`
		Kind     string `json:"kind"`
	}
	if err := decodeInput(raw, &in); err != nil {
		return nil, err
	}
	title, err := cleanTitle(in.Title)
	if err != nil {
		return nil, err
	}
	link, err := cleanLink(in.Link)
	if err != nil {
		return nil, err
	}
	if in.Priority == "" {
		in.Priority = notify.PriorityNormal
	}
	if _, ok := priorityRank[in.Priority]; !ok {
		return nil, httpx.Invalid("优先级无效: " + in.Priority)
	}
	if in.Kind == "" {
		in.Kind = "notify.message"
	}
	stored, err := m.d.Notify.Send(ctx, notify.Notification{
		Kind: in.Kind, Title: title, Body: in.Body, Link: link, Priority: in.Priority, Source: audit.Actor(ctx),
	})
	m.d.Audit.Record(ctx, "notify.send", title, map[string]any{"kind": in.Kind, "priority": in.Priority}, err)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": stored.ID}, nil
}
