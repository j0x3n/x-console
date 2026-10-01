package coding

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func conflict(msg string) error { return httpx.NewError(http.StatusConflict, "conflict", msg) }

// decodeOptional decodes a JSON body that may be empty.
func decodeOptional(r *http.Request, v any) error {
	if r.ContentLength == 0 {
		return nil
	}
	err := httpx.Decode(r, v)
	var he *httpx.Error
	if errors.As(err, &he) && strings.Contains(he.Message, io.EOF.Error()) {
		return nil
	}
	return err
}

func (m *Module) taskParams(r taskRow) protocol.CodingTaskParams {
	return protocol.CodingTaskParams{TaskID: r.ID, RepoPath: r.RepoPath, Branch: r.Branch,
		BaseBranch: r.BaseBranch, BaseCommit: r.BaseCommit}
}

// respond publishes and returns the task after a change.
func (m *Module) respond(w http.ResponseWriter, r *http.Request, id int64, err error) {
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := m.task(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("coding_task.updated", t)
	httpx.JSON(w, http.StatusOK, t)
}

// CancelTask implements POST /coding/tasks/{id}/cancel.
func (m *Module) CancelTask(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	err := m.cancel(ctx, id)
	m.d.Audit.Record(ctx, "coding_task.cancel", itoa(id), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := m.task(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (m *Module) cancel(ctx context.Context, id int64) error {
	// dispatch holds this lock while it moves a task from queued to running
	// and registers its stream, so the state seen here is consistent.
	m.dispatchMu.Lock()
	defer m.dispatchMu.Unlock()
	row, err := m.row(ctx, id)
	if err != nil {
		return err
	}
	switch row.Status {
	case statusQueued:
		now := m.now()
		n, err := m.q.CancelQueued(ctx, db.CancelQueuedParams{Error: "已取消。", Now: &now, ID: id})
		if err != nil {
			return err
		}
		if n > 0 {
			if t, err := m.task(ctx, id); err == nil {
				m.d.Bus.Publish("coding_task.updated", t)
			}
		}
		return nil
	case statusRunning:
		if run := m.getRun(id); run != nil {
			m.cancelRun(run)
			return nil
		}
		// Running in the database but without a stream here: it was left
		// over by an earlier server process.
		m.finish(ctx, id, statusCanceled, nil, "已取消。", nil)
		return nil
	}
	return conflict("只有排队或运行中的任务可以取消")
}

// GetTaskDiff implements GET /coding/tasks/{id}/diff.
func (m *Module) GetTaskDiff(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	row, err := m.row(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !oneOf(row.Status, statusReview, statusFailed, statusCommitted, statusPushed, statusPROpened) {
		httpx.Fail(w, r, conflict("这个任务现在没有可以查看的改动"))
		return
	}
	var d protocol.CodingDiff
	callCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := m.d.Agents.Call(callCtx, row.AgentID, protocol.MethodCodingDiff, m.taskParams(row), &d); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := api.TaskDiff{Diff: d.Diff, Truncated: d.Truncated, Files: make([]api.ChangedFile, 0, len(d.Files))}
	for _, f := range d.Files {
		out.Files = append(out.Files, toChangedFile(f))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func toChangedFile(f protocol.CodingChangedFile) api.ChangedFile {
	c := api.ChangedFile{Path: f.Path, Status: f.Status, Additions: f.Additions, Deletions: f.Deletions}
	if f.Binary {
		b := true
		c.Binary = &b
	}
	return c
}

// CommitTask implements POST /coding/tasks/{id}/commit.
func (m *Module) CommitTask(w http.ResponseWriter, r *http.Request, id int64) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.CommitRequest
	if err := decodeOptional(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	msg := ""
	if body.Message != nil {
		msg = *body.Message
	}
	err := m.commit(r.Context(), id, msg)
	if err == nil && body.Push != nil && *body.Push {
		err = m.push(r.Context(), id)
	}
	m.respond(w, r, id, err)
}

func (m *Module) commit(ctx context.Context, id int64, msg string) (err error) {
	row, err := m.row(ctx, id)
	if err != nil {
		return err
	}
	var sha string
	defer func() {
		m.d.Audit.Record(ctx, "coding_task.commit", itoa(id), map[string]any{"branch": row.Branch, "sha": sha}, err)
	}()
	if !oneOf(row.Status, statusReview, statusFailed) {
		return conflict("只有等你决定或失败的任务可以提交")
	}
	if m.building(id) {
		return conflict("正在构建，等构建结束再提交")
	}
	if strings.TrimSpace(msg) == "" {
		msg = commitMessage(row.ID, stripIssueHeader(row.Prompt, row.IssueKey), row.IssueKey)
	}
	var res protocol.CodingCommitResult
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := m.d.Agents.Call(callCtx, row.AgentID, protocol.MethodCodingCommit,
		protocol.CodingCommitParams{CodingTaskParams: m.taskParams(row), Message: msg}, &res); err != nil {
		return err
	}
	sha = res.SHA
	return m.q.SetCommitted(ctx, db.SetCommittedParams{CommitSha: sha, UpdatedAt: m.now(), ID: id})
}

// stripIssueHeader drops the "KEY: title" line create put in front, so the
// commit subject is the issue title rather than the key.
func stripIssueHeader(prompt, key string) string {
	if key != "" && strings.HasPrefix(prompt, key+": ") {
		return strings.TrimPrefix(prompt, key+": ")
	}
	return prompt
}

// PushTask implements POST /coding/tasks/{id}/push.
func (m *Module) PushTask(w http.ResponseWriter, r *http.Request, id int64) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.respond(w, r, id, m.push(r.Context(), id))
}

func (m *Module) push(ctx context.Context, id int64) (err error) {
	row, err := m.row(ctx, id)
	if err != nil {
		return err
	}
	defer func() {
		m.d.Audit.Record(ctx, "coding_task.push", itoa(id), map[string]any{"branch": row.Branch}, err)
	}()
	if row.Status != statusCommitted {
		return conflict("先提交，再推送")
	}
	params := protocol.CodingPushParams{CodingTaskParams: m.taskParams(row)}
	if row.RepoConnectionID != nil {
		if params.Auth, err = m.gitAuth(ctx, *row.RepoConnectionID); err != nil {
			return err
		}
	}
	callCtx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	if err := m.d.Agents.Call(callCtx, row.AgentID, protocol.MethodCodingPush, params, nil); err != nil {
		return err
	}
	return m.setStatus(ctx, id, statusCommitted, statusPushed)
}

func (m *Module) setStatus(ctx context.Context, id int64, from, to string) error {
	n, err := m.q.SetTaskStatus(ctx, db.SetTaskStatusParams{Status: to, Error: "", Now: m.now(), ID: id, FromStatus: from})
	if err != nil {
		return err
	}
	if n == 0 {
		return conflict("任务状态已经变了，请刷新")
	}
	return nil
}

// OpenPullRequest implements POST /coding/tasks/{id}/pr.
func (m *Module) OpenPullRequest(w http.ResponseWriter, r *http.Request, id int64) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.PullRequestRequest
	if err := decodeOptional(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.respond(w, r, id, m.openPR(r.Context(), id, body))
}

func (m *Module) openPR(ctx context.Context, id int64, body api.PullRequestRequest) (err error) {
	row, err := m.row(ctx, id)
	if err != nil {
		return err
	}
	// B47: repositories of a Git connection open the PR through it (GitHub
	// or Forgejo); the others through the GitHub module.
	remote := row.GithubRepo
	createPR := func(ctx context.Context, in contracts.CreatePR) (string, int, error) {
		gh, ok := m.github()
		if !ok {
			return "", 0, httpx.NewError(http.StatusNotImplemented, "feature_unavailable", "GitHub 模块没有启用，不能建 PR")
		}
		return gh.CreatePR(ctx, in)
	}
	if row.RepoConnectionID != nil {
		remote = row.RepoOwner + "/" + row.RepoRepo
		conns, err := m.gitConnections()
		if err != nil {
			return err
		}
		connID := *row.RepoConnectionID
		createPR = func(ctx context.Context, in contracts.CreatePR) (string, int, error) {
			return conns.CreatePR(ctx, connID, in)
		}
	}
	var url string
	defer func() {
		m.d.Audit.Record(ctx, "coding_task.pr", itoa(id), map[string]any{"repo": remote, "branch": row.Branch, "url": url}, err)
	}()
	if !oneOf(row.Status, statusCommitted, statusPushed) {
		return conflict("先提交，再建 PR")
	}
	if remote == "" {
		return httpx.Invalid("这个仓库的远端不是 GitHub")
	}
	if row.Status == statusCommitted {
		if err := m.push(ctx, id); err != nil {
			return err
		}
	}
	in := contracts.CreatePR{Repo: remote, Head: row.Branch, Base: row.BaseBranch, Title: titleOf(stripIssueHeader(row.Prompt, row.IssueKey))}
	if in.Base == "" {
		if repo, err := m.q.GetRepo(ctx, row.RepoID); err == nil {
			in.Base = repo.DefaultBranch
		}
	}
	if body.Title != nil && strings.TrimSpace(*body.Title) != "" {
		in.Title = strings.TrimSpace(*body.Title)
	}
	if body.Body != nil {
		in.Body = *body.Body
	} else {
		in.Body = row.Prompt + "\n\n---\nX Console coding task #" + itoa(id)
	}
	if body.Draft != nil {
		in.Draft = *body.Draft
	}
	url, number, err := createPR(ctx, in)
	if err != nil {
		return err
	}
	if err := m.q.SetPullRequest(ctx, db.SetPullRequestParams{PrUrl: url, UpdatedAt: m.now(), ID: id}); err != nil {
		return err
	}
	if row.IssueKey != "" {
		if issues, ok := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey); ok {
			ref := remote + "#" + strconv.Itoa(number)
			if err := issues.AttachLink(ctx, row.IssueKey, contracts.IssueLink{Kind: "pull_request", Title: "PR " + ref + " " + in.Title, URL: url, Ref: ref}); err != nil {
				m.d.Log.Warn("coding: link pull request", "issue", row.IssueKey, "err", err)
			}
			if err := issues.SetStatus(ctx, row.IssueKey, "in_review"); err != nil {
				m.d.Log.Warn("coding: set issue status", "issue", row.IssueKey, "err", err)
			}
		}
	}
	return nil
}

// DiscardTask implements POST /coding/tasks/{id}/discard.
func (m *Module) DiscardTask(w http.ResponseWriter, r *http.Request, id int64) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.respond(w, r, id, m.discard(r.Context(), id))
}

func (m *Module) discard(ctx context.Context, id int64) (err error) {
	row, err := m.row(ctx, id)
	if err != nil {
		return err
	}
	defer func() {
		m.d.Audit.Record(ctx, "coding_task.discard", itoa(id), map[string]any{"branch": row.Branch}, err)
	}()
	if !oneOf(row.Status, statusReview, statusFailed, statusCanceled, statusCommitted) {
		return conflict("这个任务现在不能丢弃")
	}
	if m.building(id) {
		return conflict("正在构建，等构建结束再丢弃")
	}
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := m.d.Agents.Call(callCtx, row.AgentID, protocol.MethodCodingDiscard, m.taskParams(row), nil); err != nil {
		return err
	}
	return m.setStatus(ctx, id, row.Status, statusDiscarded)
}

// ---- actions ----

type launchInput struct {
	RepoID     int64  `json:"repoId"`
	Executor   string `json:"executor"`
	Prompt     string `json:"prompt"`
	BaseBranch string `json:"baseBranch"`
	IssueKey   string `json:"issueKey"`
}

func (m *Module) actionLaunch(ctx context.Context, raw json.RawMessage) (any, error) {
	var in launchInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, httpx.Invalid("参数格式不对：" + err.Error())
	}
	if in.Executor == "" {
		in.Executor = protocol.ExecutorClaude
	}
	return m.create(ctx, contracts.LaunchCoding{RepoID: in.RepoID, Executor: in.Executor, Prompt: in.Prompt,
		BaseBranch: in.BaseBranch, IssueKey: in.IssueKey}, 0)
}

type listInput struct {
	Status []api.TaskStatus `json:"status"`
	Limit  int              `json:"limit"`
}

func (m *Module) actionList(ctx context.Context, raw json.RawMessage) (any, error) {
	var in listInput
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, httpx.Invalid("参数格式不对：" + err.Error())
		}
	}
	p := api.ListTasksParams{}
	if len(in.Status) > 0 {
		p.Status = &in.Status
	}
	limit := 20
	if in.Limit > 0 {
		limit = in.Limit
	}
	p.Limit = &limit
	tasks, err := m.listTasks(ctx, p)
	if err != nil {
		return nil, err
	}
	type item struct {
		ID       int64  `json:"id"`
		Title    string `json:"title"`
		Repo     string `json:"repo"`
		Status   string `json:"status"`
		IssueKey string `json:"issueKey,omitempty"`
		Error    string `json:"error,omitempty"`
		PRURL    string `json:"prUrl,omitempty"`
		Link     string `json:"link"`
	}
	out := make([]item, 0, len(tasks))
	for _, t := range tasks {
		it := item{ID: t.Id, Title: t.Title, Repo: t.RepoName, Status: string(t.Status), Error: t.Error, PRURL: t.PrUrl, Link: "/coding/" + itoa(t.Id)}
		if t.IssueKey != nil {
			it.IssueKey = *t.IssueKey
		}
		out = append(out, it)
	}
	return out, nil
}
