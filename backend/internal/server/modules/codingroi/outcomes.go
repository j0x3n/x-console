package codingroi

import (
	"context"
	"database/sql"
	"time"
)

// recordOutcome remembers that the pull request of a task was merged or closed.
// A merge replaces an earlier "closed" (a pull request can be reopened and
// merged), but a closed never replaces a merge.
func (m *Module) recordOutcome(ctx context.Context, taskID int64, outcome string, at time.Time, source, prURL string) error {
	_, err := m.d.DB.ExecContext(ctx, `INSERT INTO codingroi_outcomes (task_id, outcome, at, source, pr_url) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(task_id) DO UPDATE SET outcome = excluded.outcome, at = excluded.at, source = excluded.source, pr_url = excluded.pr_url
WHERE excluded.outcome = 'merged' AND codingroi_outcomes.outcome <> 'merged'`,
		taskID, outcome, at.UTC(), source, prURL)
	return err
}

// syncPulls looks up the pull requests of tasks that have no result yet in
// the GitHub cache (github_cache_v2). The cache keeps closed pull requests for 7 days, so this
// runs often and the result is kept here.
func (m *Module) syncPulls(ctx context.Context) error {
	rows, err := m.d.DB.QueryContext(ctx, `SELECT id, pr_url FROM coding_tasks
WHERE status = 'pr_opened' AND pr_url <> '' AND id NOT IN (SELECT task_id FROM codingroi_outcomes)`)
	if err != nil {
		return err
	}
	type pending struct {
		id  int64
		url string
	}
	var list []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.url); err != nil {
			rows.Close()
			return err
		}
		list = append(list, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, p := range list {
		var state, updated string
		err := m.d.DB.QueryRowContext(ctx, `SELECT COALESCE(json_extract(data, '$.state'), ''), COALESCE(json_extract(data, '$.updatedAt'), '')
FROM github_cache_v2 WHERE kind = 'pull' AND json_extract(data, '$.url') = ? LIMIT 1`, p.url).Scan(&state, &updated)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		at, perr := time.Parse(time.RFC3339Nano, updated)
		if perr != nil {
			at = m.now()
		}
		switch state {
		case "merged":
			err = m.recordOutcome(ctx, p.id, "merged", at, "github", p.url)
		case "closed":
			err = m.recordOutcome(ctx, p.id, "closed", at, "github", p.url)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
