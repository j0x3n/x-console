package calendar

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/teambition/rrule-go"

	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/db"
)

// This file expands stored events into occurrences for a time range. It is
// pure: tests call it with any rows, range and zone.

// occurrence is one event instance in a range.
type occurrence struct {
	row       db.ListEventCandidatesRow
	start     time.Time
	end       time.Time
	recurring bool
}

// maxOccurrences caps a single expansion so a broken rule cannot flood the
// response.
const maxOccurrences = 5000

// eventZone is the zone an event's wall clock values belong to.
func eventZone(r db.ListEventCandidatesRow, userLoc *time.Location) *time.Location {
	if r.AllDay == 1 {
		return userLoc
	}
	return zoneOr(r.Tzid, userLoc)
}

// span resolves the start of an event (or occurrence) and its end, keeping
// all-day events a whole number of local days long.
func span(r db.ListEventCandidatesRow, start time.Time, loc *time.Location) (time.Time, time.Time) {
	if r.AllDay == 1 {
		days := int(r.EndsAt.Sub(r.StartsAt).Hours()+12) / 24
		if days < 1 {
			days = 1
		}
		return start, start.AddDate(0, 0, days)
	}
	return start, start.Add(r.EndsAt.Sub(r.StartsAt))
}

func overlaps(start, end, from, to time.Time) bool {
	if end.Equal(start) {
		return !start.Before(from) && start.Before(to)
	}
	return start.Before(to) && end.After(from)
}

func decodeTimes(raw string) []time.Time {
	var out []time.Time
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func encodeTimes(ts []time.Time) string {
	if len(ts) == 0 {
		return "[]"
	}
	raw, _ := json.Marshal(ts)
	return string(raw)
}

// ruleFor builds the recurrence of a stored event. Rules repeating more
// often than hourly are ignored: the event then shows once.
func ruleFor(r db.ListEventCandidatesRow, dtstart time.Time, loc *time.Location) (*rrule.RRule, error) {
	s := strings.TrimSpace(r.Rrule)
	if len(s) >= 6 && strings.EqualFold(s[:6], "RRULE:") {
		s = s[6:]
	}
	opt, err := rrule.StrToROptionInLocation(s, loc)
	if err != nil {
		return nil, err
	}
	if opt.Freq == rrule.SECONDLY || opt.Freq == rrule.MINUTELY {
		return nil, fmt.Errorf("repeat too frequent")
	}
	opt.Dtstart = dtstart
	return rrule.NewRRule(*opt)
}

type overrideKey struct {
	calendarID int64
	uid        string
	at         int64 // unix seconds of the replaced occurrence
}

// expand returns the occurrences overlapping [from, to), sorted by start.
func expand(rows []db.ListEventCandidatesRow, from, to time.Time, userLoc *time.Location) []occurrence {
	overridden := map[overrideKey]bool{}
	for _, r := range rows {
		if r.RecurrenceID != nil {
			at := inZone(*r.RecurrenceID, eventZone(r, userLoc))
			overridden[overrideKey{r.CalendarID, r.Uid, at.Unix()}] = true
		}
	}

	var out []occurrence
	for _, r := range rows {
		loc := eventZone(r, userLoc)
		start, end := span(r, inZone(r.StartsAt, loc), loc)
		if r.RecurrenceID != nil || r.Rrule == "" {
			if overlaps(start, end, from, to) {
				out = append(out, occurrence{row: r, start: start, end: end, recurring: r.RecurrenceID != nil})
			}
			continue
		}
		rule, err := ruleFor(r, start, loc)
		if err != nil {
			if overlaps(start, end, from, to) {
				out = append(out, occurrence{row: r, start: start, end: end})
			}
			continue
		}
		skip := map[int64]bool{}
		for _, ex := range decodeTimes(r.Exdates) {
			skip[inZone(ex, loc).Unix()] = true
		}
		dur := end.Sub(start)
		// An occurrence that began before from may still be running.
		starts := rule.Between(from.Add(-dur).Add(-time.Second), to, true)
		for _, rd := range decodeTimes(r.Rdates) {
			starts = append(starts, inZone(rd, loc))
		}
		seen := map[int64]bool{}
		for _, s := range starts {
			key := s.Unix()
			if seen[key] || skip[key] || overridden[overrideKey{r.CalendarID, r.Uid, key}] {
				continue
			}
			seen[key] = true
			os, oe := span(r, s.In(loc), loc)
			if !overlaps(os, oe, from, to) {
				continue
			}
			out = append(out, occurrence{row: r, start: os, end: oe, recurring: true})
			if len(out) >= maxOccurrences {
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if !a.start.Equal(b.start) {
			return a.start.Before(b.start)
		}
		if (a.row.AllDay == 1) != (b.row.AllDay == 1) {
			return a.row.AllDay == 1
		}
		return a.row.Title < b.row.Title
	})
	return out
}
