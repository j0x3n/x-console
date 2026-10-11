package experiments

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/experiments/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/experiments/db"
)

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// phaseOf says where the experiment is on the given day.
func phaseOf(e db.Experiment, today string) api.ExperimentPhase {
	switch {
	case e.Stopped == 1:
		return api.ExperimentPhaseStopped
	case today < e.BaselineStart:
		return api.ExperimentPhasePlanned
	case today < e.ChangeStart:
		return api.ExperimentPhaseBaseline
	case today <= e.EndDate:
		return api.ExperimentPhaseChange
	}
	return api.ExperimentPhaseFinished
}

func (m *Module) toExperiment(e db.Experiment, metrics []db.ExperimentMetric) api.Experiment {
	refs := make([]api.ExperimentMetricRef, 0, len(metrics))
	for _, x := range metrics {
		refs = append(refs, api.ExperimentMetricRef{Key: x.Key, Label: x.Label, Unit: x.Unit, Subjective: x.Subjective == 1})
	}
	return api.Experiment{Id: e.ID, Title: e.Title, Change: e.ChangeText, BaselineStart: e.BaselineStart, ChangeStart: e.ChangeStart,
		EndDate: e.EndDate, Note: e.Note, Conclusion: e.Conclusion, Stopped: e.Stopped == 1, Phase: phaseOf(e, m.today()),
		Retrospective: e.Retrospective == 1, Metrics: refs, CreatedAt: e.CreatedAt}
}

// validate checks the input and returns cleaned values.
func (m *Module) validate(in api.ExperimentInput) (api.ExperimentInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Change = strings.TrimSpace(in.Change)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > maxTitle {
		return in, httpx.Invalid("名称要写 1 到 100 个字")
	}
	if utf8.RuneCountInString(in.Change) > maxText ||
		(in.Note != nil && utf8.RuneCountInString(*in.Note) > maxText) ||
		(in.Conclusion != nil && utf8.RuneCountInString(*in.Conclusion) > maxText) {
		return in, httpx.Invalid("文字太长了")
	}
	loc := m.loc()
	var days [3]time.Time
	for i, s := range []string{in.BaselineStart, in.ChangeStart, in.EndDate} {
		d, err := time.ParseInLocation(dateLayout, s, loc)
		if err != nil {
			return in, httpx.Invalid("日期要写成 YYYY-MM-DD")
		}
		days[i] = d
	}
	baselineDays := int(math.Round(days[1].Sub(days[0]).Hours() / 24))
	changeDays := int(math.Round(days[2].Sub(days[1]).Hours()/24)) + 1
	if baselineDays < minPhaseDays || changeDays < minPhaseDays {
		return in, httpx.Invalid("基线期和改变期各要至少 3 天")
	}
	if baselineDays+changeDays > maxSpanDays {
		return in, httpx.Invalid("一个实验最长 180 天")
	}
	if len(in.Metrics) == 0 {
		return in, httpx.Invalid("至少选一个指标")
	}
	if len(in.Metrics) > maxMetrics {
		return in, httpx.Invalid("最多选 6 个指标")
	}
	seen := map[string]bool{}
	for _, k := range in.Metrics {
		if seen[k] {
			return in, httpx.Invalid("指标重复了")
		}
		seen[k] = true
	}
	return in, nil
}

// resolveMetrics turns keys into rows. A key that is no longer offered is
// kept when the experiment already has it (a locked module hides its data
// but the experiment stays valid).
func (m *Module) resolveMetrics(ctx context.Context, keys []string, existing []db.ExperimentMetric) ([]db.ExperimentMetric, error) {
	byKey := map[string]db.ExperimentMetric{}
	for _, x := range existing {
		byKey[x.Key] = x
	}
	out := make([]db.ExperimentMetric, 0, len(keys))
	for i, k := range keys {
		if ref, ok := m.lookup(ctx, k); ok {
			out = append(out, db.ExperimentMetric{Key: k, Label: ref.Label, Unit: ref.Unit, Subjective: b2i(ref.Subjective), SortOrder: int64(i)})
			continue
		}
		if old, ok := byKey[k]; ok {
			old.SortOrder = int64(i)
			out = append(out, old)
			continue
		}
		return nil, httpx.Invalid("有指标现在选不了：" + k)
	}
	return out, nil
}

func (m *Module) metricsOf(ctx context.Context, id int64) ([]db.ExperimentMetric, error) {
	return m.q.ListExperimentMetrics(ctx, id)
}

func (m *Module) ListExperiments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := m.q.ListExperiments(ctx)
	if fail(w, r, err) {
		return
	}
	all, err := m.q.ListAllExperimentMetrics(ctx)
	if fail(w, r, err) {
		return
	}
	by := map[int64][]db.ExperimentMetric{}
	for _, x := range all {
		by[x.ExperimentID] = append(by[x.ExperimentID], x)
	}
	items := make([]api.Experiment, 0, len(rows))
	for _, e := range rows {
		items = append(items, m.toExperiment(e, by[e.ID]))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (m *Module) ListExperimentMetrics(w http.ResponseWriter, r *http.Request) {
	items := m.catalog(r.Context())
	if items == nil {
		items = []api.ExperimentMetricOption{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (m *Module) CreateExperiment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body api.ExperimentInput
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	in, err := m.validate(body)
	if fail(w, r, err) {
		return
	}
	n, err := m.q.CountExperiments(ctx)
	if fail(w, r, err) {
		return
	}
	if n >= maxExperiments {
		httpx.Fail(w, r, httpx.Invalid("实验太多了，先删掉不要的"))
		return
	}
	metrics, err := m.resolveMetrics(ctx, in.Metrics, nil)
	if fail(w, r, err) {
		return
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if fail(w, r, err) {
		return
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	now := m.now().UTC()
	row, err := q.InsertExperiment(ctx, db.InsertExperimentParams{Title: in.Title, ChangeText: in.Change, BaselineStart: in.BaselineStart,
		ChangeStart: in.ChangeStart, EndDate: in.EndDate, Note: deref(in.Note), Conclusion: deref(in.Conclusion), Stopped: b2i(in.Stopped != nil && *in.Stopped),
		Retrospective: b2i(in.BaselineStart < m.today()), CreatedAt: now, UpdatedAt: now})
	if fail(w, r, err) {
		return
	}
	if fail(w, r, putMetrics(ctx, q, row.ID, metrics)) {
		return
	}
	if fail(w, r, tx.Commit()) {
		return
	}
	m.d.Audit.Record(ctx, "experiment.create", in.Title, nil, nil)
	saved, _ := m.metricsOf(ctx, row.ID)
	httpx.JSON(w, http.StatusCreated, m.toExperiment(row, saved))
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func putMetrics(ctx context.Context, q *db.Queries, id int64, metrics []db.ExperimentMetric) error {
	if err := q.DeleteExperimentMetrics(ctx, id); err != nil {
		return err
	}
	for _, x := range metrics {
		if err := q.InsertExperimentMetric(ctx, db.InsertExperimentMetricParams{ExperimentID: id, Key: x.Key, Label: x.Label, Unit: x.Unit,
			Subjective: x.Subjective, SortOrder: x.SortOrder}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) UpdateExperiment(w http.ResponseWriter, r *http.Request, id api.ExperimentId) {
	ctx := r.Context()
	var body api.ExperimentInput
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	in, err := m.validate(body)
	if fail(w, r, err) {
		return
	}
	if _, err := m.q.GetExperiment(ctx, id); errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	} else if fail(w, r, err) {
		return
	}
	existing, err := m.metricsOf(ctx, id)
	if fail(w, r, err) {
		return
	}
	metrics, err := m.resolveMetrics(ctx, in.Metrics, existing)
	if fail(w, r, err) {
		return
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if fail(w, r, err) {
		return
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	row, err := q.UpdateExperiment(ctx, db.UpdateExperimentParams{Title: in.Title, ChangeText: in.Change, BaselineStart: in.BaselineStart,
		ChangeStart: in.ChangeStart, EndDate: in.EndDate, Note: deref(in.Note), Conclusion: deref(in.Conclusion),
		Stopped: b2i(in.Stopped != nil && *in.Stopped), UpdatedAt: m.now().UTC(), ID: id})
	if fail(w, r, err) {
		return
	}
	if fail(w, r, putMetrics(ctx, q, id, metrics)) {
		return
	}
	if fail(w, r, tx.Commit()) {
		return
	}
	m.d.Audit.Record(ctx, "experiment.update", in.Title, nil, nil)
	saved, _ := m.metricsOf(ctx, id)
	httpx.JSON(w, http.StatusOK, m.toExperiment(row, saved))
}

func (m *Module) DeleteExperiment(w http.ResponseWriter, r *http.Request, id api.ExperimentId) {
	n, err := m.q.DeleteExperiment(r.Context(), id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(r.Context(), "experiment.delete", "", map[string]any{"id": id}, err)
	if fail(w, r, err) {
		return
	}
	httpx.NoContent(w)
}

func (m *Module) GetExperiment(w http.ResponseWriter, r *http.Request, id api.ExperimentId) {
	detail, err := m.detail(r.Context(), id)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

// detail loads an experiment and works out its results.
func (m *Module) detail(ctx context.Context, id int64) (api.ExperimentDetail, error) {
	e, err := m.q.GetExperiment(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return api.ExperimentDetail{}, httpx.ErrNotFound
	}
	if err != nil {
		return api.ExperimentDetail{}, err
	}
	metrics, err := m.metricsOf(ctx, id)
	if err != nil {
		return api.ExperimentDetail{}, err
	}
	out := api.ExperimentDetail{Experiment: m.toExperiment(e, metrics), Results: []api.ExperimentResult{}}
	for _, x := range metrics {
		out.Results = append(out.Results, m.resultFor(ctx, e, x, len(metrics)))
	}
	return out, nil
}

func round(v float64) float64 { return math.Round(v*1000) / 1000 }

func phaseStats(s stats, days int) api.PhaseStats {
	return api.PhaseStats{Days: days, N: s.N, Mean: round(s.Mean), Sd: round(s.SD)}
}

// resultFor reads the days of one metric and compares the phases.
func (m *Module) resultFor(ctx context.Context, e db.Experiment, x db.ExperimentMetric, metricCount int) api.ExperimentResult {
	ref := api.ExperimentMetricRef{Key: x.Key, Label: x.Label, Unit: x.Unit, Subjective: x.Subjective == 1}
	today := m.today()
	last := e.EndDate
	if today < last {
		last = today
	}
	baseDays, changeDays := daysBetween(e.BaselineStart, e.ChangeStart, last)
	res := api.ExperimentResult{Metric: ref, Notes: []api.ExperimentResultNotes{}, Points: []struct {
		Date  string                          `json:"date"`
		Phase api.ExperimentResultPointsPhase `json:"phase"`
		Value float64                         `json:"value"`
	}{}}
	var pts []point
	if last >= e.BaselineStart {
		var err error
		pts, err = m.daily(ctx, x.Key, e.BaselineStart, last)
		if err != nil {
			m.d.Log.Warn("experiment metric", "key", x.Key, "err", err)
			res.Verdict = api.NoData
			res.Baseline, res.Change = phaseStats(stats{}, baseDays), phaseStats(stats{}, changeDays)
			return res
		}
	}
	baseline, change := split(pts, e.BaselineStart, e.ChangeStart, last)
	o := compare(values(baseline), values(change))
	res.Verdict = api.ExperimentVerdict(o.Verdict)
	res.Baseline, res.Change = phaseStats(o.Baseline, baseDays), phaseStats(o.Change, changeDays)
	if o.Verdict != verdictNoData {
		d := round(o.Diff)
		res.Diff = &d
	}
	if o.Verdict == verdictHigher || o.Verdict == verdictLower || o.Verdict == verdictInconclusive {
		lo, hi := round(o.CILow), round(o.CIHigh)
		res.CiLow, res.CiHigh = &lo, &hi
		if o.HasEffect {
			ef := round(o.Effect)
			res.EffectSize = &ef
		}
	}
	for _, p := range baseline {
		res.Points = append(res.Points, struct {
			Date  string                          `json:"date"`
			Phase api.ExperimentResultPointsPhase `json:"phase"`
			Value float64                         `json:"value"`
		}{p.Date, api.ExperimentResultPointsPhaseBaseline, round(p.Value)})
	}
	for _, p := range change {
		res.Points = append(res.Points, struct {
			Date  string                          `json:"date"`
			Phase api.ExperimentResultPointsPhase `json:"phase"`
			Value float64                         `json:"value"`
		}{p.Date, api.ExperimentResultPointsPhaseChange, round(p.Value)})
	}
	if x.Subjective == 1 {
		res.Notes = append(res.Notes, api.Subjective)
	}
	if e.Stopped == 0 && today <= e.EndDate && today >= e.ChangeStart {
		res.Notes = append(res.Notes, api.ChangeRunning)
	}
	if baseDays+changeDays > 0 && float64(o.Baseline.N+o.Change.N) < 0.6*float64(baseDays+changeDays) {
		res.Notes = append(res.Notes, api.SparseData)
	}
	if e.Retrospective == 1 {
		res.Notes = append(res.Notes, api.Retrospective)
	}
	if metricCount > 3 {
		res.Notes = append(res.Notes, api.ManyMetrics)
	}
	return res
}

// daysBetween counts the days passed in each phase, up to last.
func daysBetween(baselineStart, changeStart, last string) (baseline, change int) {
	parse := func(s string) time.Time { t, _ := time.Parse(dateLayout, s); return t }
	bs, cs, end := parse(baselineStart), parse(changeStart), parse(last)
	if end.Before(bs) {
		return 0, 0
	}
	if end.Before(cs) {
		return int(end.Sub(bs).Hours()/24) + 1, 0
	}
	return int(cs.Sub(bs).Hours() / 24), int(end.Sub(cs).Hours()/24) + 1
}
