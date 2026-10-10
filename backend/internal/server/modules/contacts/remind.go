package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/contacts/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// remindAll is the hourly job.
func (m *Module) remindAll(ctx context.Context) error {
	rows, err := m.q.ListContacts(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		m.remindNow(ctx, row)
	}
	return nil
}

func (m *Module) remindNow(ctx context.Context, row db.Contact) {
	if row.ArchivedAt != nil {
		return
	}
	m.remindEvents(ctx, row)
	m.remindLost(ctx, row)
}

// told is "<id>:<year>:<days>" of a reminder that was sent.
func told(id string, year, days int) string {
	return id + ":" + strconv.Itoa(year) + ":" + strconv.Itoa(days)
}

// remindEvents sends at most one notification per date: a reminder day that
// has come and was not told for this year's date counts, several at once are
// one notification. Day 0 is always a reminder day. The records of a date
// are cleared when its date is changed (see save).
func (m *Module) remindEvents(ctx context.Context, row db.Contact) {
	today := m.today()
	events := parseEvents(row.Events)
	if len(events) == 0 {
		return
	}
	var record []string
	_ = json.Unmarshal([]byte(row.NotifiedJson), &record)
	original := len(record)
	live := map[string]bool{}
	for _, e := range events {
		live[e.ID] = true
	}
	// forget what belongs to a date that was removed or to an earlier year
	kept := record[:0]
	for _, k := range record {
		parts := strings.Split(k, ":")
		if len(parts) == 3 && live[parts[0]] && parts[1] >= strconv.Itoa(today.Year()) {
			kept = append(kept, k)
		}
	}
	record = kept
	changed := len(record) != original
	remind := append(parseRemind(row.RemindDays), 0)
	for _, e := range events {
		on, left, years := next(e, today)
		due := []int{}
		for _, d := range remind {
			if left <= d && !slices.Contains(record, told(e.ID, on.Year(), d)) {
				due = append(due, d)
			}
		}
		if len(due) == 0 {
			continue
		}
		n := notify.Notification{
			Kind: "contacts.event", Source: "contacts", Link: "/contacts",
			Title:    eventTitle(row.Name, e.Label, string(e.Kind), left, years),
			Body:     "日期 " + on.Format(dateLayout),
			Priority: notify.PriorityNormal,
			Scope:    "contacts:" + strconv.FormatInt(row.ID, 10),
		}
		if left <= 1 {
			n.Priority = notify.PriorityHigh
		}
		if _, err := m.d.Notify.Send(ctx, n); err != nil {
			m.d.Log.Warn("contact remind", "id", row.ID, "err", err)
			continue
		}
		for _, d := range due {
			record = append(record, told(e.ID, on.Year(), d))
		}
		changed = true
	}
	if !changed {
		return
	}
	raw, _ := json.Marshal(record)
	if err := m.q.SetContactNotified(ctx, db.SetContactNotifiedParams{NotifiedJson: string(raw), ID: row.ID}); err != nil {
		m.d.Log.Warn("contact remind state", "id", row.ID, "err", err)
	}
}

func eventTitle(name, label, kind string, left int, years *int) string {
	what := name + "的" + label
	suffix := ""
	if years != nil && *years > 0 {
		if kind == "birthday" {
			suffix = fmt.Sprintf("（%d 岁）", *years)
		} else {
			suffix = fmt.Sprintf("（%d 周年）", *years)
		}
	}
	if left == 0 {
		return what + "是今天" + suffix
	}
	return fmt.Sprintf("%s还有 %d 天%s", what, left, suffix)
}

// remindLost tells once when the contact has not been in touch for longer than
// the chosen period. Recording a new contact date starts over.
func (m *Module) remindLost(ctx context.Context, row db.Contact) {
	if row.ContactEveryDays <= 0 {
		return
	}
	loc := m.loc()
	base := contactBase(row, loc)
	t, err := time.Parse(dateLayout, base)
	if err != nil {
		return
	}
	today := m.today()
	since := int(today.Sub(t).Hours() / 24)
	if since < int(row.ContactEveryDays) || row.LostFor == base {
		return
	}
	body := fmt.Sprintf("设的是 %d 天联系一次", row.ContactEveryDays)
	if row.LastContactOn != "" {
		body = "上次联系 " + row.LastContactOn + "，" + body
	} else {
		body = "还没记过联系，" + body
	}
	n := notify.Notification{
		Kind: "contacts.lost", Source: "contacts", Link: "/contacts",
		Title:    fmt.Sprintf("%s已经 %d 天没联系了", row.Name, since),
		Body:     body,
		Priority: notify.PriorityNormal,
		Scope:    "contacts:" + strconv.FormatInt(row.ID, 10) + ":lost",
	}
	if _, err := m.d.Notify.Send(ctx, n); err != nil {
		m.d.Log.Warn("contact lost remind", "id", row.ID, "err", err)
		return
	}
	if err := m.q.SetContactLost(ctx, db.SetContactLostParams{LostFor: base, ID: row.ID}); err != nil {
		m.d.Log.Warn("contact lost state", "id", row.ID, "err", err)
	}
}
