package watchers

import (
	"testing"
	"time"
)

func TestCheckWindow(t *testing.T) {
	sh, _ := time.LoadLocation("Asia/Shanghai")
	// 2026-09-07 is a Monday.
	at := func(day, hour, min int) time.Time { return time.Date(2026, 9, day, hour, min, 0, 0, sh) }
	win := func(from, to string, days ...int) checkWindow {
		t.Helper()
		w, _, _, _, err := newCheckWindow(from, to, days)
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	cases := []struct {
		name string
		w    checkWindow
		now  time.Time
		want bool
	}{
		{"no window", checkWindow{}, at(7, 3, 0), true},
		{"inside", win("09:00", "18:00"), at(7, 9, 0), true},
		{"end is excluded", win("09:00", "18:00"), at(7, 18, 0), false},
		{"before", win("09:00", "18:00"), at(7, 8, 59), false},
		{"night", win("09:00", "18:00"), at(7, 23, 0), false},
		{"weekdays only, Monday", win("09:00", "18:00", 1, 2, 3, 4, 5), at(7, 10, 0), true},
		{"weekdays only, Saturday", win("09:00", "18:00", 1, 2, 3, 4, 5), at(12, 10, 0), false},
		{"weekdays only, Sunday", win("09:00", "18:00", 1, 2, 3, 4, 5), at(13, 10, 0), false},
		{"days without times", win("", "", 6, 7), at(12, 3, 0), true},
		{"days without times, Monday", win("", "", 6, 7), at(7, 3, 0), false},
		{"overnight, evening", win("22:00", "06:00"), at(7, 23, 30), true},
		{"overnight, early morning", win("22:00", "06:00"), at(8, 5, 59), true},
		{"overnight, daytime", win("22:00", "06:00"), at(7, 12, 0), false},
		{"overnight, end", win("22:00", "06:00"), at(8, 6, 0), false},
		// The window belongs to the day it starts on: Friday 22:00 runs into Saturday.
		{"overnight Friday, Saturday morning", win("22:00", "06:00", 5), at(12, 2, 0), true},
		{"overnight Friday, Friday morning", win("22:00", "06:00", 5), at(11, 2, 0), false},
		{"overnight Friday, Friday night", win("22:00", "06:00", 5), at(11, 23, 0), true},
		{"overnight Sunday, Monday morning", win("22:00", "06:00", 7), at(14, 2, 0), true},
	}
	for _, c := range cases {
		if got := c.w.open(c.now, sh); got != c.want {
			t.Errorf("%s: open = %v, want %v", c.name, got, c.want)
		}
	}
	// The same instant is read in the server time zone: 02:00 UTC is 10:00 in Shanghai.
	if w := win("09:00", "18:00"); !w.open(time.Date(2026, 9, 7, 2, 0, 0, 0, time.UTC), sh) {
		t.Error("time zone ignored")
	}

	for _, bad := range [][2]string{{"09:00", ""}, {"", "18:00"}, {"9:00", "18:00"}, {"25:00", "18:00"}, {"09:00", "09:00"}, {"09:60", "18:00"}} {
		if _, _, _, _, err := newCheckWindow(bad[0], bad[1], nil); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	for _, d := range []int{0, 8} {
		if _, _, _, _, err := newCheckWindow("", "", []int{d}); err == nil {
			t.Errorf("day %d accepted", d)
		}
	}
	_, from, to, days, err := newCheckWindow(" 09:00", "18:30 ", []int{5, 1, 1, 3})
	if err != nil || from != "09:00" || to != "18:30" || days != "1,3,5" {
		t.Fatalf("stored form: %q %q %q %v", from, to, days, err)
	}
	// Seven days is the same as no filter.
	if _, _, _, days, _ := newCheckWindow("", "", []int{1, 2, 3, 4, 5, 6, 7}); days != "" {
		t.Fatalf("all days stored as %q", days)
	}
	// A broken stored row never silences a watcher.
	if !windowFromStored("zz", "18:00", "x").open(at(7, 3, 0), sh) {
		t.Fatal("broken row closed the window")
	}
}
