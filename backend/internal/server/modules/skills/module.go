// Package skills keeps the practice log of skills the user wants to improve
// (B138): date, minutes, what was practiced and where it got stuck, with the
// minutes per week. The minutes can be a key result of a goal (B128).
// See docs/specs/B138.md.
package skills

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/skills/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/skills/db"
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "skills.module"

const (
	maxSkills        = 100
	maxSessionsEach  = 5000
	maxName          = 50
	maxText          = 1000
	maxBackdateYears = 1
	defaultLimit     = 100
	maxLimit         = 500
)

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time
}

var _ api.ServerInterface = (*Module)(nil)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	module.Provide[contracts.GoalMetricSource](d.Registry, contracts.GoalMetricPrefix+"skills", goalMetrics{m})
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "skills" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

func (m *Module) now() time.Time { return m.nowFn() }

func (m *Module) loc() *time.Location {
	if l := m.d.Scheduler.Location(); l != nil {
		return l
	}
	return time.Local
}

func (m *Module) today() string { return m.now().In(m.loc()).Format(dateLayout) }

func fail(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	httpx.Fail(w, r, err)
	return true
}

func ptr[T any](v T) *T { return &v }

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func toSession(r db.SkillSession) api.SkillSession {
	return api.SkillSession{Id: r.ID, SkillId: r.SkillID, PracticedOn: r.PracticedOn, Minutes: int(r.Minutes),
		Content: r.Content, Stuck: r.Stuck, CreatedAt: r.CreatedAt}
}

// view builds a skill with its numbers.
func toSkill(r db.Skill, totals map[int64]db.SkillTotalsRow, weeks map[int64][]int) api.Skill {
	s := api.Skill{Id: r.ID, Name: r.Name, Archived: r.Archived == 1, CreatedAt: r.CreatedAt, Weeks: weeks[r.ID]}
	if s.Weeks == nil {
		s.Weeks = make([]int, weekCount)
	}
	s.WeekMinutes = s.Weeks[len(s.Weeks)-1]
	if r.WeeklyTarget != nil {
		s.WeeklyTarget = ptr(int(*r.WeeklyTarget))
	}
	if t, ok := totals[r.ID]; ok {
		s.TotalMinutes = int(t.Total)
		if last, ok := t.LastOn.(string); ok && last != "" {
			s.LastPracticedOn = &last
		}
	}
	return s
}

func (m *Module) list(ctx context.Context, withArchived bool) ([]api.Skill, error) {
	flag := 0
	if withArchived {
		flag = 1
	}
	rows, err := m.q.ListSkills(ctx, flag)
	if err != nil {
		return nil, err
	}
	starts := weekStarts(m.now().In(m.loc()))
	daily, err := m.q.SkillDailyMinutes(ctx, starts[0])
	if err != nil {
		return nil, err
	}
	perSkill := map[int64]map[string]int{}
	for _, d := range daily {
		if perSkill[d.SkillID] == nil {
			perSkill[d.SkillID] = map[string]int{}
		}
		perSkill[d.SkillID][d.PracticedOn] = int(d.Minutes)
	}
	weeks := map[int64][]int{}
	for id, byDay := range perSkill {
		weeks[id] = bucketWeeks(starts, byDay)
	}
	totalRows, err := m.q.SkillTotals(ctx)
	if err != nil {
		return nil, err
	}
	totals := map[int64]db.SkillTotalsRow{}
	for _, t := range totalRows {
		totals[t.SkillID] = t
	}
	out := make([]api.Skill, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSkill(r, totals, weeks))
	}
	return out, nil
}

func (m *Module) one(ctx context.Context, id int64) (api.Skill, error) {
	items, err := m.list(ctx, true)
	if err != nil {
		return api.Skill{}, err
	}
	for _, s := range items {
		if s.Id == id {
			return s, nil
		}
	}
	return api.Skill{}, httpx.ErrNotFound
}

func (m *Module) get(ctx context.Context, id int64) (db.Skill, error) {
	row, err := m.q.GetSkill(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, httpx.ErrNotFound
	}
	return row, err
}

func checkName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > maxName {
		return "", httpx.Invalid("名称要写 1 到 50 个字")
	}
	return s, nil
}

func checkTarget(n int) (*int64, error) {
	if n == 0 {
		return nil, nil
	}
	if n < 1 || n > 10080 {
		return nil, httpx.Invalid("每周目标要在 1 到 10080 分钟之间")
	}
	return ptr(int64(n)), nil
}

func checkText(s, what string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxText {
		return "", httpx.Invalid(what + "最多 1000 个字")
	}
	return s, nil
}

func checkMinutes(n int) error {
	if n < 1 || n > 1440 {
		return httpx.Invalid("分钟数要在 1 到 1440 之间")
	}
	return nil
}

// checkDay keeps the practice date between a year ago and today.
func (m *Module) checkDay(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return m.today(), nil
	}
	d, err := time.ParseInLocation(dateLayout, s, m.loc())
	if err != nil {
		return "", httpx.Invalid("日期要写成 YYYY-MM-DD，比如 2026-11-11")
	}
	now := m.now().In(m.loc())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, m.loc())
	if d.After(today) {
		return "", httpx.Invalid("练习日期不能晚于今天")
	}
	if d.Before(today.AddDate(-maxBackdateYears, 0, 0)) {
		return "", httpx.Invalid("练习日期最早是一年前")
	}
	return d.Format(dateLayout), nil
}

// nameTaken says whether another skill already uses name.
func (m *Module) nameTaken(ctx context.Context, name string, except int64) (bool, error) {
	row, err := m.q.GetSkillByName(ctx, name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return row.ID != except, nil
}

func (m *Module) create(ctx context.Context, in api.SkillInput) (api.Skill, error) {
	name, err := checkName(in.Name)
	if err != nil {
		return api.Skill{}, err
	}
	target, err := checkTarget(deref(in.WeeklyTarget))
	if err != nil {
		return api.Skill{}, err
	}
	if taken, err := m.nameTaken(ctx, name, 0); err != nil {
		return api.Skill{}, err
	} else if taken {
		return api.Skill{}, httpx.Invalid("已经有同名的技能")
	}
	n, err := m.q.CountSkills(ctx)
	if err != nil {
		return api.Skill{}, err
	}
	if n >= maxSkills {
		return api.Skill{}, httpx.Invalid("技能太多了，最多 100 项")
	}
	row, err := m.q.InsertSkill(ctx, db.InsertSkillParams{Name: name, WeeklyTarget: target, CreatedAt: m.now().UTC()})
	if err != nil {
		return api.Skill{}, err
	}
	return m.one(ctx, row.ID)
}

func (m *Module) logSession(ctx context.Context, skillID int64, in api.SkillSessionInput) (api.SkillSession, error) {
	sk, err := m.get(ctx, skillID)
	if err != nil {
		return api.SkillSession{}, err
	}
	if sk.Archived == 1 {
		return api.SkillSession{}, httpx.Invalid("这项技能已经归档，先取消归档再记")
	}
	if err := checkMinutes(in.Minutes); err != nil {
		return api.SkillSession{}, err
	}
	day, err := m.checkDay(deref(in.PracticedOn))
	if err != nil {
		return api.SkillSession{}, err
	}
	content, err := checkText(deref(in.Content), "练了什么")
	if err != nil {
		return api.SkillSession{}, err
	}
	stuck, err := checkText(deref(in.Stuck), "卡住的地方")
	if err != nil {
		return api.SkillSession{}, err
	}
	n, err := m.q.CountSkillSessions(ctx, skillID)
	if err != nil {
		return api.SkillSession{}, err
	}
	if n >= maxSessionsEach {
		return api.SkillSession{}, httpx.Invalid("这项技能的记录太多了，最多 5000 条")
	}
	row, err := m.q.InsertSkillSession(ctx, db.InsertSkillSessionParams{SkillID: skillID, PracticedOn: day,
		Minutes: int64(in.Minutes), Content: content, Stuck: stuck, CreatedAt: m.now().UTC()})
	if err != nil {
		return api.SkillSession{}, err
	}
	return toSession(row), nil
}

func (m *Module) ListSkills(w http.ResponseWriter, r *http.Request, params api.ListSkillsParams) {
	items, err := m.list(r.Context(), deref(params.Archived))
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "weekStart": monday(m.now().In(m.loc())).Format(dateLayout), "today": m.today()})
}

func (m *Module) CreateSkill(w http.ResponseWriter, r *http.Request) {
	var body api.SkillInput
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	s, err := m.create(r.Context(), body)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusCreated, s)
}

func (m *Module) UpdateSkill(w http.ResponseWriter, r *http.Request, skillId api.SkillId) {
	ctx := r.Context()
	var body api.SkillPatch
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	row, err := m.get(ctx, skillId)
	if fail(w, r, err) {
		return
	}
	p := db.UpdateSkillParams{ID: row.ID, Name: row.Name, WeeklyTarget: row.WeeklyTarget, Archived: row.Archived}
	if body.Name != nil {
		if p.Name, err = checkName(*body.Name); fail(w, r, err) {
			return
		}
		taken, err := m.nameTaken(ctx, p.Name, row.ID)
		if fail(w, r, err) {
			return
		}
		if taken {
			httpx.Fail(w, r, httpx.Invalid("已经有同名的技能"))
			return
		}
	}
	if body.WeeklyTarget != nil {
		if p.WeeklyTarget, err = checkTarget(*body.WeeklyTarget); fail(w, r, err) {
			return
		}
	}
	if body.Archived != nil {
		p.Archived = 0
		if *body.Archived {
			p.Archived = 1
		}
	}
	if _, err := m.q.UpdateSkill(ctx, p); fail(w, r, err) {
		return
	}
	s, err := m.one(ctx, row.ID)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

func (m *Module) DeleteSkill(w http.ResponseWriter, r *http.Request, skillId api.SkillId) {
	n, err := m.q.DeleteSkill(r.Context(), skillId)
	m.d.Audit.Record(r.Context(), "skills.delete", itoa(skillId), nil, err)
	if fail(w, r, err) {
		return
	}
	if n == 0 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) ListSkillSessions(w http.ResponseWriter, r *http.Request, skillId api.SkillId, params api.ListSkillSessionsParams) {
	ctx := r.Context()
	if _, err := m.get(ctx, skillId); fail(w, r, err) {
		return
	}
	limit := int64(defaultLimit)
	if params.Limit != nil {
		limit = int64(*params.Limit)
		if limit < 1 || limit > maxLimit {
			httpx.Fail(w, r, httpx.Invalid("limit 要在 1 到 500 之间"))
			return
		}
	}
	var rows []db.SkillSession
	var err error
	if deref(params.Stuck) {
		rows, err = m.q.ListStuckSkillSessions(ctx, db.ListStuckSkillSessionsParams{SkillID: skillId, Limit: limit})
	} else {
		rows, err = m.q.ListSkillSessions(ctx, db.ListSkillSessionsParams{SkillID: skillId, Limit: limit})
	}
	if fail(w, r, err) {
		return
	}
	items := make([]api.SkillSession, 0, len(rows))
	for _, row := range rows {
		items = append(items, toSession(row))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (m *Module) CreateSkillSession(w http.ResponseWriter, r *http.Request, skillId api.SkillId) {
	var body api.SkillSessionInput
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	s, err := m.logSession(r.Context(), skillId, body)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusCreated, s)
}

func (m *Module) UpdateSkillSession(w http.ResponseWriter, r *http.Request, sessionId int64) {
	ctx := r.Context()
	var body api.SkillSessionPatch
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	row, err := m.q.GetSkillSession(ctx, sessionId)
	if errors.Is(err, sql.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	if fail(w, r, err) {
		return
	}
	p := db.UpdateSkillSessionParams{ID: row.ID, PracticedOn: row.PracticedOn, Minutes: row.Minutes, Content: row.Content, Stuck: row.Stuck}
	if body.PracticedOn != nil {
		if strings.TrimSpace(*body.PracticedOn) == "" {
			httpx.Fail(w, r, httpx.Invalid("日期不能为空"))
			return
		}
		if p.PracticedOn, err = m.checkDay(*body.PracticedOn); fail(w, r, err) {
			return
		}
	}
	if body.Minutes != nil {
		if err := checkMinutes(*body.Minutes); fail(w, r, err) {
			return
		}
		p.Minutes = int64(*body.Minutes)
	}
	if body.Content != nil {
		if p.Content, err = checkText(*body.Content, "练了什么"); fail(w, r, err) {
			return
		}
	}
	if body.Stuck != nil {
		if p.Stuck, err = checkText(*body.Stuck, "卡住的地方"); fail(w, r, err) {
			return
		}
	}
	row, err = m.q.UpdateSkillSession(ctx, p)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, toSession(row))
}

func (m *Module) DeleteSkillSession(w http.ResponseWriter, r *http.Request, sessionId int64) {
	n, err := m.q.DeleteSkillSession(r.Context(), sessionId)
	if fail(w, r, err) {
		return
	}
	if n == 0 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	httpx.NoContent(w)
}
