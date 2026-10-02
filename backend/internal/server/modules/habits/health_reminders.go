package habits

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/db"
)

func stringValues[T ~string](values []T) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func jsonStrings(values []string) string {
	if values == nil {
		values = []string{}
	}
	raw, _ := json.Marshal(values)
	return string(raw)
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func habitWhen(raw string) []api.HabitRemindWhen {
	values, _ := normalizeRemindWhen(parseTimes(raw))
	out := make([]api.HabitRemindWhen, 0, len(values))
	for _, value := range values {
		out = append(out, api.HabitRemindWhen(value))
	}
	return out
}

func habitTemplate(value string) *api.HabitTemplate {
	if value == "" {
		return nil
	}
	return new(api.HabitTemplate(value))
}

func (m *Module) validateReminderHosts(ctx context.Context, fields *habitFields) error {
	if len(fields.HostIDs) == 0 && !slices.Contains(fields.When, "active") && !fields.OnHost {
		fields.HostIDs = []string{}
		return nil
	}
	visible, err := m.visibleReminderHosts(ctx)
	if err != nil {
		return err
	}
	ids := []string{}
	for _, id := range fields.HostIDs {
		if !visible[id] {
			return httpx.Invalid("提醒电脑不存在或已隐藏")
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	fields.HostIDs = ids
	if (slices.Contains(fields.When, "active") || fields.OnHost) && len(ids) == 0 {
		return httpx.Invalid("请选择提醒电脑")
	}
	return nil
}

func selectedPresence(h db.Habit, values []contracts.HostPresence) []contracts.HostPresence {
	ids := parseTimes(h.ActiveHostIds)
	out := []contracts.HostPresence{}
	for _, p := range values {
		if slices.Contains(ids, p.HostID) {
			out = append(out, p)
		}
	}
	return out
}

func (m *Module) evaluateReminder(h db.Habit, p api.HabitToday, rules scheduleRules, now time.Time, hosts []contracts.HostPresence, commit bool) (bool, *time.Time) {
	if h.ArchivedAt != nil || h.RemindMode == modeNone || p.Reached {
		return false, nil
	}
	when, _ := normalizeRemindWhen(parseTimes(h.RemindWhen))
	if h.RemindMode == modeTimes {
		if h.QuietUntil != nil && now.Before(*h.QuietUntil) {
			return false, nil
		}
		return fixedReminder(parseTimes(h.RemindTimes), h.LastRemindedAt, h.SnoozedUntil, now, rules.location())
	}
	var last *time.Time
	if len(p.Logs) > 0 {
		last = &p.Logs[0].At
	}
	policy := reminderPolicy{When: when, Window: h.RemindWindow, Interval: time.Duration(h.RemindIntervalMinutes) * time.Minute, LastCheckin: last, LastReminded: h.LastRemindedAt, QuietUntil: h.QuietUntil, SnoozedUntil: h.SnoozedUntil}
	return m.clock.evaluate(h.ID, rules, policy, now, hosts, commit)
}

func (m *Module) nextReminder(ctx context.Context, h db.Habit, p api.HabitToday, now time.Time) (*time.Time, error) {
	if h.ArchivedAt != nil || h.RemindMode == modeNone || p.Reached {
		return nil, nil
	}
	rules, err := m.loadSchedule(ctx)
	if err != nil {
		return nil, err
	}
	presence := []contracts.HostPresence{}
	if slices.Contains(parseTimes(h.RemindWhen), "active") {
		presence, err = m.habitPresence(ctx, rules)
		if err != nil {
			return nil, err
		}
	}
	_, next := m.evaluateReminder(h, p, rules, now, selectedPresence(h, presence), false)
	return next, nil
}

func (m *Module) habitAPI(ctx context.Context, h db.Habit, now time.Time) (api.Habit, error) {
	p, err := m.progressOf(ctx, h, now)
	return p.Habit, err
}

func (m *Module) followPresence(ctx context.Context) {
	provider, ok := module.Lookup[contracts.Presence](m.d.Registry, contracts.PresenceKey)
	if !ok {
		return
	}
	ch, cancel := provider.Subscribe()
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case p, ok := <-ch:
				if !ok {
					return
				}
				visible, err := m.visibleReminderHosts(ctx)
				if err != nil || !visible[p.HostID] {
					continue
				}
				rules, err := m.loadSchedule(ctx)
				if err != nil {
					continue
				}
				p = presenceForSchedule(p, rules)
				m.d.Bus.Publish("habit.presence", p)
				if err := m.tick(ctx, time.Now()); err != nil {
					m.d.Log.Warn("habit presence reminders", "err", err)
				}
			}
		}
	}()
}
