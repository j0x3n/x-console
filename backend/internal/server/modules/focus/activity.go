package focus

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// activitySource tells the journal (B118) about the focus sessions that
// started in a range. Sessions still running are left out.
type activitySource struct{ m *Module }

func (s activitySource) Activity(ctx context.Context, from, until time.Time) ([]contracts.Activity, error) {
	rows, err := s.m.d.DB.QueryContext(ctx,
		`SELECT id, issue_key, started_at, planned_minutes, actual_seconds, completed, note
		 FROM focus_sessions
		 WHERE started_at >= ? AND started_at < ? AND ended_at IS NOT NULL AND actual_seconds >= 60
		 ORDER BY started_at LIMIT 2000`, from.UTC(), until.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contracts.Activity
	for rows.Next() {
		var id, planned, seconds, completed int64
		var issue, note string
		var at time.Time
		if err := rows.Scan(&id, &issue, &at, &planned, &seconds, &completed, &note); err != nil {
			return nil, err
		}
		minutes := int(seconds / 60)
		var parts []string
		if issue != "" {
			parts = append(parts, issue)
		}
		if completed == 0 {
			parts = append(parts, fmt.Sprintf("计划 %d 分钟，提前结束", planned))
		}
		if note = strings.TrimSpace(note); note != "" {
			parts = append(parts, note)
		}
		out = append(out, contracts.Activity{
			Ref: fmt.Sprintf("focus:%d", id), Module: "calendar", Kind: "focus", At: at, Minutes: minutes,
			Title: fmt.Sprintf("专注 %d 分钟", minutes), Detail: strings.Join(parts, " · "), Link: "/calendar/focus",
		})
	}
	return out, rows.Err()
}
