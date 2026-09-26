package reminders

import (
	"reflect"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/db"
)

var shanghai = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return loc
}()

func local(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, shanghai)
}

func newReminder(t *testing.T, rr string, at, now time.Time) db.Reminder {
	t.Helper()
	rule, err := parseRule(rr, at, shanghai)
	if err != nil {
		t.Fatal(err)
	}
	return db.Reminder{ID: 1, Title: "x", Rrule: rr, Dtstart: at.UTC(), NextAt: firstNext(rule, at, now), Enabled: 1, CreatedAt: now}
}

func TestDailyCompleteBeforeFiringSkipsToTomorrow(t *testing.T) {
	now := local(2026, 10, 1, 8, 0)
	r := newReminder(t, "FREQ=DAILY", local(2026, 10, 1, 9, 0), now)
	if !r.NextAt.Equal(local(2026, 10, 1, 9, 0)) {
		t.Fatalf("first: %v", r.NextAt)
	}
	rule, _ := parseRule(r.Rrule, r.Dtstart, shanghai)
	r = complete(r, rule, now.Add(30*time.Minute))
	if !r.NextAt.Equal(local(2026, 10, 2, 9, 0)) {
		t.Fatalf("after done: %v", r.NextAt.In(shanghai))
	}
	if status(r) != api.ReminderStatusScheduled {
		t.Fatalf("status %s", status(r))
	}
}

func TestDailyFireThenComplete(t *testing.T) {
	r := newReminder(t, "FREQ=DAILY;BYHOUR=9;BYMINUTE=0", local(2026, 10, 1, 7, 13), local(2026, 10, 1, 7, 13))
	if !r.NextAt.Equal(local(2026, 10, 1, 9, 0)) {
		t.Fatalf("BYHOUR in user zone: %v", r.NextAt.In(shanghai))
	}
	rule, _ := parseRule(r.Rrule, r.Dtstart, shanghai)
	f, ok := fire(r, rule, local(2026, 10, 1, 9, 0).Add(20*time.Second))
	if !ok || f.missed || !f.scheduled.Equal(local(2026, 10, 1, 9, 0)) {
		t.Fatalf("fire: %+v %v", f, ok)
	}
	r = f.rem
	if status(r) != api.ReminderStatusPending || !r.NextAt.Equal(local(2026, 10, 2, 9, 0)) {
		t.Fatalf("after fire: %s %v", status(r), r.NextAt)
	}
	r = complete(r, rule, local(2026, 10, 1, 9, 5))
	if status(r) != api.ReminderStatusScheduled || !r.NextAt.Equal(local(2026, 10, 2, 9, 0)) {
		t.Fatalf("after done: %s %v", status(r), r.NextAt.In(shanghai))
	}
	if _, ok := fire(r, rule, local(2026, 10, 1, 23, 0)); ok {
		t.Fatal("fired again before tomorrow")
	}
}

func TestMissedRemindersCollapse(t *testing.T) {
	r := newReminder(t, "FREQ=DAILY", local(2026, 9, 28, 9, 0), local(2026, 9, 28, 8, 0))
	rule, _ := parseRule(r.Rrule, r.Dtstart, shanghai)
	f, ok := fire(r, rule, local(2026, 10, 1, 12, 0))
	if !ok || !f.missed || !f.scheduled.Equal(local(2026, 9, 28, 9, 0)) {
		t.Fatalf("missed: %+v %v", f, ok)
	}
	if !f.rem.NextAt.Equal(local(2026, 10, 2, 9, 0)) {
		t.Fatalf("next after downtime: %v", f.rem.NextAt.In(shanghai))
	}
	// Less than an hour late is not "missed".
	f, _ = fire(r, rule, local(2026, 9, 28, 9, 59))
	if f.missed {
		t.Fatal("59 minutes late counted as missed")
	}
}

func TestOneShotLifecycle(t *testing.T) {
	now := local(2026, 10, 1, 10, 0)
	r := newReminder(t, "", now.Add(time.Minute), now)
	if _, ok := fire(r, nil, now); ok {
		t.Fatal("fired early")
	}
	f, ok := fire(r, nil, now.Add(time.Minute))
	if !ok || f.rem.NextAt != nil || status(f.rem) != api.ReminderStatusPending {
		t.Fatalf("fire: %+v", f.rem)
	}
	r = snooze(f.rem, nil, 10*time.Minute, now.Add(2*time.Minute))
	if status(r) != api.ReminderStatusSnoozed || !dueAt(r).Equal(now.Add(12*time.Minute)) {
		t.Fatalf("snooze: %s %v", status(r), dueAt(r))
	}
	f, ok = fire(r, nil, now.Add(12*time.Minute))
	if !ok || status(f.rem) != api.ReminderStatusPending {
		t.Fatalf("snooze fire: %v %s", ok, status(f.rem))
	}
	r = complete(f.rem, nil, now.Add(13*time.Minute))
	if status(r) != api.ReminderStatusDone {
		t.Fatalf("done: %s", status(r))
	}
	if !inRange(r, rangeDone, now) || inRange(r, rangeToday, now) {
		t.Fatal("done reminder in the wrong tab")
	}
}

func TestRanges(t *testing.T) {
	now := local(2026, 10, 1, 10, 0)
	end := startOfDay(now, shanghai).AddDate(0, 0, 1)
	today := newReminder(t, "", local(2026, 10, 1, 18, 0), now)
	tomorrow := newReminder(t, "", local(2026, 10, 2, 8, 0), now)
	if !inRange(today, rangeToday, end) || inRange(today, rangeUpcoming, end) {
		t.Fatal("today")
	}
	if inRange(tomorrow, rangeToday, end) || !inRange(tomorrow, rangeUpcoming, end) {
		t.Fatal("upcoming")
	}
	rows := []db.Reminder{tomorrow, today}
	sortForRange(rows, rangeUpcoming)
	if rows[0].NextAt.After(*rows[1].NextAt) {
		t.Fatal("not sorted by due time")
	}
}

func TestParseRuleValidation(t *testing.T) {
	at := local(2026, 10, 1, 9, 0)
	for _, bad := range []string{"FREQ=NOPE", "FREQ=SECONDLY", "FREQ=MINUTELY;INTERVAL=1", "BYDAY=XX"} {
		if _, err := parseRule(bad, at, shanghai); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	rule, err := parseRule("RRULE:FREQ=WEEKLY;BYDAY=MO,WE", at, shanghai)
	if err != nil {
		t.Fatal(err)
	}
	// 2026-10-01 is a Thursday: next is Monday the 5th at 09:00 local.
	if next := after(rule, at, false); !next.Equal(local(2026, 10, 5, 9, 0)) {
		t.Fatalf("weekly: %v", next.In(shanghai))
	}
}

func TestQuietHours(t *testing.T) {
	q := quietHours{Enabled: true, Start: "23:00", End: "07:30"}
	cases := map[time.Time]bool{
		local(2026, 10, 1, 23, 30): true,
		local(2026, 10, 1, 3, 0):   true,
		local(2026, 10, 1, 7, 30):  false,
		local(2026, 10, 1, 12, 0):  false,
	}
	for at, want := range cases {
		if got := inQuietHours(q, at, shanghai); got != want {
			t.Errorf("%v: got %v", at, got)
		}
	}
	day := quietHours{Enabled: true, Start: "12:00", End: "14:00"}
	if !inQuietHours(day, local(2026, 10, 1, 13, 0), shanghai) || inQuietHours(day, local(2026, 10, 1, 14, 0), shanghai) {
		t.Error("same-day window")
	}
	if inQuietHours(quietHours{Start: "00:00", End: "23:59"}, local(2026, 10, 1, 13, 0), shanghai) {
		t.Error("disabled quiet hours applied")
	}
}

func TestPickChannels(t *testing.T) {
	routes := []route{
		{KindPattern: "host.alert*", MinPriority: "high", Channels: []string{"telegram"}, Enabled: true},
		{KindPattern: "reminder.*", MinPriority: "normal", Channels: []string{"bark"}, Enabled: false},
		{KindPattern: "*", MinPriority: "normal", Channels: []string{"webpush"}, Enabled: true},
	}
	cases := []struct {
		kind, prio string
		quiet      bool
		want       []string
	}{
		{"host.alert.cpu", "high", false, []string{"telegram"}},
		{"host.alert.cpu", "normal", false, []string{"webpush"}},
		{"reminder.due", "normal", false, []string{"webpush"}},
		{"reminder.due", "low", false, nil},
		{"reminder.due", "normal", true, nil},
		{"host.alert.disk", "urgent", true, []string{"telegram"}},
	}
	for _, c := range cases {
		if got := pickChannels(routes, c.kind, c.prio, c.quiet); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s/%s quiet=%v: got %v want %v", c.kind, c.prio, c.quiet, got, c.want)
		}
	}
}

func TestServerChanURL(t *testing.T) {
	if u := serverChanURL("", "SCT123abc"); u != "https://sctapi.ftqq.com/SCT123abc.send" {
		t.Error(u)
	}
	if u := serverChanURL("", "sctp42tabc"); u != "https://42.push.ft07.com/send/sctp42tabc.send" {
		t.Error(u)
	}
	if u := serverChanURL("http://fake", "k"); u != "http://fake/k.send" {
		t.Error(u)
	}
}

func TestMask(t *testing.T) {
	if mask("") != "" || mask("short") != "••••" || mask("123456:ABCDEFGH") != "••••EFGH" {
		t.Error(mask("123456:ABCDEFGH"))
	}
}
