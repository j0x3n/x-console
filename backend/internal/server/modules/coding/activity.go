package coding

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// activitySource tells the journal (B118) which agent tasks ended.
type activitySource struct{ m *Module }

var taskStatusLabels = map[string]string{
	"review": "等你审查", "failed": "失败", "canceled": "已取消", "committed": "已提交",
	"pushed": "已推送", "pr_opened": "已开 PR", "discarded": "已丢弃",
}

func (s activitySource) Activity(ctx context.Context, from, until time.Time) ([]contracts.Activity, error) {
	rows, err := s.m.d.DB.QueryContext(ctx,
		`SELECT t.id, t.executor, t.prompt, t.status, t.commit_sha, t.pr_url, t.finished_at, COALESCE(r.name, '')
		 FROM coding_tasks t LEFT JOIN coding_repos r ON r.id = t.repo_id
		 WHERE t.finished_at >= ? AND t.finished_at < ?
		 ORDER BY t.finished_at LIMIT 2000`, from.UTC(), until.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contracts.Activity
	for rows.Next() {
		var id int64
		var executor, prompt, status, sha, pr, repo string
		var at time.Time
		if err := rows.Scan(&id, &executor, &prompt, &status, &sha, &pr, &at, &repo); err != nil {
			return nil, err
		}
		title := strings.TrimSpace(strings.SplitN(strings.TrimSpace(prompt), "\n", 2)[0])
		if title == "" {
			title = "Agent 任务"
		}
		label := taskStatusLabels[status]
		if label == "" {
			label = status
		}
		parts := []string{}
		if repo != "" {
			parts = append(parts, repo)
		}
		parts = append(parts, label)
		if len(sha) >= 7 {
			parts = append(parts, "提交 "+sha[:7])
		}
		if pr != "" {
			parts = append(parts, "有 PR")
		}
		out = append(out, contracts.Activity{
			Ref: fmt.Sprintf("task:%d", id), Module: "coding", Kind: "task", At: at,
			Title: executorName(executor) + " 跑完：" + title, Detail: strings.Join(parts, " · "), Link: fmt.Sprintf("/coding/%d", id),
		})
	}
	return out, rows.Err()
}

func executorName(executor string) string {
	switch executor {
	case "claude":
		return "Claude"
	case "codex":
		return "Codex"
	}
	return executor
}
