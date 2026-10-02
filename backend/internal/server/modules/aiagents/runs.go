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
	m.notifyRun(ctx, id, "received", "任务已经排队", nil)
	return id, nil
}

// LinkRunTask implements contracts.AgentRuns.
func (m *Module) LinkRunTask(ctx context.Context, runID, agentID, taskID int64) error {
	_, err := m.d.DB.ExecContext(ctx, "UPDATE ai_agent_runs SET task_id=? WHERE id=? AND agent_id=?", taskID, runID, agentID)
	return err
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
	var previous string
	_ = m.d.DB.QueryRowContext(ctx, "SELECT status FROM ai_agent_runs WHERE id=?", id).Scan(&previous)
	var finished *time.Time
	if finalRunStatus(status) {
		now := m.now()
		finished = &now
	}
	_, err := m.d.DB.ExecContext(ctx, `UPDATE ai_agent_runs SET status=?,summary=?,started_at=CASE WHEN ?='running' THEN COALESCE(started_at,?) ELSE started_at END,finished_at=? WHERE id=?`, status, summary, status, m.now(), finished, id)
	if err != nil {
		m.d.Log.Warn("aiagents: run status", "err", err)
		return
	}
	m.appendRunEvent(ctx, id, "status", status, "", nil)
	if previous != status {
		kind := status
		switch status {
		case "running":
			if previous == "waiting" {
				return
			}
			kind = "started"
		case "waiting":
			return
		case "canceled":
			kind = "failed"
		}
		m.notifyRun(ctx, id, kind, summary, nil)
	}
}

// finishRun ends a run once. A user's cancel and the job's own ending can
// both arrive; the later one is dropped, so the log and notification stay single.
func (m *Module) finishRun(ctx context.Context, id int64, status, summary string) {
	m.finishMu.Lock()
	defer m.finishMu.Unlock()
	var current string
	if err := m.d.DB.QueryRowContext(ctx, "SELECT status FROM ai_agent_runs WHERE id=?", id).Scan(&current); err == nil && finalRunStatus(current) {
		return
	}
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
	if status == "waiting" && t.Error != "" {
		m.appendRunEvent(ctx, id, "error", t.Error, "", nil)
		m.notifyRun(ctx, id, "failed", t.Error, nil)
		return
	}
	if status == "waiting" && t.Status == "review" {
		if info, err := m.codingTasks(ctx, []int64{t.ID}); err == nil && info[t.ID].WaitingQuestion == "" {
			m.notifyRun(ctx, id, "decision", "代码已改好，等你审查", nil)
		}
	}
}

func finalRunStatus(s string) bool {
	return s == "done" || s == "failed" || s == "canceled" || s == "pr_opened"
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

const runSelect = `SELECT r.id,r.agent_id,a.name,r.issue_key,r.issue_title,r.kind,r.status,r.task_id,r.pr_url,r.summary,
 r.created_at,r.started_at,r.finished_at
 FROM ai_agent_runs r JOIN ai_agents a ON a.id=r.agent_id`

func (m *Module) codingControl() (contracts.CodingControl, error) {
	c, ok := module.Lookup[contracts.CodingControl](m.d.Registry, contracts.CodingControlKey)
	if !ok {
		return nil, httpx.ErrNotLive
	}
	return c, nil
}

func (m *Module) codingTasks(ctx context.Context, ids []int64) (map[int64]contracts.CodingTaskInfo, error) {
	if len(ids) == 0 {
		return map[int64]contracts.CodingTaskInfo{}, nil
	}
	c, err := m.codingControl()
	if err != nil {
		return nil, err
	}
	return c.CodingTasks(ctx, ids)
}

func (m *Module) runList(ctx context.Context, where string, args []any, limit int) ([]api.AiAgentRun, error) {
	args = append(args, limit)
	rows, err := m.d.DB.QueryContext(ctx, runSelect+" WHERE "+where+" ORDER BY r.id DESC LIMIT ?", args...)
	if err != nil {
		return nil, err
	}
	type runRow struct {
		run          api.AiAgentRun
		url, summary string
	}
	var list []runRow
	var taskIDs []int64
	for rows.Next() {
		var x runRow
		var task sql.NullInt64
		var started, finished sql.NullTime
		if err := rows.Scan(&x.run.Id, &x.run.AgentId, &x.run.AgentName, &x.run.IssueKey, &x.run.IssueTitle, &x.run.Kind, &x.run.Status, &task, &x.url, &x.summary, &x.run.CreatedAt, &started, &finished); err != nil {
			rows.Close()
			return nil, err
		}
		if task.Valid {
			x.run.TaskId = &task.Int64
			taskIDs = append(taskIDs, task.Int64)
		}
		if started.Valid {
			x.run.StartedAt = &started.Time
		}
		if finished.Valid {
			x.run.FinishedAt = &finished.Time
		}
		list = append(list, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// A CLI run shows its coding task while the task still exists.
	tasks, err := m.codingTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	out := make([]api.AiAgentRun, 0, len(list))
	for _, x := range list {
		r := x.run
		if r.TaskId != nil {
			if t, ok := tasks[*r.TaskId]; ok {
				r.Status = api.AiAgentRunStatus(t.Status)
				x.url = t.PrURL
				if t.Error != "" {
					x.summary = t.Error
				}
				if t.StartedAt != nil {
					r.StartedAt = t.StartedAt
				}
				if t.FinishedAt != nil {
					r.FinishedAt = t.FinishedAt
				}
			}
		}
		r.Status = api.AiAgentRunStatus(runTaskStatus(string(r.Status)))
		if x.url != "" {
			r.PrUrl = &x.url
		}
		if x.summary != "" {
			r.Summary = &x.summary
		}
		out = append(out, r)
	}
	return out, nil
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
	items := []api.AiAgentRunEvent{}
	seq := after
	add := func(e api.AiAgentRunEvent) {
		if !e.Kind.Valid() {
			e.Kind = api.Text
		}
		items = append(items, e)
		seq = e.Seq
	}
	if run.TaskId != nil {
		c, err := m.codingControl()
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		events, err := c.CodingTaskEvents(ctx, *run.TaskId, after)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		for _, e := range events {
			add(api.AiAgentRunEvent{Seq: e.Seq, At: e.At, Kind: api.AiAgentRunEventKind(e.Kind), Text: e.Text})
		}
		httpx.JSON(w, 200, map[string]any{"items": items, "lastSeq": seq})
		return
	}
	rows, err := m.d.DB.QueryContext(ctx, "SELECT seq,at,kind,text,tool,ok FROM ai_agent_run_events WHERE run_id=? AND seq>? ORDER BY seq LIMIT 5000", id, after)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var e api.AiAgentRunEvent
		var tool string
		var ok sql.NullBool
		if err := rows.Scan(&e.Seq, &e.At, &e.Kind, &e.Text, &tool, &ok); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if tool != "" {
			e.Tool = &tool
		}
		if ok.Valid {
			e.Ok = &ok.Bool
		}
		add(e)
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
	_, _ = m.d.DB.ExecContext(ctx, "UPDATE ai_agent_decisions SET status='canceled' WHERE run_id=? AND status='pending'", id)
	m.d.Bus.Publish("ai_agent.decision", map[string]any{"runId": id})
	httpx.NoContent(w)
}

// A service restart terminates built-in jobs; CLI jobs follow coding recovery.
func (m *Module) recoverRuns(ctx context.Context) error {
	_, _ = m.d.DB.ExecContext(ctx, `UPDATE ai_agent_decisions SET status='canceled' WHERE run_id IN (SELECT id FROM ai_agent_runs WHERE kind='builtin') AND status='pending'`)
	_, err := m.d.DB.ExecContext(ctx, "UPDATE ai_agent_runs SET status='failed',summary='服务重启，执行已停止',finished_at=? WHERE kind='builtin' AND status IN ('queued','running','waiting')", m.now())
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
