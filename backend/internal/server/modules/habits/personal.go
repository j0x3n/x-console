package habits

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/db"
)

//go:embed catalog/plan.json
var personalCatalogJSON []byte

var personalLibrary = func() api.PersonalLibrary {
	var out api.PersonalLibrary
	if err := json.Unmarshal(personalCatalogJSON, &out); err != nil {
		panic(err)
	}
	return out
}()

const personalProfileKey = "habits.personal_profile"
const personalDayPrefix = "habits.personal_day."

// Identities prevent deleted SQLite row IDs from binding to newly created rows.
type personalProfileState struct {
	api.PersonalProfile
	HabitCreatedAt map[string]time.Time `json:"habitCreatedAt"`
}
type personalDayState struct {
	api.PersonalDay
	HabitLogIDs      map[string]int64 `json:"habitLogIds"`
	WorkoutCreatedAt time.Time        `json:"workoutCreatedAt"`
}
type personalDB interface {
	db.DBTX
}

func personalRead(ctx context.Context, conn personalDB, key string, out any) error {
	var raw string
	err := conn.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ? AND encrypted = 0", key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), out)
}
func personalPut(ctx context.Context, conn personalDB, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO settings (key,value,encrypted,updated_at) VALUES (?,?,0,?)
 ON CONFLICT(key) DO UPDATE SET value=excluded.value, encrypted=0, updated_at=excluded.updated_at`, key, string(raw), time.Now().UTC())
	return err
}
func personalDate(date string) error {
	if len(date) != 10 {
		return httpx.Invalid("日期要写成 2026-10-03 这样")
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return httpx.Invalid("日期无效")
	}
	return nil
}
func personalProfileInput(p api.PersonalProfile) api.PersonalProfileInput {
	return api.PersonalProfileInput{Start: p.Start, Wake: p.Wake, Sleep: p.Sleep, Phase: p.Phase, Baseline: p.Baseline, StepGoal: p.StepGoal, RunLevel: p.RunLevel}
}
func personalSetProfile(p *personalProfileState, in api.PersonalProfileInput) error {
	if err := personalDate(in.Start); err != nil {
		return err
	}
	for _, v := range []string{in.Wake, in.Sleep} {
		if _, ok := parseClock(v); !ok || v == "24:00" {
			return httpx.Invalid("作息时间要写成 11:00 这样")
		}
	}
	if in.Phase < 1 || in.Phase > 3 || in.RunLevel < 0 || in.RunLevel > 8 || in.StepGoal < 500 || in.StepGoal > 30000 || math.IsNaN(float64(in.Baseline)) || in.Baseline < 30 || in.Baseline > 250 {
		return httpx.Invalid("个人设置的数值超出范围")
	}
	p.Start, p.Wake, p.Sleep = in.Start, in.Wake, in.Sleep
	p.Phase, p.Baseline, p.StepGoal, p.RunLevel = in.Phase, in.Baseline, in.StepGoal, in.RunLevel
	return nil
}
func (m *Module) personalProfile(ctx context.Context, conn personalDB) (personalProfileState, error) {
	p := personalProfileState{PersonalProfile: api.PersonalProfile{Start: dateKey(time.Now(), m.d.Config.Location), Wake: "11:00", Sleep: "03:00", Phase: 1, Baseline: 82, StepGoal: 6000, RunLevel: 0, HabitIds: map[string]int64{}}, HabitCreatedAt: map[string]time.Time{}}
	if err := personalRead(ctx, conn, personalProfileKey, &p); err != nil {
		return p, err
	}
	if p.HabitIds == nil {
		p.HabitIds = map[string]int64{}
	}
	if p.HabitCreatedAt == nil {
		p.HabitCreatedAt = map[string]time.Time{}
	}
	q := db.New(conn)
	for key, id := range p.HabitIds {
		h, err := q.GetHabit(ctx, id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return p, err
		}
		if errors.Is(err, sql.ErrNoRows) || !h.CreatedAt.Equal(p.HabitCreatedAt[key]) {
			delete(p.HabitIds, key)
			delete(p.HabitCreatedAt, key)
		}
	}
	return p, nil
}
func personalHabit(id string) (api.LibraryHabit, error) {
	for _, h := range personalLibrary.Habits {
		if h.Id == id {
			return h, nil
		}
	}
	return api.LibraryHabit{}, httpx.Invalid("个人计划中没有这个习惯")
}
func personalExercise(id string) bool {
	for _, e := range personalLibrary.Exercises {
		if e.Id == id {
			return true
		}
	}
	return false
}
func personalSetCount(key string) (int, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 3 {
		return 0, httpx.Invalid("训练组记录格式不正确")
	}
	i, e1 := strconv.Atoi(parts[1])
	n, e2 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || i < 0 || n < 0 {
		return 0, httpx.Invalid("训练组记录格式不正确")
	}
	var counts []int
	for _, s := range personalLibrary.Sessions {
		if s.Id == parts[0] {
			for _, item := range s.Items {
				counts = append(counts, item.Sets)
			}
			break
		}
	}
	if parts[0] == "rest" || parts[0] == "cardio" {
		counts = []int{1, 1, 1, 1}
	}
	if i >= len(counts) || n >= counts[i] {
		return 0, httpx.Invalid("训练组记录超出计划范围")
	}
	return counts[i], nil
}
func personalApplyDay(d *personalDayState, in api.PersonalDayInput) error {
	for _, field := range []struct {
		dst      *string
		src      *string
		min, max float64
		whole    bool
	}{{&d.Weight, in.Weight, 30, 250, false}, {&d.Waist, in.Waist, 40, 200, false}, {&d.Sleep, in.Sleep, 0, 24, false}, {&d.Steps, in.Steps, 0, 100000, true}, {&d.RestingHr, in.RestingHr, 20, 220, true}} {
		if field.src == nil {
			continue
		}
		v := strings.TrimSpace(*field.src)
		if v != "" {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < field.min || n > field.max || (field.whole && n != math.Trunc(n)) {
				return httpx.Invalid("身体记录的数值超出范围")
			}
		}
		*field.dst = v
	}
	for _, field := range []struct {
		dst   *string
		src   *string
		limit int
	}{{&d.Energy, in.Energy, 100}, {&d.Back, in.Back, 100}, {&d.Note, in.Note, 20000}, {&d.English, in.English, 20000}, {&d.Food, in.Food, 20000}} {
		if field.src != nil {
			if len([]rune(*field.src)) > field.limit {
				return httpx.Invalid("记录内容太长了")
			}
			*field.dst = strings.TrimSpace(*field.src)
		}
	}
	if in.Sets != nil {
		if len(*in.Sets) > 1000 {
			return httpx.Invalid("训练组记录太多了")
		}
		for key, done := range *in.Sets {
			if _, err := personalSetCount(key); err != nil {
				return err
			}
			d.Sets[key] = done
		}
	}
	return nil
}
func personalEmptyDay(date string) personalDayState {
	return personalDayState{PersonalDay: api.PersonalDay{Date: date, Sets: map[string]bool{}, Checks: map[string]bool{}}, HabitLogIDs: map[string]int64{}}
}
func personalMarker(date, id string) string { return "个人计划:" + date + ":" + id }
func (m *Module) personalDay(ctx context.Context, conn personalDB, date string) (personalDayState, error) {
	d := personalEmptyDay(date)
	if err := personalDate(date); err != nil {
		return d, err
	}
	if err := personalRead(ctx, conn, personalDayPrefix+date, &d); err != nil {
		return d, err
	}
	if d.Sets == nil {
		d.Sets = map[string]bool{}
	}
	if d.Checks == nil {
		d.Checks = map[string]bool{}
	}
	for id := range d.Checks {
		d.Checks[id] = false
	}
	if d.HabitLogIDs == nil {
		d.HabitLogIDs = map[string]int64{}
	}
	q := db.New(conn)
	for id, logID := range d.HabitLogIDs {
		log, err := q.GetHabitLog(ctx, logID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return d, err
		}
		if errors.Is(err, sql.ErrNoRows) || log.Note != personalMarker(date, id) {
			delete(d.HabitLogIDs, id)
			d.Checks[id] = false
		} else {
			d.Checks[id] = true
		}
	}
	if d.WorkoutLogId != nil {
		var created time.Time
		var actualDate string
		var duration int
		err := conn.QueryRowContext(ctx, "SELECT created_at,date,duration_minutes FROM workout_logs WHERE id = ?", *d.WorkoutLogId).Scan(&created, &actualDate, &duration)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return d, err
		}
		if errors.Is(err, sql.ErrNoRows) || actualDate != date || !created.Equal(d.WorkoutCreatedAt) {
			d.WorkoutLogId = nil
			d.WorkoutCreatedAt = time.Time{}
			d.WorkoutDurationMinutes = nil
		} else {
			d.WorkoutDurationMinutes = &duration
		}
	}
	return d, nil
}
func (m *Module) personalTx(ctx context.Context, action string, fn func(*sql.Tx, *personalProfileState) error) (api.PersonalProfile, error) {
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return api.PersonalProfile{}, err
	}
	defer tx.Rollback()
	p, err := m.personalProfile(ctx, tx)
	if err != nil {
		return p.PersonalProfile, err
	}
	if err = fn(tx, &p); err == nil {
		err = personalPut(ctx, tx, personalProfileKey, p)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		_ = tx.Rollback()
	}
	m.d.Audit.Record(ctx, action, "", nil, err)
	if err == nil {
		m.d.Bus.Publish("habit.personal_updated", map[string]string{"action": action})
	}
	return p.PersonalProfile, err
}
func (m *Module) personalActivate(ctx context.Context, tx *sql.Tx, p *personalProfileState, ids []string) error {
	if len(ids) > 50 {
		return httpx.Invalid("一次最多加入 50 个习惯")
	}
	q := db.New(tx)
	for _, id := range ids {
		template, err := personalHabit(id)
		if err != nil {
			return err
		}
		if _, ok := p.HabitIds[id]; ok {
			continue
		}
		all, err := q.ListHabits(ctx, 1)
		if err != nil {
			return err
		}
		var existing *db.Habit
		for _, h := range all {
			if h.Name == template.Name && h.Kind == "count" {
				existing = &h
				break
			}
		}
		if existing == nil {
			order, err := q.MaxHabitSortOrder(ctx)
			if err != nil {
				return err
			}
			icon := "🧘"
			if template.Category == api.LibraryHabitCategoryFood {
				icon = "🥗"
			}
			if template.Category == api.LibraryHabitCategoryEnglish {
				icon = "📖"
			}
			row, err := q.CreateHabit(ctx, db.CreateHabitParams{Name: template.Name, Icon: icon, Color: "accent", Unit: "次", DailyTarget: 1, RemindMode: "none", RemindTimes: "[]", SortOrder: order + 1, CreatedAt: time.Now().UTC(), Kind: "count", RemindWhen: "[]", ActiveHostIds: "[]"})
			if err != nil {
				return err
			}
			existing = &row
		}
		p.HabitIds[id] = existing.ID
		p.HabitCreatedAt[id] = existing.CreatedAt
	}
	return nil
}
func (m *Module) personalCheck(ctx context.Context, tx *sql.Tx, p *personalProfileState, d *personalDayState, id string, done bool, loc *time.Location) error {
	if _, err := personalHabit(id); err != nil {
		return err
	}
	if !done {
		if logID, ok := d.HabitLogIDs[id]; ok {
			if _, err := db.New(tx).DeleteHabitLog(ctx, logID); err != nil {
				return err
			}
			delete(d.HabitLogIDs, id)
		}
		d.Checks[id] = false
		return nil
	}
	habitID, ok := p.HabitIds[id]
	if !ok {
		return httpx.Invalid("请先把这个项目加入习惯")
	}
	h, err := db.New(tx).GetHabit(ctx, habitID)
	if err != nil {
		return notFound(err)
	}
	if h.ArchivedAt != nil {
		return httpx.NewError(409, "habit_archived", "这个习惯已归档，请先恢复")
	}
	if d.Checks[id] {
		return nil
	}
	day, _ := time.Parse(time.DateOnly, d.Date)
	at := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc).UTC()
	log, err := db.New(tx).CreateHabitLog(ctx, db.CreateHabitLogParams{HabitID: h.ID, At: at, Amount: 1, Source: "web", Note: personalMarker(d.Date, id)})
	if err != nil {
		return err
	}
	d.HabitLogIDs[id] = log.ID
	d.Checks[id] = true
	return nil
}
func personalReply(w http.ResponseWriter, r *http.Request, value any, err error) {
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, value)
}

// Publish after commit so automations see the native record and rolled-back writes stay silent.
func (m *Module) personalPublishCheckin(ctx context.Context, logID int64) {
	log, err := m.q.GetHabitLog(ctx, logID)
	if err != nil {
		return
	}
	h, err := m.get(ctx, log.HabitID)
	if err != nil {
		return
	}
	p, err := m.progressOf(ctx, h, log.At)
	if err != nil {
		return
	}
	m.d.Bus.Publish("habit.checked_in", map[string]any{"habitId": h.ID, "name": h.Name, "log": logToAPI(log), "done": p.Done, "target": h.DailyTarget, "source": log.Source})
	rules, err := m.loadSchedule(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	if dateKey(log.At, rules.location()) != dateKey(now, rules.location()) {
		return
	}
	m.clock.reset(h.ID, now)
	_ = m.q.SetHabitSnoozedUntil(ctx, db.SetHabitSnoozedUntilParams{ID: h.ID})
	if p.Reached && p.Done-log.Amount < h.DailyTarget {
		m.d.Bus.Publish("habit.goal_reached", map[string]any{"habitId": h.ID, "name": h.Name, "done": p.Done, "target": h.DailyTarget, "streak": p.Streak})
	}
}
func (m *Module) GetPersonalLibrary(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, personalLibrary)
}
func (m *Module) GetPersonalProfile(w http.ResponseWriter, r *http.Request) {
	p, err := m.personalProfile(r.Context(), m.d.DB)
	personalReply(w, r, p.PersonalProfile, err)
}
func (m *Module) UpdatePersonalProfile(w http.ResponseWriter, r *http.Request) {
	var in api.PersonalProfileInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := m.personalTx(r.Context(), "habit.personal_profile.update", func(tx *sql.Tx, p *personalProfileState) error { return personalSetProfile(p, in) })
	personalReply(w, r, p, err)
}
func (m *Module) ActivatePersonalHabits(w http.ResponseWriter, r *http.Request) {
	var in api.ActivatePersonalHabitsJSONBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := m.personalTx(r.Context(), "habit.personal.activate", func(tx *sql.Tx, p *personalProfileState) error { return m.personalActivate(r.Context(), tx, p, in.Ids) })
	personalReply(w, r, p, err)
}
func (m *Module) GetPersonalDay(w http.ResponseWriter, r *http.Request, date string) {
	d, err := m.personalDay(r.Context(), m.d.DB, date)
	personalReply(w, r, d.PersonalDay, err)
}
func (m *Module) UpdatePersonalDay(w http.ResponseWriter, r *http.Request, date string) {
	var in api.PersonalDayInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var day personalDayState
	_, err := m.personalTx(r.Context(), "habit.personal_day.update", func(tx *sql.Tx, _ *personalProfileState) error {
		var err error
		day, err = m.personalDay(r.Context(), tx, date)
		if err != nil {
			return err
		}
		if err = personalApplyDay(&day, in); err != nil {
			return err
		}
		return personalPut(r.Context(), tx, personalDayPrefix+date, day)
	})
	personalReply(w, r, day.PersonalDay, err)
}
func (m *Module) CheckPersonalHabit(w http.ResponseWriter, r *http.Request, date string) {
	var in api.CheckPersonalHabitJSONBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rules, err := m.loadSchedule(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var day personalDayState
	wasDone := false
	_, err = m.personalTx(r.Context(), "habit.personal.check", func(tx *sql.Tx, p *personalProfileState) error {
		var err error
		day, err = m.personalDay(r.Context(), tx, date)
		if err != nil {
			return err
		}
		wasDone = day.Checks[in.Id]
		if err = m.personalCheck(r.Context(), tx, p, &day, in.Id, in.Done, rules.location()); err != nil {
			return err
		}
		return personalPut(r.Context(), tx, personalDayPrefix+date, day)
	})
	if err == nil && in.Done && !wasDone {
		m.personalPublishCheckin(r.Context(), day.HabitLogIDs[in.Id])
	}
	personalReply(w, r, day.PersonalDay, err)
}
func (m *Module) personalDays(ctx context.Context, since string) ([]personalDayState, error) {
	rows, err := m.d.DB.QueryContext(ctx, "SELECT key FROM settings WHERE key >= ? AND key < ? ORDER BY key DESC", personalDayPrefix+since, personalDayPrefix+"9999-99-99")
	if err != nil {
		return nil, err
	}
	var dates []string
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return nil, err
		}
		dates = append(dates, strings.TrimPrefix(key, personalDayPrefix))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	days := make([]personalDayState, 0, len(dates))
	for _, date := range dates {
		d, err := m.personalDay(ctx, m.d.DB, date)
		if err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	return days, nil
}
func (m *Module) ListPersonalDays(w http.ResponseWriter, r *http.Request, params api.ListPersonalDaysParams) {
	days := 30
	if params.Days != nil {
		days = *params.Days
	}
	if days < 1 || days > 366 {
		httpx.Fail(w, r, httpx.Invalid("天数要在 1 到 366 之间"))
		return
	}
	rules, err := m.loadSchedule(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	since := dateKey(time.Now().In(rules.location()).AddDate(0, 0, -(days-1)), rules.location())
	rows, err := m.personalDays(r.Context(), since)
	out := make([]api.PersonalDay, 0, len(rows))
	today := dateKey(time.Now(), rules.location())
	for _, d := range rows {
		if d.Date <= today {
			out = append(out, d.PersonalDay)
		}
	}
	personalReply(w, r, out, err)
}
func personalDayInput(d api.PersonalDay) api.PersonalDayInput {
	return api.PersonalDayInput{Weight: &d.Weight, Waist: &d.Waist, Sleep: &d.Sleep, Steps: &d.Steps, RestingHr: &d.RestingHr, Energy: &d.Energy, Back: &d.Back, Note: &d.Note, English: &d.English, Food: &d.Food}
}
func (m *Module) personalBackup(ctx context.Context) (api.PersonalBackup, error) {
	p, err := m.personalProfile(ctx, m.d.DB)
	if err != nil {
		return api.PersonalBackup{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	ids := make([]string, 0, len(p.HabitIds))
	for id := range p.HabitIds {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := api.PersonalBackup{Version: 1, ExportedAt: &now, Profile: personalProfileInput(p.PersonalProfile), SelectedHabits: &ids, Checks: map[string]bool{}, Sets: map[string]bool{}, Logs: map[string]api.PersonalDayInput{}}
	days, err := m.personalDays(ctx, "0000-01-01")
	if err != nil {
		return out, err
	}
	for _, d := range days {
		out.Logs[d.Date] = personalDayInput(d.PersonalDay)
		for id, done := range d.Checks {
			out.Checks[d.Date+":"+id] = done
		}
		for key, done := range d.Sets {
			out.Sets[d.Date+":"+key] = done
		}
	}
	return out, nil
}
func (m *Module) ExportPersonalBackup(w http.ResponseWriter, r *http.Request) {
	out, err := m.personalBackup(r.Context())
	personalReply(w, r, out, err)
}
func personalLegacyDateKey(key string) (string, string, error) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", "", httpx.Invalid("备份记录格式不正确")
	}
	if err := personalDate(parts[0]); err != nil {
		return "", "", err
	}
	return parts[0], parts[1], nil
}
func (m *Module) personalImport(ctx context.Context, in api.PersonalBackup, loc *time.Location) (api.PersonalProfile, error) {
	if in.Version != 1 || in.Checks == nil || in.Sets == nil || in.Logs == nil || len(in.Logs) > 5000 {
		return api.PersonalProfile{}, httpx.Invalid("请选择个人计划导出的版本 1 JSON 备份")
	}
	// Validate everything before opening the transaction; no partial import on malformed input.
	profiles := personalProfileState{}
	if err := personalSetProfile(&profiles, in.Profile); err != nil {
		return api.PersonalProfile{}, err
	}
	dayInputs := map[string]api.PersonalDayInput{}
	checks := map[string]map[string]bool{}
	selected := map[string]bool{}
	if in.SelectedHabits != nil {
		for _, id := range *in.SelectedHabits {
			if _, err := personalHabit(id); err != nil {
				return api.PersonalProfile{}, err
			}
			selected[id] = true
		}
	}
	getInput := func(date string) api.PersonalDayInput {
		input := dayInputs[date]
		if input.Sets == nil {
			empty := map[string]bool{}
			input.Sets = &empty
		}
		return input
	}
	for date, input := range in.Logs {
		if err := personalDate(date); err != nil {
			return api.PersonalProfile{}, err
		}
		d := personalEmptyDay(date)
		if err := personalApplyDay(&d, input); err != nil {
			return api.PersonalProfile{}, err
		}
		dayInputs[date] = input
	}
	for key, done := range in.Checks {
		date, id, err := personalLegacyDateKey(key)
		if err != nil {
			return api.PersonalProfile{}, err
		}
		if strings.HasPrefix(id, "exercise-") {
			parts := strings.Split(strings.TrimPrefix(id, "exercise-"), "-")
			if len(parts) != 2 {
				return api.PersonalProfile{}, httpx.Invalid("备份训练格式不正确")
			}
			count, err := personalSetCount(parts[0] + ":" + parts[1] + ":0")
			if err != nil {
				return api.PersonalProfile{}, err
			}
			input := getInput(date)
			for i := 0; i < count; i++ {
				(*input.Sets)[fmt.Sprintf("%s:%s:%d", parts[0], parts[1], i)] = done
			}
			dayInputs[date] = input
		} else {
			if _, err := personalHabit(id); err != nil {
				return api.PersonalProfile{}, err
			}
			if checks[date] == nil {
				checks[date] = map[string]bool{}
			}
			checks[date][id] = done
			if done {
				selected[id] = true
			}
			dayInputs[date] = getInput(date)
		}
	}
	for key, done := range in.Sets {
		date, set, err := personalLegacyDateKey(key)
		if err != nil {
			return api.PersonalProfile{}, err
		}
		if _, err := personalSetCount(set); err != nil {
			return api.PersonalProfile{}, err
		}
		input := getInput(date)
		(*input.Sets)[set] = done
		dayInputs[date] = input
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return m.personalTx(ctx, "habit.personal.import", func(tx *sql.Tx, p *personalProfileState) error {
		if err := personalSetProfile(p, in.Profile); err != nil {
			return err
		}
		if err := m.personalActivate(ctx, tx, p, ids); err != nil {
			return err
		}
		for date, input := range dayInputs {
			day, err := m.personalDay(ctx, tx, date)
			if err != nil {
				return err
			}
			if err = personalApplyDay(&day, input); err != nil {
				return err
			}
			for id, done := range checks[date] {
				if err = m.personalCheck(ctx, tx, p, &day, id, done, loc); err != nil {
					return err
				}
			}
			if err = personalPut(ctx, tx, personalDayPrefix+date, day); err != nil {
				return err
			}
		}
		return nil
	})
}
func (m *Module) ImportPersonalBackup(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid("读取备份失败"))
		return
	}
	if len(raw) > 2<<20 {
		httpx.Fail(w, r, httpx.Invalid("备份不能超过 2 MB"))
		return
	}
	var in api.PersonalBackup
	if err = json.Unmarshal(raw, &in); err != nil {
		httpx.Fail(w, r, httpx.Invalid("备份 JSON 格式不正确"))
		return
	}
	rules, err := m.loadSchedule(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := m.personalImport(r.Context(), in, rules.location())
	personalReply(w, r, p, err)
}
func (m *Module) personalWorkout(ctx context.Context, date string, in api.LogPersonalWorkoutJSONBody, loc *time.Location) (api.PersonalDay, error) {
	if err := personalDate(date); err != nil {
		return api.PersonalDay{}, err
	}
	items, err := cleanItems(in.Items)
	if err != nil {
		return api.PersonalDay{}, err
	}
	if len(items) == 0 || len(items) > 200 || in.DurationMinutes < 0 || in.DurationMinutes > 1440 || len([]rune(in.Note)) > 20000 {
		return api.PersonalDay{}, httpx.Invalid("请填写完成的动作和有效训练时长")
	}
	var day personalDayState
	var row db.WorkoutLog
	var newCheckins []db.HabitLog
	_, err = m.personalTx(ctx, "habit.personal.workout", func(tx *sql.Tx, _ *personalProfileState) error {
		var err error
		day, err = m.personalDay(ctx, tx, date)
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(items)
		q := db.New(tx)
		if day.WorkoutLogId != nil {
			_, err = tx.ExecContext(ctx, "UPDATE workout_logs SET items=?,duration_minutes=?,note=? WHERE id=?", string(raw), in.DurationMinutes, strings.TrimSpace(in.Note), *day.WorkoutLogId)
		} else {
			row, err = q.CreateWorkoutLog(ctx, db.CreateWorkoutLogParams{Date: date, Items: string(raw), DurationMinutes: int64(in.DurationMinutes), Note: strings.TrimSpace(in.Note), CreatedAt: time.Now().UTC()})
			if err != nil {
				return err
			}
			day.WorkoutLogId = &row.ID
			day.WorkoutCreatedAt = row.CreatedAt
			habits, err := q.ListActiveWorkoutHabits(ctx)
			if err != nil {
				return err
			}
			d, _ := time.Parse(time.DateOnly, date)
			at := time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, loc).UTC()
			for _, h := range habits {
				log, err := q.CreateHabitLog(ctx, db.CreateHabitLogParams{HabitID: h.ID, At: at, Amount: 1, Source: "workout", WorkoutLogID: &row.ID})
				if err != nil {
					return err
				}
				newCheckins = append(newCheckins, log)
			}
		}
		if err != nil {
			return err
		}
		day.Note = strings.TrimSpace(in.Note)
		day.WorkoutDurationMinutes = &in.DurationMinutes
		return personalPut(ctx, tx, personalDayPrefix+date, day)
	})
	if err == nil {
		m.d.Bus.Publish("workout.personal_saved", map[string]any{"id": day.WorkoutLogId, "date": date})
		for _, log := range newCheckins {
			m.personalPublishCheckin(ctx, log.ID)
		}
	}
	return day.PersonalDay, err
}
func (m *Module) LogPersonalWorkout(w http.ResponseWriter, r *http.Request, date string) {
	var in api.LogPersonalWorkoutJSONBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rules, err := m.loadSchedule(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.personalWorkout(r.Context(), date, in, rules.location())
	personalReply(w, r, out, err)
}
