package watchers

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ---- check window (B236) ----

// checkWindow is when a watcher may be checked. from and to are minutes since
// midnight; hasTimes is false when the watcher checks all day. days holds ISO
// weekdays (Monday 1, Sunday 7), empty for every day. to < from crosses
// midnight and the window belongs to the day it starts on.
type checkWindow struct {
	from, to int
	hasTimes bool
	days     []int
}

// parseClock reads HH:MM.
func parseClock(s string) (int, bool) {
	h, m, ok := strings.Cut(s, ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return 0, false
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, false
	}
	return hh*60 + mm, true
}

func formatClock(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

// newCheckWindow validates user input and returns the window together with
// its stored form (from, to, comma separated days).
func newCheckWindow(from, to string, days []int) (w checkWindow, outFrom, outTo, outDays string, err error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from != "" || to != "" {
		f, okF := parseClock(from)
		t, okT := parseClock(to)
		switch {
		case from == "" || to == "":
			return w, "", "", "", errors.New("检查时段要同时填开始和结束时间")
		case !okF || !okT:
			return w, "", "", "", errors.New("检查时段的时间格式是 HH:MM，例如 09:00")
		case f == t:
			return w, "", "", "", errors.New("检查时段的开始和结束不能是同一时间")
		}
		w.from, w.to, w.hasTimes = f, t, true
		outFrom, outTo = formatClock(f), formatClock(t)
	}
	for _, d := range days {
		if d < 1 || d > 7 {
			return w, "", "", "", errors.New("检查日期要在 1（周一）到 7（周日）之间")
		}
		if !slices.Contains(w.days, d) {
			w.days = append(w.days, d)
		}
	}
	slices.Sort(w.days)
	if len(w.days) == 7 {
		w.days = nil
	}
	parts := make([]string, len(w.days))
	for i, d := range w.days {
		parts[i] = strconv.Itoa(d)
	}
	return w, outFrom, outTo, strings.Join(parts, ","), nil
}

// windowFromStored rebuilds the window from the stored columns. Bad values
// (which validation keeps out) mean no window, so a watcher is never silenced
// by a broken row.
func windowFromStored(from, to, days string) checkWindow {
	var ds []int
	if days != "" {
		for _, part := range strings.Split(days, ",") {
			if d, err := strconv.Atoi(part); err == nil {
				ds = append(ds, d)
			}
		}
	}
	w, _, _, _, err := newCheckWindow(from, to, ds)
	if err != nil {
		return checkWindow{}
	}
	return w
}

// isoWeekday returns Monday 1 to Sunday 7.
func isoWeekday(t time.Time) int {
	if d := int(t.Weekday()); d != 0 {
		return d
	}
	return 7
}

// open reports whether now, read in loc, is inside the window.
func (w checkWindow) open(now time.Time, loc *time.Location) bool {
	if loc != nil {
		now = now.In(loc)
	}
	today := isoWeekday(now)
	dayOK := func(d int) bool { return len(w.days) == 0 || slices.Contains(w.days, d) }
	if !w.hasTimes {
		return dayOK(today)
	}
	mm := now.Hour()*60 + now.Minute()
	if w.from < w.to {
		return dayOK(today) && mm >= w.from && mm < w.to
	}
	if mm >= w.from {
		return dayOK(today)
	}
	yesterday := today - 1
	if yesterday == 0 {
		yesterday = 7
	}
	return mm < w.to && dayOK(yesterday)
}
