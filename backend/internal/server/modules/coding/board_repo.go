package coding

import (
	"context"
	"database/sql"
	"net/http"
	"slices"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/db"
)

// ResolveBoardRepo checks permission before cloning anything. Authorization
// belongs to the registered remote, even when cloning onto another runner.
func (m *Module) ResolveBoardRepo(ctx context.Context, aiAgentID int64, runner string, remote contracts.GitRepository) (int64, error) {
	rows, err := m.q.ListRepos(ctx)
	if err != nil {
		return 0, err
	}
	owner, name, _ := strings.Cut(remote.FullName, "/")
	var seed *db.CodingRepo
	for _, row := range rows {
		if row.ConnectionID != nil && *row.ConnectionID == remote.ConnectionID && row.Owner == owner && row.Repo == name {
			a, err := m.aiAgent(ctx, aiAgentID, row.ID)
			if err != nil {
				continue
			}
			if !slices.Contains(a.RepoIDs, row.ID) {
				continue
			}
			copy := row
			seed = &copy
			if runner == "" {
				runner = a.RunnerAgentID
			}
			break
		}
	}
	if seed == nil {
		return 0, httpx.NewError(http.StatusForbidden, "repo_forbidden", "Agent 不能操作看板的仓库，请先在 Agent 设置里允许这个仓库")
	}
	row, err := m.repoOn(ctx, *seed, runner)
	return row.ID, err
}

func (m *Module) CancelCoding(ctx context.Context, id int64) error {
	row, err := m.row(ctx, id)
	if err != nil {
		return err
	}
	if row.Status == statusReview && row.WaitingQuestion != "" {
		return m.discard(ctx, id)
	}
	return m.cancel(ctx, id)
}

func (m *Module) ResumeCoding(ctx context.Context, id int64, answer string) error {
	row, err := m.row(ctx, id)
	if err != nil {
		return err
	}
	if row.Status != statusReview || row.WaitingQuestion == "" {
		return httpx.ErrConflict
	}
	if row.AiAgentID == nil {
		return httpx.ErrConflict
	}
	if _, err = m.aiAgent(ctx, *row.AiAgentID, row.RepoID); err != nil {
		return err
	}
	if _, err = m.d.DB.ExecContext(ctx, "UPDATE coding_tasks SET waiting_question='' WHERE id=?", id); err != nil {
		return err
	}
	err = m.resume(m.runCtx(), row, "继续执行原任务。用户对问题的回答：\n"+answer)
	return err
}

// CodingTasks implements contracts.CodingControl.
func (m *Module) CodingTasks(ctx context.Context, ids []int64) (map[int64]contracts.CodingTaskInfo, error) {
	out := map[int64]contracts.CodingTaskInfo{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id,status,pr_url,error,waiting_question,started_at,finished_at FROM coding_tasks WHERE id IN (?"+strings.Repeat(",?", len(ids)-1)+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var t contracts.CodingTaskInfo
		var started, finished sql.NullTime
		if err := rows.Scan(&id, &t.Status, &t.PrURL, &t.Error, &t.WaitingQuestion, &started, &finished); err != nil {
			return nil, err
		}
		if started.Valid {
			t.StartedAt = &started.Time
		}
		if finished.Valid {
			t.FinishedAt = &finished.Time
		}
		out[id] = t
	}
	return out, rows.Err()
}

// CodingTaskEvents implements contracts.CodingControl.
func (m *Module) CodingTaskEvents(ctx context.Context, id, after int64) ([]contracts.CodingTaskEvent, error) {
	rows, err := m.d.DB.QueryContext(ctx, "SELECT seq,at,kind,text FROM coding_task_events WHERE task_id=? AND seq>? AND seq>(SELECT COALESCE(MAX(seq),0)-5000 FROM coding_task_events WHERE task_id=?) ORDER BY seq LIMIT 5000", id, after, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.CodingTaskEvent{}
	for rows.Next() {
		var e contracts.CodingTaskEvent
		if err := rows.Scan(&e.Seq, &e.At, &e.Kind, &e.Text); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
