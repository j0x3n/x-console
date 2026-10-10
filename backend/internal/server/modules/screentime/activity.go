package screentime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// activitySource tells the journal (B118) how long the computers were used,
// one item per day with the categories that took the most time.
type activitySource struct{ m *Module }

var categoryLabels = map[string]string{
	catCoding: "编码", catAI: "AI 工具", catChat: "通讯", catWeb: "网页",
	catEntertainment: "娱乐", catOffice: "办公", catOther: "其他",
}

func duration(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%d 分钟", minutes)
	}
	if minutes%60 == 0 {
		return fmt.Sprintf("%d 小时", minutes/60)
	}
	return fmt.Sprintf("%d 小时 %d 分", minutes/60, minutes%60)
}

func (s activitySource) Activity(ctx context.Context, from, until time.Time) ([]contracts.Activity, error) {
	loc := s.m.location()
	now := s.m.now()
	var out []contracts.Activity
	start := time.Date(from.In(loc).Year(), from.In(loc).Month(), from.In(loc).Day(), 0, 0, 0, 0, loc)
	for d := start; d.Before(until); d = d.AddDate(0, 0, 1) {
		next := d.AddDate(0, 0, 1)
		totals, err := s.m.categoryTotals(ctx, minuteOf(d), minuteOf(next), "")
		if err != nil {
			return nil, err
		}
		total := 0
		type cat struct {
			name    string
			minutes int
		}
		var cats []cat
		for _, c := range categories {
			if totals[c] > 0 {
				total += totals[c]
				cats = append(cats, cat{categoryLabels[c], totals[c]})
			}
		}
		if total == 0 {
			continue
		}
		sort.SliceStable(cats, func(i, j int) bool { return cats[i].minutes > cats[j].minutes })
		var parts []string
		for i, c := range cats {
			if i == 3 {
				break
			}
			parts = append(parts, c.name+" "+duration(c.minutes))
		}
		// the item sits at the end of its day, or now while the day is not over
		at := next.Add(-time.Minute)
		if at.After(now) {
			at = now
		}
		if at.Before(from) || !at.Before(until) {
			continue
		}
		out = append(out, contracts.Activity{
			Ref: "screen:" + d.Format(dateLayout), Module: "screentime", Kind: "screen", At: at, Minutes: total,
			Title: "电脑用了 " + duration(total), Detail: strings.Join(parts, " · "), Link: "/screentime?date=" + d.Format(dateLayout),
		})
	}
	return out, nil
}
