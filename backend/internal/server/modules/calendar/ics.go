package calendar

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

// This file turns ICS text into event rows. Nothing here touches the
// database or the network.
//
// Times are kept as wall clock ("floating") values in a UTC container plus
// the IANA zone they belong to. That keeps recurrence expansion correct
// across daylight saving changes: "every Monday 09:00 America/New_York" stays
// at 09:00 local time all year.

// parsedEvent is one VEVENT ready to store.
type parsedEvent struct {
	UID          string
	Title        string
	Location     string
	Description  string
	Start        time.Time // wall clock in TZID (UTC container)
	End          time.Time // wall clock in TZID (UTC container)
	AllDay       bool
	TZID         string // IANA name; empty means floating (user's zone)
	RRule        string
	RDates       []time.Time // wall clock in TZID
	ExDates      []time.Time // wall clock in TZID
	RecurrenceID *time.Time  // wall clock in TZID
	Href         string
	ETag         string
}

// icalTime is a parsed DATE or DATE-TIME value.
type icalTime struct {
	wall     time.Time      // components as written, UTC container
	loc      *time.Location // nil for floating values
	dateOnly bool
}

// instant resolves the value to an absolute time. Floating values and dates
// use userLoc.
func (t icalTime) instant(userLoc *time.Location) time.Time {
	loc := t.loc
	if loc == nil || t.dateOnly {
		loc = userLoc
	}
	return inZone(t.wall, loc)
}

// inZone reads the components of a wall clock value in loc.
func inZone(wall time.Time, loc *time.Location) time.Time {
	return time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), 0, loc)
}

// wallOf stores the components of t as a wall clock value.
func wallOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
}

// resolveZone maps a TZID to a location. It accepts IANA names, Windows
// names (Outlook, Exchange) and prefixed names such as
// "/mozilla.org/20050126_1/Europe/Berlin". Unknown zones return nil.
func resolveZone(tzid string) *time.Location {
	tzid = strings.Trim(strings.TrimSpace(tzid), `"`)
	if tzid == "" {
		return nil
	}
	if strings.EqualFold(tzid, "UTC") || strings.EqualFold(tzid, "GMT") || tzid == "Z" {
		return time.UTC
	}
	if loc, err := time.LoadLocation(tzid); err == nil {
		return loc
	}
	if loc := ics.WindowsTimezoneToIANA(tzid); loc != nil {
		return loc
	}
	// Try the longest "Area/City" suffix of a prefixed id.
	parts := strings.Split(tzid, "/")
	for i := 1; i < len(parts)-1; i++ {
		if loc, err := time.LoadLocation(strings.Join(parts[i:], "/")); err == nil {
			return loc
		}
	}
	return nil
}

var timeValue = regexp.MustCompile(`^(\d{8})(?:T(\d{6})(Z?))?$`)

// parseTime reads a DATE or DATE-TIME property value. calZone is the
// calendar's X-WR-TIMEZONE, used when a value has neither TZID nor Z.
func parseTime(value string, params map[string][]string, calZone *time.Location) (icalTime, error) {
	value = strings.TrimSpace(value)
	m := timeValue.FindStringSubmatch(value)
	if m == nil {
		return icalTime{}, fmt.Errorf("时间格式不对: %q", value)
	}
	if m[2] == "" {
		d, err := time.ParseInLocation("20060102", m[1], time.UTC)
		if err != nil {
			return icalTime{}, err
		}
		return icalTime{wall: d, dateOnly: true}, nil
	}
	wall, err := time.ParseInLocation("20060102150405", m[1]+m[2], time.UTC)
	if err != nil {
		return icalTime{}, err
	}
	if m[3] == "Z" {
		return icalTime{wall: wall, loc: time.UTC}, nil
	}
	if tz := firstParam(params, "TZID"); tz != "" {
		// An unknown zone falls back to floating, the best guess we have.
		return icalTime{wall: wall, loc: resolveZone(tz)}, nil
	}
	return icalTime{wall: wall, loc: calZone}, nil
}

func firstParam(params map[string][]string, name string) string {
	for k, v := range params {
		if strings.EqualFold(k, name) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

var durationValue = regexp.MustCompile(`^([+-])?P(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

// parseDuration reads an RFC 5545 DURATION such as PT1H30M or P1D.
func parseDuration(s string) (days int, d time.Duration, err error) {
	m := durationValue.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil || s == "P" || s == "PT" {
		return 0, 0, fmt.Errorf("时长格式不对: %q", s)
	}
	num := func(i int) int {
		n, _ := strconv.Atoi(m[i])
		return n
	}
	days = num(2)*7 + num(3)
	d = time.Duration(num(4))*time.Hour + time.Duration(num(5))*time.Minute + time.Duration(num(6))*time.Second
	if m[1] == "-" {
		days, d = -days, -d
	}
	return days, d, nil
}

func prop(e *ics.VEvent, p ics.ComponentProperty) *ics.IANAProperty {
	return e.GetProperty(p)
}

func textProp(e *ics.VEvent, p ics.ComponentProperty) string {
	if v := prop(e, p); v != nil {
		return strings.TrimSpace(ics.FromText(v.Value))
	}
	return ""
}

// parseICS reads every VEVENT of an ICS document. Cancelled events are
// dropped; a cancelled occurrence of a repeating event becomes an EXDATE of
// its master. Broken events are skipped, the rest still load.
func parseICS(data []byte) ([]parsedEvent, error) {
	cal, err := ics.ParseCalendar(bytes.NewReader(normalizeICS(data)))
	if err != nil {
		return nil, fmt.Errorf("日历内容无法解析: %w", err)
	}
	var calZone *time.Location
	for _, p := range cal.CalendarProperties {
		if strings.EqualFold(p.IANAToken, "X-WR-TIMEZONE") {
			calZone = resolveZone(p.Value)
		}
	}

	var out []parsedEvent
	type cancel struct {
		uid string
		rid time.Time // instant
	}
	var cancelled []cancel
	var errs []error
	for _, e := range cal.Events() {
		ev, isCancelled, err := parseEvent(e, calZone)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if isCancelled {
			if ev.RecurrenceID != nil {
				cancelled = append(cancelled, cancel{uid: ev.UID, rid: inZone(*ev.RecurrenceID, zoneOr(ev.TZID, time.UTC))})
			}
			continue
		}
		out = append(out, ev)
	}
	if len(out) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	for _, c := range cancelled {
		for i := range out {
			ev := &out[i]
			if ev.UID != c.uid || ev.RRule == "" || ev.RecurrenceID != nil {
				continue
			}
			ev.ExDates = append(ev.ExDates, wallOf(c.rid.In(zoneOr(ev.TZID, time.UTC))))
		}
	}
	return out, nil
}

// normalizeICS fixes line endings so files saved with bare LF parse too.
func normalizeICS(data []byte) []byte {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))
}

// zoneOr loads a stored IANA name. Empty means floating and returns def.
func zoneOr(tzid string, def *time.Location) *time.Location {
	if tzid == "" {
		return def
	}
	if loc := resolveZone(tzid); loc != nil {
		return loc
	}
	return def
}

func parseEvent(e *ics.VEvent, calZone *time.Location) (ev parsedEvent, cancelled bool, err error) {
	ev.UID = textProp(e, ics.ComponentPropertyUniqueId)
	ev.Title = textProp(e, ics.ComponentPropertySummary)
	ev.Location = textProp(e, ics.ComponentPropertyLocation)
	ev.Description = textProp(e, ics.ComponentPropertyDescription)
	cancelled = strings.EqualFold(textProp(e, ics.ComponentPropertyStatus), "CANCELLED")

	sp := prop(e, ics.ComponentPropertyDtStart)
	if sp == nil {
		return ev, false, fmt.Errorf("事件 %q 没有开始时间", ev.Title)
	}
	start, err := parseTime(sp.Value, sp.ICalParameters, calZone)
	if err != nil {
		return ev, false, err
	}
	ev.AllDay = start.dateOnly
	if start.loc != nil && !start.dateOnly {
		ev.TZID = start.loc.String()
	}
	eventLoc := start.loc // nil: floating
	// toWall converts another time value of this event to the event's zone.
	toWall := func(t icalTime) time.Time {
		switch {
		case ev.AllDay || t.dateOnly:
			return time.Date(t.wall.Year(), t.wall.Month(), t.wall.Day(), 0, 0, 0, 0, time.UTC)
		case t.loc == nil || eventLoc == nil || t.loc == eventLoc:
			return t.wall
		default:
			return wallOf(inZone(t.wall, t.loc).In(eventLoc))
		}
	}
	ev.Start = toWall(start)

	switch {
	case prop(e, ics.ComponentPropertyDtEnd) != nil:
		ep := prop(e, ics.ComponentPropertyDtEnd)
		end, err := parseTime(ep.Value, ep.ICalParameters, calZone)
		if err != nil {
			return ev, false, err
		}
		ev.End = toWall(end)
	case prop(e, ics.ComponentPropertyDuration) != nil:
		days, d, err := parseDuration(prop(e, ics.ComponentPropertyDuration).Value)
		if err != nil {
			return ev, false, err
		}
		ev.End = ev.Start.AddDate(0, 0, days).Add(d)
	case ev.AllDay:
		ev.End = ev.Start.AddDate(0, 0, 1)
	default:
		ev.End = ev.Start
	}
	if ev.AllDay && !ev.End.After(ev.Start) {
		ev.End = ev.Start.AddDate(0, 0, 1)
	}
	if ev.End.Before(ev.Start) {
		ev.End = ev.Start
	}

	if rp := prop(e, ics.ComponentPropertyRrule); rp != nil {
		ev.RRule = strings.TrimSpace(rp.Value)
	}
	for _, kind := range []ics.ComponentProperty{ics.ComponentPropertyExdate, ics.ComponentPropertyRdate} {
		for _, p := range e.GetProperties(kind) {
			for _, v := range strings.Split(p.Value, ",") {
				if strings.TrimSpace(v) == "" {
					continue
				}
				t, err := parseTime(v, p.ICalParameters, calZone)
				if err != nil {
					continue
				}
				if kind == ics.ComponentPropertyExdate {
					ev.ExDates = append(ev.ExDates, toWall(t))
				} else {
					ev.RDates = append(ev.RDates, toWall(t))
				}
			}
		}
	}
	if rp := prop(e, ics.ComponentPropertyRecurrenceId); rp != nil {
		t, err := parseTime(rp.Value, rp.ICalParameters, calZone)
		if err == nil {
			w := toWall(t)
			ev.RecurrenceID = &w
		}
	}
	if ev.UID == "" {
		ev.UID = fmt.Sprintf("%s@%s", ev.Title, ev.Start.Format("20060102T150405"))
	}
	return ev, cancelled, nil
}
