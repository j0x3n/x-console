package coding

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/db"
)

// taskRow is a task joined with its repository.
type taskRow struct {
	db.CodingTask
	RepoName   string
	RepoPath   string
	AgentID    string
	GithubRepo string
}

func (m *Module) row(ctx context.Context, id int64) (taskRow, error) {
	r, err := m.q.GetTask(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return taskRow{}, httpx.ErrNotFound
	}
	if err != nil {
		return taskRow{}, err
	}
	return taskRow{r.CodingTask, r.RepoName, r.RepoPath, r.AgentID, r.GithubRepo}, nil
}

func (m *Module) task(ctx context.Context, id int64) (api.Task, error) {
	r, err := m.row(ctx, id)
	if err != nil {
		return api.Task{}, err
	}
	return m.toTask(ctx, r), nil
}

func (m *Module) toTask(ctx context.Context, r taskRow) api.Task {
	t := api.Task{
		Id: r.ID, RepoId: r.RepoID, RepoName: r.RepoName, AgentId: r.AgentID, Executor: api.ExecutorName(r.Executor),
		Prompt: r.Prompt, Title: titleOf(r.Prompt), BaseBranch: r.BaseBranch, Branch: r.Branch,
		Status: api.TaskStatus(r.Status), Error: r.Error, CommitSha: r.CommitSha, PrUrl: r.PrUrl,
		TimeoutMinutes: int(r.TimeoutMinutes), CreatedAt: r.CreatedAt, StartedAt: r.StartedAt,
		FinishedAt: r.FinishedAt, UpdatedAt: r.UpdatedAt, ChangedFiles: []api.ChangedFile{},
		IssueKey: nonEmpty(r.IssueKey), BaseCommit: nonEmpty(r.BaseCommit),
	}
	if r.ExitCode != nil {
		c := int(*r.ExitCode)
		t.ExitCode = &c
	}
	_ = json.Unmarshal([]byte(r.ChangedFiles), &t.ChangedFiles)
	if t.ChangedFiles == nil {
		t.ChangedFiles = []api.ChangedFile{}
	}
	if r.Status == statusQueued {
		if n, err := m.q.QueuePosition(ctx, r.ID); err == nil {
			pos := int(n)
			t.QueuePosition = &pos
		}
	}
	return t
}

// ListTasks implements GET /coding/tasks.
func (m *Module) ListTasks(w http.ResponseWriter, r *http.Request, params api.ListTasksParams) {
	items, err := m.listTasks(r.Context(), params)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, api.TaskList{Items: items})
}

func (m *Module) listTasks(ctx context.Context, params api.ListTasksParams) ([]api.Task, error) {
	statuses := allStatuses
	if params.Status != nil && len(*params.Status) > 0 {
		statuses = nil
		for _, s := range *params.Status {
			if !s.Valid() {
				return nil, httpx.Invalid("未知的状态：" + string(s))
			}
			statuses = append(statuses, string(s))
		}
	}
	limit := 100
	if params.Limit != nil {
		limit = min(max(*params.Limit, 1), 500)
	}
	statusJSON, _ := json.Marshal(statuses)
	p := db.ListTasksParams{Statuses: string(statusJSON), Lim: int64(limit)}
	if params.RepoId != nil {
		p.RepoID = *params.RepoId
	}
	if params.IssueKey != nil && *params.IssueKey != "" {
		p.IssueKey = strings.ToUpper(*params.IssueKey)
	}
	rows, err := m.q.ListTasks(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]api.Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, m.toTask(ctx, taskRow{r.CodingTask, r.RepoName, r.RepoPath, r.AgentID, r.GithubRepo}))
	}
	return out, nil
}

// GetTask implements GET /coding/tasks/{id}.
func (m *Module) GetTask(w http.ResponseWriter, r *http.Request, id int64) {
	t, err := m.task(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

// CreateTask implements POST /coding/tasks.
func (m *Module) CreateTask(w http.ResponseWriter, r *http.Request) {
	var body api.CreateTask
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := contracts.LaunchCoding{RepoID: body.RepoId, Executor: string(body.Executor)}
	if body.Prompt != nil {
		in.Prompt = *body.Prompt
	}
	if body.BaseBranch != nil {
		in.BaseBranch = *body.BaseBranch
	}
	if body.IssueKey != nil {
		in.IssueKey = *body.IssueKey
	}
	timeout := 0
	if body.TimeoutMinutes != nil {
		timeout = *body.TimeoutMinutes
	}
	t, err := m.create(r.Context(), in, timeout)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}

// Launch implements contracts.Coding.
func (m *Module) Launch(ctx context.Context, in contracts.LaunchCoding) (int64, error) {
	t, err := m.create(ctx, in, 0)
	return t.Id, err
}

// create validates, stores and queues a task, then tries to start it.
func (m *Module) create(ctx context.Context, in contracts.LaunchCoding, timeoutMinutes int) (t api.Task, err error) {
	defer func() {
		m.d.Audit.Record(ctx, "coding_task.create", itoa(t.Id), map[string]any{
			"repoId": in.RepoID, "executor": in.Executor, "issueKey": in.IssueKey}, err)
	}()
	if !api.ExecutorName(in.Executor).Valid() {
		return t, httpx.Invalid("执行器只能是 claude 或 codex")
	}
	if timeoutMinutes == 0 {
		timeoutMinutes = m.defaultTimeout(ctx)
	}
	if timeoutMinutes < 1 || timeoutMinutes > 1440 {
		return t, httpx.Invalid("超时时间要在 1 到 1440 分钟之间")
	}
	repo, err := m.q.GetRepo(ctx, in.RepoID)
	if errors.Is(err, sql.ErrNoRows) {
		return t, httpx.Invalid("仓库不存在")
	}
	if err != nil {
		return t, err
	}
	prompt := strings.TrimSpace(in.Prompt)
	issueKey := strings.ToUpper(strings.TrimSpace(in.IssueKey))
	slugFrom := []string{titleOf(prompt)}
	if issueKey != "" {
		issues, ok := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey)
		if !ok {
			return t, httpx.NewError(http.StatusNotImplemented, "feature_unavailable", "项目模块没有启用")
		}
		issue, err := issues.Get(ctx, issueKey)
		var he *httpx.Error
		if errors.As(err, &he) && he.Status == http.StatusNotFound {
			return t, httpx.Invalid("Issue " + issueKey + " 不存在")
		}
		if err != nil {
			return t, err
		}
		issueKey = issue.Key
		slugFrom = append([]string{issue.Title}, slugFrom[0], issue.Key)
		prompt = issuePrompt(issue.Key, issue.Title, issue.Description, prompt)
	}
	if prompt == "" {
		return t, httpx.Invalid("请填写需求")
	}
	if len(prompt) > 100_000 {
		return t, httpx.Invalid("需求太长了")
	}
	base := strings.TrimSpace(in.BaseBranch)
	if base == "" {
		base = repo.DefaultBranch
	}
	now := m.now()
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return t, err
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	id, err := q.CreateTask(ctx, db.CreateTaskParams{RepoID: repo.ID, IssueKey: issueKey, Executor: in.Executor,
		Prompt: prompt, BaseBranch: base, TimeoutMinutes: int64(timeoutMinutes), CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return t, err
	}
	if err := q.SetTaskBranch(ctx, db.SetTaskBranchParams{Branch: branchName(id, slugFrom...), ID: id}); err != nil {
		return t, err
	}
	if err := tx.Commit(); err != nil {
		return t, err
	}
	if t, err = m.task(ctx, id); err != nil {
		return t, err
	}
	if files, ok := module.Lookup[contracts.Files](m.d.Registry, contracts.FilesKey); ok {
		// The task is saved; a failed claim only leaves images to the
		// unclaimed-file cleanup. Failing here would make the client retry
		// and create the task twice.
		if err := files.Claim(ctx, "coding", id, in.Prompt); err != nil {
			m.d.Log.Error("claim task images failed", "task", id, "err", err)
		}
	}
	m.d.Bus.Publish("coding_task.created", t)
	m.dispatch()
	// Reload: it may be running already.
	if fresh, err := m.task(ctx, id); err == nil {
		t = fresh
	}
	return t, nil
}

// ListTaskEvents implements GET /coding/tasks/{id}/events.
func (m *Module) ListTaskEvents(w http.ResponseWriter, r *http.Request, id int64, params api.ListTaskEventsParams) {
	ctx := r.Context()
	if _, err := m.row(ctx, id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	after := int64(0)
	if params.After != nil {
		after = max(*params.After, 0)
	}
	limit := 2000
	if params.Limit != nil {
		limit = min(max(*params.Limit, 1), 5000)
	}
	rows, err := m.q.ListEvents(ctx, db.ListEventsParams{TaskID: id, After: after, Lim: int64(limit)})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := api.TaskEventList{Items: make([]api.TaskEvent, 0, len(rows)), LastSeq: after}
	for _, e := range rows {
		out.Items = append(out.Items, toEvent(e))
		out.LastSeq = e.Seq
	}
	httpx.JSON(w, http.StatusOK, out)
}

// toEvent converts a stored event. The data column keeps the exit code of
// a done event under "_exitCode".
func toEvent(e db.CodingTaskEvent) api.TaskEvent {
	ev := api.TaskEvent{Seq: e.Seq, At: e.At, Kind: api.TaskEventKind(e.Kind), Text: e.Text}
	if e.Data != "" {
		var data map[string]any
		if json.Unmarshal([]byte(e.Data), &data) == nil {
			if code, ok := data["_exitCode"].(float64); ok {
				c := int(code)
				ev.ExitCode = &c
				delete(data, "_exitCode")
			}
			if len(data) > 0 {
				ev.Data = &data
			}
		}
	}
	return ev
}
