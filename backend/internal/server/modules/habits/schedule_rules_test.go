package habits

import (
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

func scheduleMoment(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestScheduleCrossMidnightWorkdaysAndTimezone(t *testing.T) {
	s := scheduleRules{WorkDays: []int{1, 2, 3, 4, 5}, WakeTime: "12:00", SleepTime: "03:30", WorkStart: "22:00", WorkEnd: "02:30", Timezone: "Asia/Shanghai", IdleMinutes: 5}
	cases := []struct {
		at   string
		when []string
		want bool
	}{
		{"2026-10-06T02:00:00+08:00", []string{"awake"}, true},
		{"2026-10-06T04:00:00+08:00", []string{"awake"}, false},
		{"2026-10-06T08:00:00+08:00", []string{"awake"}, false},
		{"2026-10-10T02:00:00+08:00", []string{"work"}, true},
		{"2026-10-11T02:00:00+08:00", []string{"work"}, false},
		{"2026-10-05T16:00:00Z", []string{"awake", "work"}, true},
		{"2026-10-05T20:00:00Z", []string{"awake", "work"}, false},
	}
	for _, tc := range cases {
		start, end, ok := s.window(scheduleMoment(t, tc.at), tc.when, "")
		if ok != tc.want || ok && (!start.Before(end) || start.After(scheduleMoment(t, tc.at)) || !end.After(scheduleMoment(t, tc.at))) {
			t.Errorf("%s %v: %v %v %v", tc.at, tc.when, start, end, ok)
		}
	}
}

func TestContinuousActivityResetNoCatchupAndSnooze(t *testing.T) {
	s := scheduleRules{WakeTime: "00:00", SleepTime: "23:59", Timezone: "UTC", IdleMinutes: 5}
	start := scheduleMoment(t, "2026-10-05T12:00:00Z")
	st := activityReminder{}
	policy := reminderPolicy{When: []string{"active"}, Interval: 20 * time.Minute}
	presence := contracts.HostPresence{Online: true, Known: true, IdleSeconds: new(int64(0)), State: "active", UpdatedAt: start}
	step := func(at time.Time, p contracts.HostPresence) (bool, *time.Time) {
		return st.evaluate(s, policy, at, []contracts.HostPresence{p})
	}
	if due, next := step(start, presence); due || next == nil || !next.Equal(start.Add(20*time.Minute)) {
		t.Fatal("start")
	}
	presence.UpdatedAt = start.Add(20 * time.Minute)
	if due, _ := step(start.Add(20*time.Minute), presence); !due {
		t.Fatal("continuous not due")
	}
	st.sent(start.Add(20 * time.Minute))
	if due, next := step(start.Add(21*time.Minute), presence); due || next == nil || !next.Equal(start.Add(40*time.Minute)) {
		t.Fatal("reminder reset")
	}
	presence.IdleSeconds = new(int64(600))
	presence.UpdatedAt = start.Add(31 * time.Minute)
	if due, next := step(start.Add(31*time.Minute), presence); due || next != nil {
		t.Fatal("idle due")
	}
	presence.IdleSeconds = new(int64(0))
	presence.UpdatedAt = start.Add(35 * time.Minute)
	if due, next := step(start.Add(35*time.Minute), presence); due || next == nil || !next.Equal(start.Add(55*time.Minute)) {
		t.Fatal("away reset")
	}
	st.snooze(start.Add(40 * time.Minute))
	presence.UpdatedAt = start.Add(50 * time.Minute)
	if due, _ := step(start.Add(50*time.Minute), presence); !due {
		t.Fatal("snooze not due")
	}
	st.sent(start.Add(50 * time.Minute))
	st.checkin(start.Add(52 * time.Minute))
	if due, next := step(start.Add(53*time.Minute), presence); due || next == nil || !next.Equal(start.Add(72*time.Minute)) {
		t.Fatal("checkin reset")
	}
	presence.Locked = new(true)
	presence.Since = start.Add(71 * time.Minute)
	presence.UpdatedAt = start.Add(72 * time.Minute)
	if due, _ := step(start.Add(72*time.Minute), presence); due {
		t.Fatal("locked due")
	}
	presence.Locked = new(false)
	presence.UpdatedAt = start.Add(73 * time.Minute)
	if due, next := step(start.Add(73*time.Minute), presence); due || next == nil || !next.Equal(start.Add(93*time.Minute)) {
		t.Fatal("missed reminder caught up")
	}
}

func TestActiveFullDayIntervalHasEstimate(t *testing.T) {
	s := scheduleRules{Timezone: "UTC", IdleMinutes: 5}
	now := scheduleMoment(t, "2026-10-05T12:00:00Z")
	st := activityReminder{}
	policy := reminderPolicy{When: []string{"active"}, Interval: 24 * time.Hour}
	if due, next := st.evaluate(s, policy, now, []contracts.HostPresence{{Online: true}}); due || next == nil || !next.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("full-day active: %v %v", due, next)
	}
}

func TestMissedActiveSnoozeRestartsInterval(t *testing.T) {
	s := scheduleRules{Timezone: "UTC", IdleMinutes: 5}
	now := scheduleMoment(t, "2026-10-05T12:00:00Z")
	last := now
	until := now.Add(10 * time.Minute)
	policy := reminderPolicy{When: []string{"active"}, Interval: 20 * time.Minute, LastReminded: &last, SnoozedUntil: &until}
	st := activityReminder{}
	p := contracts.HostPresence{Online: true, Known: false}
	st.evaluate(s, policy, now, []contracts.HostPresence{p})
	p.Locked = new(true)
	p.Since = until.Add(-time.Minute)
	st.evaluate(s, policy, until, []contracts.HostPresence{p})
	p.Locked = new(false)
	if due, next := st.evaluate(s, policy, until.Add(time.Minute), []contracts.HostPresence{p}); due || next == nil || !next.Equal(until.Add(21*time.Minute)) {
		t.Fatalf("missed active snooze caught up: %v %v", due, next)
	}
}

func TestRestoredSnoozeAndQuietRemainIndependent(t *testing.T) {
	s := scheduleRules{Timezone: "UTC", IdleMinutes: 5}
	now := scheduleMoment(t, "2026-10-05T12:00:00Z")
	last := now.Add(-time.Minute)
	snoozed := now.Add(9 * time.Minute)
	policy := reminderPolicy{When: []string{"window"}, Window: "09:00-21:00", Interval: time.Hour, LastReminded: &last, SnoozedUntil: &snoozed}
	st := activityReminder{}
	if due, next := st.evaluate(s, policy, now, nil); due || next == nil || !next.Equal(snoozed) {
		t.Fatalf("restored snooze: %v %v", due, next)
	}
	if due, _ := st.evaluate(s, policy, snoozed, nil); !due {
		t.Fatal("restored snooze not due")
	}
	quiet := scheduleMoment(t, "2026-10-06T00:00:00Z")
	policy.QuietUntil = &quiet
	if due, next := st.evaluate(s, policy, now, nil); due || next == nil || !next.Equal(scheduleMoment(t, "2026-10-06T10:00:00Z")) {
		t.Fatalf("skip and snooze: %v %v", due, next)
	}
}

func TestRemindWhenDefaultsAndValidation(t *testing.T) {
	when, err := normalizeRemindWhen(nil)
	if err != nil || len(when) != 1 || when[0] != "window" {
		t.Fatalf("defaults: %v %v", when, err)
	}
	when, err = normalizeRemindWhen([]string{"active", "awake", "active"})
	if err != nil || len(when) != 2 || when[0] != "active" || when[1] != "awake" {
		t.Fatalf("conditions: %v %v", when, err)
	}
	if _, err := normalizeRemindWhen([]string{"unknown"}); err == nil {
		t.Fatal("unknown condition accepted")
	}
}

func TestScheduleValidation(t *testing.T) {
	base := scheduleRules{WorkDays: []int{1, 5}, WakeTime: "12:00", SleepTime: "03:30", Timezone: "Asia/Shanghai", IdleMinutes: 5}
	if err := base.validate(); err != nil {
		t.Fatal(err)
	}
	cases := []scheduleRules{base, base, base, base, base, base, base}
	cases[0].WorkDays = []int{0}
	cases[1].WorkDays = []int{1, 1}
	cases[2].WakeTime = "24:00"
	cases[3].SleepTime = "12:00"
	cases[4].Timezone = "Invalid/Zone"
	cases[5].IdleMinutes = 0
	cases[6].WorkStart = "14:00"
	for i, rule := range cases {
		if err := rule.validate(); err == nil {
			t.Errorf("invalid schedule %d accepted", i)
		}
	}
	base.WorkDays = []int{}
	if err := base.validate(); err != nil {
		t.Fatal("empty workdays should disable work gates", err)
	}
}

func TestFixedTimesNoCatchupAndSnooze(t *testing.T) {
	now := scheduleMoment(t, "2026-10-05T09:00:35+08:00")
	loc, _ := time.LoadLocation("Asia/Shanghai")
	points := []string{"09:00", "21:00"}
	if due, next := fixedReminder(points, nil, nil, now, loc); !due || next == nil || next.Hour() != 9 {
		t.Fatalf("fixed point: %v %v", due, next)
	}
	late := now.Add(2 * time.Minute)
	if due, next := fixedReminder(points, nil, nil, late, loc); due || next == nil || next.Hour() != 21 {
		t.Fatalf("missed point caught up: %v %v", due, next)
	}
	last := now
	if due, next := fixedReminder(points, &last, nil, now, loc); due || next == nil || next.Hour() != 21 {
		t.Fatalf("duplicate point: %v %v", due, next)
	}
	quiet := now.Add(10 * time.Minute)
	if due, next := fixedReminder(points, &last, &quiet, now, loc); due || next == nil || !next.Equal(quiet) {
		t.Fatalf("snooze next: %v %v", due, next)
	}
	if due, _ := fixedReminder(points, &last, &quiet, quiet, loc); !due {
		t.Fatal("snooze not due")
	}
	if due, _ := fixedReminder(points, &last, &quiet, quiet.Add(2*time.Minute), loc); due {
		t.Fatal("missed snooze caught up")
	}
}

func TestScheduleClockWindowsAndDST(t *testing.T) {
	s := scheduleRules{Timezone: "America/New_York", WakeTime: "22:00", SleepTime: "04:00", IdleMinutes: 5}
	now := scheduleMoment(t, "2026-03-08T03:30:00-04:00")
	start, end, ok := s.window(now, []string{"awake"}, "")
	if !ok || end.Sub(start) != 5*time.Hour {
		t.Fatalf("spring DST: %v %v %v", start, end, ok)
	}
	now = scheduleMoment(t, "2026-11-01T03:30:00-05:00")
	start, end, ok = s.window(now, []string{"awake"}, "")
	if !ok || end.Sub(start) != 7*time.Hour {
		t.Fatalf("fall DST: %v %v %v", start, end, ok)
	}
	s.Timezone = "Asia/Shanghai"
	now = scheduleMoment(t, "2026-10-06T02:00:00+08:00")
	if _, _, ok = s.window(now, []string{"window"}, "22:00-03:30"); !ok {
		t.Fatal("cross-midnight legacy window")
	}
	if _, _, ok = s.window(scheduleMoment(t, "2026-10-06T03:30:00+08:00"), []string{"window"}, "22:00-03:30"); ok {
		t.Fatal("window end included")
	}
}

func TestScheduleFutureWorkWindowAndCombinedGates(t *testing.T) {
	s := scheduleRules{Timezone: "UTC", WorkDays: []int{1, 2, 3, 4, 5}, WakeTime: "12:00", SleepTime: "03:30", WorkStart: "14:00", WorkEnd: "23:00", IdleMinutes: 5}
	now := scheduleMoment(t, "2026-10-10T12:00:00Z")
	st := activityReminder{}
	policy := reminderPolicy{When: []string{"awake", "work"}, Interval: time.Hour}
	if due, next := st.evaluate(s, policy, now, nil); due || next == nil || !next.Equal(scheduleMoment(t, "2026-10-12T15:00:00Z")) {
		t.Fatalf("weekend next: %v %v", due, next)
	}
	policy.When = []string{"awake", "work", "active"}
	if due, next := st.evaluate(s, policy, now, []contracts.HostPresence{{Online: true}}); due || next != nil {
		t.Fatalf("active outside gates: %v %v", due, next)
	}
}

func TestMultipleHostAbsenceStartsWhenLastHostStops(t *testing.T) {
	s := scheduleRules{Timezone: "UTC", IdleMinutes: 5}
	now := scheduleMoment(t, "2026-10-05T12:00:00Z")
	st := activityReminder{}
	policy := reminderPolicy{When: []string{"active"}, Interval: 20 * time.Minute}
	oldIdle := contracts.HostPresence{Online: true, Known: true, IdleSeconds: new(int64(600)), Since: now.Add(-10 * time.Minute)}
	active := contracts.HostPresence{Online: true, Known: false}
	st.evaluate(s, policy, now, []contracts.HostPresence{oldIdle, active})
	active.Locked = new(true)
	active.Since = now.Add(5 * time.Minute)
	st.evaluate(s, policy, now.Add(6*time.Minute), []contracts.HostPresence{oldIdle, active})
	active.Locked = new(false)
	if due, next := st.evaluate(s, policy, now.Add(8*time.Minute), []contracts.HostPresence{oldIdle, active}); due || next == nil || !next.Equal(now.Add(20*time.Minute)) {
		t.Fatalf("old idle host reset short absence: %v %v", due, next)
	}
}

func TestLockAbsenceUsesReportedSince(t *testing.T) {
	s := scheduleRules{Timezone: "UTC", IdleMinutes: 5}
	now := scheduleMoment(t, "2026-10-05T12:00:00Z")
	st := activityReminder{}
	policy := reminderPolicy{When: []string{"active"}, Interval: 20 * time.Minute}
	active := contracts.HostPresence{Online: true, Known: false}
	st.evaluate(s, policy, now, []contracts.HostPresence{active})
	locked := contracts.HostPresence{Online: true, Known: true, IdleSeconds: new(int64(0)), Locked: new(true), Since: now.Add(2 * time.Minute)}
	st.evaluate(s, policy, now.Add(8*time.Minute), []contracts.HostPresence{locked})
	if due, next := st.evaluate(s, policy, now.Add(9*time.Minute), []contracts.HostPresence{active}); due || next == nil || !next.Equal(now.Add(29*time.Minute)) {
		t.Fatalf("reported lock duration: %v %v", due, next)
	}
}

func TestActivityGateReentryRestarts(t *testing.T) {
	s := scheduleRules{Timezone: "UTC", WakeTime: "12:00", SleepTime: "03:30", IdleMinutes: 5}
	st := activityReminder{}
	policy := reminderPolicy{When: []string{"awake", "active"}, Interval: 45 * time.Minute}
	p := []contracts.HostPresence{{Online: true, Known: false}}
	st.evaluate(s, policy, scheduleMoment(t, "2026-10-05T03:00:00Z"), p)
	if due, next := st.evaluate(s, policy, scheduleMoment(t, "2026-10-05T03:30:00Z"), p); due || next != nil {
		t.Fatalf("gate end: %v %v", due, next)
	}
	now := scheduleMoment(t, "2026-10-05T12:00:00Z")
	if due, next := st.evaluate(s, policy, now, p); due || next == nil || !next.Equal(now.Add(45*time.Minute)) {
		t.Fatalf("gate reentry: %v %v", due, next)
	}
}

func TestIdleAndDisplayChecksUseAnySelectedHost(t *testing.T) {
	cases := []struct {
		p    contracts.HostPresence
		want bool
	}{
		{contracts.HostPresence{Online: true, Known: true, IdleSeconds: new(int64(299))}, true},
		{contracts.HostPresence{Online: true, Known: true, IdleSeconds: new(int64(300))}, false},
		{contracts.HostPresence{Online: true, Known: true}, false},
		{contracts.HostPresence{Online: true, Known: false, Locked: new(true)}, false},
		{contracts.HostPresence{Online: true, Known: false, DisplayOff: new(true)}, false},
		{contracts.HostPresence{Online: true, Known: false}, true},
	}
	for _, tc := range cases {
		if got := hostActive(tc.p, 5); got != tc.want {
			t.Errorf("active %+v: %v", tc.p, got)
		}
	}
	s := scheduleRules{Timezone: "UTC", IdleMinutes: 5}
	st := activityReminder{}
	now := scheduleMoment(t, "2026-10-05T12:00:00Z")
	policy := reminderPolicy{When: []string{"active"}, Interval: 20 * time.Minute}
	p := []contracts.HostPresence{{Online: true, Known: true, IdleSeconds: new(int64(600))}, {Online: true, Known: true, IdleSeconds: new(int64(0))}}
	st.evaluate(s, policy, now, p)
	if due, _ := st.evaluate(s, policy, now.Add(20*time.Minute), p); !due {
		t.Fatal("idle host blocked active host")
	}
}

func TestContinuousBriefAbsenceKeepsTimer(t *testing.T) {
	s := scheduleRules{Timezone: "UTC", IdleMinutes: 5}
	now := scheduleMoment(t, "2026-10-05T12:00:00Z")
	st := activityReminder{}
	policy := reminderPolicy{When: []string{"active"}, Interval: 20 * time.Minute}
	p := contracts.HostPresence{Online: true, Known: true, IdleSeconds: new(int64(0))}
	st.evaluate(s, policy, now, []contracts.HostPresence{p})
	p.Locked = new(true)
	st.evaluate(s, policy, now.Add(5*time.Minute), []contracts.HostPresence{p})
	p.Locked = new(false)
	if due, next := st.evaluate(s, policy, now.Add(8*time.Minute), []contracts.HostPresence{p}); due || next == nil || !next.Equal(now.Add(20*time.Minute)) {
		t.Fatalf("brief lock reset: %v %v", due, next)
	}
	p.DisplayOff = new(true)
	st.evaluate(s, policy, now.Add(10*time.Minute), []contracts.HostPresence{p})
	p.DisplayOff = new(false)
	if due, next := st.evaluate(s, policy, now.Add(15*time.Minute), []contracts.HostPresence{p}); due || next == nil || !next.Equal(now.Add(35*time.Minute)) {
		t.Fatalf("long display off did not reset: %v %v", due, next)
	}
}

func TestUnknownOnlineFallbackAndAnyHost(t *testing.T) {
	s := scheduleRules{WakeTime: "00:00", SleepTime: "23:59", Timezone: "UTC", IdleMinutes: 5}
	now := scheduleMoment(t, "2026-10-05T12:00:00Z")
	st := activityReminder{}
	policy := reminderPolicy{When: []string{"active"}, Interval: 5 * time.Minute}
	unknown := contracts.HostPresence{Online: true, Known: false, State: "unknown", Since: now, UpdatedAt: now}
	if due, next := st.evaluate(s, policy, now, []contracts.HostPresence{{Online: false}, unknown}); due || next == nil {
		t.Fatal("unknown fallback")
	}
	if due, _ := st.evaluate(s, policy, now.Add(5*time.Minute), []contracts.HostPresence{unknown}); !due {
		t.Fatal("unknown online duration")
	}
	st = activityReminder{}
	if _, next := st.evaluate(s, policy, now, []contracts.HostPresence{{Online: false, Known: false}}); next != nil {
		t.Fatal("offline unknown active")
	}
}
