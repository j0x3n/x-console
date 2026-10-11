package screentime

import (
	"context"
	"errors"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// goalMetrics gives the goals module (B128) the minutes spent on the
// computers, in one category or in all of them.
type goalMetrics struct{ m *Module }

func (goalMetrics) Kinds() []contracts.GoalMetricKind {
	return []contracts.GoalMetricKind{
		{Kind: contracts.GoalMetricScreenMinutes, Module: "screentime", RefKind: contracts.GoalRefCategory},
	}
}

// Options lists the categories. The empty value means all of them.
func (goalMetrics) Options(_ context.Context, kind string) ([]contracts.GoalMetricOption, error) {
	if kind != contracts.GoalMetricScreenMinutes {
		return nil, nil
	}
	out := []contracts.GoalMetricOption{{Value: "", Label: "全部分类"}}
	for _, c := range categories {
		out = append(out, contracts.GoalMetricOption{Value: c, Label: categoryLabels[c]})
	}
	return out, nil
}

func (g goalMetrics) Measure(ctx context.Context, kind, ref string, from, until time.Time) (float64, error) {
	if kind != contracts.GoalMetricScreenMinutes {
		return 0, errors.New("不认识的数据来源")
	}
	if ref != "" && !validCategory(ref) {
		return 0, errors.New("不认识的分类")
	}
	totals, err := g.m.categoryTotals(ctx, minuteOf(from), minuteOf(until), "")
	if err != nil {
		return 0, err
	}
	if ref != "" {
		return float64(totals[ref]), nil
	}
	sum := 0
	for _, v := range totals {
		sum += v
	}
	return float64(sum), nil
}
