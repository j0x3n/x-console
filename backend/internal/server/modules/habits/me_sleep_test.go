package habits_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
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
