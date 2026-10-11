package experiments

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/experiments/api"
)

// Metric keys:
//
//	checkin:<question id>   daily average of a check-in question (B132)
//	body:<metric>           weight, waist, sleep, restingHr, steps
//	goal:<kind>:<ref>       a measure of the goals module, taken day by day
const (
	keyCheckin = "checkin:"
	keyBody    = "body:"
	keyGoal    = "goal:"
)

var bodyLabels = []struct{ metric, label, unit string }{
	{"weight", "体重", "kg"},
	{"waist", "腰围", "cm"},
	{"sleep", "睡眠", "小时"},
	{"restingHr", "静息心率", "次/分"},
	{"steps", "步数", "步"},
}

// goalKinds are the measures of the goals module that make sense day by day.
// GitHub is left out: its numbers come from a cache that can be incomplete.
var goalKinds = map[string]struct{ label, unit, group string }{
	contracts.GoalMetricHabitCheckins: {"习惯打卡次数", "次", "habit"},
	contracts.GoalMetricWorkouts:      {"训练次数", "次", "habit"},
	contracts.GoalMetricFocusMinutes:  {"专注分钟", "分钟", "focus"},
	contracts.GoalMetricScreenMinutes: {"电脑时间", "分钟", "screen"},
}

// metricInfo is what the module knows about one key at some moment.
type metricInfo struct {
	Ref   api.ExperimentMetricRef
	Group string
}

// catalog lists the metrics that can be chosen now. A module that is hidden
// and locked contributes nothing.
func (m *Module) catalog(ctx context.Context) []api.ExperimentMetricOption {
	hidden := m.hidden(ctx)
	var out []api.ExperimentMetricOption
	add := func(group, key, label, unit string, subjective bool) {
		out = append(out, api.ExperimentMetricOption{Key: key, Label: label, Unit: unit, Subjective: subjective,
			Group: api.ExperimentMetricOptionGroup(group)})
	}
	if cs, ok := module.Lookup[contracts.CheckinSeries](m.d.Registry, contracts.CheckinSeriesKey); ok && !hidden("checkins") {
		_ = cs
		rows, err := m.d.DB.QueryContext(ctx, `SELECT id, label FROM checkin_questions ORDER BY sort_order, id`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id int64
				var label string
				if rows.Scan(&id, &label) == nil {
					add("checkin", keyCheckin+strconv.FormatInt(id, 10), label, "分", true)
				}
			}
		}
	}
	if _, ok := module.Lookup[contracts.BodySeries](m.d.Registry, contracts.BodySeriesKey); ok && !hidden("habits") {
		for _, b := range bodyLabels {
			add("body", keyBody+b.metric, b.label, b.unit, false)
		}
	}
	kinds := m.goalSources()
	names := make([]string, 0, len(kinds))
	for k := range kinds {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, kind := range names {
		src := kinds[kind]
		info := goalKinds[kind]
		for _, k := range src.Kinds() {
			if k.Kind != kind || hidden(k.Module) {
				continue
			}
			if k.RefKind == contracts.GoalRefNone {
				add(info.group, keyGoal+kind+":", info.label, info.unit, false)
				continue
			}
			opts, err := src.Options(ctx, kind)
			if err != nil {
				continue
			}
			for _, o := range opts {
				label := info.label + "：" + o.Label
				add(info.group, keyGoal+kind+":"+o.Value, label, info.unit, false)
			}
		}
	}
	return out
}

// goalSources are the goal metric sources by kind, restricted to the kinds
// that work day by day.
func (m *Module) goalSources() map[string]contracts.GoalMetricSource {
	out := map[string]contracts.GoalMetricSource{}
	for key, src := range module.All[contracts.GoalMetricSource](m.d.Registry) {
		if !strings.HasPrefix(key, contracts.GoalMetricPrefix) {
			continue
		}
		for _, k := range src.Kinds() {
			if _, ok := goalKinds[k.Kind]; ok {
				out[k.Kind] = src
			}
		}
	}
	return out
}

// lookup finds a key in the current catalog.
func (m *Module) lookup(ctx context.Context, key string) (api.ExperimentMetricRef, bool) {
	for _, o := range m.catalog(ctx) {
		if o.Key == key {
			return api.ExperimentMetricRef{Key: o.Key, Label: o.Label, Unit: o.Unit, Subjective: o.Subjective}, true
		}
	}
	return api.ExperimentMetricRef{}, false
}

var errUnavailable = errors.New("没有这个数据来源")

// daily returns the values of a metric for each day from..to (both dates
// included, YYYY-MM-DD). Days without data are left out.
func (m *Module) daily(ctx context.Context, key, from, to string) ([]point, error) {
	loc := m.loc()
	f, err := time.ParseInLocation(dateLayout, from, loc)
	if err != nil {
		return nil, err
	}
	t, err := time.ParseInLocation(dateLayout, to, loc)
	if err != nil {
		return nil, err
	}
	hidden := m.hidden(ctx)
	switch {
	case strings.HasPrefix(key, keyCheckin):
		id, err := strconv.ParseInt(strings.TrimPrefix(key, keyCheckin), 10, 64)
		cs, ok := module.Lookup[contracts.CheckinSeries](m.d.Registry, contracts.CheckinSeriesKey)
		if err != nil || !ok || hidden("checkins") {
			return nil, errUnavailable
		}
		days, err := cs.Daily(ctx, f, t)
		if err != nil {
			return nil, err
		}
		var out []point
		for _, d := range days {
			if d.QuestionID == id {
				out = append(out, point{Date: d.Date, Value: d.Average})
			}
		}
		return out, nil
	case strings.HasPrefix(key, keyBody):
		bs, ok := module.Lookup[contracts.BodySeries](m.d.Registry, contracts.BodySeriesKey)
		if !ok || hidden("habits") {
			return nil, errUnavailable
		}
		pts, err := bs.Daily(ctx, strings.TrimPrefix(key, keyBody), f, t)
		if err != nil {
			return nil, err
		}
		out := make([]point, 0, len(pts))
		for _, p := range pts {
			out = append(out, point{Date: p.Date, Value: p.Value})
		}
		return out, nil
	case strings.HasPrefix(key, keyGoal):
		rest := strings.TrimPrefix(key, keyGoal)
		kind, ref, _ := strings.Cut(rest, ":")
		src, ok := m.goalSources()[kind]
		if !ok {
			return nil, errUnavailable
		}
		for _, k := range src.Kinds() {
			if k.Kind == kind && hidden(k.Module) {
				return nil, errUnavailable
			}
		}
		var out []point
		for d := f; !d.After(t); d = d.AddDate(0, 0, 1) {
			v, err := src.Measure(ctx, kind, ref, d, d.AddDate(0, 0, 1))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", kind, err)
			}
			// No computer time recorded means the computer was off or the
			// agent was not running, not that the time was 0.
			if kind == contracts.GoalMetricScreenMinutes && v == 0 {
				continue
			}
			out = append(out, point{Date: d.Format(dateLayout), Value: v})
		}
		return out, nil
	}
	return nil, errUnavailable
}
