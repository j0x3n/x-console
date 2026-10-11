package experiments_test

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/experiments"
	"github.com/j0x3n/x-console/backend/internal/server/modules/experiments/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type fixture struct {
	env   *testutil.Env
	m     *experiments.Module
	today time.Time
}

// day gives the date n days from today (negative is the past).
func (f fixture) day(n int) string { return f.today.AddDate(0, 0, n).Format("2006-01-02") }

func setup(t *testing.T) fixture {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*experiments.Module](env.App.Deps.Registry, experiments.ServiceKey)
	if !ok {
		t.Fatal("experiments module not registered")
	}
	loc := experiments.Loc(m)
	now := time.Now().In(loc)
	experiments.SetNow(m, func() time.Time { return now })
	return fixture{env: env, m: m, today: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)}
}

func (f fixture) energyKey(t *testing.T) string {
	t.Helper()
	var qs struct{ Items []struct{ Id int64 } }
	f.env.MustDo(http.MethodGet, "/checkins/questions", nil, &qs)
	return "checkin:" + strconv.FormatInt(qs.Items[0].Id, 10)
}

// answer records a check-in score on a day.
func (f fixture) answer(t *testing.T, key string, offset, value int) {
	t.Helper()
	id, _ := strconv.ParseInt(key[len("checkin:"):], 10, 64)
	date := f.day(offset)
	f.env.MustDo(http.MethodPut, "/checkins/answers", map[string]any{"questionId": id, "value": value, "date": date}, nil)
}

func (f fixture) create(t *testing.T, in api.ExperimentInput) api.Experiment {
	t.Helper()
	var e api.Experiment
	f.env.MustDo(http.MethodPost, "/experiments", in, &e)
	return e
}

func (f fixture) input(metrics ...string) api.ExperimentInput {
	return api.ExperimentInput{Title: "下午不喝咖啡", Change: "两点后不喝咖啡", BaselineStart: f.day(-9), ChangeStart: f.day(-4), EndDate: f.day(3), Metrics: metrics}
}

func TestCatalog(t *testing.T) {
	f := setup(t)
	var out struct{ Items []api.ExperimentMetricOption }
	f.env.MustDo(http.MethodGet, "/experiments/metrics", nil, &out)
	groups := map[api.ExperimentMetricOptionGroup]int{}
	keys := map[string]api.ExperimentMetricOption{}
	for _, o := range out.Items {
		groups[o.Group]++
		keys[o.Key] = o
	}
	if groups[api.Checkin] != 2 || groups[api.Body] != 5 || groups[api.Focus] != 1 || groups[api.Screen] == 0 {
		t.Fatalf("groups: %v", groups)
	}
	if !keys[f.energyKey(t)].Subjective || keys["body:sleep"].Subjective {
		t.Errorf("subjective flags wrong: %+v %+v", keys[f.energyKey(t)], keys["body:sleep"])
	}
	if _, ok := keys["goal:focus_minutes:"]; !ok {
		t.Errorf("focus minutes missing: %v", keys)
	}
	for k := range keys {
		if len(k) > 5 && k[:5] == "goal:" && (k[5:11] == "github") {
			t.Errorf("github must not be offered: %s", k)
		}
	}
}

func TestCreateValidation(t *testing.T) {
	f := setup(t)
	key := f.energyKey(t)
	good := f.input(key)
	bad := map[string]func(in *api.ExperimentInput){
		"empty title":        func(in *api.ExperimentInput) { in.Title = " " },
		"bad date":           func(in *api.ExperimentInput) { in.EndDate = "2026-13-40" },
		"baseline too short": func(in *api.ExperimentInput) { in.ChangeStart = f.day(-8) },
		"change too short":   func(in *api.ExperimentInput) { in.EndDate = f.day(-3) },
		"too long":           func(in *api.ExperimentInput) { in.EndDate = f.day(400) },
		"no metrics":         func(in *api.ExperimentInput) { in.Metrics = nil },
		"unknown metric":     func(in *api.ExperimentInput) { in.Metrics = []string{"nope:1"} },
		"duplicate metric":   func(in *api.ExperimentInput) { in.Metrics = []string{key, key} },
		"seven metrics": func(in *api.ExperimentInput) {
			in.Metrics = []string{key, "body:weight", "body:waist", "body:sleep", "body:steps", "body:restingHr", "goal:focus_minutes:"}
		},
	}
	for name, mutate := range bad {
		in := good
		mutate(&in)
		if status, _ := f.env.Do(http.MethodPost, "/experiments", in, nil); status != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, status)
		}
	}
	e := f.create(t, good)
	if e.Phase != api.ExperimentPhaseChange || !e.Retrospective || len(e.Metrics) != 1 || e.Metrics[0].Label != "现在精力怎么样" || !e.Metrics[0].Subjective {
		t.Fatalf("created: %+v", e)
	}
}

func TestResultHigher(t *testing.T) {
	f := setup(t)
	key := f.energyKey(t)
	e := f.create(t, f.input(key))
	for i, v := range []int{2, 3, 2, 3, 2} {
		f.answer(t, key, -9+i, v)
	}
	for i, v := range []int{4, 5, 4, 5, 4} {
		f.answer(t, key, -4+i, v)
	}
	var d api.ExperimentDetail
	f.env.MustDo(http.MethodGet, fmt.Sprintf("/experiments/%d", e.Id), nil, &d)
	r := d.Results[0]
	if r.Verdict != api.Higher || r.Diff == nil || *r.Diff != 2 || r.CiLow == nil || *r.CiLow <= 0 || *r.CiHigh <= *r.CiLow {
		t.Fatalf("result: %+v", r)
	}
	if r.Baseline.N != 5 || r.Baseline.Days != 5 || r.Baseline.Mean != 2.4 || r.Change.N != 5 || r.Change.Days != 5 || r.Change.Mean != 4.4 {
		t.Errorf("phases: %+v %+v", r.Baseline, r.Change)
	}
	if len(r.Points) != 10 || r.Points[0].Phase != api.ExperimentResultPointsPhaseBaseline || r.Points[9].Phase != api.ExperimentResultPointsPhaseChange {
		t.Errorf("points: %+v", r.Points)
	}
	notes := map[api.ExperimentResultNotes]bool{}
	for _, n := range r.Notes {
		notes[n] = true
	}
	if !notes[api.Subjective] || !notes[api.ChangeRunning] || !notes[api.Retrospective] || notes[api.SparseData] {
		t.Errorf("notes: %v", r.Notes)
	}
}

func TestResultTooFewAndNoData(t *testing.T) {
	f := setup(t)
	key := f.energyKey(t)
	e := f.create(t, f.input(key, "body:weight"))
	for i := 0; i < 3; i++ { // three baseline days and two change days
		f.answer(t, key, -9+i, 3)
	}
	f.answer(t, key, -4, 4)
	f.answer(t, key, -3, 4)
	var d api.ExperimentDetail
	f.env.MustDo(http.MethodGet, fmt.Sprintf("/experiments/%d", e.Id), nil, &d)
	if d.Results[0].Verdict != api.TooFew || d.Results[0].Diff == nil || d.Results[0].CiLow != nil {
		t.Errorf("too few: %+v", d.Results[0])
	}
	var sparse bool
	for _, n := range d.Results[0].Notes {
		sparse = sparse || n == api.SparseData
	}
	if !sparse {
		t.Errorf("sparse note missing: %v", d.Results[0].Notes)
	}
	if d.Results[1].Verdict != api.NoData || d.Results[1].Diff != nil {
		t.Errorf("no data: %+v", d.Results[1])
	}
}

func TestBodyAndGoalSources(t *testing.T) {
	f := setup(t)
	e := f.create(t, f.input("body:weight", "goal:focus_minutes:"))
	for i := 0; i < 10; i++ {
		w := "70.0"
		if i >= 5 {
			w = "68.5"
		}
		f.env.MustDo(http.MethodPatch, "/habits/personal/days/"+f.day(-9+i), map[string]any{"weight": w}, nil)
	}
	var d api.ExperimentDetail
	f.env.MustDo(http.MethodGet, fmt.Sprintf("/experiments/%d", e.Id), nil, &d)
	w := d.Results[0]
	if w.Verdict != api.Lower || *w.Diff != -1.5 || w.Baseline.N != 5 || w.Change.N != 5 {
		t.Errorf("weight: %+v", w)
	}
	// No focus sessions at all: every day is a real zero, so both phases are
	// constant 0 and the difference is not a difference.
	foc := d.Results[1]
	if foc.Verdict != api.Inconclusive || foc.Baseline.N != 5 || foc.Change.N != 5 {
		t.Errorf("focus: %+v", foc)
	}
}

func TestPhasesStopUpdateDelete(t *testing.T) {
	f := setup(t)
	key := f.energyKey(t)
	cases := []struct {
		name    string
		in      func(api.ExperimentInput) api.ExperimentInput
		want    api.ExperimentPhase
		retro   bool
		stopped bool
	}{
		{"planned", func(in api.ExperimentInput) api.ExperimentInput {
			in.BaselineStart, in.ChangeStart, in.EndDate = f.day(2), f.day(5), f.day(9)
			return in
		}, api.ExperimentPhasePlanned, false, false},
		{"baseline", func(in api.ExperimentInput) api.ExperimentInput {
			in.BaselineStart, in.ChangeStart, in.EndDate = f.day(-2), f.day(2), f.day(6)
			return in
		}, api.ExperimentPhaseBaseline, true, false},
		{"finished", func(in api.ExperimentInput) api.ExperimentInput {
			in.BaselineStart, in.ChangeStart, in.EndDate = f.day(-12), f.day(-8), f.day(-3)
			return in
		}, api.ExperimentPhaseFinished, true, false},
		{"stopped", func(in api.ExperimentInput) api.ExperimentInput {
			stopped := true
			in.Stopped = &stopped
			return in
		}, api.ExperimentPhaseStopped, true, true},
	}
	var last api.Experiment
	for _, c := range cases {
		e := f.create(t, c.in(f.input(key)))
		if e.Phase != c.want || e.Retrospective != c.retro || e.Stopped != c.stopped {
			t.Errorf("%s: %+v", c.name, e)
		}
		last = e
	}
	var list struct{ Items []api.Experiment }
	f.env.MustDo(http.MethodGet, "/experiments", nil, &list)
	if len(list.Items) != 4 || list.Items[0].Id != last.Id {
		t.Fatalf("list: %+v", list.Items)
	}
	// Edit: conclusion and metrics.
	in := f.input(key, "body:sleep")
	concl := "咖啡对精力没有明显影响"
	in.Conclusion = &concl
	var e api.Experiment
	f.env.MustDo(http.MethodPut, fmt.Sprintf("/experiments/%d", last.Id), in, &e)
	if e.Conclusion != concl || len(e.Metrics) != 2 || e.Stopped || e.Phase != api.ExperimentPhaseChange {
		t.Errorf("updated: %+v", e)
	}
	if status, _ := f.env.Do(http.MethodPut, "/experiments/9999", in, nil); status != http.StatusNotFound {
		t.Errorf("update unknown: %d", status)
	}
	if status, _ := f.env.Do(http.MethodGet, "/experiments/9999", nil, nil); status != http.StatusNotFound {
		t.Errorf("get unknown: %d", status)
	}
	f.env.MustDo(http.MethodDelete, fmt.Sprintf("/experiments/%d", last.Id), nil, nil)
	if status, _ := f.env.Do(http.MethodDelete, fmt.Sprintf("/experiments/%d", last.Id), nil, nil); status != http.StatusNotFound {
		t.Errorf("delete twice: %d", status)
	}
}

func TestActions(t *testing.T) {
	f := setup(t)
	key := f.energyKey(t)
	e := f.create(t, f.input(key))
	ctx := context.Background()
	if _, err := f.env.App.Deps.Actions.Run(ctx, "experiments.list", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	res, err := f.env.App.Deps.Actions.Run(ctx, "experiments.result", []byte(fmt.Sprintf(`{"id":%d}`, e.Id)))
	if err != nil || res == nil {
		t.Fatalf("result: %v %v", res, err)
	}
	if _, err := f.env.App.Deps.Actions.Run(ctx, "experiments.result", []byte(`{"id":9999}`)); err == nil {
		t.Fatal("unknown id should fail")
	}
}
