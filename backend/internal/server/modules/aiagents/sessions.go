package aiagents

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// SessionRuns 复用现有状态换算，保留编码任务的实时状态。
func (m *Module) SessionRuns(ctx context.Context) ([]contracts.AISessionRun, error) {
	// 最近的 300 次执行就够列表用了，排队和执行中的都是新的。
	runs, err := m.runList(ctx, "1=1", nil, 300)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.AISessionRun, 0, len(runs))
	indexes := map[int64]int{}
	for _, r := range runs {
		x := contracts.AISessionRun{ID: r.Id, AgentID: r.AgentId, AgentName: r.AgentName, AgentKind: string(r.Kind), IssueKey: r.IssueKey, IssueTitle: r.IssueTitle, Status: string(r.Status), TaskID: r.TaskId, UpdatedAt: r.CreatedAt}
		if r.Summary != nil {
			x.Summary = *r.Summary
		}
		for _, at := range []*time.Time{r.StartedAt, r.FinishedAt} {
			if at != nil && at.After(x.UpdatedAt) {
				x.UpdatedAt = *at
			}
		}
		indexes[r.Id] = len(out)
		out = append(out, x)
	}
	rows, err := m.d.DB.QueryContext(ctx, `SELECT r.id,a.avatar,a.color,
 (SELECT count(*) FROM ai_agent_decisions d WHERE d.run_id=r.id AND d.status='pending')
 FROM ai_agent_runs r JOIN ai_agents a ON a.id=r.agent_id ORDER BY r.id DESC LIMIT 300`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var avatar, color string
		var count int
		if err = rows.Scan(&id, &avatar, &color, &count); err != nil {
			break
		}
		if i, ok := indexes[id]; ok {
			out[i].AgentAvatar, out[i].AgentColor, out[i].PendingCount = avatar, color, count
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	// 直接读时间列，保留 SQLite 驱动的 DATETIME 转换。
	rows, err = m.d.DB.QueryContext(ctx, `SELECT run_id,at FROM ai_agent_run_events WHERE (run_id,seq) IN (SELECT run_id,max(seq) FROM ai_agent_run_events WHERE run_id >= (SELECT COALESCE(min(id),0) FROM (SELECT id FROM ai_agent_runs ORDER BY id DESC LIMIT 300)) GROUP BY run_id)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		if i, ok := indexes[id]; ok && at.After(out[i].UpdatedAt) {
			out[i].UpdatedAt = at
		}
	}
	return out, rows.Err()
}
