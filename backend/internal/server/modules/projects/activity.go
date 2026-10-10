package projects

import (
	"context"
	"fmt"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// activitySource tells the journal (B118) which cards were finished.
type activitySource struct{ m *Module }

func (s activitySource) Activity(ctx context.Context, from, until time.Time) ([]contracts.Activity, error) {
	rows, err := s.m.d.DB.QueryContext(ctx,
		`SELECT p.key, i.number, i.title, i.completed_at
		 FROM issues i JOIN projects p ON p.id = i.project_id
		 WHERE i.status = 'done' AND i.completed_at >= ? AND i.completed_at < ?
		 ORDER BY i.completed_at LIMIT 2000`, from.UTC(), until.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contracts.Activity
	for rows.Next() {
		var key, title string
		var number int64
		var at time.Time
		if err := rows.Scan(&key, &number, &title, &at); err != nil {
			return nil, err
		}
		issue := issueKey(key, number)
		out = append(out, contracts.Activity{
			Ref: "issue:" + issue, Module: "projects", Kind: "card", At: at,
			Title: "完成 " + issue + " " + title, Link: fmt.Sprintf("/projects/%s/%d", key, number),
		})
	}
	return out, rows.Err()
}
