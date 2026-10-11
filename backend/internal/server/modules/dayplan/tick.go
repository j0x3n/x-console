package dayplan

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// tick runs once a minute: it makes the day's draft, reminds at the start
// of an item, and asks in the evening what got done.
func (m *Module) tick(ctx context.Context) error {
	opts := m.options(ctx)
	now := m.now().In(m.loc())
	nowMin := now.Hour()*60 + now.Minute()
	date := now.Format(dateLayout)

	if opts.Auto {
		at, _ := parseHM(opts.GenerateTime)
		if nowMin >= at {
			if _, err := m.q.GetDayPlan(ctx, date); errors.Is(err, sql.ErrNoRows) {
				plan, err := m.generate(ctx, date, false)
				if err != nil {
					return err
				}
				if len(plan.Items) > 0 {
					m.send(ctx, notify.Notification{Kind: "dayplan.draft", Title: "今天的计划草稿排好了", Body: "看一眼，改一改，确认。",
						Link: "/plan", Source: "dayplan", Immediate: true})
				}
			} else if err != nil {
				return err
			}
		}
	}

	plan, err := m.q.GetDayPlan(ctx, date)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && plan.Status != "confirmed") {
		return nil
	}
	if err != nil {
		return err
	}
	items, err := m.q.ListDayPlanItems(ctx, date)
	if err != nil {
		return err
	}
	open := 0
	for _, it := range items {
		if it.Done == 1 {
			continue
		}
		open++
		start, ok := parseHM(it.StartTime)
		if !ok || it.Notified == 1 || nowMin < start || nowMin >= start+int(startWindow.Minutes()) {
			continue
		}
		// Marked before sending: a failed send is not repeated every minute.
		if err := m.q.SetDayPlanItemNotified(ctx, it.ID); err != nil {
			return err
		}
		m.send(ctx, notify.Notification{Kind: "dayplan.start", Title: "该做：" + it.Title, Body: it.StartTime + " 到 " + it.EndTime,
			Link: "/plan", Source: "dayplan", Immediate: true,
			Actions: []notify.Action{{ID: fmt.Sprintf("%sdone:%d", actionPrefix, it.ID), Label: "做完了"}}})
	}
	if opts.Review && plan.ReviewSent == 0 && open > 0 {
		at, _ := parseHM(opts.ReviewTime)
		if nowMin >= at {
			if err := m.q.SetDayPlanReviewSent(ctx, date); err != nil {
				return err
			}
			m.send(ctx, notify.Notification{Kind: "dayplan.review", Title: "今天的计划做完了吗",
				Body: fmt.Sprintf("还有 %d 件没勾掉。", open), Link: "/plan", Source: "dayplan", Immediate: true})
		}
	}
	return nil
}

func (m *Module) send(ctx context.Context, n notify.Notification) {
	if _, err := m.d.Notify.Send(ctx, n); err != nil {
		m.d.Log.Warn("day plan notify", "kind", n.Kind, "err", err)
	}
}

// handleAction runs the "做完了" button of a start reminder.
func (m *Module) handleAction(ctx context.Context, id string) error {
	verb, num, ok := strings.Cut(strings.TrimPrefix(id, actionPrefix), ":")
	n, err := strconv.ParseInt(num, 10, 64)
	if !ok || verb != "done" || err != nil {
		return httpx.Invalid("不认识的按钮")
	}
	_, err = m.setDone(ctx, n, true)
	return err
}

// Lines implements contracts.DayPlanSource.
func (m *Module) Lines(ctx context.Context, date string) (string, []string, error) {
	plan, err := m.load(ctx, date)
	if errors.Is(err, httpx.ErrNotFound) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	lines := make([]string, 0, len(plan.Items))
	for _, it := range plan.Items {
		line := it.Title
		if it.Start != "" {
			line = it.Start + "–" + it.End + " " + it.Title
		}
		if it.Done {
			line += "（已完成）"
		}
		lines = append(lines, line)
	}
	return string(plan.Status), lines, nil
}
