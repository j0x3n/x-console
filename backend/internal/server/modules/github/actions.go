package github

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/db"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "github.list_pulls",
		Title: "查看打开的 PR",
		Description: "List open pull requests of the watched GitHub repositories from the local cache (refreshed every 5 minutes). " +
			"Each item has repo, number, title, author, head/base branch, draft, reviewState and checkState (success, failure, pending, none). " +
			"Optionally filter by repo (owner/name).",
		Input:  actions.Schema(`{"type":"object","properties":{"repo":{"type":"string","description":"owner/name"}},"additionalProperties":false}`),
		Effect: actions.Read,
		Run:    m.actionListPulls,
	})
	m.d.Actions.Register(actions.Action{
		Name:        "github.create_pr",
		Title:       "新建 PR",
		Description: "Open a pull request on GitHub from branch head into base. Returns its url and number.",
		Input: actions.Schema(`{"type":"object","properties":{
			"repo":{"type":"string","description":"owner/name"},
			"head":{"type":"string","description":"Branch with the changes"},
			"base":{"type":"string","description":"Target branch, e.g. main"},
			"title":{"type":"string"},
			"body":{"type":"string"},
			"draft":{"type":"boolean"}},
			"required":["repo","head","base","title"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run:    m.actionCreatePR,
	})
}

func (m *Module) actionListPulls(ctx context.Context, in json.RawMessage) (any, error) {
	var args struct {
		Repo string `json:"repo"`
	}
	if err := json.Unmarshal(in, &args); err != nil {
		return nil, httpx.Invalid("参数格式不正确")
	}
	return m.listPulls(ctx, strings.TrimSpace(args.Repo))
}

func (m *Module) actionCreatePR(ctx context.Context, in json.RawMessage) (any, error) {
	var args contracts.CreatePR
	if err := json.Unmarshal(in, &args); err != nil {
		return nil, httpx.Invalid("参数格式不正确")
	}
	u, n, err := m.CreatePR(ctx, args)
	if err != nil {
		return nil, err
	}
	return map[string]any{"url": u, "number": n}, nil
}

// CreatePR implements contracts.GitHub.
func (m *Module) CreatePR(ctx context.Context, in contracts.CreatePR) (prURL string, number int, err error) {
	defer func() {
		m.d.Audit.Record(ctx, "github.create_pr", fmt.Sprintf("%s#%d", in.Repo, number),
			map[string]any{"head": in.Head, "base": in.Base, "draft": in.Draft}, err)
	}()
	repo, err := normalizeRepo(in.Repo)
	if err != nil {
		return "", 0, err
	}
	in.Repo = repo
	in.Head, in.Base, in.Title = strings.TrimSpace(in.Head), strings.TrimSpace(in.Base), strings.TrimSpace(in.Title)
	if in.Head == "" || in.Base == "" {
		return "", 0, httpx.Invalid("需要填写源分支和目标分支")
	}
	if in.Title == "" {
		return "", 0, httpx.Invalid("标题不能为空")
	}
	cfg, err := m.requireConfigured(ctx)
	if err != nil {
		return "", 0, err
	}
	var p ghPull
	body := map[string]any{"title": in.Title, "head": in.Head, "base": in.Base, "body": in.Body, "draft": in.Draft}
	if err := m.client(cfg).post(ctx, "/repos/"+repo+"/pulls", body, &p); err != nil {
		if statusOf(err) > 0 || isRateLimited(err) {
			return "", 0, httpx.NewError(502, "github_error", err.Error())
		}
		return "", 0, err
	}
	// Show it right away when the repository is watched; the next sync
	// fills in reviews and checks.
	if slices.Contains(cfg.Repos, repo) {
		now := m.now()
		if err := m.q.UpsertPull(ctx, db.UpsertPullParams{
			Repo: repo, Number: int64(p.Number), Title: p.Title, Author: p.User.Login, Url: p.HTMLURL,
			HeadRef: p.Head.Ref, HeadSha: p.Head.SHA, BaseRef: p.Base.Ref, Draft: p.Draft,
			ReviewState: "none", CheckState: "none", CreatedAt: p.CreatedAt.UTC(), UpdatedAt: p.UpdatedAt.UTC(), SyncedAt: now,
		}); err != nil {
			m.log.Warn("github cache new pull", "err", err)
		}
	}
	m.linkPull(ctx, repo, p.Number, p.Title, p.Head.Ref, p.HTMLURL)
	m.d.Bus.Publish("github.pull_created", map[string]any{"repo": repo, "number": p.Number, "url": p.HTMLURL})
	return p.HTMLURL, p.Number, nil
}
