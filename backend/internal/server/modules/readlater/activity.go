package readlater

import (
	"context"
	"fmt"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// activitySource tells the journal (B118) which links were saved and which
// were read.
type activitySource struct{ m *Module }

func (s activitySource) Activity(ctx context.Context, from, until time.Time) ([]contracts.Activity, error) {
	rows, err := s.m.d.DB.QueryContext(ctx,
		`SELECT id, title, site, created_at, read_at FROM read_items
		 WHERE (created_at >= ?1 AND created_at < ?2) OR (read_at >= ?1 AND read_at < ?2)
		 ORDER BY id LIMIT 2000`, from.UTC(), until.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contracts.Activity
	for rows.Next() {
		var id int64
		var title, site string
		var created time.Time
		var readAt *time.Time
		if err := rows.Scan(&id, &title, &site, &created, &readAt); err != nil {
			return nil, err
		}
		if !created.Before(from) && created.Before(until) {
			out = append(out, contracts.Activity{
				Ref: fmt.Sprintf("saved:%d", id), Module: "readlater", Kind: "link", At: created,
				Title: "存了链接：" + title, Detail: site, Link: "/readlater?view=all",
			})
		}
		if readAt != nil && !readAt.Before(from) && readAt.Before(until) {
			out = append(out, contracts.Activity{
				Ref: fmt.Sprintf("read:%d", id), Module: "readlater", Kind: "link", At: *readAt,
				Title: "读完：" + title, Detail: site, Link: "/readlater?view=all",
			})
		}
	}
	return out, rows.Err()
}
