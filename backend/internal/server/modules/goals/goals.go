package goals

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/goals/db"
)

// ---------- list and read ----------

// ListGoals implements api.ServerInterface.
func (m *Module) ListGoals(w http.ResponseWriter, r *http.Request, params api.ListGoalsParams) {
	if (params.Year == nil) != (params.Quarter == nil) {
		httpx.Fail(w, r, httpx.Invalid("年和季度要一起给"))
		return
	}
	if params.Status != nil && !params.Status.Valid() {
		httpx.Fail(w, r, httpx.Invalid("不认识的状态"))
		return
	}
	items, err := m.list(r.Context(), params.Year, params.Quarter, params.Status)
	if fail(w, r, err) {
		return
	}
	year, quarter := quarterOf(m.now().In(m.loc()))
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "current": api.GoalPeriod{Year: year, Quarter: quarter}})
}

// list returns goals with their key results and progress. year and quarter
// are both set or both nil.
func (m *Module) list(ctx context.Context, year, quarter *int, status *api.GoalStatus) ([]api.Goal, error) {
	var rows []db.Goal
	var err error
	if year != nil && quarter != nil {
		rows, err = m.q.ListGoalsByPeriod(ctx, db.ListGoalsByPeriodParams{Year: int64(*year), Quarter: int64(*quarter)})
	} else {
		rows, err = m.q.ListGoals(ctx)
	}
	if err != nil {
		return nil, err
	}
	results, err := m.q.ListAllGoalResults(ctx)
	if err != nil {
		return nil, err
	}
	byGoal := map[int64][]db.GoalResult{}
	for _, r := range results {
		byGoal[r.GoalID] = append(byGoal[r.GoalID], r)
	}
	cat, hidden, now := m.catalog(), m.hiddenFn(ctx), m.now()
	out := []api.Goal{}
	for _, g := range rows {
		if status != nil && g.Status != string(*status) {
			continue
		}
		out = append(out, m.goalView(ctx, cat, hidden, g, byGoal[g.ID], now))
	}
	return out, nil
}

// get returns one goal.
func (m *Module) get(ctx context.Context, q *db.Queries, id int64) (api.Goal, error) {
	g, err := q.GetGoal(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return api.Goal{}, httpx.ErrNotFound
	}
	if err != nil {
		return api.Goal{}, err
	}
	rows, err := q.ListGoalResults(ctx, id)
	if err != nil {
		return api.Goal{}, err
	}
	return m.goalView(ctx, m.catalog(), m.hiddenFn(ctx), g, rows, m.now()), nil
}

// GetGoal implements api.ServerInterface.
func (m *Module) GetGoal(w http.ResponseWriter, r *http.Request, goalID api.GoalId) {
	g, err := m.get(r.Context(), m.q, goalID)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, g)
}

// ---------- validation ----------

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) <= 1e12 }

func checkTitle(label, v string, max int, required bool) (string, error) {
	v = strings.TrimSpace(v)
	if required && v == "" {
		return "", httpx.Invalid("请填写" + label)
	}
	if utf8.RuneCountInString(v) > max {
		return "", httpx.Invalid(label + "太长了")
	}
	return v, nil
}

type goalFields struct {
	title, note   string
	year, quarter int
}

func checkGoal(body api.GoalInput) (goalFields, error) {
	title, err := checkTitle("目标名称", body.Title, maxTitle, true)
	if err != nil {
		return goalFields{}, err
	}
	note := ""
	if body.Note != nil {
		if note, err = checkTitle("备注", *body.Note, maxNote, false); err != nil {
			return goalFields{}, err
		}
	}
	if body.Year < 2020 || body.Year > 2100 {
		return goalFields{}, httpx.Invalid("年份要在 2020 到 2100 之间")
	}
	if body.Quarter < 1 || body.Quarter > 4 {
		return goalFields{}, httpx.Invalid("季度要在 1 到 4 之间")
	}
	if body.Results != nil && len(*body.Results) > maxResults {
		return goalFields{}, httpx.Invalid("一个目标最多 20 个关键结果")
	}
	return goalFields{title: title, note: note, year: body.Year, quarter: body.Quarter}, nil
}

// resultFields is a checked key result input.
type resultFields struct {
	id          int64
	name, unit  string
	source, ref string
	target      float64
	start       float64
}

// checkResult validates one key result. existing is the stored row when the
// input edits one: if the source and the choice did not change, the source is
// not checked again, so a goal can still be edited while its source is hidden
// or switched off.
func (m *Module) checkResult(ctx context.Context, cat *catalog, hidden func(string) bool, in api.GoalResultInput, existing *db.GoalResult) (resultFields, error) {
	name, err := checkTitle("关键结果名称", in.Name, maxName, true)
	if err != nil {
		return resultFields{}, err
	}
	unit := ""
	if in.Unit != nil {
		if unit, err = checkTitle("单位", *in.Unit, maxUnit, false); err != nil {
			return resultFields{}, err
		}
	}
	if !in.Source.Valid() {
		return resultFields{}, httpx.Invalid("不认识的数据来源")
	}
	if !finite(in.Target) {
		return resultFields{}, httpx.Invalid("目标值不对")
	}
	out := resultFields{name: name, unit: unit, source: string(in.Source), target: in.Target}
	ref := ""
	if in.Ref != nil {
		ref = strings.TrimSpace(*in.Ref)
	}
	if in.Source == api.Manual {
		if in.StartValue != nil {
			out.start = *in.StartValue
		}
		if !finite(out.start) {
			return resultFields{}, httpx.Invalid("起点不对")
		}
		if out.start == out.target {
			return resultFields{}, httpx.Invalid("目标值和起点不能相同")
		}
		if ref != "" {
			return resultFields{}, httpx.Invalid("手动填的关键结果不需要选项")
		}
		return out, nil
	}
	if out.target <= 0 {
		return resultFields{}, httpx.Invalid("自动统计的目标值要大于 0")
	}
	out.ref = ref
	if existing != nil && existing.Source == out.source && existing.Ref == ref {
		return out, nil
	}
	kind, ok := cat.kinds[out.source]
	if !ok {
		return resultFields{}, httpx.Invalid("这个数据来源现在用不了")
	}
	if hidden(kind.Module) {
		return resultFields{}, httpx.Invalid("这个数据来源现在用不了")
	}
	if kind.RefKind == "none" {
		if ref != "" {
			return resultFields{}, httpx.Invalid("这个数据来源不需要选项")
		}
		return out, nil
	}
	opts, err := cat.sources[out.source].Options(ctx, out.source)
	if err != nil {
		return resultFields{}, err
	}
	for _, o := range opts {
		if o.Value == ref {
			return out, nil
		}
	}
	if kind.RefKind == "habit" {
		return resultFields{}, httpx.Invalid("请选一个习惯")
	}
	return resultFields{}, httpx.Invalid("请选一个分类")
}

// ---------- create and update ----------

// CreateGoal implements api.ServerInterface.
func (m *Module) CreateGoal(w http.ResponseWriter, r *http.Request) {
	var body api.GoalInput
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	g, err := m.create(r.Context(), body)
	m.d.Audit.Record(r.Context(), "goal.create", strconv.FormatInt(g.Id, 10), map[string]any{"title": body.Title}, err)
	if fail(w, r, err) {
		return
	}
	m.d.Bus.Publish("goal.created", g)
	httpx.JSON(w, http.StatusCreated, g)
}

func (m *Module) create(ctx context.Context, body api.GoalInput) (api.Goal, error) {
	f, err := checkGoal(body)
	if err != nil {
		return api.Goal{}, err
	}
	cat, hidden := m.catalog(), m.hiddenFn(ctx)
	var inputs []resultFields
	if body.Results != nil {
		for _, in := range *body.Results {
			rf, err := m.checkResult(ctx, cat, hidden, in, nil)
			if err != nil {
				return api.Goal{}, err
			}
			inputs = append(inputs, rf)
		}
	}
	all, err := m.q.ListGoals(ctx)
	if err != nil {
		return api.Goal{}, err
	}
	if len(all) >= maxGoals {
		return api.Goal{}, httpx.Invalid("目标太多了，先删掉不用的")
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return api.Goal{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := m.q.WithTx(tx)
	now := m.now().UTC()
	order, err := q.MaxGoalSortOrder(ctx)
	if err != nil {
		return api.Goal{}, err
	}
	g, err := q.InsertGoal(ctx, db.InsertGoalParams{
		Title: f.title, Year: int64(f.year), Quarter: int64(f.quarter), Note: f.note, SortOrder: order + 1, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return api.Goal{}, err
	}
	for i, rf := range inputs {
		if _, err := q.InsertGoalResult(ctx, insertParams(g.ID, rf, i, now)); err != nil {
			return api.Goal{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return api.Goal{}, err
	}
	return m.get(ctx, m.q, g.ID)
}

func insertParams(goalID int64, rf resultFields, order int, now time.Time) db.InsertGoalResultParams {
	return db.InsertGoalResultParams{
		GoalID: goalID, Name: rf.name, Target: rf.target, Unit: rf.unit, StartValue: rf.start, Source: rf.source, Ref: rf.ref,
		ManualValue: rf.start, SortOrder: int64(order), CreatedAt: now, UpdatedAt: now,
	}
}

// UpdateGoal implements api.ServerInterface.
func (m *Module) UpdateGoal(w http.ResponseWriter, r *http.Request, goalID api.GoalId) {
	var body api.GoalInput
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	g, err := m.update(r.Context(), goalID, body)
	m.d.Audit.Record(r.Context(), "goal.update", strconv.FormatInt(goalID, 10), map[string]any{"title": body.Title}, err)
	if fail(w, r, err) {
		return
	}
	m.d.Bus.Publish("goal.updated", g)
	httpx.JSON(w, http.StatusOK, g)
}

func (m *Module) update(ctx context.Context, id int64, body api.GoalInput) (api.Goal, error) {
	f, err := checkGoal(body)
	if err != nil {
		return api.Goal{}, err
	}
	if _, err := m.q.GetGoal(ctx, id); errors.Is(err, sql.ErrNoRows) {
		return api.Goal{}, httpx.ErrNotFound
	} else if err != nil {
		return api.Goal{}, err
	}
	stored, err := m.q.ListGoalResults(ctx, id)
	if err != nil {
		return api.Goal{}, err
	}
	byID := map[int64]db.GoalResult{}
	for _, row := range stored {
		byID[row.ID] = row
	}
	cat, hidden := m.catalog(), m.hiddenFn(ctx)
	type pending struct {
		fields resultFields
		row    *db.GoalResult
	}
	var inputs []pending
	seen := map[int64]bool{}
	if body.Results != nil {
		for _, in := range *body.Results {
			var row *db.GoalResult
			if in.Id != nil {
				old, ok := byID[*in.Id]
				if !ok {
					return api.Goal{}, httpx.Invalid("关键结果不属于这个目标")
				}
				if seen[old.ID] {
					return api.Goal{}, httpx.Invalid("同一个关键结果出现了两次")
				}
				seen[old.ID] = true
				row = &old
			}
			rf, err := m.checkResult(ctx, cat, hidden, in, row)
			if err != nil {
				return api.Goal{}, err
			}
			inputs = append(inputs, pending{rf, row})
		}
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return api.Goal{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := m.q.WithTx(tx)
	now := m.now().UTC()
	if _, err := q.UpdateGoal(ctx, db.UpdateGoalParams{Title: f.title, Year: int64(f.year), Quarter: int64(f.quarter), Note: f.note, UpdatedAt: now, ID: id}); err != nil {
		return api.Goal{}, err
	}
	for i, p := range inputs {
		if p.row == nil {
			if _, err := q.InsertGoalResult(ctx, insertParams(id, p.fields, i, now)); err != nil {
				return api.Goal{}, err
			}
			continue
		}
		value := p.row.ManualValue
		if p.row.Source != string(api.Manual) && p.fields.source == string(api.Manual) {
			value = p.fields.start
		}
		// a manual key result nobody has touched follows its start value
		if p.fields.source == string(api.Manual) && value == p.row.StartValue && p.fields.start != p.row.StartValue {
			if _, err := q.LatestGoalResultLog(ctx, db.LatestGoalResultLogParams{ResultID: p.row.ID, At: now.AddDate(1, 0, 0)}); errors.Is(err, sql.ErrNoRows) {
				value = p.fields.start
			} else if err != nil {
				return api.Goal{}, err
			}
		}
		if _, err := q.UpdateGoalResult(ctx, db.UpdateGoalResultParams{
			Name: p.fields.name, Target: p.fields.target, Unit: p.fields.unit, StartValue: p.fields.start, Source: p.fields.source,
			Ref: p.fields.ref, ManualValue: value, SortOrder: int64(i), UpdatedAt: now, ID: p.row.ID, GoalID: id,
		}); err != nil {
			return api.Goal{}, err
		}
	}
	for _, row := range stored {
		if !seen[row.ID] {
			if err := q.DeleteGoalResult(ctx, row.ID); err != nil {
				return api.Goal{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return api.Goal{}, err
	}
	return m.get(ctx, m.q, id)
}

// SetGoalStatus implements api.ServerInterface.
func (m *Module) SetGoalStatus(w http.ResponseWriter, r *http.Request, goalID api.GoalId) {
	var body api.SetGoalStatusJSONBody
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	g, err := m.setStatus(r.Context(), goalID, body.Status)
	m.d.Audit.Record(r.Context(), "goal.status", strconv.FormatInt(goalID, 10), map[string]any{"status": string(body.Status)}, err)
	if fail(w, r, err) {
		return
	}
	m.d.Bus.Publish("goal.updated", g)
	httpx.JSON(w, http.StatusOK, g)
}

func (m *Module) setStatus(ctx context.Context, id int64, status api.GoalStatus) (api.Goal, error) {
	if !status.Valid() {
		return api.Goal{}, httpx.Invalid("不认识的状态")
	}
	_, err := m.q.SetGoalStatus(ctx, db.SetGoalStatusParams{Status: string(status), UpdatedAt: m.now().UTC(), ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return api.Goal{}, httpx.ErrNotFound
	}
	if err != nil {
		return api.Goal{}, err
	}
	return m.get(ctx, m.q, id)
}

// DeleteGoal implements api.ServerInterface.
func (m *Module) DeleteGoal(w http.ResponseWriter, r *http.Request, goalID api.GoalId) {
	n, err := m.q.DeleteGoal(r.Context(), goalID)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(r.Context(), "goal.delete", strconv.FormatInt(goalID, 10), nil, err)
	if fail(w, r, err) {
		return
	}
	m.d.Bus.Publish("goal.deleted", map[string]any{"id": goalID})
	w.WriteHeader(http.StatusNoContent)
}

// ---------- manual progress ----------

// SetGoalResultValue implements api.ServerInterface.
func (m *Module) SetGoalResultValue(w http.ResponseWriter, r *http.Request, resultID int64) {
	var body api.SetGoalResultValueJSONBody
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	res, err := m.setValue(r.Context(), resultID, body.Value)
	m.d.Audit.Record(r.Context(), "goal.progress", strconv.FormatInt(resultID, 10), map[string]any{"value": body.Value}, err)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

// setValue sets the current value of a manual key result and records it.
func (m *Module) setValue(ctx context.Context, resultID int64, value float64) (api.GoalResult, error) {
	if !finite(value) {
		return api.GoalResult{}, httpx.Invalid("数值不对")
	}
	row, err := m.q.GetGoalResult(ctx, resultID)
	if errors.Is(err, sql.ErrNoRows) {
		return api.GoalResult{}, httpx.ErrNotFound
	}
	if err != nil {
		return api.GoalResult{}, err
	}
	if row.Source != string(api.Manual) {
		return api.GoalResult{}, httpx.Invalid("这个关键结果的数值是自动统计的，不能直接改")
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return api.GoalResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := m.q.WithTx(tx)
	now := m.now().UTC()
	row, err = q.SetGoalResultValue(ctx, db.SetGoalResultValueParams{ManualValue: value, UpdatedAt: now, ID: resultID})
	if err != nil {
		return api.GoalResult{}, err
	}
	if err := q.InsertGoalResultLog(ctx, db.InsertGoalResultLogParams{ResultID: resultID, At: now, Value: value}); err != nil {
		return api.GoalResult{}, err
	}
	if err := q.TouchGoal(ctx, db.TouchGoalParams{UpdatedAt: now, ID: row.GoalID}); err != nil {
		return api.GoalResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return api.GoalResult{}, err
	}
	g, err := m.q.GetGoal(ctx, row.GoalID)
	if err != nil {
		return api.GoalResult{}, err
	}
	from, to := quarterBounds(int(g.Year), int(g.Quarter), m.loc())
	res := m.resultView(ctx, m.catalog(), m.hiddenFn(ctx), row, from, to, m.now())
	m.d.Bus.Publish("goal.progress", res)
	return res, nil
}

// ---------- metrics ----------

// ListGoalMetrics implements api.ServerInterface.
func (m *Module) ListGoalMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cat, hidden := m.catalog(), m.hiddenFn(ctx)
	items := []api.GoalMetric{}
	for _, source := range metricOrder {
		kind, ok := cat.kinds[string(source)]
		if !ok || hidden(kind.Module) {
			continue
		}
		opts, err := cat.sources[string(source)].Options(ctx, string(source))
		if fail(w, r, err) {
			return
		}
		out := make([]api.GoalMetricOption, 0, len(opts))
		for _, o := range opts {
			out = append(out, api.GoalMetricOption{Value: o.Value, Label: o.Label})
		}
		items = append(items, api.GoalMetric{Source: source, RefKind: api.GoalMetricRefKind(kind.RefKind), Options: out})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// metricOrder is the order the sources are offered in.
var metricOrder = []api.GoalSource{
	api.HabitCheckins, api.Workouts, api.FocusMinutes, api.ScreenMinutes, api.GithubCommits, api.GithubMergedPrs,
}
