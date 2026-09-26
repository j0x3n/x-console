package calendar

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	cases := map[string]struct {
		days int
		d    time.Duration
	}{
		"PT1H30M":  {0, 90 * time.Minute},
		"P1D":      {1, 0},
		"P2W":      {14, 0},
		"P1DT2H":   {1, 2 * time.Hour},
		"-PT15M":   {0, -15 * time.Minute},
		"PT45S":    {0, 45 * time.Second},
		"P0DT0H0M": {0, 0},
	}
	for in, want := range cases {
		days, d, err := parseDuration(in)
		if err != nil || days != want.days || d != want.d {
			t.Fatalf("%s: %d %v %v", in, days, d, err)
		}
	}
	for _, bad := range []string{"", "P", "1H", "PT1X"} {
		if _, _, err := parseDuration(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestResolveZone(t *testing.T) {
	cases := map[string]string{
		"Europe/Berlin":                         "Europe/Berlin",
		"China Standard Time":                   "Asia/Shanghai",
		"/mozilla.org/20050126_1/Europe/Berlin": "Europe/Berlin",
		"UTC":                                   "UTC",
		`"America/New_York"`:                    "America/New_York",
	}
	for in, want := range cases {
		if loc := resolveZone(in); loc == nil || loc.String() != want {
			t.Fatalf("%s: %v", in, loc)
		}
	}
	if resolveZone("Mars/Olympus") != nil {
		t.Fatal("unknown zone resolved")
	}
}

func TestParseICSWithBareNewlinesAndCalendarZone(t *testing.T) {
	data := "BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:x\nX-WR-TIMEZONE:Asia/Tokyo\nBEGIN:VEVENT\nUID:a\nDTSTAMP:20261001T000000Z\n" +
		"DTSTART:20261020T090000\nSUMMARY:Local to the calendar\nEND:VEVENT\nEND:VCALENDAR\n"
	events, err := parseICS([]byte(data))
	if err != nil || len(events) != 1 {
		t.Fatalf("%v %v", err, events)
	}
	e := events[0]
	if e.TZID != "Asia/Tokyo" || !e.End.Equal(e.Start) || e.Title != "Local to the calendar" {
		t.Fatalf("%+v", e)
	}
	if _, err := parseICS([]byte("not a calendar")); err == nil {
		t.Fatal("garbage parsed")
	}
}
