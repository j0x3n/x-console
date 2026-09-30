package projects

import (
	"context"
	"fmt"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

const dueLayout = time.RFC3339

type dueCandidate struct {
	id, number                int64
	key, title, dueAt, remind string
	notified                  *string
}

func (m *Module) activeDue(ctx context.Context, until time.Time) ([]dueCandidate, error) {
	rows, err := m.d.DB.QueryContext(ctx, `SELECT i.id,p.key,i.number,i.title,i.due_at,i.due_remind,i.due_notified_at
FROM issues i JOIN projects p ON p.id=i.project_id
WHERE i.due_at IS NOT NULL AND i.due_at<=? AND i.status NOT IN ('done','canceled')
AND p.archived_at IS NULL ORDER BY i.due_at,i.id`, until.UTC().Format(dueLayout))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []dueCandidate{}
	for rows.Next() {
		var item dueCandidate
		if err := rows.Scan(&item.id, &item.key, &item.number, &item.title, &item.dueAt, &item.remind, &item.notified); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (m *Module) Upcoming(ctx context.Context, from, until time.Time) ([]contracts.ExternalReminder, error) {
	items, err := m.activeDue(ctx, until)
	if err != nil {
		return nil, err
	}
	out := []contracts.ExternalReminder{}
	for _, item := range items {
		at, err := time.Parse(dueLayout, item.dueAt)
		if err != nil {
			return nil, err
		}
		if !at.Before(until) {
			continue
		}
		issue := issueKey(item.key, item.number)
		out = append(out, contracts.ExternalReminder{ID: "issue:" + issue, Source: "issue", SourceLabel: "Issue", Title: issue + " " + item.title, At: at, Link: fmt.Sprintf("/projects/%s/%d", item.key, item.number)})
	}
	return out, nil
}

func (m *Module) dueAtForDate(date string) (*time.Time, error) {
	if err := validDate(date); err != nil {
		return nil, err
	}
	loc := m.d.Scheduler.Location()
	if loc == nil {
		loc = time.Local
	}
	day, err := time.ParseInLocation(dateLayout, date, loc)
	if err != nil {
		return nil, err
	}
	end := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 0, 0, loc).UTC()
	return &end, nil
}

func dueString(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := value.UTC().Format(dueLayout)
	return &text
}

func parseDue(value *string) *time.Time {
	if value == nil {
		return nil
	}
	t, err := time.Parse(dueLayout, *value)
	if err != nil {
		return nil
	}
	return &t
}

func validDueRemind(value string) bool {
	switch value {
	case "none", "at_due", "15m", "1h", "1d":
		return true
	}
	return false
}

// backfillDue is idempotent. It runs at startup after the additive migration.
func (m *Module) backfillDue(ctx context.Context) error {
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id,due_date FROM issues WHERE due_date IS NOT NULL AND due_at IS NULL")
	if err != nil {
		return err
	}
	type legacy struct {
		id   int64
		date string
	}
	var old []legacy
	for rows.Next() {
		var x legacy
		if err = rows.Scan(&x.id, &x.date); err != nil {
			break
		}
		old = append(old, x)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, x := range old {
		when, err := m.dueAtForDate(x.date)
		if err != nil {
			return err
		}
		if _, err = m.d.DB.ExecContext(ctx, "UPDATE issues SET due_at=? WHERE id=? AND due_at IS NULL", dueString(when), x.id); err != nil {
			return err
		}
	}
	return nil
}

func reminderOffset(value string) time.Duration {
	switch value {
	case "15m":
		return 15 * time.Minute
	case "1h":
		return time.Hour
	case "1d":
		return 24 * time.Hour
	}
	return 0
}

// sendDue scans eligible issues. The compare-and-set update claims each due_at
// before delivery, so overlapping scheduler runs cannot send duplicates.
func (m *Module) sendDue(ctx context.Context) error {
	now := m.now().UTC()
	candidates, err := m.activeDue(ctx, now.Add(24*time.Hour))
	if err != nil {
		return err
	}
	for _, x := range candidates {
		if x.remind == "none" || (x.notified != nil && *x.notified == x.dueAt) {
			continue
		}
		due, parseErr := time.Parse(dueLayout, x.dueAt)
		if parseErr != nil {
			return parseErr
		}
		if due.Add(-reminderOffset(x.remind)).After(now) {
			continue
		}
		updated, e := m.d.DB.ExecContext(ctx, "UPDATE issues SET due_notified_at=? WHERE id=? AND due_at=? AND (due_notified_at IS NULL OR due_notified_at<>due_at)", x.dueAt, x.id, x.dueAt)
		if e != nil {
			return e
		}
		count, _ := updated.RowsAffected()
		if count == 0 {
			continue
		}
		if due.Before(now.Add(-24 * time.Hour)) {
			continue
		}
		issueKey := issueKey(x.key, x.number)
		title := issueKey + " 到期了"
		if now.Before(due) {
			title = issueKey + " 快到期了"
		}
		loc := m.d.Scheduler.Location()
		if loc == nil {
			loc = time.Local
		}
		body := fmt.Sprintf("%s，%s", x.title, due.In(loc).Format("1月2日 15:04"))
		n := notify.Notification{Kind: "issue.due", Title: title, Body: body, Link: fmt.Sprintf("/projects/%s/%d", x.key, x.number), Source: "projects"}
		if _, e = m.d.Notify.Send(ctx, n); e != nil {
			m.d.Log.Error("issue due notification failed", "issue", issueKey, "err", e)
		}
	}
	return nil
}
