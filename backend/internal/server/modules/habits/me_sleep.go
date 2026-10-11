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

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
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
