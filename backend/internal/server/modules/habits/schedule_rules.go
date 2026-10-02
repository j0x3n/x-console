package habits

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

type scheduleRules struct {
	WorkDays                                          []int
	WakeTime, SleepTime, WorkStart, WorkEnd, Timezone string
	IdleMinutes                                       int
}

type reminderPolicy struct {
	When                                                []string
	Window                                              string
	Interval                                            time.Duration
	LastCheckin, LastReminded, QuietUntil, SnoozedUntil *time.Time
}

type activityReminder struct {
	Started       time.Time
	LastActive    time.Time
	InactiveSince time.Time
	WindowStart   time.Time
	Next          time.Time
	Snoozed       bool
}

func normalizeRemindWhen(in []string) ([]string, error) {
	out := []string{}
	for _, value := range in {
		switch value {
		case "window", "awake", "work", "active":
			if !slices.Contains(out, value) {
				out = append(out, value)
			}
		default:
			return nil, fmt.Errorf("提醒条件无效")
		}
	}
	if len(out) == 0 {
		out = append(out, "window")
	}
	return out, nil
}

func (s scheduleRules) validate() error {
	seen := map[int]bool{}
	for _, day := range s.WorkDays {
		if day < 1 || day > 7 || seen[day] {
			return fmt.Errorf("工作日要选周一到周日，同一天只能选一次")
		}
		seen[day] = true
	}
	wake, validWake := parseClock(s.WakeTime)
	sleep, validSleep := parseClock(s.SleepTime)
	if !validWake || !validSleep || wake >= 1440 || sleep >= 1440 || wake == sleep {
		return fmt.Errorf("起床和睡觉时间要写成 12:00 这样，两个时间要不同")
	}
	if s.WorkStart != "" || s.WorkEnd != "" {
		start, validStart := parseClock(s.WorkStart)
		end, validEnd := parseClock(s.WorkEnd)
		if !validStart || !validEnd || start >= 1440 || end >= 1440 || start == end {
			return fmt.Errorf("工作开始和结束时间要一起填写，两个时间要不同")
		}
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil || strings.TrimSpace(s.Timezone) == "" || s.Timezone == "Local" {
		return fmt.Errorf("时区无效")
	}
	if s.IdleMinutes < 1 || s.IdleMinutes > 120 {
		return fmt.Errorf("空闲时间要在 1 到 120 分钟之间")
	}
	return nil
}

func (s scheduleRules) location() *time.Location {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func parseScheduleWindow(value string) (int, int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, 1440, nil
	}
	from, to, ok := strings.Cut(strings.TrimSpace(value), "-")
	start, validStart := parseClock(from)
	end, validEnd := parseClock(to)
	if !ok || !validStart || !validEnd || start >= 1440 || start == end {
		return 0, 0, fmt.Errorf("提醒时段要写成 09:00-21:00 这样，开始和结束要不同")
	}
	return start, end, nil
}

func clockOn(day time.Time, minute int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), minute/60, minute%60, 0, 0, day.Location())
}

func clockWindow(now time.Time, from, to string, loc *time.Location) (time.Time, time.Time, bool) {
	start, ok1 := parseClock(from)
	end, ok2 := parseClock(to)
	if !ok1 || !ok2 || start >= 1440 || start == end {
		return time.Time{}, time.Time{}, false
	}
	day := startOfDay(now, loc)
	for _, offset := range []int{-1, 0} {
		date := day.AddDate(0, 0, offset)
		a, b := clockOn(date, start), clockOn(date, end)
		if end < start {
			b = clockOn(date.AddDate(0, 0, 1), end)
		}
		if !now.Before(a) && now.Before(b) {
			return a, b, true
		}
	}
	return time.Time{}, time.Time{}, false
}

func (s scheduleRules) window(now time.Time, when []string, legacy string) (time.Time, time.Time, bool) {
	loc := s.location()
	start, end := time.Time{}, time.Time{}
	if len(when) == 0 {
		when = []string{"window"}
	}
	for _, condition := range when {
		if condition == "active" {
			continue
		}
		var a, b time.Time
		var ok bool
		switch condition {
		case "awake":
			a, b, ok = clockWindow(now, s.WakeTime, s.SleepTime, loc)
		case "work":
			a, b, ok = clockWindow(now, s.WorkStart, s.WorkEnd, loc)
			if ok {
				ok = slices.Contains(s.WorkDays, int(isoWeekday(a)))
			}
		case "window":
			from, to, err := parseScheduleWindow(legacy)
			if err == nil {
				a, b, ok = clockWindow(now, clockString(from), clockString(to), loc)
			}
		default:
			return time.Time{}, time.Time{}, false
		}
		if !ok {
			return time.Time{}, time.Time{}, false
		}
		if start.IsZero() || a.After(start) {
			start = a
		}
		if end.IsZero() || b.Before(end) {
			end = b
		}
	}
	if start.IsZero() {
		start = now.Add(-24 * time.Hour)
		end = now.AddDate(100, 0, 0)
	}
	return start, end, start.Before(end) && !now.Before(start) && now.Before(end)
}

func clockString(minute int) string {
	if minute == 1440 {
		return "24:00"
	}
	return time.Date(2000, 1, 1, minute/60, minute%60, 0, 0, time.UTC).Format("15:04")
}

func (s scheduleRules) nextWindow(now time.Time, when []string, legacy string) (time.Time, time.Time, bool) {
	if a, b, ok := s.window(now, when, legacy); ok {
		return a, b, true
	}
	if len(when) == 0 {
		when = []string{"window"}
	}
	minutes := []int{}
	for _, condition := range when {
		switch condition {
		case "awake":
			if minute, ok := parseClock(s.WakeTime); ok {
				minutes = append(minutes, minute)
			}
		case "work":
			if len(s.WorkDays) == 0 {
				return time.Time{}, time.Time{}, false
			}
			if minute, ok := parseClock(s.WorkStart); ok {
				minutes = append(minutes, minute)
			}
		case "window":
			if minute, _, err := parseScheduleWindow(legacy); err == nil {
				minutes = append(minutes, minute)
			}
		}
	}
	day := startOfDay(now, s.location())
	points := []time.Time{}
	for offset := -1; offset < 9; offset++ {
		for _, minute := range minutes {
			point := clockOn(day.AddDate(0, 0, offset), minute)
			if point.After(now) {
				points = append(points, point)
			}
		}
	}
	slices.SortFunc(points, func(a, b time.Time) int { return a.Compare(b) })
	for _, point := range points {
		if a, b, ok := s.window(point, when, legacy); ok {
			return a, b, true
		}
	}
	return time.Time{}, time.Time{}, false
}

func hostActive(p contracts.HostPresence, idleMinutes int) bool {
	if !p.Online {
		return false
	}
	if p.Locked != nil && *p.Locked || p.DisplayOff != nil && *p.DisplayOff {
		return false
	}
	return !p.Known || p.IdleSeconds != nil && *p.IdleSeconds < int64(idleMinutes*60)
}

func (a *activityReminder) evaluate(s scheduleRules, p reminderPolicy, now time.Time, hosts []contracts.HostPresence) (bool, *time.Time) {
	if p.Interval <= 0 {
		return false, nil
	}
	start, end, allowed := s.window(now, p.When, p.Window)
	activeRequired := slices.Contains(p.When, "active")
	active := !activeRequired
	for _, host := range hosts {
		active = active || hostActive(host, max(1, s.IdleMinutes))
	}
	if !allowed {
		a.Started = time.Time{}
		a.Next = time.Time{}
		a.WindowStart = time.Time{}
		a.Snoozed = false
		if activeRequired {
			return false, nil
		}
		if start, end, ok := s.nextWindow(now, p.When, p.Window); ok {
			next := start.Add(p.Interval)
			if next.Before(end) {
				return false, &next
			}
		}
		return false, nil
	}
	if activeRequired && !active {
		if a.InactiveSince.IsZero() {
			var latest time.Time
			for _, host := range hosts {
				at := host.Since
				if at.IsZero() || at.After(now) {
					at = now
				}
				if host.Known && host.IdleSeconds != nil && *host.IdleSeconds >= int64(max(1, s.IdleMinutes)*60) {
					idleAt := now.Add(-time.Duration(min(*host.IdleSeconds, int64(365*24*60*60))) * time.Second)
					if idleAt.Before(at) {
						at = idleAt
					}
				}
				if at.After(latest) {
					latest = at
				}
			}
			if latest.IsZero() {
				latest = now
			}
			a.InactiveSince = latest
		}
		reset := now.Sub(a.InactiveSince) >= time.Duration(max(1, s.IdleMinutes))*time.Minute
		if p.SnoozedUntil != nil && !now.Before(*p.SnoozedUntil) {
			reset = true
		}
		if reset || !a.Next.IsZero() && !now.Before(a.Next) {
			a.Started = time.Time{}
			a.Next = time.Time{}
			a.Snoozed = false
		}
		return false, nil
	}
	if activeRequired {
		if !a.InactiveSince.IsZero() {
			if now.Sub(a.InactiveSince) >= time.Duration(max(1, s.IdleMinutes))*time.Minute || !a.Next.IsZero() && !now.Before(a.Next) {
				a.Started = time.Time{}
				a.Next = time.Time{}
				a.Snoozed = false
			}
			a.InactiveSince = time.Time{}
		}
		if a.Started.IsZero() {
			a.Started = now
			a.Next = now.Add(p.Interval)
		}
		a.LastActive = now
	} else if a.WindowStart.IsZero() || !a.WindowStart.Equal(start) {
		a.WindowStart = start
		a.Started = start
		a.Next = start.Add(p.Interval)
		a.Snoozed = false
	}
	if p.SnoozedUntil != nil && (p.LastReminded == nil || p.SnoozedUntil.After(*p.LastReminded)) && (p.LastCheckin == nil || p.SnoozedUntil.After(*p.LastCheckin)) && (!activeRequired || !a.Started.After(*p.SnoozedUntil)) {
		a.Next = *p.SnoozedUntil
		a.Snoozed = true
	}
	if !a.Snoozed {
		ref := a.Started
		for _, at := range []*time.Time{p.LastCheckin, p.LastReminded} {
			if at != nil && at.After(ref) {
				ref = *at
			}
		}
		expected := ref.Add(p.Interval)
		if a.Next.IsZero() || expected.After(a.Next) {
			a.Next = expected
		}
	}
	if p.QuietUntil != nil && p.QuietUntil.After(a.Next) {
		a.Next = *p.QuietUntil
	}
	if !a.Next.Before(end) {
		if activeRequired {
			return false, nil
		}
		if nextStart, nextEnd, ok := s.nextWindow(end, p.When, p.Window); ok {
			next := nextStart.Add(p.Interval)
			if next.Before(nextEnd) {
				return false, &next
			}
		}
		return false, nil
	}
	next := a.Next
	return !now.Before(next), &next
}

func fixedReminder(points []string, last, quiet *time.Time, now time.Time, loc *time.Location) (bool, *time.Time) {
	if quiet != nil && (last == nil || quiet.After(*last)) {
		if now.Before(*quiet) {
			return false, quiet
		}
		if now.Sub(*quiet) < time.Minute {
			return true, quiet
		}
	}
	day := startOfDay(now, loc)
	for offset := 0; offset < 2; offset++ {
		for _, raw := range points {
			minute, ok := parseClock(raw)
			if !ok || minute >= 1440 {
				continue
			}
			point := clockOn(day.AddDate(0, 0, offset), minute)
			if last != nil && !last.Before(point) || quiet != nil && point.Before(*quiet) {
				continue
			}
			if point.After(now) {
				return false, &point
			}
			if now.Sub(point) < time.Minute {
				return true, &point
			}
		}
	}
	return false, nil
}

func (a *activityReminder) sent(now time.Time) {
	a.Started = now
	a.Next = time.Time{}
	a.Snoozed = false
}
func (a *activityReminder) checkin(now time.Time) {
	a.Started = now
	a.Next = time.Time{}
	a.Snoozed = false
}
func (a *activityReminder) snooze(now time.Time) {
	a.Next = now.Add(10 * time.Minute)
	a.Snoozed = true
}
