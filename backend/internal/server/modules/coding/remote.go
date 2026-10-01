package coding

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// B47: repositories registered from a Git connection. The agent clones them
// into its repository folder (coding.ensure_repo) and fetches before every
// task; tasks start from origin/<base>. Pushing and cloning get the token of
// the connection for that one call.

const ensureTimeout = 35 * time.Minute

var remoteRepoPart = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,99}$`)

func (m *Module) gitConnections() (contracts.GitConnections, error) {
	c, ok := module.Lookup[contracts.GitConnections](m.d.Registry, contracts.GitConnectionsKey)
	if !ok {
		return nil, httpx.NewError(http.StatusNotImplemented, "feature_unavailable", "Agent 模块没有启用，不能用 Git 连接")
	}
	return c, nil
}

// gitAuth returns the token of a connection for one agent call.
func (m *Module) gitAuth(ctx context.Context, connectionID int64) (*protocol.CodingGitAuth, error) {
	conns, err := m.gitConnections()
	if err != nil {
		return nil, err
	}
	user, token, err := conns.CloneAuth(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	return &protocol.CodingGitAuth{Username: user, Token: token}, nil
}

// remoteAgent checks that the agent can clone (B47 capability).
func (m *Module) remoteAgent(ctx context.Context, agentID string) error {
	if err := m.codingAgent(ctx, agentID); err != nil {
		return err
	}
	if hello, ok := m.d.Agents.Hello(agentID); ok && !hasCap(hello.Capabilities, protocol.CapCodingRemote) {
		return httpx.NewError(http.StatusNotImplemented, "feature_unavailable", "这台机器的代理版本太旧，不能 clone 仓库，请先升级代理")
	}
	return nil
}

func hasCap(caps []string, c string) bool {
	for _, x := range caps {
		if x == c {
			return true
		}
	}
	return false
}

// ensureClone clones or fetches a remote repository on its agent and
// refreshes the stored default branch.
func (m *Module) ensureClone(ctx context.Context, row db.CodingRepo) (protocol.CodingRepo, error) {
	if row.ConnectionID == nil {
		return protocol.CodingRepo{}, httpx.Invalid("这个仓库不是按 Git 连接登记的")
	}
	auth, err := m.gitAuth(ctx, *row.ConnectionID)
	if err != nil {
		return protocol.CodingRepo{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, ensureTimeout)
	defer cancel()
	var info protocol.CodingRepo
	err = m.d.Agents.Call(cctx, row.AgentID, protocol.MethodCodingEnsureRepo, protocol.CodingEnsureRepoParams{
		Dir: cloneDir(*row.ConnectionID, row.Owner, row.Repo), CloneURL: row.CloneUrl, Auth: auth}, &info)
	if err != nil {
		return info, err
	}
	if info.DefaultBranch != "" && info.DefaultBranch != row.DefaultBranch {
		if err := m.q.SetRepoDefaultBranch(ctx, db.SetRepoDefaultBranchParams{DefaultBranch: info.DefaultBranch, ID: row.ID}); err != nil {
			return info, err
		}
	}
	return info, nil
}

func cloneDir(connectionID int64, owner, repo string) string {
	return strconv.FormatInt(connectionID, 10) + "/" + owner + "/" + repo
}

// createRemoteRepo registers owner/name of a Git connection on an agent and
// clones it there.
func (m *Module) createRemoteRepo(ctx context.Context, agentID string, connectionID int64, remote, cloneURL string) (api.Repo, error) {
	owner, name, ok := strings.Cut(strings.TrimSpace(remote), "/")
	if !ok || !remoteRepoPart.MatchString(owner) || !remoteRepoPart.MatchString(name) {
		return api.Repo{}, httpx.Invalid("仓库要写成 owner/name")
	}
	u, err := url.Parse(strings.TrimSpace(cloneURL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return api.Repo{}, httpx.Invalid("clone 地址要是 https 地址，不能带用户名和密码")
	}
	if agentID == "" {
		return api.Repo{}, httpx.Invalid("请选择机器")
	}
	if err := m.remoteAgent(ctx, agentID); err != nil {
		return api.Repo{}, err
	}
	if _, err := m.gitAuth(ctx, connectionID); err != nil {
		return api.Repo{}, err
	}
	if _, err := m.q.GetRemoteRepoOn(ctx, db.GetRemoteRepoOnParams{AgentID: agentID, ConnectionID: &connectionID, Owner: owner, Repo: name}); err == nil {
		return api.Repo{}, httpx.NewError(http.StatusConflict, "conflict", "这台机器上已经登记过这个仓库了")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return api.Repo{}, err
	}
	row := db.CodingRepo{AgentID: agentID, ConnectionID: &connectionID, Owner: owner, Repo: name, CloneUrl: u.String()}
	info, err := m.ensureClone(ctx, row)
	if err != nil {
		return api.Repo{}, err
	}
	gh := ""
	if strings.EqualFold(u.Host, "github.com") {
		gh = owner + "/" + name
	}
	created, err := m.q.CreateRemoteRepo(ctx, db.CreateRemoteRepoParams{AgentID: agentID, Path: info.Path, Name: name,
		DefaultBranch: info.DefaultBranch, RemoteUrl: u.String(), GithubRepo: gh, CreatedAt: m.now(),
		ConnectionID: &connectionID, Owner: owner, Repo: name, CloneUrl: u.String()})
	if err != nil {
		return api.Repo{}, err
	}
	return m.toRepo(created, m.agentNames(ctx)), nil
}

// repoOn returns the repository to run on agentID: repo itself when it is
// on that agent, or the clone of the same remote there, registered and
// cloned on first use.
func (m *Module) repoOn(ctx context.Context, repo db.CodingRepo, agentID string) (db.CodingRepo, error) {
	if agentID == "" || agentID == repo.AgentID {
		return repo, nil
	}
	if repo.ConnectionID == nil {
		return repo, httpx.Invalid("按本地路径登记的仓库只能在它所在的机器上跑")
	}
	other, err := m.q.GetRemoteRepoOn(ctx, db.GetRemoteRepoOnParams{AgentID: agentID, ConnectionID: repo.ConnectionID, Owner: repo.Owner, Repo: repo.Repo})
	if err == nil {
		return other, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return repo, err
	}
	created, err := m.createRemoteRepo(ctx, agentID, *repo.ConnectionID, repo.Owner+"/"+repo.Repo, repo.CloneUrl)
	if err != nil {
		return repo, err
	}
	m.d.Bus.Publish("coding_repo.created", created)
	return m.q.GetRepo(ctx, created.Id)
}
