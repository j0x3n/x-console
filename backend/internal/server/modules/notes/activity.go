package notes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// activitySource tells the journal (B118) which notes were created.
type activitySource struct{ m *Module }

func (s activitySource) Activity(ctx context.Context, from, until time.Time) ([]contracts.Activity, error) {
	rows, err := s.m.d.DB.QueryContext(ctx,
		`SELECT id, title, created_at FROM notes WHERE created_at >= ? AND created_at < ? ORDER BY created_at LIMIT 2000`,
		from.UTC(), until.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contracts.Activity
	for rows.Next() {
		var id int64
		var title string
		var at time.Time
		if err := rows.Scan(&id, &title, &at); err != nil {
			return nil, err
		}
		title = strings.TrimSpace(title)
		if title == "" {
			title = "无标题"
		}
		out = append(out, contracts.Activity{
			Ref: fmt.Sprintf("note:%d", id), Module: "notes", Kind: "note", At: at,
			Title: "新建笔记：" + title, Link: fmt.Sprintf("/notes/%d", id),
		})
	}
	return out, rows.Err()
}
