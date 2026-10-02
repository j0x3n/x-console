package projects

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/db"
)

func boardRepoAPI(r db.GetBoardRepoRow) *api.BoardRepo {
	n := int(r.SyncedCount)
	return &api.BoardRepo{ConnectionId: r.ConnectionID, ConnectionName: r.ConnectionName, FullName: r.FullName,
		HtmlUrl: r.HtmlUrl, SyncIssues: r.SyncIssues != 0, LastSyncedAt: r.LastSyncedAt, LastError: &r.LastError, SyncedCount: &n}
}

func (m *Module) gitIssues() (contracts.GitIssues, error) {
	g, ok := module.Lookup[contracts.GitIssues](m.d.Registry, contracts.GitIssuesKey)
	if !ok {
		return nil, httpx.ErrNotLive
	}
	return g, nil
}

func (m *Module) RepositoryForIssue(ctx context.Context, key string) (contracts.GitRepository, error) {
	i, err := m.findIssue(ctx, m.q, key)
	if err != nil {
		return contracts.GitRepository{}, err
	}
	if i.Issue.BoardID == nil {
		return contracts.GitRepository{}, missingBoardRepo()
	}
	r, err := m.q.GetBoardRepo(ctx, *i.Issue.BoardID)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.GitRepository{}, missingBoardRepo()
	}
	if err != nil {
		return contracts.GitRepository{}, err
	}
	return contracts.GitRepository{ConnectionID: r.ConnectionID, ConnectionName: r.ConnectionName, Kind: r.Kind,
		FullName: r.FullName, HTMLURL: r.HtmlUrl, CloneURL: r.CloneUrl, DefaultBranch: r.DefaultBranch}, nil
}

func missingBoardRepo() error {
	return httpx.NewError(409, "board_repo_missing", "这个看板还没绑定仓库")
}

func (m *Module) repoBoard(ctx context.Context, id int64) (api.Board, error) {
	b, err := m.q.GetBoard(ctx, id)
	if err != nil {
		return api.Board{}, notFound(err)
	}
	return m.boardWithLists(ctx, m.q, b, false)
}

func detachRepo(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE issues SET external_source='', external_id='', external_url='' WHERE board_id=? AND external_source IN ('github','forgejo')`, id)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "DELETE FROM board_repos WHERE board_id=?", id)
	return err
}

func (m *Module) BindBoardRepo(w http.ResponseWriter, r *http.Request, id int64) {
	var in api.BindBoardRepo
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	err := func() error {
		if err := auth.RequireElevated(ctx); err != nil {
			return err
		}
		if _, err := m.q.GetBoard(ctx, id); err != nil {
			return notFound(err)
		}
		g, err := m.gitIssues()
		if err != nil {
			return err
		}
		repo, err := g.Repository(ctx, in.ConnectionId, strings.TrimSpace(in.FullName))
		if err != nil {
			return err
		}
		sync := in.SyncIssues == nil || *in.SyncIssues
		if err := m.bindRepo(ctx, id, repo, sync); err != nil {
			return err
		}
		if sync {
			return m.syncRepo(ctx, id, 0)
		}
		return nil
	}()
	m.d.Audit.Record(ctx, "board.repo_bind", fmt.Sprint(id), map[string]any{"connectionId": in.ConnectionId, "fullName": in.FullName}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("board.changed", map[string]any{"boardId": id})
	out, err := m.repoBoard(ctx, id)
	writeOr(w, r, 200, out, err)
}

func (m *Module) bindRepo(ctx context.Context, id int64, repo contracts.GitRepository, sync bool) error {
	m.repoMu.Lock()
	defer m.repoMu.Unlock()
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var other int64
	err = tx.QueryRowContext(ctx, "SELECT board_id FROM board_repos WHERE full_name=?", repo.FullName).Scan(&other)
	if err == nil && other != id {
		return httpx.NewError(409, "conflict", "这个仓库已经绑定了别的看板")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var oldName string
	var oldConnection int64
	err = tx.QueryRowContext(ctx, "SELECT full_name,connection_id FROM board_repos WHERE board_id=?", id).Scan(&oldName, &oldConnection)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && (oldName != repo.FullName || oldConnection != repo.ConnectionID) {
		if err := detachRepo(ctx, tx, id); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO board_repos(board_id,connection_id,full_name,kind,html_url,clone_url,default_branch,sync_issues) VALUES(?,?,?,?,?,?,?,?)
		 ON CONFLICT(board_id) DO UPDATE SET sync_issues=excluded.sync_issues,html_url=excluded.html_url,clone_url=excluded.clone_url,default_branch=excluded.default_branch`,
		id, repo.ConnectionID, repo.FullName, repo.Kind, repo.HTMLURL, repo.CloneURL, repo.DefaultBranch, sync)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (m *Module) UnbindBoardRepo(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	err := func() error {
		if err := auth.RequireElevated(ctx); err != nil {
			return err
		}
		m.repoMu.Lock()
		defer m.repoMu.Unlock()
		if _, err := m.q.GetBoardRepo(ctx, id); err != nil {
			return notFound(err)
		}
		tx, err := m.d.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := detachRepo(ctx, tx, id); err != nil {
			return err
		}
		return tx.Commit()
	}()
	m.d.Audit.Record(ctx, "board.repo_unbind", fmt.Sprint(id), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("board.changed", map[string]any{"boardId": id})
	httpx.NoContent(w)
}

func (m *Module) SyncBoardRepo(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	err := func() error {
		if err := auth.RequireElevated(ctx); err != nil {
			return err
		}
		return m.syncRepo(ctx, id, 0)
	}()
	m.d.Audit.Record(ctx, "board.repo_sync", fmt.Sprint(id), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.repoBoard(ctx, id)
	writeOr(w, r, 200, out, err)
}

func (m *Module) repoError(ctx context.Context, id int64, err error) {
	text := ""
	if err != nil {
		text = err.Error()
	}
	if _, e := m.d.DB.ExecContext(ctx, "UPDATE board_repos SET last_error=? WHERE board_id=?", text, id); e != nil {
		slog.Warn("projects: save sync error", "err", e)
	}
}

// syncRepo publishes one board event, without per-card notifications.
// onlyNumber reads just that issue (webhooks). The Git service is read
// without repoMu, so a slow service does not hold up card edits.
func (m *Module) syncRepo(ctx context.Context, id, onlyNumber int64) (err error) {
	defer func() { m.repoError(ctx, id, err); m.d.Bus.Publish("board.changed", map[string]any{"boardId": id}) }()
	r, err := m.q.GetBoardRepo(ctx, id)
	if err != nil {
		return notFound(err)
	}
	if r.SyncIssues == 0 {
		return nil
	}
	g, err := m.gitIssues()
	if err != nil {
		return err
	}
	var items []contracts.GitIssue
	if onlyNumber != 0 {
		item, err := g.Issue(ctx, r.ConnectionID, r.FullName, onlyNumber)
		if err != nil {
			return err
		}
		if !item.PullRequest {
			items = append(items, item)
		}
	} else if items, err = g.Issues(ctx, r.ConnectionID, r.FullName); err != nil {
		return err
	}
	m.repoMu.Lock()
	defer m.repoMu.Unlock()
	cur, err := m.q.GetBoardRepo(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (cur.ConnectionID != r.ConnectionID || cur.FullName != r.FullName || cur.SyncIssues == 0) {
		return nil // unbound or rebound while reading
	}
	if err != nil {
		return err
	}
	b, err := m.q.GetBoard(ctx, id)
	if err != nil {
		return err
	}
	for _, item := range items {
		external := fmt.Sprintf("%s#%d", r.FullName, item.Number)
		old, e := m.q.GetIssueByExternal(ctx, db.GetIssueByExternalParams{ExternalSource: r.Kind, ExternalID: external})
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		status := "todo"
		if item.State == "closed" {
			status = "done"
		}
		if errors.Is(e, sql.ErrNoRows) && item.State == "closed" {
			continue
		}
		labels, e2 := m.repoLabels(ctx, b.ProjectID, item.Labels)
		if e2 != nil {
			return e2
		}
		var card api.Issue
		if e == nil {
			if old.Issue.BoardID == nil || *old.Issue.BoardID != id {
				return httpx.NewError(409, "conflict", "这个 Issue 已经在别的看板上")
			}
			if item.State != "closed" && !closedStatus(old.Issue.Status) {
				status = old.Issue.Status
			}
			card, _, err = m.patchIssue(ctx, issueKey(old.ProjectKey, old.Issue.Number), issuePatch{Title: &item.Title, Description: &item.Body, Status: &status, LabelIDs: &labels})
		} else {
			card, err = m.insertIssue(ctx, b.ProjectID, issueInput{Title: item.Title, Description: item.Body, Status: status, BoardID: &id, LabelIDs: labels, ExternalSource: r.Kind, ExternalID: external})
		}
		if err != nil {
			return err
		}
		if err = m.q.SetIssueExternalURL(ctx, db.SetIssueExternalURLParams{ExternalUrl: item.URL, ID: card.Id}); err != nil {
			return err
		}
	}
	_, err = m.d.DB.ExecContext(ctx, "UPDATE board_repos SET last_synced_at=?,last_error='' WHERE board_id=?", m.now(), id)
	return err
}

func (m *Module) repoLabels(ctx context.Context, projectID int64, names []string) ([]int64, error) {
	labels, err := m.q.ListLabels(ctx, &projectID)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	seen := map[int64]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var id int64
		for _, l := range labels {
			if l.Name == name {
				id = l.ID
				break
			}
		}
		if id == 0 {
			l, err := m.q.CreateLabel(ctx, db.CreateLabelParams{ProjectID: &projectID, Name: name, Color: "#64748b"})
			if err != nil {
				return nil, err
			}
			id = l.ID
			labels = append(labels, l)
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids, nil
}

func (m *Module) pushRepoIssue(ctx context.Context, i api.Issue) {
	if i.BoardId == nil || (i.ExternalSource != "github" && i.ExternalSource != "forgejo") {
		return
	}
	r, err := m.q.GetBoardRepo(ctx, *i.BoardId)
	if err != nil || r.SyncIssues == 0 {
		return
	}
	str, ok := strings.CutPrefix(i.ExternalId, r.FullName+"#")
	if !ok {
		return
	}
	n, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return
	}
	g, err := m.gitIssues()
	if err == nil {
		state := "open"
		if closedStatus(string(i.Status)) {
			state = "closed"
		}
		err = g.UpdateGitIssue(ctx, r.ConnectionID, r.FullName, contracts.GitIssue{Number: n, Title: i.Title, Body: i.Description, State: state})
	}
	m.repoError(ctx, *i.BoardId, err)
	m.d.Bus.Publish("board.changed", map[string]any{"boardId": *i.BoardId})
}

func (m *Module) syncRepos(ctx context.Context) error {
	rows, err := m.d.DB.QueryContext(ctx, "SELECT board_id FROM board_repos WHERE sync_issues=1")
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if err = m.syncRepo(ctx, id, 0); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Module) ReceiveGitWebhook(ctx context.Context, hook contracts.GitWebhook) error {
	if hook.Event != "issues" {
		return nil
	}
	var body struct {
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Issue struct {
			Number int64 `json:"number"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(hook.Body, &body); err != nil {
		return httpx.Invalid("请求体不是 JSON")
	}
	var id int64
	err := m.d.DB.QueryRowContext(ctx, "SELECT board_id FROM board_repos WHERE connection_id=? AND full_name=? AND sync_issues=1", hook.ConnectionID, body.Repository.FullName).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if body.Issue.Number <= 0 {
		return nil
	}
	return m.syncRepo(ctx, id, body.Issue.Number)
}

// UnbindGitConnection implements contracts.BoardGitUnbinder.
func (m *Module) UnbindGitConnection(ctx context.Context, connectionID int64) error {
	m.repoMu.Lock()
	defer m.repoMu.Unlock()
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT board_id FROM board_repos WHERE connection_id=?", connectionID)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := detachRepo(ctx, tx, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, id := range ids {
		m.d.Bus.Publish("board.changed", map[string]any{"boardId": id})
	}
	return nil
}
