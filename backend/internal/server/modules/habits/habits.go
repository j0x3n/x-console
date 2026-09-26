package habits

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// Check-in sources.
var sources = map[string]bool{"web": true, "telegram": true, "webpush": true, "ha": true, "ai": true, "automation": true}

func decodeStrict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return httpx.Invalid("请求体格式不正确: " + err.Error())
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.ErrNotFound
	}
	return err
}

func parseTimes(raw string) []string {
	out := []string{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func toAPI(h db.Habit) api.Habit {
	return api.Habit{
		Id: h.ID, Name: h.Name, Icon: h.Icon, Color: h.Color, Unit: h.Unit, DailyTarget: h.DailyTarget,
		RemindMode: api.RemindMode(h.RemindMode), RemindIntervalMinutes: int(h.RemindIntervalMinutes),
		RemindWindow: h.RemindWindow, RemindTimes: parseTimes(h.RemindTimes), HaEntityId: h.HaEntityID,
		Archived: h.ArchivedAt != nil, SortOrder: int(h.SortOrder), CreatedAt: h.CreatedAt,
	}
}

func logToAPI(l db.HabitLog) api.HabitLog {
	return api.HabitLog{Id: l.ID, HabitId: l.HabitID, At: l.At, Amount: l.Amount, Source: l.Source, Note: l.Note}
}

// habitFields is the editable part of a habit, shared by create and update.
type habitFields struct {
	Name, Icon, Color, Unit string
	Target                  float64
	Mode                    string
	Interval                int
	Window                  string
	Times                   []string
	Entity                  string
	SortOrder               int
}

// validate cleans the fields and checks the reminder settings.
func (f *habitFields) validate() error {
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" {
		return httpx.Invalid("名称不能为空")
	}
	if len([]rune(f.Name)) > 100 {
		return httpx.Invalid("名称太长了")
	}
	f.Unit = strings.TrimSpace(f.Unit)
	if f.Unit == "" {
		f.Unit = "次"
	}
	if f.Target <= 0 || f.Target > 100000 || math.IsNaN(f.Target) {
		return httpx.Invalid("每日目标要大于 0")
	}
	f.Entity = strings.TrimSpace(f.Entity)
	if f.Entity != "" && !strings.Contains(f.Entity, ".") {
		return httpx.Invalid("Home Assistant 实体要写成 light.kitchen 这样")
	}
	if f.Mode == "" {
		f.Mode = modeNone
	}
	f.Window = strings.TrimSpace(f.Window)
	if _, _, err := parseWindow(f.Window); err != nil {
		return httpx.Invalid(err.Error())
	}
	times, err := normalizeTimes(f.Times)
	if err != nil {
		return httpx.Invalid(err.Error())
	}
	f.Times = times
	switch f.Mode {
	case modeNone:
	case modeInterval:
		if f.Interval < 5 || f.Interval > 24*60 {
			return httpx.Invalid("提醒间隔要在 5 到 1440 分钟之间")
		}
	case modeTimes:
		if len(f.Times) == 0 {
			return httpx.Invalid("至少填一个提醒时间")
		}
	default:
		return httpx.Invalid("提醒方式无效: " + f.Mode)
	}
	return nil
}

func (m *Module) get(ctx context.Context, id int64) (db.Habit, error) {
	h, err := m.q.GetHabit(ctx, id)
	return h, notFound(err)
}

func (m *Module) create(ctx context.Context, in api.HabitInput) (db.Habit, error) {
	f := habitFields{Name: in.Name, Target: 1}
	setIf(&f.Icon, in.Icon)
	setIf(&f.Color, in.Color)
	setIf(&f.Unit, in.Unit)
	setIf(&f.Target, in.DailyTarget)
	setIf(&f.Interval, in.RemindIntervalMinutes)
	setIf(&f.Window, in.RemindWindow)
	setIf(&f.Times, in.RemindTimes)
	setIf(&f.Entity, in.HaEntityId)
	if in.RemindMode != nil {
		f.Mode = string(*in.RemindMode)
	}
	if in.SortOrder != nil {
		f.SortOrder = *in.SortOrder
	} else {
		max, err := m.q.MaxHabitSortOrder(ctx)
		if err != nil {
			return db.Habit{}, err
		}
		f.SortOrder = int(max) + 1
	}
	if err := f.validate(); err != nil {
		return db.Habit{}, err
	}
	times, _ := json.Marshal(f.Times)
	h, err := m.q.CreateHabit(ctx, db.CreateHabitParams{
		Name: f.Name, Icon: f.Icon, Color: f.Color, Unit: f.Unit, DailyTarget: f.Target, RemindMode: f.Mode,
		RemindIntervalMinutes: int64(f.Interval), RemindWindow: f.Window, RemindTimes: string(times),
		HaEntityID: f.Entity, SortOrder: int64(f.SortOrder), CreatedAt: time.Now().UTC(),
	})
	m.d.Audit.Record(ctx, "habit.create", strconv.FormatInt(h.ID, 10), map[string]any{"name": f.Name}, err)
	if err != nil {
		return h, err
	}
	m.watch(ctx, h.HaEntityID)
	m.d.Bus.Publish("habit.created", toAPI(h))
	return h, nil
}

func setIf[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func (m *Module) update(ctx context.Context, id int64, p api.HabitPatch) (db.Habit, error) {
	h, err := m.get(ctx, id)
	if err != nil {
		return h, err
	}
	f := habitFields{
		Name: h.Name, Icon: h.Icon, Color: h.Color, Unit: h.Unit, Target: h.DailyTarget, Mode: h.RemindMode,
		Interval: int(h.RemindIntervalMinutes), Window: h.RemindWindow, Times: parseTimes(h.RemindTimes),
		Entity: h.HaEntityID, SortOrder: int(h.SortOrder),
	}
	setIf(&f.Name, p.Name)
	setIf(&f.Icon, p.Icon)
	setIf(&f.Color, p.Color)
	setIf(&f.Unit, p.Unit)
	setIf(&f.Target, p.DailyTarget)
	setIf(&f.Interval, p.RemindIntervalMinutes)
	setIf(&f.Window, p.RemindWindow)
	setIf(&f.Times, p.RemindTimes)
	setIf(&f.Entity, p.HaEntityId)
	setIf(&f.SortOrder, p.SortOrder)
	if p.RemindMode != nil {
		f.Mode = string(*p.RemindMode)
	}
	if err := f.validate(); err != nil {
		return h, err
	}
	archived := h.ArchivedAt
	if p.Archived != nil {
		switch {
		case *p.Archived && archived == nil:
			now := time.Now().UTC()
			archived = &now
		case !*p.Archived:
			archived = nil
		}
	}
	times, _ := json.Marshal(f.Times)
	row, err := m.q.UpdateHabit(ctx, db.UpdateHabitParams{
		Name: f.Name, Icon: f.Icon, Color: f.Color, Unit: f.Unit, DailyTarget: f.Target, RemindMode: f.Mode,
		RemindIntervalMinutes: int64(f.Interval), RemindWindow: f.Window, RemindTimes: string(times),
		HaEntityID: f.Entity, ArchivedAt: archived, SortOrder: int64(f.SortOrder), ID: id,
	})
	m.d.Audit.Record(ctx, "habit.update", strconv.FormatInt(id, 10), nil, err)
	if err != nil {
		return row, notFound(err)
	}
	if err := m.refreshWatches(ctx); err != nil {
		m.d.Log.Warn("refresh Home Assistant watches", "err", err)
	}
	m.d.Bus.Publish("habit.updated", toAPI(row))
	return row, nil
}

func (m *Module) remove(ctx context.Context, id int64) error {
	n, err := m.q.DeleteHabit(ctx, id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(ctx, "habit.delete", strconv.FormatInt(id, 10), nil, err)
	if err != nil {
		return err
	}
	if err := m.refreshWatches(ctx); err != nil {
		m.d.Log.Warn("refresh Home Assistant watches", "err", err)
	}
	m.d.Bus.Publish("habit.deleted", map[string]int64{"id": id})
	return nil
}

// dailyTotals sums logs by local date.
func dailyTotals(logs []db.HabitLog, loc *time.Location) map[string]float64 {
	out := map[string]float64{}
	for _, l := range logs {
		out[dateKey(l.At, loc)] += l.Amount
	}
	return out
}

// historyStart is how far back streaks look.
func historyStart(now time.Time, loc *time.Location) time.Time {
	return startOfDay(now, loc).AddDate(-1, 0, -1).UTC()
}

// progress builds today's view of one habit from its logs (newest first).
func progress(h db.Habit, logs []db.HabitLog, now time.Time, loc *time.Location) api.HabitToday {
	totals := dailyTotals(logs, loc)
	today := dateKey(now, loc)
	out := api.HabitToday{Habit: toAPI(h), Done: totals[today], Logs: []api.HabitLog{}}
	out.Reached = out.Done >= h.DailyTarget
	out.Streak = streak(totals, h.DailyTarget, now, loc)
	for _, l := range logs {
		if dateKey(l.At, loc) == today {
			out.Logs = append(out.Logs, logToAPI(l))
		}
	}
	return out
}

// today returns the progress of every active habit.
func (m *Module) today(ctx context.Context, now time.Time) ([]api.HabitToday, error) {
	loc := m.d.Config.Location
	habits, err := m.q.ListHabits(ctx, 0)
	if err != nil {
		return nil, err
	}
	logs, err := m.q.ListHabitLogsSince(ctx, historyStart(now, loc))
	if err != nil {
		return nil, err
	}
	byHabit := map[int64][]db.HabitLog{}
	for _, l := range logs {
		byHabit[l.HabitID] = append(byHabit[l.HabitID], l)
	}
	out := make([]api.HabitToday, 0, len(habits))
	for _, h := range habits {
		out = append(out, progress(h, byHabit[h.ID], now, loc))
	}
	return out, nil
}

// progressOf returns today's view of one habit.
func (m *Module) progressOf(ctx context.Context, h db.Habit, now time.Time) (api.HabitToday, error) {
	logs, err := m.q.ListHabitLogsForHabitSince(ctx, db.ListHabitLogsForHabitSinceParams{
		HabitID: h.ID, At: historyStart(now, m.d.Config.Location),
	})
	if err != nil {
		return api.HabitToday{}, err
	}
	return progress(h, logs, now, m.d.Config.Location), nil
}

// checkin records amount for a habit and publishes habit.checked_in, plus
// habit.goal_reached when this check-in reaches today's target.
func (m *Module) checkin(ctx context.Context, id int64, amount float64, note, source string, now time.Time) (db.HabitLog, api.HabitToday, error) {
	if amount <= 0 || amount > 100000 || math.IsNaN(amount) {
		return db.HabitLog{}, api.HabitToday{}, httpx.Invalid("数量要大于 0")
	}
	if !sources[source] {
		source = "web"
	}
	h, err := m.get(ctx, id)
	if err != nil {
		return db.HabitLog{}, api.HabitToday{}, err
	}
	if h.ArchivedAt != nil {
		return db.HabitLog{}, api.HabitToday{}, httpx.NewError(409, "conflict", "这个习惯已经归档")
	}
	before, err := m.progressOf(ctx, h, now)
	if err != nil {
		return db.HabitLog{}, api.HabitToday{}, err
	}
	log, err := m.q.CreateHabitLog(ctx, db.CreateHabitLogParams{
		HabitID: id, At: now.UTC(), Amount: amount, Source: source, Note: strings.TrimSpace(note),
	})
	m.d.Audit.Record(ctx, "habit.checkin", strconv.FormatInt(id, 10), map[string]any{"amount": amount, "source": source}, err)
	if err != nil {
		return log, api.HabitToday{}, err
	}
	after, err := m.progressOf(ctx, h, now)
	if err != nil {
		return log, after, err
	}
	m.d.Bus.Publish("habit.checked_in", map[string]any{
		"habitId": id, "name": h.Name, "log": logToAPI(log), "done": after.Done, "target": h.DailyTarget, "source": source,
	})
	if !before.Reached && after.Reached {
		m.d.Bus.Publish("habit.goal_reached", map[string]any{
			"habitId": id, "name": h.Name, "done": after.Done, "target": h.DailyTarget, "streak": after.Streak,
		})
	}
	return log, after, nil
}

func (m *Module) undo(ctx context.Context, logID int64) error {
	l, err := m.q.GetHabitLog(ctx, logID)
	if err != nil {
		return notFound(err)
	}
	_, err = m.q.DeleteHabitLog(ctx, logID)
	m.d.Audit.Record(ctx, "habit.checkin_undo", strconv.FormatInt(l.HabitID, 10), map[string]any{"logId": logID}, err)
	if err != nil {
		return err
	}
	m.d.Bus.Publish("habit.log_deleted", map[string]int64{"habitId": l.HabitID, "logId": logID})
	return nil
}

func (m *Module) stats(ctx context.Context, id int64, days int, now time.Time) (api.HabitStats, error) {
	if days < 1 || days > 366 {
		return api.HabitStats{}, httpx.Invalid("days 要在 1 到 366 之间")
	}
	h, err := m.get(ctx, id)
	if err != nil {
		return api.HabitStats{}, err
	}
	loc := m.d.Config.Location
	logs, err := m.q.ListHabitLogsForHabitSince(ctx, db.ListHabitLogsForHabitSinceParams{HabitID: id, At: historyStart(now, loc)})
	if err != nil {
		return api.HabitStats{}, err
	}
	totals := dailyTotals(logs, loc)
	list, best := series(totals, h.DailyTarget, days, now, loc)
	out := api.HabitStats{HabitId: id, Streak: streak(totals, h.DailyTarget, now, loc), BestStreak: best, Days: []api.HabitDay{}}
	for _, d := range list {
		out.Days = append(out.Days, api.HabitDay{Date: d.Date, Amount: d.Amount, Reached: d.Reached})
		out.Total += d.Amount
		if d.Reached {
			out.ReachedDays++
		}
	}
	return out, nil
}

// ---- reminders ----

// remindAll sends due habit reminders.
func (m *Module) remindAll(ctx context.Context, now time.Time) error {
	habits, err := m.q.ListHabits(ctx, 0)
	if err != nil {
		return err
	}
	for _, h := range habits {
		if h.RemindMode == modeNone {
			continue
		}
		p, err := m.progressOf(ctx, h, now)
		if err != nil {
			return err
		}
		var last *time.Time
		if len(p.Logs) > 0 {
			last = &p.Logs[0].At
		}
		st := remindState{
			Mode: h.RemindMode, Interval: time.Duration(h.RemindIntervalMinutes) * time.Minute, Window: h.RemindWindow,
			Times: parseTimes(h.RemindTimes), Target: h.DailyTarget, DoneToday: p.Done,
			LastCheckin: last, LastReminded: h.LastRemindedAt, QuietUntil: h.QuietUntil,
		}
		if !shouldRemind(st, now, m.d.Config.Location) {
			continue
		}
		at := now.UTC()
		if err := m.q.MarkHabitReminded(ctx, db.MarkHabitRemindedParams{LastRemindedAt: &at, ID: h.ID}); err != nil {
			return err
		}
		if _, err := m.d.Notify.Send(ctx, reminderNotification(h, p.Done)); err != nil {
			return err
		}
	}
	return nil
}

func formatAmount(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func reminderNotification(h db.Habit, done float64) notify.Notification {
	id := strconv.FormatInt(h.ID, 10)
	return notify.Notification{
		Kind:   "habit.reminder",
		Title:  h.Name,
		Body:   fmt.Sprintf("今天 %s/%s %s。", formatAmount(done), formatAmount(h.DailyTarget), h.Unit),
		Link:   "/habits",
		Source: "habits",
		Actions: []notify.Action{
			{ID: "habit.checkin:" + id + ":1", Label: "+1 " + h.Unit},
			{ID: "habit.skip:" + id, Label: "跳过"},
		},
		Data: map[string]any{"habitId": h.ID},
	}
}

// sourceFromActor maps who pressed a notification button to a log source.
func sourceFromActor(ctx context.Context) string {
	actor := audit.Actor(ctx)
	switch {
	case actor == "telegram":
		return "telegram"
	case strings.HasPrefix(actor, "webpush"):
		return "webpush"
	case strings.HasPrefix(actor, "automation"):
		return "automation"
	}
	return "web"
}

// handleAction runs notification buttons: habit.checkin:<id>:<amount> and
// habit.skip:<id> (no more reminders for this habit today).
func (m *Module) handleAction(ctx context.Context, actionID string) error {
	parts := strings.Split(strings.TrimPrefix(actionID, "habit."), ":")
	if len(parts) < 2 {
		return httpx.ErrNotFound
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return httpx.ErrNotFound
	}
	now := time.Now()
	switch {
	case parts[0] == "checkin" && len(parts) == 3:
		amount, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return httpx.ErrNotFound
		}
		_, _, err = m.checkin(ctx, id, amount, "", sourceFromActor(ctx), now)
		return err
	case parts[0] == "skip" && len(parts) == 2:
		if _, err := m.get(ctx, id); err != nil {
			return err
		}
		until := startOfDay(now, m.d.Config.Location).AddDate(0, 0, 1).UTC()
		err := m.q.SetHabitQuietUntil(ctx, db.SetHabitQuietUntilParams{QuietUntil: &until, ID: id})
		m.d.Audit.Record(ctx, "habit.skip", strconv.FormatInt(id, 10), nil, err)
		return err
	}
	return httpx.ErrNotFound
}

// ---- contracts.Habits ----

// Checkin implements contracts.Habits.
func (m *Module) Checkin(ctx context.Context, habitID int64, amount float64, source string) error {
	_, _, err := m.checkin(ctx, habitID, amount, "", source, time.Now())
	return err
}

// Today implements contracts.Habits.
func (m *Module) Today(ctx context.Context) ([]contracts.HabitProgress, error) {
	list, err := m.today(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	out := make([]contracts.HabitProgress, 0, len(list))
	for _, p := range list {
		out = append(out, contracts.HabitProgress{ID: p.Habit.Id, Name: p.Habit.Name, Unit: p.Habit.Unit,
			Target: p.Habit.DailyTarget, Done: p.Done, Streak: p.Streak})
	}
	return out, nil
}
