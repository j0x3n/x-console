package habits

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// This file holds the pure rules: reminder timing, streaks and daily totals.
// Every function takes "now" so tests can drive the clock.

// Reminder modes.
const (
	modeNone     = "none"
	modeInterval = "interval"
	modeTimes    = "times"

	// Target modes (B202).
	targetFixed   = "fixed"
	targetPrompts = "prompts"

	// Automatic check-ins from the sleep the "me" module saw (B201).
	autoNone    = "none"
	autoBedtime = "bedtime"
	autoWake    = "wake"
)

// timesGrace is how late a fixed-time reminder may still go out, for example
// right after the server restarted.
const timesGrace = time.Hour

// dateKey is the local calendar date of t, "2006-01-02".
func dateKey(t time.Time, loc *time.Location) string { return t.In(loc).Format(time.DateOnly) }

// startOfDay returns local midnight of t's day.
func startOfDay(t time.Time, loc *time.Location) time.Time {
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
}

// parseClock turns "HH:MM" into minutes after midnight. "24:00" is allowed
// as the end of a window.
func parseClock(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "24:00" {
		return 24 * 60, true
	}
	t, err := time.Parse("15:04", s)
	if err != nil || len(s) != 5 {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

// parseWindow parses "09:00-21:00". Empty means the whole day.
func parseWindow(s string) (start, end int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 24 * 60, nil
	}
	a, b, ok := strings.Cut(s, "-")
	start, ok1 := parseClock(a)
	end, ok2 := parseClock(b)
	if !ok || !ok1 || !ok2 || start >= end || start == 24*60 {
		return 0, 0, fmt.Errorf("提醒时段要写成 09:00-21:00 这样，开始早于结束")
	}
	return start, end, nil
}

// remindState is everything shouldRemind needs to know about one habit.
type remindState struct {
	Mode         string
	Interval     time.Duration
	Window       string
	Times        []string
	Target       float64
	DoneToday    float64
	LastCheckin  *time.Time // latest log of any day
	LastReminded *time.Time
	QuietUntil   *time.Time // "skip" pressed on a notification
}

// shouldRemind decides whether a habit reminder goes out at now.
//
// Interval mode: inside the window, once interval has passed since the later
// of the window start, the last check-in and the last reminder.
// Times mode: at each listed time, at most once, and not more than an hour
// late. Both stop once today's target is reached.
func shouldRemind(s remindState, now time.Time, loc *time.Location) bool {
	if s.Mode == modeNone || s.Mode == "" || s.DoneToday >= s.Target {
		return false
	}
	if s.QuietUntil != nil && now.Before(*s.QuietUntil) {
		return false
	}
	day := startOfDay(now, loc)
	local := now.In(loc)
	minute := local.Hour()*60 + local.Minute()
	switch s.Mode {
	case modeInterval:
		if s.Interval <= 0 {
			return false
		}
		start, end, err := parseWindow(s.Window)
		if err != nil || minute < start || minute >= end {
			return false
		}
		ref := day.Add(time.Duration(start) * time.Minute)
		for _, t := range []*time.Time{s.LastCheckin, s.LastReminded} {
			if t != nil && t.After(ref) {
				ref = *t
			}
		}
		return now.Sub(ref) >= s.Interval
	case modeTimes:
		var latest time.Time
		for _, raw := range s.Times {
			m, ok := parseClock(raw)
			if !ok {
				continue
			}
			point := day.Add(time.Duration(m) * time.Minute)
			if !point.After(now) && point.After(latest) {
				latest = point
			}
		}
		if latest.IsZero() || now.Sub(latest) > timesGrace {
			return false
		}
		return s.LastReminded == nil || s.LastReminded.Before(latest)
	}
	return false
}

// normalizeTimes validates, sorts and de-duplicates "HH:MM" values.
func normalizeTimes(in []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if _, ok := parseClock(t); !ok || t == "24:00" {
			return nil, fmt.Errorf("时间点要写成 08:00 这样: %s", t)
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out, nil
}

// dayGoal tells the target of a local date and whether the day was reached.
// A day that does not count (no reminder went out and nothing was done in
// the prompts mode) neither adds to a streak nor breaks it.
type dayGoal func(day string, done float64) (target float64, reached, counts bool)

// fixedGoal is the same target every day.
func fixedGoal(target float64) dayGoal {
	return func(_ string, done float64) (float64, bool, bool) { return target, done >= target, true }
}

// goalOf is the daily goal of a habit. prompts maps local dates to the number
// of reminders sent that day (B202).
func goalOf(targetMode string, target float64, prompts map[string]float64) dayGoal {
	if targetMode != targetPrompts {
		return fixedGoal(target)
	}
	return func(day string, done float64) (float64, bool, bool) {
		n := prompts[day]
		if n == 0 {
			return 0, done > 0, done > 0
		}
		return n, done >= n, true
	}
}

// streak counts reached days going back from yesterday, plus today when it
// is already reached. totals maps local dates to amounts.
func streak(totals map[string]float64, goal dayGoal, now time.Time, loc *time.Location) int {
	day := startOfDay(now, loc)
	n := 0
	for i := 1; i <= 3660; i++ {
		key := day.AddDate(0, 0, -i).Format(time.DateOnly)
		_, reached, counts := goal(key, totals[key])
		if !counts {
			continue
		}
		if !reached {
			break
		}
		n++
	}
	key := day.Format(time.DateOnly)
	if _, reached, _ := goal(key, totals[key]); reached {
		n++
	}
	return n
}

// dayStat is one day of the stats series.
type dayStat struct {
	Date    string
	Amount  float64
	Target  float64
	Reached bool
}

// series returns the last `days` days ending today, oldest first, and the
// longest run of reached days inside it.
func series(totals map[string]float64, goal dayGoal, days int, now time.Time, loc *time.Location) (out []dayStat, best int) {
	day := startOfDay(now, loc)
	run := 0
	for i := days - 1; i >= 0; i-- {
		key := day.AddDate(0, 0, -i).Format(time.DateOnly)
		target, reached, counts := goal(key, totals[key])
		d := dayStat{Date: key, Amount: totals[key], Target: target, Reached: reached}
		if d.Reached {
			run++
			if run > best {
				best = run
			}
		} else if counts {
			run = 0
		}
		out = append(out, d)
	}
	return out, best
}

// workoutDue reports whether the daily workout notice should go out at now:
// at or after the configured time, at most 2 hours late, once per day.
func workoutDue(enabled bool, at string, lastDate string, now time.Time, loc *time.Location) bool {
	if !enabled {
		return false
	}
	m, ok := parseClock(at)
	if !ok {
		return false
	}
	point := startOfDay(now, loc).Add(time.Duration(m) * time.Minute)
	if now.Before(point) || now.Sub(point) > 2*time.Hour {
		return false
	}
	return lastDate != dateKey(now, loc)
}

// isoWeekday is 1 for Monday through 7 for Sunday.
func isoWeekday(t time.Time) int64 {
	if t.Weekday() == time.Sunday {
		return 7
	}
	return int64(t.Weekday())
}
