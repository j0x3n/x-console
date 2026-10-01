package coding

import (
	"context"
	"database/sql"
	"errors"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"net/http"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// codingAgent checks that agentID is a paired agent that can run coding
// tasks. An offline agent passes; calls to it then answer 503.
func (m *Module) codingAgent(ctx context.Context, agentID string) error {
	a, err := m.d.Agents.Get(ctx, agentID)
	if err != nil {
		return err
	}
	if a.Online && !a.Has(protocol.CapCoding) {
		return httpx.NewError(http.StatusNotImplemented, "feature_unavailable", "这台机器的代理不支持编码任务")
	}
	return nil
}

// ListExecutors implements GET /coding/executors.
func (m *Module) ListExecutors(w http.ResponseWriter, r *http.Request, params api.ListExecutorsParams) {
	ctx := r.Context()
	if err := m.codingAgent(ctx, params.AgentId); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var list protocol.CodingExecutorList
	callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := m.d.Agents.Call(callCtx, params.AgentId, protocol.MethodCodingExecutors, nil, &list); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.Executor, 0, len(list.Items))
	for _, e := range list.Items {
		out = append(out, api.Executor{Name: api.ExecutorName(e.Name), Available: e.Available,
			Path: nonEmpty(e.Path), Version: nonEmpty(e.Version), Error: nonEmpty(e.Error)})
	}
	httpx.JSON(w, http.StatusOK, out)
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ListRepos implements GET /coding/repos.
func (m *Module) ListRepos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := m.q.ListRepos(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	names := m.agentNames(ctx)
	out := make([]api.Repo, 0, len(rows))
	for _, row := range rows {
		out = append(out, m.toRepo(row, names))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) agentNames(ctx context.Context) map[string]string {
	names := map[string]string{}
	agents, err := m.d.Agents.List(ctx)
	if err != nil {
		return names
	}
	for _, a := range agents {
		names[a.ID] = a.Name
	}
	return names
}

func (m *Module) toRepo(row db.CodingRepo, names map[string]string) api.Repo {
	return api.Repo{
		Id: row.ID, AgentId: row.AgentID, AgentName: names[row.AgentID], AgentOnline: m.d.Agents.Online(row.AgentID),
		Path: row.Path, Name: row.Name, DefaultBranch: row.DefaultBranch, RemoteUrl: row.RemoteUrl,
		GithubRepo: row.GithubRepo, CreatedAt: row.CreatedAt, ConnectionId: row.ConnectionID,
		RemoteRepo: nonEmpty(remoteName(row)), BuildConfig: toAPIBuildConfig(row.BuildConfig),
	}
}

// remoteName is owner/name of a repository registered from a Git connection.
func remoteName(row db.CodingRepo) string {
	if row.ConnectionID == nil {
		return ""
	}
	return row.Owner + "/" + row.Repo
}

// DiscoverRepos implements GET /coding/repos/discover.
func (m *Module) DiscoverRepos(w http.ResponseWriter, r *http.Request, params api.DiscoverReposParams) {
	ctx := r.Context()
	if err := m.codingAgent(ctx, params.AgentId); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p := protocol.CodingReposParams{}
	if params.Root != nil && strings.TrimSpace(*params.Root) != "" {
		p.Roots = []string{strings.TrimSpace(*params.Root)}
	}
	var list protocol.CodingRepoList
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := m.d.Agents.Call(callCtx, params.AgentId, protocol.MethodCodingRepos, p, &list); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	known, err := m.q.ListRepoPaths(ctx, params.AgentId)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ids := map[string]int64{}
	for _, k := range known {
		ids[k.Path] = k.ID
	}
	out := api.DiscoverResult{Roots: list.Roots, Items: make([]api.DiscoveredRepo, 0, len(list.Items))}
	if out.Roots == nil {
		out.Roots = []string{}
	}
	for _, it := range list.Items {
		d := api.DiscoveredRepo{Path: it.Path, Name: it.Name, CurrentBranch: it.CurrentBranch,
			DefaultBranch: it.DefaultBranch, RemoteUrl: it.RemoteURL}
		if id, ok := ids[it.Path]; ok {
			d.RepoId = &id
		}
		out.Items = append(out.Items, d)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// CreateRepo implements POST /coding/repos.
func (m *Module) CreateRepo(w http.ResponseWriter, r *http.Request) {
	var body api.CreateRepo
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	var repo api.Repo
	var err error
	target := ""
	if body.ConnectionId != nil {
		remote, cloneURL := "", ""
		if body.RemoteRepo != nil {
			remote = *body.RemoteRepo
		}
		if body.CloneUrl != nil {
			cloneURL = *body.CloneUrl
		}
		target = remote
		repo, err = m.createRemoteRepo(ctx, body.AgentId, *body.ConnectionId, remote, cloneURL)
	} else {
		path := ""
		if body.Path != nil {
			path = strings.TrimSpace(*body.Path)
		}
		target = path
		repo, err = m.createRepo(ctx, body.AgentId, path)
	}
	m.d.Audit.Record(ctx, "coding_repo.create", target, map[string]any{"agentId": body.AgentId, "connectionId": body.ConnectionId}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("coding_repo.created", repo)
	httpx.JSON(w, http.StatusCreated, repo)
}

func (m *Module) createRepo(ctx context.Context, agentID, path string) (api.Repo, error) {
	if agentID == "" || path == "" {
		return api.Repo{}, httpx.Invalid("请选择机器并填写仓库路径")
	}
	if err := m.codingAgent(ctx, agentID); err != nil {
		return api.Repo{}, err
	}
	var list protocol.CodingRepoList
	if err := m.d.Agents.Call(ctx, agentID, protocol.MethodCodingRepos,
		protocol.CodingReposParams{Roots: []string{path}, Depth: -1}, &list); err != nil {
		return api.Repo{}, err
	}
	if len(list.Items) == 0 {
		return api.Repo{}, httpx.Invalid("这个目录不是 git 仓库的根目录")
	}
	info := list.Items[0]
	if _, err := m.q.GetRepoByPath(ctx, db.GetRepoByPathParams{AgentID: agentID, Path: info.Path}); err == nil {
		return api.Repo{}, httpx.NewError(http.StatusConflict, "conflict", "这个仓库已经登记过了")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return api.Repo{}, err
	}
	row, err := m.q.CreateRepo(ctx, db.CreateRepoParams{
		AgentID: agentID, Path: info.Path, Name: info.Name, DefaultBranch: info.DefaultBranch,
		RemoteUrl: info.RemoteURL, GithubRepo: githubRepo(info.RemoteURL), CreatedAt: m.now(),
	})
	if err != nil {
		return api.Repo{}, err
	}
	return m.toRepo(row, m.agentNames(ctx)), nil
}

// DeleteRepo implements DELETE /coding/repos/{id}.
func (m *Module) DeleteRepo(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	err := m.deleteRepo(ctx, id)
	m.d.Audit.Record(ctx, "coding_repo.delete", itoa(id), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("coding_repo.deleted", map[string]int64{"id": id})
	httpx.NoContent(w)
}

func (m *Module) deleteRepo(ctx context.Context, id int64) error {
	active, err := m.q.CountActiveTasksForRepo(ctx, id)
	if err != nil {
		return err
	}
	if active > 0 {
		return httpx.NewError(http.StatusConflict, "conflict", "这个仓库还有排队或运行中的任务")
	}
	// The tasks go with the repo (ON DELETE CASCADE); their images must go too.
	taskIDs, err := m.repoTaskIDs(ctx, id)
	if err != nil {
		return err
	}
	n, err := m.q.DeleteRepo(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return httpx.ErrNotFound
	}
	m.deleteArtifacts(ctx, taskIDs)
	if files, ok := module.Lookup[contracts.Files](m.d.Registry, contracts.FilesKey); ok {
		for _, taskID := range taskIDs {
			if err := files.DeleteOwned(ctx, "coding", taskID); err != nil {
				m.d.Log.Error("coding task images not deleted", "task", taskID, "err", err)
			}
		}
	}
	return nil
}

func (m *Module) repoTaskIDs(ctx context.Context, repoID int64) ([]int64, error) {
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id FROM coding_tasks WHERE repo_id=?", repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
