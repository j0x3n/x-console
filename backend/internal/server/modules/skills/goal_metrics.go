package skills

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// goalMetrics gives the goals module (B128) the practice minutes of one skill.
type goalMetrics struct{ m *Module }

func (goalMetrics) Kinds() []contracts.GoalMetricKind {
	return []contracts.GoalMetricKind{
		{Kind: contracts.GoalMetricSkillMinutes, Module: "skills", RefKind: contracts.GoalRefSkill},
	}
}

// Options lists the skills that are not archived.
func (g goalMetrics) Options(ctx context.Context, kind string) ([]contracts.GoalMetricOption, error) {
	if kind != contracts.GoalMetricSkillMinutes {
		return nil, nil
	}
	rows, err := g.m.q.ListSkills(ctx, 0)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.GoalMetricOption, 0, len(rows))
	for _, r := range rows {
		out = append(out, contracts.GoalMetricOption{Value: itoa(r.ID), Label: r.Name})
	}
	return out, nil
}

// Measure adds up the minutes practiced on the days in [from, until). The
// dates are those of the server time zone.
func (g goalMetrics) Measure(ctx context.Context, kind, ref string, from, until time.Time) (float64, error) {
	if kind != contracts.GoalMetricSkillMinutes {
		return 0, errors.New("不认识的数据来源")
	}
	id, err := strconv.ParseInt(ref, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("没有选技能")
	}
	loc := g.m.loc()
	// until is "now" for a running quarter: today's practice counts, and so
	// does the last day when until is exactly midnight (the exclusive bound).
	u := until.In(loc)
	if u.Hour() != 0 || u.Minute() != 0 || u.Second() != 0 || u.Nanosecond() != 0 {
		u = u.AddDate(0, 0, 1)
	}
	var minutes int64
	err = g.m.d.DB.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(minutes), 0) FROM skill_sessions WHERE skill_id = ? AND practiced_on >= ? AND practiced_on < ?`,
		id, from.In(loc).Format(dateLayout), u.Format(dateLayout)).Scan(&minutes)
	return float64(minutes), err
}

func parseID(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }
