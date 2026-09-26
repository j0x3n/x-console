package reminders

import (
	"sort"
	"strings"
	"time"

	"github.com/teambition/rrule-go"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/db"
)

// This file holds the pure scheduling rules. Nothing here touches the
// database or the clock, so tests drive it with any "now".

// missedAfter is how late a reminder may fire before it counts as missed.
const missedAfter = time.Hour

// parseRule builds the recurrence of a reminder in the user's time zone.
// An empty rule means a one-shot reminder and returns nil.
func parseRule(s string, dtstart time.Time, loc *time.Location) (*rrule.RRule, error) {
	s = normalizeRule(s)
	if s == "" {
		return nil, nil
	}
	opt, err := rrule.StrToROptionInLocation(s, loc)
	if err != nil {
		return nil, httpx.Invalid("重复规则无效: " + err.Error())
	}
	switch {
	case opt.Freq == rrule.SECONDLY:
		return nil, httpx.Invalid("重复规则不能按秒")
	case opt.Freq == rrule.MINUTELY && opt.Interval < 5:
		return nil, httpx.Invalid("按分钟重复时间隔至少 5 分钟")
	}
	opt.Dtstart = dtstart.In(loc)
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return nil, httpx.Invalid("重复规则无效: " + err.Error())
	}
	return r, nil
}

// normalizeRule trims an optional "RRULE:" prefix and whitespace.
func normalizeRule(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 6 && strings.EqualFold(s[:6], "RRULE:") {
		s = strings.TrimSpace(s[6:])
	}
	return s
}

// after returns the first occurrence after t (or at t when inclusive), in
// UTC, or nil when the rule has ended.
func after(rule *rrule.RRule, t time.Time, inclusive bool) *time.Time {
	next := rule.After(t, inclusive)
	if next.IsZero() {
		return nil
	}
	next = next.UTC()
	return &next
}

// firstNext is next_at of a new or rescheduled reminder: dtstart for a
// one-shot, else the first occurrence at or after max(dtstart, now) so a
// repeating reminder never starts with a backlog.
func firstNext(rule *rrule.RRule, dtstart, now time.Time) *time.Time {
	if rule == nil {
		t := dtstart.UTC()
		return &t
	}
	from := dtstart
	if now.After(from) {
		from = now
	}
	return after(rule, from, true)
}

// isPending reports a reminder that fired and was not handled yet.
func isPending(r db.Reminder) bool {
	return r.LastFiredAt != nil && (r.DoneAt == nil || r.DoneAt.Before(*r.LastFiredAt))
}

// dueAt is when the reminder will notify next, considering a snooze.
func dueAt(r db.Reminder) *time.Time {
	switch {
	case r.SnoozedUntil != nil && (r.NextAt == nil || r.SnoozedUntil.Before(*r.NextAt)):
		return r.SnoozedUntil
	default:
		return r.NextAt
	}
}

func status(r db.Reminder) api.ReminderStatus {
	switch {
	case r.Rrule == "" && r.DoneAt != nil && !isPending(r) && r.SnoozedUntil == nil:
		return api.ReminderStatusDone
	case r.SnoozedUntil != nil:
		return api.ReminderStatusSnoozed
	case isPending(r):
		return api.ReminderStatusPending
	case r.NextAt != nil:
		return api.ReminderStatusScheduled
	default:
		return api.ReminderStatusEnded
	}
}

// fired is the outcome of a reminder coming due.
type fired struct {
	rem       db.Reminder // row to store
	scheduled time.Time   // the time it was meant to fire
	missed    bool        // more than missedAfter late, for example after downtime
}

// fire advances a reminder that is due at now. ok is false when nothing is
// due. Several missed occurrences collapse into one notification and the
// next occurrence is the first one after now.
func fire(r db.Reminder, rule *rrule.RRule, now time.Time) (out fired, ok bool) {
	now = now.UTC()
	if r.Enabled == 0 {
		return out, false
	}
	if r.SnoozedUntil != nil && !r.SnoozedUntil.After(now) {
		ok, out.scheduled = true, *r.SnoozedUntil
		r.SnoozedUntil = nil
	}
	if r.NextAt != nil && !r.NextAt.After(now) {
		if !ok || r.NextAt.Before(out.scheduled) {
			out.scheduled = *r.NextAt
		}
		ok = true
		if rule == nil {
			r.NextAt = nil
		} else {
			r.NextAt = after(rule, now, false)
		}
	}
	if !ok {
		return out, false
	}
	r.LastFiredAt = &now
	out.rem = r
	out.missed = now.Sub(out.scheduled) > missedAfter
	return out, true
}

// complete marks a reminder done. A one-shot is finished. A repeating
// reminder completes its current occurrence: one that already fired is
// acknowledged, otherwise the upcoming occurrence is skipped.
func complete(r db.Reminder, rule *rrule.RRule, now time.Time) db.Reminder {
	now = now.UTC()
	// Never record a completion before the firing it answers (clock skew).
	if r.LastFiredAt != nil && r.LastFiredAt.After(now) {
		now = *r.LastFiredAt
	}
	if rule == nil {
		r.DoneAt, r.NextAt, r.SnoozedUntil = &now, nil, nil
		return r
	}
	if isPending(r) || r.SnoozedUntil != nil {
		r.DoneAt, r.SnoozedUntil = &now, nil
		return r
	}
	if r.NextAt != nil {
		r.NextAt = after(rule, *r.NextAt, false)
	}
	r.DoneAt = &now
	return r
}

// snooze postpones a reminder by d. For a one-shot that replaces its time.
// For a repeating reminder an occurrence inside the snooze window is skipped.
func snooze(r db.Reminder, rule *rrule.RRule, d time.Duration, now time.Time) db.Reminder {
	until := now.UTC().Add(d)
	pending := isPending(r)
	r.SnoozedUntil = &until
	if rule == nil {
		r.NextAt, r.DoneAt = nil, nil
		return r
	}
	for !pending && r.NextAt != nil && !r.NextAt.After(until) {
		r.NextAt = after(rule, *r.NextAt, false)
	}
	return r
}

// Ranges of GET /reminders.
const (
	rangeToday    = "today"
	rangeUpcoming = "upcoming"
	rangeDone     = "done"
)

// inRange classifies a reminder for the list tabs. endOfToday is the next
// local midnight.
func inRange(r db.Reminder, rng string, endOfToday time.Time) bool {
	st := status(r)
	open := st == api.ReminderStatusScheduled || st == api.ReminderStatusSnoozed
	today := st == api.ReminderStatusPending ||
		(open && r.Enabled == 1 && dueAt(r) != nil && dueAt(r).Before(endOfToday))
	switch rng {
	case rangeToday:
		return today
	case rangeUpcoming:
		return !today && open
	case rangeDone:
		return st == api.ReminderStatusDone || st == api.ReminderStatusEnded
	}
	return true
}

// sortForRange orders a filtered list: done newest first, others by due time
// with pending ones first.
func sortForRange(rows []db.Reminder, rng string) {
	key := func(r db.Reminder) time.Time {
		if rng == rangeDone {
			for _, t := range []*time.Time{r.DoneAt, r.LastFiredAt} {
				if t != nil {
					return *t
				}
			}
			return r.CreatedAt
		}
		if isPending(r) && r.SnoozedUntil == nil {
			return *r.LastFiredAt
		}
		if d := dueAt(r); d != nil {
			return *d
		}
		return r.CreatedAt
	}
	sort.SliceStable(rows, func(i, j int) bool {
		pi := isPending(rows[i]) && rows[i].SnoozedUntil == nil
		pj := isPending(rows[j]) && rows[j].SnoozedUntil == nil
		if rng != rangeDone && pi != pj {
			return pi
		}
		if rng == rangeDone {
			return key(rows[i]).After(key(rows[j]))
		}
		return key(rows[i]).Before(key(rows[j]))
	})
}

// startOfDay returns local midnight of t's day in loc.
func startOfDay(t time.Time, loc *time.Location) time.Time {
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
}

// quietHours is the stored value of notify.quiet_hours.
type quietHours struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

// parseClock turns "HH:MM" into minutes after midnight.
func parseClock(s string) (int, bool) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

// inQuietHours reports whether now falls in the quiet period. The period may
// cross midnight, for example 23:00-07:30.
func inQuietHours(q quietHours, now time.Time, loc *time.Location) bool {
	if !q.Enabled {
		return false
	}
	start, ok1 := parseClock(q.Start)
	end, ok2 := parseClock(q.End)
	if !ok1 || !ok2 || start == end {
		return false
	}
	l := now.In(loc)
	m := l.Hour()*60 + l.Minute()
	if start < end {
		return m >= start && m < end
	}
	return m >= start || m < end
}
