package habits_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// B162: the me module says when the user woke up; the night's sleep goes into
// the body record of the day they woke up on.
func TestSleepFilledFromWakeUp(t *testing.T) {
	env, _ := setup(t)
	cst := time.FixedZone("CST", 8*3600)
	bus := env.App.Deps.Bus
	sleep := func(date string) string { return bodyDay(t, env, date).Sleep }

	// 03:40 to 11:55 is 8 hours 15 minutes, kept with one decimal.
	bus.Publish("me.woke", contracts.MeWoke{SleptAt: time.Date(2026, 10, 12, 3, 40, 0, 0, cst), WokeAt: time.Date(2026, 10, 12, 11, 55, 0, 0, cst)})
	waitFor(t, "sleep hours", func() bool { return sleep("2026-10-12") == "8.3" })

	// A day that already has a value is left alone.
	env.MustDo(http.MethodPatch, "/habits/personal/days/2026-10-13", map[string]any{"sleep": "6"}, nil)
	bus.Publish("me.woke", contracts.MeWoke{SleptAt: time.Date(2026, 10, 13, 1, 0, 0, 0, cst), WokeAt: time.Date(2026, 10, 13, 9, 0, 0, 0, cst)})
	// A guessed wake-up is not written, and it does not stop the next real one.
	bus.Publish("me.woke", contracts.MeWoke{SleptAt: time.Date(2026, 10, 14, 1, 0, 0, 0, cst), WokeAt: time.Date(2026, 10, 14, 9, 0, 0, 0, cst), Guessed: true})
	// The day is the one in the schedule's time zone: 17:00 UTC is 01:00 the next day in Shanghai.
	bus.Publish("me.woke", contracts.MeWoke{SleptAt: time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC), WokeAt: time.Date(2026, 10, 15, 17, 0, 0, 0, time.UTC)})
	waitFor(t, "sleep hours of the next local day", func() bool { return sleep("2026-10-16") == "9" })
	if got := sleep("2026-10-13"); got != "6" {
		t.Fatalf("a filled day was overwritten: %q", got)
	}
	if got := sleep("2026-10-14"); got != "" {
		t.Fatalf("a guessed wake-up was written: %q", got)
	}
	if got := sleep("2026-10-15"); got != "" {
		t.Fatalf("the UTC day was used: %q", got)
	}
}

func TestScheduleIsOfferedToOtherModules(t *testing.T) {
	env, _ := setup(t)
	s, ok := moduleSchedule(env)
	if !ok {
		t.Fatal("contracts.Schedule is not provided")
	}
	rules, err := s.Rules(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if rules.WakeTime != "12:00" || rules.SleepTime != "03:30" || rules.Timezone == "" || len(rules.WorkDays) != 5 {
		t.Fatalf("rules: %+v", rules)
	}
}

func moduleSchedule(env *testutil.Env) (contracts.Schedule, bool) {
	return module.Lookup[contracts.Schedule](env.App.Deps.Registry, contracts.ScheduleKey)
}

// B201: early-sleep and early-wake habits are checked in from the wake-up.
func TestSleepHabitsCheckedInFromWakeUp(t *testing.T) {
	env, _ := setup(t)
	cst := time.FixedZone("CST", 8*3600)
	bus := env.App.Deps.Bus
	var bed, wake api.Habit
	env.MustDo(http.MethodPost, "/habits", map[string]any{"name": "早睡", "autoCheck": map[string]any{"kind": "bedtime", "before": "23:30"}}, &bed)
	env.MustDo(http.MethodPost, "/habits", map[string]any{"name": "早起", "autoCheck": map[string]any{"kind": "wake", "before": "07:30"}}, &wake)
	if bed.AutoCheck == nil || bed.AutoCheck.Kind != api.HabitAutoCheckKindBedtime || bed.AutoCheck.Before != "23:30" {
		t.Fatalf("saved rule: %+v", bed.AutoCheck)
	}
	if status, _ := env.Do(http.MethodPost, "/habits", map[string]any{"name": "坏", "autoCheck": map[string]any{"kind": "wake", "before": "7点"}}, nil); status != http.StatusBadRequest {
		t.Fatalf("bad time: %d", status)
	}
	logsOn := func(id int64, day string) []api.HabitLog {
		rows, err := env.App.Deps.DB.Query(`SELECT at, source, note FROM habit_logs WHERE habit_id = ?`, id)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []api.HabitLog
		for rows.Next() {
			var l api.HabitLog
			if err := rows.Scan(&l.At, &l.Source, &l.Note); err != nil {
				t.Fatal(err)
			}
			if l.At.In(cst).Format(time.DateOnly) == day {
				out = append(out, l)
			}
		}
		return out
	}
	// Asleep at 23:10, up at 07:05: both habits are done. Bedtime goes on the evening.
	bus.Publish("me.woke", contracts.MeWoke{SleptAt: time.Date(2026, 10, 20, 23, 10, 0, 0, cst), WokeAt: time.Date(2026, 10, 21, 7, 5, 0, 0, cst)})
	waitFor(t, "early wake", func() bool { return len(logsOn(wake.Id, "2026-10-21")) == 1 })
	waitFor(t, "early sleep", func() bool { return len(logsOn(bed.Id, "2026-10-20")) == 1 })
	if l := logsOn(bed.Id, "2026-10-20")[0]; l.Source != "me" || l.Note != "23:10 睡着" {
		t.Fatalf("bedtime log: %+v", l)
	}
	// Asleep at 00:20 is late for 23:30, but early for a 00:30 rule, and the
	// check-in still goes on the evening before.
	env.MustDo(http.MethodPatch, fmt.Sprintf("/habits/%d", bed.Id), map[string]any{"autoCheck": map[string]any{"kind": "bedtime", "before": "00:30"}}, nil)
	bus.Publish("me.woke", contracts.MeWoke{SleptAt: time.Date(2026, 10, 22, 0, 20, 0, 0, cst), WokeAt: time.Date(2026, 10, 22, 9, 0, 0, 0, cst)})
	waitFor(t, "sleep after midnight", func() bool { return len(logsOn(bed.Id, "2026-10-21")) == 1 })
	if n := len(logsOn(wake.Id, "2026-10-22")); n != 0 {
		t.Fatalf("09:00 counted as early: %d", n)
	}
	// Naps, guesses and a second event on the same day add nothing.
	bus.Publish("me.woke", contracts.MeWoke{SleptAt: time.Date(2026, 10, 23, 5, 0, 0, 0, cst), WokeAt: time.Date(2026, 10, 23, 6, 0, 0, 0, cst)})
	bus.Publish("me.woke", contracts.MeWoke{SleptAt: time.Date(2026, 10, 23, 22, 0, 0, 0, cst), WokeAt: time.Date(2026, 10, 24, 6, 0, 0, 0, cst), Guessed: true})
	bus.Publish("me.woke", contracts.MeWoke{SleptAt: time.Date(2026, 10, 20, 22, 0, 0, 0, cst), WokeAt: time.Date(2026, 10, 21, 6, 0, 0, 0, cst)})
	// A last real event shows the earlier ones were handled.
	bus.Publish("me.woke", contracts.MeWoke{SleptAt: time.Date(2026, 10, 24, 23, 0, 0, 0, cst), WokeAt: time.Date(2026, 10, 25, 7, 0, 0, 0, cst)})
	waitFor(t, "last event", func() bool { return len(logsOn(wake.Id, "2026-10-25")) == 1 })
	if n := len(logsOn(wake.Id, "2026-10-23")) + len(logsOn(wake.Id, "2026-10-24")); n != 0 {
		t.Fatalf("nap or guess counted: %d", n)
	}
	if n := len(logsOn(wake.Id, "2026-10-21")); n != 1 {
		t.Fatalf("same day counted twice: %d", n)
	}
	// Turning the rule off.
	var off api.Habit
	env.MustDo(http.MethodPatch, fmt.Sprintf("/habits/%d", wake.Id), map[string]any{"autoCheck": map[string]any{"kind": "none", "before": ""}}, &off)
	if off.AutoCheck != nil {
		t.Fatalf("rule kept: %+v", off.AutoCheck)
	}
}
