package habits

import (
	"reflect"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/db"
)

var shanghai = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return loc
}()

func at(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, shanghai) }

// simulate runs the reminder rule minute by minute over one day. When
// drink is true the user checks in right after each reminder.
func simulate(s remindState, day int, drink bool) (fired []string) {
	for now := at(day, 0, 0); now.Before(at(day+1, 0, 0)); now = now.Add(time.Minute) {
		if !shouldRemind(s, now, shanghai) {
			continue
		}
		fired = append(fired, now.Format("15:04"))
		t := now
		s.LastReminded = &t
		if drink {
			s.DoneToday++
			s.LastCheckin = &t
		}
	}
	return fired
}

func water() remindState {
	return remindState{Mode: modeInterval, Interval: time.Hour, Window: "09:00-21:00", Target: 8}
}

func TestIntervalRemindersStayInWindow(t *testing.T) {
	got := simulate(water(), 1, false)
	want := []string{"10:00", "11:00", "12:00", "13:00", "14:00", "15:00", "16:00", "17:00", "18:00", "19:00", "20:00"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestIntervalRemindersStopAtTarget(t *testing.T) {
	got := simulate(water(), 1, true)
	if len(got) != 8 || got[7] != "17:00" {
		t.Fatalf("got %v", got)
	}
}

func TestIntervalCountsFromLastCheckin(t *testing.T) {
	s := water()
	drank := at(1, 9, 40)
	s.LastCheckin = &drank
	s.DoneToday = 1
	if shouldRemind(s, at(1, 10, 0), shanghai) {
		t.Fatal("reminded 20 minutes after drinking")
	}
	if !shouldRemind(s, at(1, 10, 40), shanghai) {
		t.Fatal("no reminder an hour after drinking")
	}
	quiet := at(2, 0, 0)
	s.QuietUntil = &quiet
	if shouldRemind(s, at(1, 12, 0), shanghai) {
		t.Fatal("reminded after skip")
	}
}

func TestIntervalNewDayStartsOver(t *testing.T) {
	s := water()
	s.DoneToday = 0 // a new day: the caller computes today's total by local date
	yesterday := at(1, 20, 0)
	s.LastReminded = &yesterday
	s.LastCheckin = &yesterday
	got := simulate(s, 2, false)
	if len(got) == 0 || got[0] != "10:00" {
		t.Fatalf("next day: %v", got)
	}
}

func TestTimesMode(t *testing.T) {
	s := remindState{Mode: modeTimes, Times: []string{"08:00", "20:00"}, Target: 1}
	if got := simulate(s, 1, false); !reflect.DeepEqual(got, []string{"08:00", "20:00"}) {
		t.Fatalf("got %v", got)
	}
	s.DoneToday = 1
	if got := simulate(s, 1, false); len(got) != 0 {
		t.Fatalf("reached but reminded: %v", got)
	}
	// Server was down at 08:00: no reminder more than an hour late.
	s.DoneToday = 0
	if shouldRemind(s, at(1, 9, 30), shanghai) {
		t.Fatal("late reminder")
	}
	if !shouldRemind(s, at(1, 8, 45), shanghai) {
		t.Fatal("slightly late reminder skipped")
	}
}

func TestDailyTotalsUseLocalDate(t *testing.T) {
	logs := []db.HabitLog{
		{At: at(1, 23, 59).UTC(), Amount: 1},
		{At: at(2, 0, 1).UTC(), Amount: 2},
		{At: at(1, 7, 0).UTC(), Amount: 1}, // 23:00 UTC on Sep 30
	}
	totals := dailyTotals(logs, shanghai)
	if totals["2026-10-01"] != 2 || totals["2026-10-02"] != 2 {
		t.Fatalf("totals: %v", totals)
	}
}

func TestStreakAndSeries(t *testing.T) {
	totals := map[string]float64{
		"2026-10-05": 8, // today
		"2026-10-04": 8,
		"2026-10-03": 9,
		"2026-10-02": 3,
		"2026-10-01": 8,
	}
	now := at(5, 12, 0)
	if n := streak(totals, 8, now, shanghai); n != 3 {
		t.Fatalf("streak with today: %d", n)
	}
	totals["2026-10-05"] = 2
	if n := streak(totals, 8, now, shanghai); n != 2 {
		t.Fatalf("streak without today: %d", n)
	}
	days, best := series(totals, 8, 5, now, shanghai)
	if len(days) != 5 || days[0].Date != "2026-10-01" || days[4].Date != "2026-10-05" || best != 2 {
		t.Fatalf("series %+v best %d", days, best)
	}
}

func TestParseWindowAndTimes(t *testing.T) {
	for _, bad := range []string{"9-21", "21:00-09:00", "09:00", "09:00-25:00"} {
		if _, _, err := parseWindow(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if s, e, err := parseWindow("09:00-24:00"); err != nil || s != 540 || e != 1440 {
		t.Errorf("24:00 end: %d %d %v", s, e, err)
	}
	got, err := normalizeTimes([]string{"20:00", "08:00", "20:00"})
	if err != nil || !reflect.DeepEqual(got, []string{"08:00", "20:00"}) {
		t.Errorf("times: %v %v", got, err)
	}
	if _, err := normalizeTimes([]string{"8am"}); err == nil {
		t.Error("8am accepted")
	}
}

func TestWorkoutDue(t *testing.T) {
	if workoutDue(true, "08:00", "", at(1, 7, 59), shanghai) {
		t.Error("early")
	}
	if !workoutDue(true, "08:00", "2026-09-30", at(1, 8, 0), shanghai) {
		t.Error("on time")
	}
	if workoutDue(true, "08:00", "2026-10-01", at(1, 8, 5), shanghai) {
		t.Error("twice a day")
	}
	if workoutDue(true, "08:00", "", at(1, 11, 0), shanghai) {
		t.Error("3 hours late")
	}
	if workoutDue(false, "08:00", "", at(1, 8, 0), shanghai) {
		t.Error("disabled")
	}
	if isoWeekday(at(4, 12, 0)) != 7 || isoWeekday(at(5, 12, 0)) != 1 {
		t.Error("weekday numbering")
	}
}
