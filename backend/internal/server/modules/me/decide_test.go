package me

import (
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

var cst = time.FixedZone("CST", 8*3600)

// at builds a time on 2026-10-11 (Sunday) in the schedule's zone.
func at(h, m int) time.Time { return time.Date(2026, 10, 11, h, m, 0, 0, cst) }

// nightOwl is the default schedule: up at 12:00, bed at 03:30.
var nightOwl = &schedule{Wake: 12 * 60, Sleep: 3*60 + 30, Loc: cst}

// regular goes to bed at 23:00 and gets up at 07:00, so its window crosses midnight.
var regular = &schedule{Wake: 7 * 60, Sleep: 23 * 60, Loc: cst}

func TestDecideRuleOrder(t *testing.T) {
	now := at(15, 0)
	cases := []struct {
		name string
		in   input
		want string
		src  string
	}{
		{"nothing known", input{Now: now}, contracts.MeUnknown, ""},
		{"manual wins over everything", input{Now: now, Manual: &manualSetting{State: contracts.MeOut, Until: now.Add(time.Hour)}, PCActive: true, Sleep: report{Event: evSleepStart, Source: srcPhone, At: at(14, 0)}}, contracts.MeOut, srcManual},
		{"manual with no end", input{Now: now, Manual: &manualSetting{State: contracts.MeSleeping}, PCActive: true}, contracts.MeSleeping, srcManual},
		{"manual that ran out", input{Now: now, Manual: &manualSetting{State: contracts.MeOut, Until: now}, PCActive: true}, contracts.MeWorking, srcPC},
		{"computer in use", input{Now: now, PCActive: true}, contracts.MeWorking, srcPC},
		{"computer in use beats sleep", input{Now: now, PCActive: true, Sleep: report{Event: evSleepStart, Source: srcPhone, At: at(14, 0)}}, contracts.MeWorking, srcPC},
		{"computer in use beats out", input{Now: now, PCActive: true, Place: report{Event: evHomeLeave, Source: srcPhone, At: at(14, 0)}}, contracts.MeWorking, srcPC},
		{"sleep started on the phone", input{Now: now, Sleep: report{Event: evSleepStart, Source: srcPhone, At: at(14, 0)}}, contracts.MeSleeping, srcPhone},
		{"sleep from Home Assistant", input{Now: now, Sleep: report{Event: evSleepStart, Source: srcHA, At: at(14, 0)}}, contracts.MeSleeping, srcHA},
		{"sleep ended", input{Now: now, Sleep: report{Event: evSleepEnd, Source: srcPhone, At: at(14, 0)}}, contracts.MeUnknown, ""},
		{"a sleep nobody ended is dropped after 16 hours", input{Now: now, Sleep: report{Event: evSleepStart, Source: srcPhone, At: now.Add(-16 * time.Hour)}}, contracts.MeUnknown, ""},
		{"sleep beats out", input{Now: now, Sleep: report{Event: evSleepStart, Source: srcPhone, At: at(14, 0)}, Place: report{Event: evHomeLeave, Source: srcPhone, At: at(13, 0)}}, contracts.MeSleeping, srcPhone},
		{"left home", input{Now: now, Place: report{Event: evHomeLeave, Source: srcPhone, At: at(14, 0)}}, contracts.MeOut, srcPhone},
		{"out beats away", input{Now: now, LastActive: now.Add(-5 * time.Minute), Place: report{Event: evHomeLeave, Source: srcHA, At: at(14, 55)}}, contracts.MeOut, srcHA},
		{"away just after use", input{Now: now, LastActive: now.Add(-time.Second)}, contracts.MeAway, srcPC},
		{"away 29 minutes after use", input{Now: now, LastActive: now.Add(-29*time.Minute - 59*time.Second)}, contracts.MeAway, srcPC},
		{"not away after 30 minutes", input{Now: now, LastActive: now.Add(-30 * time.Minute)}, contracts.MeUnknown, ""},
		{"away beats home", input{Now: now, LastActive: now.Add(-10 * time.Minute), Place: report{Event: evHomeArrive, Source: srcPhone, At: at(9, 0)}}, contracts.MeAway, srcPC},
		{"at home", input{Now: now, LastActive: now.Add(-40 * time.Minute), Place: report{Event: evHomeArrive, Source: srcPhone, At: at(9, 0)}}, contracts.MeHome, srcPhone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := decide(c.in)
			if got.State != c.want || got.Source != c.src {
				t.Fatalf("got %s from %q, want %s from %q", got.State, got.Source, c.want, c.src)
			}
			if got.Manual != (c.src == srcManual) {
				t.Fatalf("manual = %v", got.Manual)
			}
			if got.Guessed {
				t.Fatalf("guessed without a schedule")
			}
		})
	}
}

func TestDecideAtHome(t *testing.T) {
	now := at(15, 0)
	if d := decide(input{Now: now}); d.AtHome != nil {
		t.Fatalf("atHome = %v with no report", *d.AtHome)
	}
	if d := decide(input{Now: now, Place: report{Event: evHomeArrive, At: at(9, 0)}}); d.AtHome == nil || !*d.AtHome {
		t.Fatalf("atHome after arriving: %v", d.AtHome)
	}
	if d := decide(input{Now: now, PCActive: true, Place: report{Event: evHomeLeave, At: at(9, 0)}}); d.AtHome == nil || *d.AtHome {
		t.Fatalf("atHome after leaving: %v", d.AtHome)
	}
}

func TestGuessedSleepHours(t *testing.T) {
	long := func(now time.Time, s *schedule) decision {
		return decide(input{Now: now, GuessSleep: true, Schedule: s, LastActive: now.Add(-time.Hour)})
	}
	cases := []struct {
		name  string
		s     *schedule
		h, m  int
		sleep bool
	}{
		{"night owl, before the window", nightOwl, 2, 29, false},
		{"night owl, window opens an hour before bed", nightOwl, 2, 30, true},
		{"night owl, bedtime", nightOwl, 3, 30, true},
		{"night owl, morning", nightOwl, 8, 0, true},
		{"night owl, one minute before wake", nightOwl, 11, 59, true},
		{"night owl, wake time", nightOwl, 12, 0, false},
		{"night owl, afternoon", nightOwl, 15, 0, false},
		{"regular, before the window", regular, 21, 59, false},
		{"regular, window opens", regular, 22, 0, true},
		{"regular, midnight", regular, 0, 0, true},
		{"regular, early morning", regular, 6, 59, true},
		{"regular, wake time", regular, 7, 0, false},
		{"regular, noon", regular, 12, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := long(at(c.h, c.m), c.s)
			if c.sleep {
				if d.State != contracts.MeSleeping || !d.Guessed || d.Source != srcSchedule {
					t.Fatalf("got %+v, want guessed sleeping", d)
				}
			} else if d.State == contracts.MeSleeping {
				t.Fatalf("got %+v, want not sleeping", d)
			}
		})
	}
}

func TestGuessedSleepNeedsAnIdleComputer(t *testing.T) {
	now := at(4, 0)
	base := input{Now: now, GuessSleep: true, Schedule: nightOwl}
	// Used 29 minutes ago: away, not asleep.
	away := base
	away.LastActive = now.Add(-29 * time.Minute)
	if d := decide(away); d.State != contracts.MeAway {
		t.Fatalf("29 minutes: %+v", d)
	}
	// Used 30 minutes ago: guessed asleep.
	idle := base
	idle.LastActive = now.Add(-30 * time.Minute)
	if d := decide(idle); d.State != contracts.MeSleeping || !d.Guessed {
		t.Fatalf("30 minutes: %+v", d)
	}
	// In use: working, never asleep.
	use := base
	use.PCActive = true
	if d := decide(use); d.State != contracts.MeWorking {
		t.Fatalf("in use: %+v", d)
	}
	// Left home: out.
	out := idle
	out.Place = report{Event: evHomeLeave, At: at(1, 0)}
	if d := decide(out); d.State != contracts.MeOut {
		t.Fatalf("out: %+v", d)
	}
	// Arrived home: still a guess of sleeping, which comes before "home".
	home := idle
	home.Place = report{Event: evHomeArrive, At: at(1, 0)}
	if d := decide(home); d.State != contracts.MeSleeping || !d.Guessed {
		t.Fatalf("home: %+v", d)
	}
	// The switch is off, or there is no schedule.
	off := idle
	off.GuessSleep = false
	if d := decide(off); d.State != contracts.MeUnknown {
		t.Fatalf("switch off: %+v", d)
	}
	none := idle
	none.Schedule = nil
	if d := decide(none); d.State != contracts.MeUnknown {
		t.Fatalf("no schedule: %+v", d)
	}
	// A reported sleep is not a guess.
	phone := idle
	phone.Sleep = report{Event: evSleepStart, Source: srcPhone, At: at(3, 40)}
	if d := decide(phone); d.State != contracts.MeSleeping || d.Guessed || d.Source != srcPhone {
		t.Fatalf("phone: %+v", d)
	}
}

func TestGuessedSleepStopsAtASleepEndInTheWindow(t *testing.T) {
	// The phone said "sleep ended" at 11:55, before the 12:00 wake time.
	now := at(11, 58)
	in := input{Now: now, GuessSleep: true, Schedule: nightOwl, LastActive: at(1, 0), Sleep: report{Event: evSleepEnd, Source: srcPhone, At: at(11, 55)}}
	if d := decide(in); d.State == contracts.MeSleeping {
		t.Fatalf("guessed sleep after the sleep ended: %+v", d)
	}
	// A "sleep ended" from before the window opened (yesterday) does not count.
	old := in
	old.Now = at(3, 0)
	old.Sleep = report{Event: evSleepEnd, Source: srcPhone, At: time.Date(2026, 10, 10, 12, 0, 0, 0, cst)}
	if d := decide(old); d.State != contracts.MeSleeping || !d.Guessed {
		t.Fatalf("old sleep end: %+v", d)
	}
}

func TestGuessedSleepNotAgainAfterWaking(t *testing.T) {
	// Up at 09:30 after a long guessed sleep, away from the computer for
	// 40 minutes at 10:30: still before the wake time, but not asleep again.
	now := at(10, 30)
	in := input{Now: now, GuessSleep: true, Schedule: nightOwl, LastActive: at(9, 50), LastWoke: at(9, 30)}
	if d := decide(in); d.State == contracts.MeSleeping {
		t.Fatalf("slept again in the same window: %+v", d)
	}
	// The next night's window is a new one.
	next := input{Now: time.Date(2026, 10, 12, 3, 0, 0, 0, cst), GuessSleep: true, Schedule: nightOwl, LastActive: at(23, 0), LastWoke: at(9, 30)}
	if d := decide(next); d.State != contracts.MeSleeping {
		t.Fatalf("next night: %+v", d)
	}
	// A wake-up from before the window opened (yesterday) does not block.
	old := input{Now: at(3, 0), GuessSleep: true, Schedule: nightOwl, LastActive: at(1, 0), LastWoke: time.Date(2026, 10, 10, 11, 0, 0, 0, cst)}
	if d := decide(old); d.State != contracts.MeSleeping {
		t.Fatalf("after an old wake-up: %+v", d)
	}
}

func TestSleepWindowStart(t *testing.T) {
	got, ok := sleepWindowStart(at(8, 0), *nightOwl)
	if !ok || !got.Equal(at(2, 30)) {
		t.Fatalf("night owl: %v %v", got, ok)
	}
	got, ok = sleepWindowStart(at(23, 30), *regular)
	if !ok || !got.Equal(at(22, 0)) {
		t.Fatalf("regular before midnight: %v %v", got, ok)
	}
	got, ok = sleepWindowStart(at(5, 0), *regular)
	if !ok || !got.Equal(time.Date(2026, 10, 10, 22, 0, 0, 0, cst)) {
		t.Fatalf("regular after midnight: %v %v", got, ok)
	}
	// A bedtime just after midnight puts the window start before it.
	early := &schedule{Wake: 8 * 60, Sleep: 30, Loc: cst} // bed 00:30, window from 23:30
	got, ok = sleepWindowStart(at(23, 45), *early)
	if !ok || !got.Equal(at(23, 30)) {
		t.Fatalf("bed at 00:30: %v %v", got, ok)
	}
	// The time zone of the schedule decides, not the clock's.
	got, ok = sleepWindowStart(at(8, 0).UTC(), *nightOwl)
	if !ok || !got.Equal(at(2, 30)) {
		t.Fatalf("utc input: %v %v", got, ok)
	}
}

func TestNextWake(t *testing.T) {
	if got := nextWake(at(3, 0), *nightOwl); !got.Equal(at(12, 0)) {
		t.Fatalf("3 am: %v", got)
	}
	if got := nextWake(at(12, 0), *nightOwl); !got.Equal(time.Date(2026, 10, 12, 12, 0, 0, 0, cst)) {
		t.Fatalf("at wake time: %v", got)
	}
	if got := nextWake(at(14, 0), *regular); !got.Equal(time.Date(2026, 10, 12, 7, 0, 0, 0, cst)) {
		t.Fatalf("2 pm: %v", got)
	}
}

func TestParseClock(t *testing.T) {
	for in, want := range map[string]int{"00:00": 0, "03:30": 210, "12:00": 720, "23:59": 1439} {
		if got, ok := parseClock(in); !ok || got != want {
			t.Errorf("%q = %d %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "24:00", "3", "12:60", "ab:cd", "-1:00"} {
		if _, ok := parseClock(in); ok {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestParseReport(t *testing.T) {
	now := at(12, 0)
	ev, when, err := parseReport([]byte(`{"event":"sleep.start"}`), now)
	if err != nil || ev != evSleepStart || !when.Equal(now) {
		t.Fatalf("plain: %v %v %v", ev, when, err)
	}
	ev, when, err = parseReport([]byte(`{"event":"home.leave","at":"2026-10-11T11:30:00+08:00"}`), now)
	if err != nil || ev != evHomeLeave || !when.Equal(at(11, 30)) {
		t.Fatalf("with at: %v %v %v", ev, when, err)
	}
	if _, _, err = parseReport([]byte(`{"event":"home.arrive","at":"2026-10-11T12:05:00+08:00"}`), now); err != nil {
		t.Fatalf("5 minutes ahead is fine: %v", err)
	}
	if _, _, err = parseReport([]byte(`{"event":"home.arrive","at":""}`), now); err != nil {
		t.Fatalf("empty at: %v", err)
	}
	bad := []string{
		``, `[]`, `{}`, `null`, `{"event":"sleep.start"} {}`,
		`{"event":"wake"}`, `{"event":"sleep.start","lat":1}`, `{"event":"sleep.start","at":"yesterday"}`,
		`{"event":"sleep.start","at":"2026-10-11T12:06:00+08:00"}`, // more than 5 minutes ahead
		`{"event":"sleep.start","at":"2026-10-10T23:59:00+08:00"}`, // more than 12 hours back
		`{"event":5}`, `{"event":"sleep.start","at":5}`,
	}
	for _, body := range bad {
		if _, _, err = parseReport([]byte(body), now); err == nil {
			t.Errorf("%q accepted", body)
		}
	}
}

func TestHAEvent(t *testing.T) {
	cfg := config{HAHome: "person.me", HASleep: "binary_sensor.phone_focus"}
	cases := []struct {
		entity, state, want string
	}{
		{"person.me", "home", evHomeArrive},
		{"person.me", "not_home", evHomeLeave},
		{"person.me", "work", evHomeLeave},
		{"person.me", "unavailable", ""},
		{"person.me", "unknown", ""},
		{"person.me", "", ""},
		{"binary_sensor.phone_focus", "on", evSleepStart},
		{"binary_sensor.phone_focus", "off", evSleepEnd},
		{"binary_sensor.phone_focus", "unavailable", ""},
		{"light.desk", "on", ""},
	}
	for _, c := range cases {
		got, ok := haEvent(cfg, c.entity, c.state)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("%s %s = %q %v, want %q", c.entity, c.state, got, ok, c.want)
		}
	}
	if _, ok := haEvent(config{}, "person.me", "home"); ok {
		t.Error("an entity that is not chosen counts")
	}
}
