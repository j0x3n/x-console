package habits

// B162: the "me" module tells when the user woke up (event me.woke). The sleep
// of that night goes into the daily body record, unless the day already has a
// sleep value or the wake-up was only guessed from the schedule.

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/db"
)

// scheduleProvider gives other modules the daily schedule.
type scheduleProvider struct{ m *Module }

// Rules implements contracts.Schedule.
func (p scheduleProvider) Rules(ctx context.Context) (contracts.ScheduleRules, error) {
	s, err := p.m.loadSchedule(ctx)
	if err != nil {
		return contracts.ScheduleRules{}, err
	}
	return contracts.ScheduleRules{WorkDays: append([]int{}, s.WorkDays...), WakeTime: s.WakeTime, SleepTime: s.SleepTime, Timezone: s.Timezone}, nil
}

func (m *Module) followMeWoke(ctx context.Context) {
	ch, cancel := m.d.Bus.Subscribe("me.woke", 16)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				raw, _ := json.Marshal(ev.Data)
				var woke contracts.MeWoke
				if json.Unmarshal(raw, &woke) != nil {
					continue
				}
				if err := m.recordSleepFromWoke(ctx, woke); err != nil {
					m.d.Log.Warn("habit sleep from wake-up", "err", err)
				}
				if err := m.autoCheckSleep(ctx, woke); err != nil {
					m.d.Log.Warn("habit early sleep or wake-up", "err", err)
				}
			}
		}
	}()
}

// recordSleepFromWoke fills the sleep hours of the day the user woke up on.
func (m *Module) recordSleepFromWoke(ctx context.Context, woke contracts.MeWoke) error {
	if woke.Guessed || !woke.WokeAt.After(woke.SleptAt) {
		return nil
	}
	hours := math.Round(woke.WokeAt.Sub(woke.SleptAt).Hours()*10) / 10
	if hours <= 0 || hours > 24 {
		return nil
	}
	rules, err := m.loadSchedule(ctx)
	if err != nil {
		return err
	}
	date := dateKey(woke.WokeAt, rules.location())
	text := strconv.FormatFloat(hours, 'f', -1, 64)
	if day, err := m.personalDay(ctx, m.d.DB, date); err != nil || day.Sleep != "" {
		return err
	}
	_, err = m.personalTx(ctx, "habit.body.sleep_from_wake", func(tx *sql.Tx, _ *personalProfileState) error {
		day, err := m.personalDay(ctx, tx, date)
		if err != nil {
			return err
		}
		if day.Sleep != "" {
			return nil
		}
		if err = personalApplyDay(&day, api.PersonalDayInput{Sleep: &text}); err != nil {
			return err
		}
		return personalPut(ctx, tx, personalDayPrefix+date, day)
	})
	return err
}

// minAutoSleep keeps naps out of the early-sleep and early-wake habits (B201).
const minAutoSleep = 3 * time.Hour

// nightMinutes puts a clock time on a night that runs from noon to noon, so
// that 00:30 is later than 23:30.
func nightMinutes(clock int) int {
	if clock < 12*60 {
		return clock + 24*60
	}
	return clock
}

// sleepCheckin says whether one night meets a rule, and when the check-in
// goes: at the wake-up for wake, and on the evening the user went to bed for
// bedtime.
func sleepCheckin(kind, before string, woke contracts.MeWoke, loc *time.Location) (time.Time, string, bool) {
	limit, ok := parseClock(before)
	if !ok {
		return time.Time{}, "", false
	}
	switch kind {
	case autoWake:
		w := woke.WokeAt.In(loc)
		return woke.WokeAt, w.Format("15:04") + " 起床", w.Hour()*60+w.Minute() <= limit
	case autoBedtime:
		s := woke.SleptAt.In(loc)
		if nightMinutes(s.Hour()*60+s.Minute()) > nightMinutes(limit) {
			return time.Time{}, "", false
		}
		note := s.Format("15:04") + " 睡着"
		if s.Hour() < 12 {
			d := s.AddDate(0, 0, -1)
			return time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 0, 0, loc), note, true
		}
		return woke.SleptAt, note, true
	}
	return time.Time{}, "", false
}

// autoCheckSleep checks in the early-sleep and early-wake habits once per
// day. A day that already has a check-in, by hand or from an earlier event,
// is left alone.
func (m *Module) autoCheckSleep(ctx context.Context, woke contracts.MeWoke) error {
	if woke.Guessed || woke.WokeAt.Sub(woke.SleptAt) < minAutoSleep {
		return nil
	}
	habits, err := m.q.ListAutoCheckHabits(ctx)
	if err != nil || len(habits) == 0 {
		return err
	}
	rules, err := m.loadSchedule(ctx)
	if err != nil {
		return err
	}
	loc := rules.location()
	for _, h := range habits {
		at, note, ok := sleepCheckin(h.AutoCheck, h.AutoBefore, woke, loc)
		if !ok {
			continue
		}
		logs, err := m.q.ListHabitLogsForHabitSince(ctx, db.ListHabitLogsForHabitSinceParams{HabitID: h.ID, At: startOfDay(at, loc).UTC()})
		if err != nil {
			return err
		}
		day := dateKey(at, loc)
		done := false
		for _, l := range logs {
			done = done || dateKey(l.At, loc) == day
		}
		if done {
			continue
		}
		if _, _, err := m.checkin(ctx, h.ID, 1, note, "me", at); err != nil {
			return err
		}
	}
	return nil
}
