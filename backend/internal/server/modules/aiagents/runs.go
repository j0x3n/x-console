package aiagents

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/api"
)

func (m *Module) createRun(ctx context.Context, a contracts.AIAgent, b contracts.IssueBrief, taskID *int64) (int64, error) {
	var id int64
	err := m.d.DB.QueryRowContext(ctx, `INSERT INTO ai_agent_runs(agent_id,issue_key,issue_title,kind,task_id,created_at) VALUES(?,?,?,?,?,?) RETURNING id`, a.ID, b.Key, b.Title, a.Kind, taskID, m.now()).Scan(&id)
	if err != nil {
		return 0, err
	}
	m.pruneRuns(ctx)
	m.d.Bus.Publish("ai_agent.run_event", map[string]any{"runId": id})
	return id, nil
}

func (m *Module) pruneRuns(ctx context.Context) {
	_, err := m.d.DB.ExecContext(ctx, `DELETE FROM ai_agent_runs WHERE status NOT IN ('queued','running','waiting') AND id NOT IN (SELECT id FROM ai_agent_runs ORDER BY id DESC LIMIT 200)`)
	if err != nil {
		m.d.Log.Warn("aiagents: prune runs", "err", err)
	}
}

func (m *Module) appendRunEvent(ctx context.Context, id int64, kind, text, tool string, ok *bool) {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		m.d.Log.Warn("aiagents: event transaction", "err", err)
		return
	}
	defer tx.Rollback()
	var seq int64
	err = tx.QueryRowContext(ctx, `INSERT INTO ai_agent_run_events(run_id,seq,at,kind,text,tool,ok) SELECT ?,COALESCE(MAX(seq),0)+1,?,?,?,?,? FROM ai_agent_run_events WHERE run_id=? RETURNING seq`, id, m.now(), kind, text, tool, ok, id).Scan(&seq)
	if err != nil {
		m.d.Log.Warn("aiagents: record event", "err", err)
		return
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM ai_agent_run_events WHERE run_id=? AND seq<=?", id, seq-5000); err != nil {
		return
	}
	if err = tx.Commit(); err != nil {
		m.d.Log.Warn("aiagents: event commit", "err", err)
		return
	}
	m.d.Bus.Publish("ai_agent.run_event", map[string]any{"runId": id, "seq": seq})
}

type runObserver struct {
	m  *Module
	id int64
}

func (o *runObserver) RecordToolEvent(ctx context.Context, kind, text, tool string, ok bool) {
	tool = strings.ReplaceAll(tool, "__", ".")
	o.m.appendRunEvent(ctx, o.id, kind, text, tool, &ok)
}

func (m *Module) setRunStatus(ctx context.Context, id int64, status, summary string) {
	var finished *time.Time
	if status == "done" || status == "failed" || status == "canceled" || status == "pr_opened" {
		now := m.now()
		finished = &now
	}
	_, err := m.d.DB.ExecContext(ctx, `UPDATE ai_agent_runs SET status=?,summary=?,started_at=CASE WHEN ?='running' THEN COALESCE(started_at,?) ELSE started_at END,finished_at=? WHERE id=?`, status, summary, status, m.now(), finished, id)
	if err != nil {
		m.d.Log.Warn("aiagents: run status", "err", err)
		return
	}
	m.appendRunEvent(ctx, id, "status", status, "", nil)
}

func (m *Module) finishRun(ctx context.Context, id int64, status, summary string) {
	if status == "failed" {
		m.appendRunEvent(ctx, id, "error", summary, "", nil)
	}
	m.setRunStatus(ctx, id, status, summary)
	m.pruneRuns(ctx)
}

func (m *Module) followRunTask(ctx context.Context, t taskEvent) {
	var id int64
	err := m.d.DB.QueryRowContext(ctx, "SELECT id FROM ai_agent_runs WHERE task_id=?", t.ID).Scan(&id)
	if err != nil {
		return
	}
	status := runTaskStatus(t.Status)
	_, _ = m.d.DB.ExecContext(ctx, "UPDATE ai_agent_runs SET pr_url=? WHERE id=?", t.PrURL, id)
	m.setRunStatus(ctx, id, status, t.Error)
}

func runTaskStatus(s string) string {
	switch s {
	case "review", "committed", "pushed":
		return "waiting"
	case "discarded":
		return "canceled"
	default:
		return s
	}
}

const runSelect = `SELECT r.id,r.agent_id,a.name,r.issue_key,r.issue_title,r.kind,
 COALESCE(t.status,r.status),r.task_id,COALESCE(t.pr_url,r.pr_url),CASE WHEN t.error<>'' THEN t.error ELSE r.summary END,
 r.created_at,r.started_at,t.started_at,r.finished_at,t.finished_at
 FROM ai_agent_runs r JOIN ai_agents a ON a.id=r.agent_id LEFT JOIN coding_tasks t ON t.id=r.task_id`

func (m *Module) runList(ctx context.Context, where string, args []any, limit int) ([]api.AiAgentRun, error) {
	args = append(args, limit)
	rows, err := m.d.DB.QueryContext(ctx, runSelect+" WHERE "+where+" ORDER BY r.id DESC LIMIT ?", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []api.AiAgentRun{}
	for rows.Next() {
		var r api.AiAgentRun
		var task sql.NullInt64
		var started, finished, taskStarted, taskFinished sql.NullTime
		var url, summary string
		if err := rows.Scan(&r.Id, &r.AgentId, &r.AgentName, &r.IssueKey, &r.IssueTitle, &r.Kind, &r.Status, &task, &url, &summary, &r.CreatedAt, &started, &taskStarted, &finished, &taskFinished); err != nil {
			return nil, err
		}
		r.Status = api.AiAgentRunStatus(runTaskStatus(string(r.Status)))
		if taskStarted.Valid {
			started = taskStarted
		}
		if taskFinished.Valid {
			finished = taskFinished
		}
		if task.Valid {
			r.TaskId = &task.Int64
		}
		if started.Valid {
			r.StartedAt = &started.Time
		}
		if finished.Valid {
			r.FinishedAt = &finished.Time
		}
		if url != "" {
			r.PrUrl = &url
		}
		if summary != "" {
			r.Summary = &summary
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (m *Module) run(ctx context.Context, id int64) (api.AiAgentRun, error) {
	rows, err := m.runList(ctx, "r.id=?", []any{id}, 1)
	if err != nil {
		return api.AiAgentRun{}, err
	}
	if len(rows) == 0 {
		return api.AiAgentRun{}, httpx.ErrNotFound
	}
	return rows[0], nil
}

func (m *Module) ListAiAgentRuns(w http.ResponseWriter, r *http.Request, p api.ListAiAgentRunsParams) {
	where := "1=1"
	args := []any{}
	if p.AgentId != nil {
		where += " AND r.agent_id=?"
		args = append(args, *p.AgentId)
	}
	if p.IssueKey != nil {
		where += " AND r.issue_key=?"
		args = append(args, strings.ToUpper(*p.IssueKey))
	}
	limit := 50
	if p.Limit != nil {
		limit = min(max(int(*p.Limit), 1), 200)
	}
	out, err := m.runList(r.Context(), where, args, limit)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, out)
}

func (m *Module) ListAiAgentRunEvents(w http.ResponseWriter, r *http.Request, id int64, p api.ListAiAgentRunEventsParams) {
	ctx := r.Context()
	run, err := m.run(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	after := int64(0)
	if p.After != nil {
		after = max(*p.After, 0)
	}
	query := "SELECT seq,at,kind,text,tool,ok FROM ai_agent_run_events WHERE run_id=? AND seq>? ORDER BY seq LIMIT 5000"
	key := id
	if run.TaskId != nil {
		query = "SELECT seq,at,kind,text,'' AS tool,NULL AS ok FROM coding_task_events WHERE task_id=? AND seq>? AND seq>(SELECT COALESCE(MAX(seq),0)-5000 FROM coding_task_events WHERE task_id=?) ORDER BY seq LIMIT 5000"
		key = *run.TaskId
	}
	args := []any{key, after}
	if run.TaskId != nil {
		args = append(args, key)
	}
	rows, err := m.d.DB.QueryContext(ctx, query, args...)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	items := []api.AiAgentRunEvent{}
	seq := after
	for rows.Next() {
		var e api.AiAgentRunEvent
		var tool string
		var ok sql.NullBool
		if err := rows.Scan(&e.Seq, &e.At, &e.Kind, &e.Text, &tool, &ok); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if !e.Kind.Valid() {
			e.Kind = api.Text
		}
		if tool != "" {
			e.Tool = &tool
		}
		if ok.Valid {
			e.Ok = &ok.Bool
		}
		items = append(items, e)
		seq = e.Seq
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"items": items, "lastSeq": seq})
}

func (m *Module) CancelAiAgentRun(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	run, err := m.run(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if run.Status != api.Queued && run.Status != api.Running && run.Status != api.Waiting {
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	if run.TaskId != nil {
		control, ok := module.Lookup[contracts.CodingControl](m.d.Registry, contracts.CodingControlKey)
		if !ok {
			httpx.Fail(w, r, httpx.ErrNotLive)
			return
		}
		err = control.CancelCoding(ctx, *run.TaskId)
	} else {
		m.runMu.Lock()
		cancel := m.activeRuns[id]
		m.runMu.Unlock()
		if cancel == nil {
			err = httpx.ErrConflict
		} else {
			cancel()
			m.finishRun(ctx, id, "canceled", "用户中断了任务")
		}
	}
	m.d.Audit.Record(ctx, "ai_agent.run_cancel", fmt.Sprint(id), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// A service restart terminates built-in jobs; CLI jobs follow coding recovery.
func (m *Module) recoverRuns(ctx context.Context) error {
	_, err := m.d.DB.ExecContext(ctx, "UPDATE ai_agent_runs SET status='failed',summary='服务重启，执行已停止',finished_at=? WHERE kind='builtin' AND status IN ('queued','running','waiting')", m.now())
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
