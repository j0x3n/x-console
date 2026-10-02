package habits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Settings keys.
const (
	workoutSettingsKey   = "habits.workout_notify"
	workoutLastNoticeKey = "habits.workout_last_notice"
)

func parseItems(raw string) []api.WorkoutItem {
	out := []api.WorkoutItem{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func cleanItems(items []api.WorkoutItem) ([]api.WorkoutItem, error) {
	out := make([]api.WorkoutItem, 0, len(items))
	for _, it := range items {
		if it.ExerciseId != nil && !personalExercise(*it.ExerciseId) {
			return nil, httpx.Invalid("动作库中没有这个动作")
		}
		if it.Prescription != nil && len([]rune(*it.Prescription)) > 300 {
			return nil, httpx.Invalid("训练要求太长了")
		}
		it.Name = strings.TrimSpace(it.Name)
		if it.Name == "" {
			return nil, httpx.Invalid("动作名称不能为空")
		}
		if (it.Sets != nil && *it.Sets < 0) || (it.Reps != nil && *it.Reps < 0) || (it.Weight != nil && *it.Weight < 0) {
			return nil, httpx.Invalid("组数、次数和重量不能是负数")
		}
		out = append(out, it)
	}
	return out, nil
}

func planToAPI(p db.WorkoutPlan) api.WorkoutPlan {
	id := p.ID
	return api.WorkoutPlan{Id: &id, Weekday: int(p.Weekday), Title: p.Title, Items: parseItems(p.Items)}
}

func workoutLogToAPI(l db.WorkoutLog) api.WorkoutLog {
	return api.WorkoutLog{Id: l.ID, Date: l.Date, PlanId: l.PlanID, Items: parseItems(l.Items),
		DurationMinutes: int(l.DurationMinutes), Note: l.Note, CreatedAt: l.CreatedAt}
}

func (m *Module) plans(ctx context.Context) ([]api.WorkoutPlan, error) {
	rows, err := m.q.ListWorkoutPlans(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.WorkoutPlan, 0, len(rows))
	for _, p := range rows {
		out = append(out, planToAPI(p))
	}
	return out, nil
}

func (m *Module) replacePlans(ctx context.Context, in []api.WorkoutPlan) error {
	type clean struct {
		weekday int64
		title   string
		items   string
	}
	rows := make([]clean, 0, len(in))
	for _, p := range in {
		if p.Weekday < 1 || p.Weekday > 7 {
			return httpx.Invalid("星期要在 1 到 7 之间")
		}
		items, err := cleanItems(p.Items)
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(items)
		rows = append(rows, clean{int64(p.Weekday), strings.TrimSpace(p.Title), string(raw)})
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	if err := q.DeleteWorkoutPlans(ctx); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := q.InsertWorkoutPlan(ctx, db.InsertWorkoutPlanParams{Weekday: r.weekday, Title: r.title, Items: r.items}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (m *Module) logWorkout(ctx context.Context, in api.WorkoutLogInput, now time.Time) (db.WorkoutLog, error) {
	rules, err := m.loadSchedule(ctx)
	if err != nil {
		return db.WorkoutLog{}, err
	}
	loc := rules.location()
	date := dateKey(now, loc)
	if in.Date != nil && strings.TrimSpace(*in.Date) != "" {
		d, err := time.Parse(time.DateOnly, strings.TrimSpace(*in.Date))
		if err != nil {
			return db.WorkoutLog{}, httpx.Invalid("日期要写成 2026-09-27 这样")
		}
		date = d.Format(time.DateOnly)
	}
	var items []api.WorkoutItem
	if in.Items != nil {
		var err error
		if items, err = cleanItems(*in.Items); err != nil {
			return db.WorkoutLog{}, err
		}
	}
	if in.PlanId != nil {
		plans, err := m.q.ListWorkoutPlans(ctx)
		if err != nil {
			return db.WorkoutLog{}, err
		}
		found := false
		for _, p := range plans {
			if p.ID == *in.PlanId {
				found = true
				if in.Items == nil {
					items = parseItems(p.Items)
				}
			}
		}
		if !found {
			return db.WorkoutLog{}, httpx.Invalid("训练计划不存在")
		}
	}
	if items == nil {
		items = []api.WorkoutItem{}
	}
	duration := 0
	if in.DurationMinutes != nil {
		duration = *in.DurationMinutes
	}
	if duration < 0 || duration > 24*60 {
		return db.WorkoutLog{}, httpx.Invalid("时长要在 0 到 1440 分钟之间")
	}
	note := ""
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}
	raw, _ := json.Marshal(items)
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return db.WorkoutLog{}, err
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	row, err := q.CreateWorkoutLog(ctx, db.CreateWorkoutLogParams{
		Date: date, PlanID: in.PlanId, Items: string(raw), DurationMinutes: int64(duration), Note: note, CreatedAt: now.UTC(),
	})
	if err != nil {
		return row, err
	}
	habits, err := q.ListActiveWorkoutHabits(ctx)
	if err != nil {
		return row, err
	}
	when := now.UTC()
	if date != dateKey(now, loc) {
		d, _ := time.Parse(time.DateOnly, date)
		when = time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, loc).UTC()
	}
	type checkin struct {
		habit db.Habit
		log   db.HabitLog
	}
	checkins := make([]checkin, 0, len(habits))
	for _, h := range habits {
		log, err := q.CreateHabitLog(ctx, db.CreateHabitLogParams{
			HabitID: h.ID, At: when, Amount: 1, Source: "workout", WorkoutLogID: &row.ID,
		})
		if err != nil {
			return row, err
		}
		checkins = append(checkins, checkin{h, log})
	}
	if err := tx.Commit(); err != nil {
		return row, err
	}
	m.d.Audit.Record(ctx, "workout.log", date, map[string]any{"minutes": duration}, nil)
	m.d.Bus.Publish("workout.logged", workoutLogToAPI(row))
	for _, item := range checkins {
		p, err := m.progressOf(ctx, item.habit, now)
		if err != nil {
			m.d.Log.Warn("workout habit progress", "habit", item.habit.ID, "err", err)
			continue
		}
		m.d.Bus.Publish("habit.checked_in", map[string]any{
			"habitId": item.habit.ID, "name": item.habit.Name, "log": logToAPI(item.log),
			"done": p.Done, "target": item.habit.DailyTarget, "source": "workout",
		})
		if date == dateKey(now, loc) && p.Reached && p.Done-1 < item.habit.DailyTarget {
			m.d.Bus.Publish("habit.goal_reached", map[string]any{
				"habitId": item.habit.ID, "name": item.habit.Name, "done": p.Done,
				"target": item.habit.DailyTarget, "streak": p.Streak,
			})
		}
	}
	return row, nil
}

type workoutSettings struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`
}

func (m *Module) workoutSettings(ctx context.Context) (workoutSettings, error) {
	s := workoutSettings{Enabled: true, Time: "08:00"}
	err := m.d.Settings.Get(ctx, workoutSettingsKey, &s)
	if errors.Is(err, settings.ErrNotSet) {
		err = nil
	}
	return s, err
}

// workoutNotice sends today's plan once a day at the configured time.
func (m *Module) workoutNotice(ctx context.Context, now time.Time) error {
	rules, err := m.loadSchedule(ctx)
	if err != nil {
		return err
	}
	loc := rules.location()
	cfg, err := m.workoutSettings(ctx)
	if err != nil {
		return err
	}
	var last string
	if err := m.d.Settings.Get(ctx, workoutLastNoticeKey, &last); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return err
	}
	if !workoutDue(cfg.Enabled, cfg.Time, last, now, loc) {
		return nil
	}
	if err := m.d.Settings.Set(ctx, workoutLastNoticeKey, dateKey(now, loc)); err != nil {
		return err
	}
	rows, err := m.q.ListWorkoutPlans(ctx)
	if err != nil {
		return err
	}
	weekday := isoWeekday(now.In(loc))
	var today []db.WorkoutPlan
	for _, p := range rows {
		if p.Weekday == weekday {
			today = append(today, p)
		}
	}
	if len(today) == 0 {
		return nil
	}
	_, err = m.d.Notify.Send(ctx, workoutNotification(today))
	return err
}

func describeItem(it api.WorkoutItem) string {
	s := it.Name
	if it.Prescription != nil && strings.TrimSpace(*it.Prescription) != "" {
		if it.Sets != nil && *it.Sets > 0 {
			s += fmt.Sprintf(" %d 组 ×", *it.Sets)
		}
		s += " " + strings.TrimSpace(*it.Prescription)
	} else if it.Sets != nil && it.Reps != nil && *it.Sets > 0 && *it.Reps > 0 {
		s += fmt.Sprintf(" %d×%d", *it.Sets, *it.Reps)
	} else if it.Sets != nil && *it.Sets > 0 {
		s += fmt.Sprintf(" %d 组", *it.Sets)
	}
	if it.Weight != nil && *it.Weight > 0 {
		s += " " + strconv.FormatFloat(*it.Weight, 'f', -1, 64) + "kg"
	}
	return s
}

func workoutNotification(plans []db.WorkoutPlan) notify.Notification {
	var titles, lines []string
	for _, p := range plans {
		if p.Title != "" {
			titles = append(titles, p.Title)
		}
		for _, it := range parseItems(p.Items) {
			lines = append(lines, describeItem(it))
		}
	}
	title := "今天有训练计划"
	if len(titles) > 0 {
		title = "今天的训练：" + strings.Join(titles, "、")
	}
	return notify.Notification{
		Kind: "workout.plan", Title: title, Body: strings.Join(lines, "\n"), Link: "/habits/workout", Source: "habits",
	}
}

// ---- HTTP handlers: workouts ----

func (m *Module) ListWorkoutPlans(w http.ResponseWriter, r *http.Request) {
	out, err := m.plans(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) ReplaceWorkoutPlans(w http.ResponseWriter, r *http.Request) {
	var body api.ReplaceWorkoutPlansJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err := m.replacePlans(r.Context(), body)
	m.d.Audit.Record(r.Context(), "workout.plans.update", "", map[string]any{"count": len(body)}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.plans(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("workout.plans_updated", out)
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) ListWorkoutLogs(w http.ResponseWriter, r *http.Request, params api.ListWorkoutLogsParams) {
	days := 30
	if params.Days != nil {
		days = *params.Days
	}
	if days < 1 || days > 366 {
		httpx.Fail(w, r, httpx.Invalid("days 要在 1 到 366 之间"))
		return
	}
	rules, err := m.loadSchedule(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	since := startOfDay(time.Now(), rules.location()).AddDate(0, 0, -(days - 1)).Format(time.DateOnly)
	rows, err := m.q.ListWorkoutLogsSince(r.Context(), since)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.WorkoutLog, 0, len(rows))
	for _, l := range rows {
		out = append(out, workoutLogToAPI(l))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateWorkoutLog(w http.ResponseWriter, r *http.Request) {
	var body api.CreateWorkoutLogJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.logWorkout(r.Context(), body, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, workoutLogToAPI(row))
}

func (m *Module) DeleteWorkoutLog(w http.ResponseWriter, r *http.Request, logID int64) {
	tx, err := m.d.DB.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	checkins, err := q.DeleteWorkoutCheckins(r.Context(), &logID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	n, err := q.DeleteWorkoutLog(r.Context(), logID)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(r.Context(), "workout.log_delete", strconv.FormatInt(logID, 10), nil, nil)
	m.d.Bus.Publish("workout.log_deleted", map[string]int64{"id": logID})
	for _, item := range checkins {
		m.d.Bus.Publish("habit.log_deleted", map[string]int64{"habitId": item.HabitID, "logId": item.ID})
	}
	httpx.NoContent(w)
}

func (m *Module) GetWorkoutSettings(w http.ResponseWriter, r *http.Request) {
	s, err := m.workoutSettings(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, api.WorkoutSettings{NotifyEnabled: s.Enabled, NotifyTime: s.Time})
}

func (m *Module) UpdateWorkoutSettings(w http.ResponseWriter, r *http.Request) {
	var body api.UpdateWorkoutSettingsJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, ok := parseClock(body.NotifyTime); !ok || body.NotifyTime == "24:00" {
		httpx.Fail(w, r, httpx.Invalid("时间要写成 08:00 这样"))
		return
	}
	err := m.d.Settings.Set(r.Context(), workoutSettingsKey, workoutSettings{Enabled: body.NotifyEnabled, Time: body.NotifyTime})
	m.d.Audit.Record(r.Context(), "workout.settings.update", "", map[string]any{"enabled": body.NotifyEnabled, "time": body.NotifyTime}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, body)
}
