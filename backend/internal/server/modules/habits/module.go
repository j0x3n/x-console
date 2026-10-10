// Package habits is M8: habits with daily targets, check-ins, reminders,
// streaks and stats, plus a weekly workout plan and workout logs.
//
// Reminders go through notify.Send (kind habit.reminder) with "+1" and
// "skip" buttons. Habits linked to a Home Assistant entity check in when the
// entity changes state ("ha.state_changed" on the bus).
package habits

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/db"
)

// Module implements api.ServerInterface and contracts.Habits.
type Module struct {
	d      *module.Deps
	q      *db.Queries
	clock  *reminderClock
	tickMu sync.Mutex
	bodyMu sync.Mutex // B119: guards the report token setting

	haMu     sync.Mutex
	watched  map[string]bool   // entity ids linked to a habit
	haStates map[string]string // last known state per watched entity
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ contracts.Habits    = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), clock: newReminderClock(), watched: map[string]bool{}, haStates: map[string]string{}}
	if _, err := m.loadSchedule(context.Background()); err != nil {
		return nil, err
	}
	d.Notify.OnAction("habit.", m.handleAction)
	module.Provide[contracts.Habits](d.Registry, contracts.HabitsKey, m)
	module.Provide[contracts.ActivitySource](d.Registry, contracts.ActivitySourcePrefix+"habits", activitySource{m}) // B118
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "habits" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules reminders and hooks up Home Assistant.
func (m *Module) Start(ctx context.Context) error {
	m.d.Scheduler.Every("habits.tick", time.Minute, func(ctx context.Context) error {
		return m.tick(ctx, time.Now())
	})
	m.followPresence(ctx)
	ch, cancel := m.d.Bus.Subscribe("ha.state_changed", 256)
	go func() {
		<-ctx.Done()
		cancel()
	}()
	go func() {
		for ev := range ch {
			if st, ok := toHAState(ev.Data); ok {
				if err := m.onHAState(ctx, st); err != nil {
					m.d.Log.Warn("habit check-in from Home Assistant", "entity", st.EntityID, "err", err)
				}
			}
		}
	}()
	return m.refreshWatches(ctx)
}

// tick runs habit reminders and the workout notice. The scheduler calls it
// every minute; tests call it with any time.
func (m *Module) tick(ctx context.Context, now time.Time) error {
	m.tickMu.Lock()
	defer m.tickMu.Unlock()
	if err := m.remindAll(ctx, now); err != nil {
		return err
	}
	return m.workoutNotice(ctx, now)
}

// ---- HTTP handlers: habits ----

func (m *Module) ListHabits(w http.ResponseWriter, r *http.Request, params api.ListHabitsParams) {
	var archived int64
	if params.Archived != nil && *params.Archived {
		archived = 1
	}
	rows, err := m.q.ListHabits(r.Context(), archived)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.Habit, 0, len(rows))
	for _, row := range rows {
		h, err := m.habitAPI(r.Context(), row, time.Now())
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		out = append(out, h)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateHabit(w http.ResponseWriter, r *http.Request) {
	var body api.CreateHabitJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.create(r.Context(), body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h, err := m.habitAPI(r.Context(), row, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, h)
}

func (m *Module) GetHabit(w http.ResponseWriter, r *http.Request, id api.HabitId) {
	row, err := m.get(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h, err := m.habitAPI(r.Context(), row, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, h)
}

func (m *Module) UpdateHabit(w http.ResponseWriter, r *http.Request, id api.HabitId) {
	var body api.UpdateHabitJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.update(r.Context(), id, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h, err := m.habitAPI(r.Context(), row, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, h)
}

func (m *Module) DeleteHabit(w http.ResponseWriter, r *http.Request, id api.HabitId) {
	if err := m.remove(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) HabitsToday(w http.ResponseWriter, r *http.Request) {
	out, err := m.today(r.Context(), time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CheckinHabit(w http.ResponseWriter, r *http.Request, id api.HabitId) {
	var body api.CheckinHabitJSONRequestBody
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid("请求体读取失败"))
		return
	}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := decodeStrict(raw, &body); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	amount := 1.0
	if body.Amount != nil {
		amount = *body.Amount
	}
	note := ""
	if body.Note != nil {
		note = *body.Note
	}
	log, today, err := m.checkin(r.Context(), id, amount, note, "web", time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"log": logToAPI(log), "today": today})
}

func (m *Module) DeleteHabitLog(w http.ResponseWriter, r *http.Request, logID int64) {
	if err := m.undo(r.Context(), logID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) HabitStats(w http.ResponseWriter, r *http.Request, id api.HabitId, params api.HabitStatsParams) {
	days := 30
	if params.Days != nil {
		days = *params.Days
	}
	out, err := m.stats(r.Context(), id, days, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
