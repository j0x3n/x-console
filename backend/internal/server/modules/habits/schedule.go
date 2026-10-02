package habits

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

type habitSchedule struct {
	WorkDays    []int   `json:"workDays"`
	WakeTime    string  `json:"wakeTime"`
	SleepTime   string  `json:"sleepTime"`
	WorkStart   *string `json:"workStart,omitempty"`
	WorkEnd     *string `json:"workEnd,omitempty"`
	Timezone    string  `json:"timezone"`
	IdleMinutes int     `json:"idleMinutes"`
}

func scheduleToAPI(s scheduleRules) habitSchedule {
	out := habitSchedule{WorkDays: s.WorkDays, WakeTime: s.WakeTime, SleepTime: s.SleepTime, Timezone: s.Timezone, IdleMinutes: s.IdleMinutes}
	if s.WorkStart != "" {
		out.WorkStart = new(s.WorkStart)
		out.WorkEnd = new(s.WorkEnd)
	}
	return out
}

func (m *Module) loadSchedule(ctx context.Context) (scheduleRules, error) {
	s := scheduleRules{}
	var workDays string
	err := m.d.DB.QueryRowContext(ctx, "SELECT work_days, wake_time, sleep_time, work_start, work_end, timezone, idle_minutes FROM habit_schedule WHERE id = 1").Scan(&workDays, &s.WakeTime, &s.SleepTime, &s.WorkStart, &s.WorkEnd, &s.Timezone, &s.IdleMinutes)
	if errors.Is(err, sql.ErrNoRows) {
		s = scheduleRules{WorkDays: []int{1, 2, 3, 4, 5}, WakeTime: "12:00", SleepTime: "03:30", Timezone: m.d.Config.Location.String(), IdleMinutes: 5}
		if err := m.saveSchedule(ctx, s); err != nil {
			return s, err
		}
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal([]byte(workDays), &s.WorkDays); err != nil {
		return s, err
	}
	if s.WorkDays == nil {
		s.WorkDays = []int{}
	}
	return s, nil
}

func (m *Module) saveSchedule(ctx context.Context, s scheduleRules) error {
	workDays, err := json.Marshal(s.WorkDays)
	if err != nil {
		return err
	}
	_, err = m.d.DB.ExecContext(ctx, `INSERT INTO habit_schedule (id, work_days, wake_time, sleep_time, work_start, work_end, timezone, idle_minutes)
VALUES (1, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET work_days = excluded.work_days, wake_time = excluded.wake_time, sleep_time = excluded.sleep_time, work_start = excluded.work_start, work_end = excluded.work_end, timezone = excluded.timezone, idle_minutes = excluded.idle_minutes`, string(workDays), s.WakeTime, s.SleepTime, s.WorkStart, s.WorkEnd, s.Timezone, s.IdleMinutes)
	return err
}

func (m *Module) GetHabitSchedule(w http.ResponseWriter, r *http.Request) {
	s, err := m.loadSchedule(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, scheduleToAPI(s))
}

func (m *Module) UpdateHabitSchedule(w http.ResponseWriter, r *http.Request) {
	var in habitSchedule
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s := scheduleRules{WorkDays: in.WorkDays, WakeTime: in.WakeTime, SleepTime: in.SleepTime, Timezone: in.Timezone, IdleMinutes: in.IdleMinutes}
	if in.WorkStart != nil {
		s.WorkStart = *in.WorkStart
	}
	if in.WorkEnd != nil {
		s.WorkEnd = *in.WorkEnd
	}
	if s.WorkDays == nil {
		s.WorkDays = []int{}
	}
	if err := s.validate(); err != nil {
		httpx.Fail(w, r, httpx.Invalid(err.Error()))
		return
	}
	slices.Sort(s.WorkDays)
	err := m.saveSchedule(r.Context(), s)
	m.d.Audit.Record(r.Context(), "habit.schedule_update", "1", map[string]any{"timezone": s.Timezone, "idleMinutes": s.IdleMinutes}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.clock.clear()
	m.d.Bus.Publish("habit.schedule", scheduleToAPI(s))
	httpx.JSON(w, http.StatusOK, scheduleToAPI(s))
}

func presenceForSchedule(p contracts.HostPresence, rules scheduleRules) contracts.HostPresence {
	if !p.Online {
		p.State = "offline"
		return p
	}
	if p.Locked != nil && *p.Locked {
		p.State = "locked"
		return p
	}
	if p.DisplayOff != nil && *p.DisplayOff {
		p.State = "idle"
		return p
	}
	if !p.Known {
		p.State = "unknown"
		p.IdleSeconds = nil
		return p
	}
	if hostActive(p, rules.IdleMinutes) {
		p.State = "active"
	} else {
		p.State = "idle"
	}
	return p
}

func (m *Module) habitPresence(ctx context.Context, rules scheduleRules) ([]contracts.HostPresence, error) {
	visible, err := m.visibleReminderHosts(ctx)
	if err != nil {
		return nil, err
	}
	agents, err := m.d.Agents.List(ctx)
	if err != nil {
		return nil, err
	}
	service, ok := module.Lookup[contracts.Presence](m.d.Registry, contracts.PresenceKey)
	out := []contracts.HostPresence{}
	for _, agent := range agents {
		if !visible[agent.ID] {
			continue
		}
		p := contracts.HostPresence{HostID: agent.ID, Name: agent.Name, Online: agent.Online, State: "unknown", Since: agent.CreatedAt}
		if ok {
			p = service.State(agent.ID)
		}
		p.HostID, p.Name = agent.ID, agent.Name
		if p.Since.IsZero() {
			p.Since = time.Now().UTC()
		}
		out = append(out, presenceForSchedule(p, rules))
	}
	return out, nil
}

func (m *Module) GetHabitPresence(w http.ResponseWriter, r *http.Request) {
	rules, err := m.loadSchedule(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.habitPresence(r.Context(), rules)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
