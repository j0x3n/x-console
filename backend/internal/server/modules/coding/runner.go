package coding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// taskRun is a task whose coding.run stream is open.
type taskRun struct {
	id       int64
	executor string
	model    string // reported by the executor when the session starts
	started  time.Time
	stream   *rpc.Stream
	seq      int64
	canceled atomic.Bool
	once     sync.Once
}

// running reports how many tasks have an open stream.
func (m *Module) running() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.runs)
}

func (m *Module) getRun(id int64) *taskRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runs[id]
}

// dispatch starts queued tasks, oldest first, while fewer than the limit
// run. Tasks of offline agents stay queued and do not block others.
func (m *Module) dispatch() {
	m.dispatchMu.Lock()
	defer m.dispatchMu.Unlock()
	ctx := m.runCtx()
	if ctx == nil || ctx.Err() != nil {
		return
	}
	limit := m.maxConcurrent(ctx)
	if m.running() >= limit {
		return
	}
	agents, _ := module.Lookup[contracts.AIAgents](m.d.Registry, contracts.AIAgentsKey)
	queued, err := m.q.ListQueued(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("coding: list queued tasks", "err", err)
		}
		return
	}
	for _, q := range queued {
		if m.running() >= limit {
			return
		}
		if !m.d.Agents.Online(q.AgentID) {
			continue
		}
		if q.AiAgentID != nil && agents != nil && !m.aiAgentFree(ctx, agents, *q.AiAgentID) {
			continue
		}
		m.start(ctx, q.ID)
	}
}

// start moves a queued task to running and opens its stream.
func (m *Module) start(ctx context.Context, id int64) {
	r, err := m.row(ctx, id)
	if err != nil {
		slog.Error("coding: load task", "task", id, "err", err)
		return
	}
	now := m.now()
	if n, err := m.q.StartTask(ctx, db.StartTaskParams{Now: &now, ID: id}); err != nil || n == 0 {
		return
	}
	params := protocol.CodingRunParams{
		TaskID: r.ID, RepoPath: r.RepoPath, Executor: r.Executor, Prompt: r.Prompt,
		BaseBranch: r.BaseBranch, Branch: r.Branch, TimeoutSeconds: int(time.Duration(r.TimeoutMinutes) * m.timeoutUnit / time.Second),
		Model: r.Model, Permission: r.Permission,
	}
	if r.RepoConnectionID != nil {
		// B47: fetch first, then branch from origin/<base>.
		if _, err := m.fetchForTask(ctx, r); err != nil {
			m.finish(ctx, id, statusFailed, nil, "更新仓库失败："+err.Error(), nil)
			return
		}
		params.PreferRemote = true
	}
	st, err := m.d.Agents.Open(ctx, r.AgentID, protocol.MethodCodingRun, params)
	if err != nil {
		m.finish(ctx, id, statusFailed, nil, "无法在代理上启动任务："+err.Error(), nil)
		return
	}
	run := &taskRun{id: id, stream: st, executor: r.Executor, started: m.now()}
	m.mu.Lock()
	m.runs[id] = run
	m.mu.Unlock()
	if t, err := m.task(ctx, id); err == nil {
		m.d.Bus.Publish("coding_task.updated", t)
	}
	m.linkIssueOnStart(ctx, r)
	m.wg.Add(1)
	go m.consume(ctx, run, r)
}

// linkIssueOnStart moves the issue to in_progress and links the task.
func (m *Module) linkIssueOnStart(ctx context.Context, r taskRow) {
	if r.IssueKey == "" {
		return
	}
	issues, ok := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey)
	if !ok {
		return
	}
	if err := issues.SetStatus(ctx, r.IssueKey, "in_progress"); err != nil {
		slog.Warn("coding: set issue status", "issue", r.IssueKey, "err", err)
	}
	link := contracts.IssueLink{Kind: "coding_task", Title: "编码任务 #" + itoa(r.ID) + " " + shorten(titleOf(stripIssueHeader(r.Prompt, r.IssueKey)), 60),
		URL: "/coding/" + itoa(r.ID), Ref: itoa(r.ID)}
	if err := issues.AttachLink(ctx, r.IssueKey, link); err != nil {
		slog.Warn("coding: link issue", "issue", r.IssueKey, "err", err)
	}
}

// consume reads the stream until it ends, storing and publishing events
// in batches, then records how the task ended.
func (m *Module) consume(ctx context.Context, run *taskRun, r taskRow) {
	defer m.wg.Done()
	events := make(chan protocol.CodingEvent, 256)
	recvErr := make(chan error, 1)
	go func() {
		defer close(events)
		for {
			chunk, err := run.stream.Recv(ctx)
			if err != nil {
				recvErr <- err
				return
			}
			for _, line := range bytes.Split(chunk, []byte("\n")) {
				if len(bytes.TrimSpace(line)) == 0 {
					continue
				}
				var ev protocol.CodingEvent
				if err := json.Unmarshal(line, &ev); err != nil || ev.Kind == "" {
					ev = protocol.CodingEvent{Kind: protocol.CodingEventText, Text: string(line), At: m.now()}
				}
				events <- ev
			}
		}
	}()

	ticker := time.NewTicker(m.flushEvery)
	defer ticker.Stop()
	deadline := time.NewTimer(time.Duration(r.TimeoutMinutes)*m.timeoutUnit + m.overtime)
	defer deadline.Stop()
	var pending []protocol.CodingEvent
	var done *protocol.CodingEvent
	overdue := false
loop:
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				break loop
			}
			pending = append(pending, ev)
			if ev.Kind == protocol.CodingEventDone {
				done = &ev
			}
		case <-ticker.C:
			pending = m.flush(ctx, run, pending)
		case <-deadline.C:
			overdue = true
			run.stream.Close(nil)
		}
	}
	err := <-recvErr
	run.stream.Close(nil)
	if ctx.Err() != nil {
		return // server shutting down; the next start marks the task failed
	}
	m.flush(ctx, run, pending)

	status, errText, files := statusFailed, "", []protocol.CodingChangedFile(nil)
	var exit *int64
	switch {
	case done != nil:
		var d protocol.CodingDone
		_ = json.Unmarshal(done.Data, &d)
		files = d.Files
		code := 0
		if done.ExitCode != nil {
			code = *done.ExitCode
		}
		c64 := int64(code)
		exit = &c64
		switch d.Reason {
		case protocol.CodingEndExited:
			if code == 0 {
				status = statusReview
			} else {
				errText = fmt.Sprintf("执行器退出码是 %d。", code)
			}
		case protocol.CodingEndTimeout:
			errText = fmt.Sprintf("超过 %d 分钟还没完成，已经停止。", r.TimeoutMinutes)
		case protocol.CodingEndCanceled:
			if run.canceled.Load() {
				status, errText = statusCanceled, "已取消。"
			} else {
				errText = "代理中止了任务。"
			}
		case protocol.CodingEndStartFailed:
			errText = "无法启动：" + d.Error
		default:
			errText = "代理返回了未知的结束原因：" + d.Reason
		}
	case run.canceled.Load():
		status, errText = statusCanceled, "已取消。代理没有及时回应，服务端已断开。"
	case overdue:
		errText = "超时后代理一直没有结束任务，服务端已断开。"
	case errors.Is(err, rpc.ErrClosed):
		errText = "代理断开了连接，任务中止。"
	case errors.Is(err, io.EOF):
		errText = "代理没有报告结果就结束了任务。"
	default:
		errText = "任务中止：" + err.Error()
	}
	m.finish(ctx, r.ID, status, exit, errText, files)
	m.mu.Lock()
	delete(m.runs, r.ID)
	m.mu.Unlock()
	go m.dispatch()
}

// flush stores pending events and publishes them as one coding_task.output.
func (m *Module) flush(ctx context.Context, run *taskRun, pending []protocol.CodingEvent) []protocol.CodingEvent {
	if len(pending) == 0 {
		return pending
	}
	out := make([]api.TaskEvent, 0, len(pending))
	seqBefore := run.seq
	err := func() error {
		tx, err := m.d.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		q := m.q.WithTx(tx)
		for _, ev := range pending {
			run.seq++
			at := ev.At
			if at.IsZero() {
				at = m.now()
			}
			data := storedEventData(ev)
			row := db.CodingTaskEvent{TaskID: run.id, Seq: run.seq, At: at.UTC(), Kind: ev.Kind, Text: ev.Text, Data: data}
			if err := q.InsertEvent(ctx, db.InsertEventParams{TaskID: row.TaskID, Seq: row.Seq, At: row.At,
				Kind: row.Kind, Text: row.Text, Data: row.Data}); err != nil {
				return err
			}
			if base := baseCommitOf(ev); base != "" {
				if err := q.SetBaseCommit(ctx, db.SetBaseCommitParams{BaseCommit: base, ID: run.id}); err != nil {
					return err
				}
			}
			out = append(out, toEvent(row))
		}
		return tx.Commit()
	}()
	if err != nil {
		slog.Error("coding: store events", "task", run.id, "err", err)
		run.seq = seqBefore
		return pending[:0]
	}
	m.d.Bus.Publish("coding_task.output", map[string]any{"taskId": run.id, "events": out})
	for _, ev := range pending {
		m.noteUsage(ctx, run, ev)
	}
	return pending[:0]
}

// storedEventData is the JSON kept in the data column.
func storedEventData(ev protocol.CodingEvent) string {
	if ev.Kind == protocol.CodingEventDone && ev.ExitCode != nil {
		data := map[string]any{}
		_ = json.Unmarshal(ev.Data, &data)
		data["_exitCode"] = *ev.ExitCode
		raw, _ := json.Marshal(data)
		return string(raw)
	}
	if len(ev.Data) == 0 || !bytes.HasPrefix(bytes.TrimSpace(ev.Data), []byte("{")) {
		return ""
	}
	return string(ev.Data)
}

// baseCommitOf returns the base commit of a worktree_ready status event.
func baseCommitOf(ev protocol.CodingEvent) string {
	if ev.Kind != protocol.CodingEventStatus {
		return ""
	}
	var d struct {
		Code       string `json:"code"`
		BaseCommit string `json:"baseCommit"`
	}
	if json.Unmarshal(ev.Data, &d) != nil || d.Code != "worktree_ready" {
		return ""
	}
	return d.BaseCommit
}

// finish records the end of a queued or running task, publishes it and
// sends the review or failure notification.
func (m *Module) finish(ctx context.Context, id int64, status string, exit *int64, errText string, files []protocol.CodingChangedFile) {
	if files == nil {
		files = []protocol.CodingChangedFile{}
	}
	raw, _ := json.Marshal(files)
	now := m.now()
	n, err := m.q.FinishTask(ctx, db.FinishTaskParams{Status: status, ExitCode: exit, Error: errText,
		ChangedFiles: string(raw), Now: &now, ID: id})
	if err != nil {
		slog.Error("coding: finish task", "task", id, "err", err)
		return
	}
	if n == 0 {
		return
	}
	t, err := m.task(ctx, id)
	if err != nil {
		return
	}
	m.d.Bus.Publish("coding_task.updated", t)
	switch status {
	case statusReview:
		body := t.Title
		if len(files) > 0 {
			body += "\n改了 " + strconv.Itoa(len(files)) + " 个文件。"
		}
		m.notify(ctx, notify.Notification{Kind: "coding_task.review", Title: "编码任务完成了，等你决定",
			Body: body, Link: "/coding/" + itoa(id), Source: "coding", Data: map[string]any{"taskId": id}})
	case statusFailed:
		m.notify(ctx, notify.Notification{Kind: "coding_task.failed", Title: "编码任务失败了",
			Body: t.Title + "\n" + errText, Link: "/coding/" + itoa(id), Source: "coding",
			Priority: notify.PriorityHigh, Data: map[string]any{"taskId": id}})
	}
}

func (m *Module) notify(ctx context.Context, n notify.Notification) {
	if _, err := m.d.Notify.Send(ctx, n); err != nil {
		slog.Warn("coding: notify", "kind", n.Kind, "err", err)
	}
}

// cancelRun asks the agent to stop a running task. When the agent does not
// finish within cancelWait the stream is closed and the task is recorded
// as canceled anyway.
func (m *Module) cancelRun(run *taskRun) {
	run.canceled.Store(true)
	run.once.Do(func() {
		ctx := m.runCtx()
		msg, _ := json.Marshal(protocol.CodingControl{Op: "cancel"})
		if err := run.stream.Send(ctx, msg); err != nil {
			run.stream.Close(nil)
			return
		}
		go func() {
			t := time.NewTimer(m.cancelWait)
			defer t.Stop()
			select {
			case <-t.C:
				run.stream.Close(nil)
			case <-run.stream.Context().Done():
			case <-ctx.Done():
			}
		}()
	})
}

// aiAgentFree reports whether an AI agent may start another task now: it is
// enabled, under budget and below its parallel limit (B47). Its queued
// tasks wait otherwise.
func (m *Module) aiAgentFree(ctx context.Context, agents contracts.AIAgents, id int64) bool {
	a, err := agents.Get(ctx, id)
	if err != nil {
		// Deleted agents keep no tasks queued; anything else waits.
		return false
	}
	if !a.Enabled || a.OverBudget {
		return false
	}
	n, err := m.q.CountRunningForAgent(ctx, &id)
	return err == nil && int(n) < max(a.MaxParallel, 1)
}

// fetchForTask brings the clone of a remote repository up to date.
func (m *Module) fetchForTask(ctx context.Context, r taskRow) (protocol.CodingRepo, error) {
	repo, err := m.q.GetRepo(ctx, r.RepoID)
	if err != nil {
		return protocol.CodingRepo{}, err
	}
	fctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	return m.ensureClone(fctx, repo)
}
