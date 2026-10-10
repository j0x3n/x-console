package habits

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// activitySource tells the journal (B118) about check-ins and workouts.
type activitySource struct{ m *Module }

func (s activitySource) Activity(ctx context.Context, from, until time.Time) ([]contracts.Activity, error) {
	loc := s.m.d.Scheduler.Location()
	if loc == nil {
		loc = time.Local
	}
	out, err := s.checkins(ctx, from, until, loc)
	if err != nil {
		return nil, err
	}
	workouts, err := s.workouts(ctx, from, until)
	if err != nil {
		return nil, err
	}
	return append(out, workouts...), nil
}

// checkins gives one item per habit and day: how many times and how much.
func (s activitySource) checkins(ctx context.Context, from, until time.Time, loc *time.Location) ([]contracts.Activity, error) {
	rows, err := s.m.d.DB.QueryContext(ctx,
		`SELECT l.habit_id, h.name, h.unit, l.at, l.amount
		 FROM habit_logs l JOIN habits h ON h.id = l.habit_id
		 WHERE l.at >= ? AND l.at < ? ORDER BY l.at LIMIT 5000`, from.UTC(), until.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type group struct {
		id     int64
		name   string
		unit   string
		last   time.Time
		count  int
		amount float64
		day    string
	}
	var order []string
	groups := map[string]*group{}
	for rows.Next() {
		var id int64
		var name, unit string
		var at time.Time
		var amount float64
		if err := rows.Scan(&id, &name, &unit, &at, &amount); err != nil {
			return nil, err
		}
		day := at.In(loc).Format("2006-01-02")
		key := fmt.Sprintf("%d:%s", id, day)
		g := groups[key]
		if g == nil {
			g = &group{id: id, name: name, unit: unit, day: day}
			groups[key] = g
			order = append(order, key)
		}
		g.count++
		g.amount += amount
		g.last = at
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []contracts.Activity
	for _, key := range order {
		g := groups[key]
		detail := fmt.Sprintf("%s %s", strconv.FormatFloat(g.amount, 'f', -1, 64), g.unit)
		if g.count > 1 {
			detail = fmt.Sprintf("打卡 %d 次，共 %s", g.count, detail)
		}
		out = append(out, contracts.Activity{
			Ref: "habit:" + key, Module: "habits", Kind: "habit", At: g.last,
			Title: "习惯打卡：" + g.name, Detail: strings.TrimSpace(detail), Link: "/habits",
		})
	}
	return out, nil
}

func (s activitySource) workouts(ctx context.Context, from, until time.Time) ([]contracts.Activity, error) {
	rows, err := s.m.d.DB.QueryContext(ctx,
		`SELECT id, items, duration_minutes, note, created_at FROM workout_logs
		 WHERE created_at >= ? AND created_at < ? ORDER BY created_at LIMIT 2000`, from.UTC(), until.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contracts.Activity
	for rows.Next() {
		var id, minutes int64
		var raw, note string
		var at time.Time
		if err := rows.Scan(&id, &raw, &minutes, &note, &at); err != nil {
			return nil, err
		}
		var items []struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal([]byte(raw), &items)
		var names []string
		for _, it := range items {
			if it.Name != "" {
				names = append(names, it.Name)
			}
		}
		title := "训练"
		if minutes > 0 {
			title = fmt.Sprintf("训练 %d 分钟", minutes)
		}
		detail := strings.Join(names, "、")
		if note = strings.TrimSpace(note); note != "" {
			detail = strings.TrimSpace(detail + " · " + note)
		}
		out = append(out, contracts.Activity{
			Ref: fmt.Sprintf("workout:%d", id), Module: "habits", Kind: "workout", At: at, Minutes: int(minutes),
			Title: title, Detail: detail, Link: "/habits/fitness",
		})
	}
	return out, rows.Err()
}
