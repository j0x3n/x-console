package me

// The decision of which state the user is in. Everything here is a pure
// function of its input, so each rule can be tested with a made-up clock.

import (
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

const (
	// awayWindow is how long after the work computer was last in use the user
	// still counts as "away" (stepped away, did not leave).
	awayWindow = 30 * time.Minute
	// guessLead is how long before the usual bedtime the schedule starts to
	// guess that the user is asleep.
	guessLead = time.Hour
	// minSleep is the shortest sleep that counts as waking up. Naps and a
	// night-time trip to the toilet are shorter.
	minSleep = 3 * time.Hour
	// staleSleep is how long a "sleep started" report is believed when nothing
	// ends it.
	staleSleep = 16 * time.Hour
)

// Report events.
const (
	evSleepStart = "sleep.start"
	evSleepEnd   = "sleep.end"
	evHomeArrive = "home.arrive"
	evHomeLeave  = "home.leave"
)

// Sources of a state or a report.
const (
	srcPC       = "pc"
	srcPhone    = "phone"
	srcHA       = "ha"
	srcManual   = "manual"
	srcSchedule = "schedule"
)

// report is the latest report of one kind (sleep or location).
type report struct {
	Event  string
	Source string
	At     time.Time
}

func (r report) ok() bool { return r.Event != "" }

// manualSetting is a state the user chose by hand. A zero Until means "until I
// change it back".
type manualSetting struct {
	State string    `json:"state"`
	Until time.Time `json:"until,omitempty"`
}

// schedule is the part of the daily schedule the guess needs.
type schedule struct {
	Wake  int // minutes after midnight
	Sleep int
	Loc   *time.Location
}

// input is everything decide looks at.
type input struct {
	Now        time.Time
	Manual     *manualSetting
	PCActive   bool      // some work computer is in use now
	LastActive time.Time // the last time one was; zero when never
	Sleep      report    // latest sleep.start or sleep.end
	Place      report    // latest home.arrive or home.leave
	GuessSleep bool
	Schedule   *schedule
	LastWoke   time.Time // when the last sleep of three hours or more ended
}

// decision is the outcome.
type decision struct {
	State   string
	Source  string
	Guessed bool
	Manual  bool
	// SleptAt is when the sleep began, for a sleep reported by the phone.
	SleptAt time.Time
	AtHome  *bool
	// Filled in after decide (B164), they do not change State.
	PhoneActive bool
	Host        string
}

func (in input) sleepStarted() bool {
	return in.Sleep.Event == evSleepStart && in.Now.Sub(in.Sleep.At) < staleSleep
}

// decide applies the rules in order; the first that holds wins.
func decide(in input) decision {
	var atHome *bool
	switch in.Place.Event {
	case evHomeArrive:
		atHome = ptr(true)
	case evHomeLeave:
		atHome = ptr(false)
	}
	d := decision{AtHome: atHome}
	switch {
	case in.Manual != nil && (in.Manual.Until.IsZero() || in.Now.Before(in.Manual.Until)):
		d.State, d.Source, d.Manual = in.Manual.State, srcManual, true
	case in.PCActive:
		d.State, d.Source = contracts.MeWorking, srcPC
	case in.sleepStarted():
		d.State, d.Source, d.SleptAt = contracts.MeSleeping, in.Sleep.Source, in.Sleep.At
	case in.Place.Event == evHomeLeave:
		d.State, d.Source = contracts.MeOut, in.Place.Source
	case !in.LastActive.IsZero() && in.Now.Sub(in.LastActive) < awayWindow:
		d.State, d.Source = contracts.MeAway, srcPC
	case in.GuessSleep && in.guessAsleep():
		d.State, d.Source, d.Guessed = contracts.MeSleeping, srcSchedule, true
	case in.Place.Event == evHomeArrive:
		d.State, d.Source = contracts.MeHome, in.Place.Source
	default:
		d.State, d.Source = contracts.MeUnknown, ""
	}
	return d
}

// guessAsleep is rule 6: inside the sleeping hours of the schedule, with the
// work computer unused for 30 minutes (decide has checked that) and nobody
// reported leaving the house. A night the user already woke from does not
// count again, and neither does one that something said is over (the phone,
// the computer or the user by hand reported "sleep ended" in this window).
func (in input) guessAsleep() bool {
	if in.Schedule == nil {
		return false
	}
	start, ok := sleepWindowStart(in.Now, *in.Schedule)
	if !ok {
		return false
	}
	if in.Sleep.Event == evSleepEnd && !in.Sleep.At.Before(start) {
		return false
	}
	return in.LastWoke.IsZero() || in.LastWoke.Before(start)
}

// sleepWindowStart tells whether now is inside the sleeping hours (from one
// hour before the bedtime to the wake-up time) and when this window began.
func sleepWindowStart(now time.Time, s schedule) (time.Time, bool) {
	loc := s.Loc
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)
	m := local.Hour()*60 + local.Minute()
	from := ((s.Sleep-int(guessLead/time.Minute))%1440 + 1440) % 1440
	to := s.Wake
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	at := func(base time.Time, minutes int) time.Time {
		return time.Date(base.Year(), base.Month(), base.Day(), minutes/60, minutes%60, 0, 0, loc)
	}
	switch {
	case from < to: // the window stays inside one day
		if m >= from && m < to {
			return at(day, from), true
		}
	case from > to: // the window crosses midnight
		if m >= from {
			return at(day, from), true
		}
		if m < to {
			return at(day.AddDate(0, 0, -1), from), true
		}
	}
	return time.Time{}, false
}

// nextWake is the next time the wake-up time comes after now.
func nextWake(now time.Time, s schedule) time.Time {
	loc := s.Loc
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)
	t := time.Date(local.Year(), local.Month(), local.Day(), s.Wake/60, s.Wake%60, 0, 0, loc)
	if !t.After(now) {
		t = time.Date(local.Year(), local.Month(), local.Day()+1, s.Wake/60, s.Wake%60, 0, 0, loc)
	}
	return t
}

func parseClock(v string) (int, bool) {
	h, m, ok := strings.Cut(strings.TrimSpace(v), ":")
	if !ok {
		return 0, false
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, false
	}
	return hh*60 + mm, true
}

func ptr[T any](v T) *T { return &v }
