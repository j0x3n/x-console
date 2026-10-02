package aiagents

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

var repoNameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func issueRepoPath(name string) (string, error) {
	if !repoNameRE.MatchString(name) {
		return "", httpx.Invalid("仓库要写成 owner/name")
	}
	return "/repos/" + name, nil
}

func (m *Module) Repository(ctx context.Context, id int64, name string) (contracts.GitRepository, error) {
	path, err := issueRepoPath(name)
	if err != nil {
		return contracts.GitRepository{}, err
	}
	c, err := m.connection(ctx, id)
	if err != nil {
		return contracts.GitRepository{}, err
	}
	g, err := m.client(ctx, c)
	if err != nil {
		return contracts.GitRepository{}, err
	}
	var r remoteRepo
	if err := g.do(ctx, http.MethodGet, path, nil, &r); err != nil {
		return contracts.GitRepository{}, gitFailure(err)
	}
	return contracts.GitRepository{ConnectionID: id, ConnectionName: c.Name, Kind: c.Kind, FullName: name,
		HTMLURL: r.HTMLURL, CloneURL: r.CloneURL, DefaultBranch: r.DefaultBranch}, nil
}

type gitIssueJSON struct {
	Number      int64  `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	State       string `json:"state"`
	URL         string `json:"html_url"`
	PullRequest any    `json:"pull_request"`
	Labels      []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (r gitIssueJSON) issue() contracts.GitIssue {
	i := contracts.GitIssue{Number: r.Number, Title: r.Title, Body: r.Body, State: r.State, URL: r.URL, PullRequest: r.PullRequest != nil}
	for _, l := range r.Labels {
		i.Labels = append(i.Labels, l.Name)
	}
	return i
}

func (m *Module) Issue(ctx context.Context, id int64, name string, number int64) (contracts.GitIssue, error) {
	path, err := issueRepoPath(name)
	if err != nil {
		return contracts.GitIssue{}, err
	}
	if number <= 0 {
		return contracts.GitIssue{}, httpx.Invalid("Issue 编号不正确")
	}
	c, err := m.connection(ctx, id)
	if err != nil {
		return contracts.GitIssue{}, err
	}
	g, err := m.client(ctx, c)
	if err != nil {
		return contracts.GitIssue{}, err
	}
	var r gitIssueJSON
	if err := g.do(ctx, http.MethodGet, path+"/issues/"+url.PathEscape(fmt.Sprint(number)), nil, &r); err != nil {
		return contracts.GitIssue{}, gitFailure(err)
	}
	return r.issue(), nil
}

func (m *Module) Issues(ctx context.Context, id int64, name string) ([]contracts.GitIssue, error) {
	path, err := issueRepoPath(name)
	if err != nil {
		return nil, err
	}
	c, err := m.connection(ctx, id)
	if err != nil {
		return nil, err
	}
	g, err := m.client(ctx, c)
	if err != nil {
		return nil, err
	}
	out := []contracts.GitIssue{}
	for page := 1; ; page++ {
		var rows []gitIssueJSON
		query := fmt.Sprintf("?state=all&per_page=100&limit=100&page=%d&type=issues", page)
		if err := g.do(ctx, http.MethodGet, path+"/issues"+query, nil, &rows); err != nil {
			return nil, gitFailure(err)
		}
		for _, r := range rows {
			if r.PullRequest != nil {
				continue
			}
			out = append(out, r.issue())
		}
		if len(rows) < 100 {
			return out, nil
		}
		if page >= 1000 {
			return nil, httpx.Invalid("仓库 Issue 太多，请缩小同步范围")
		}
	}
}

func (m *Module) UpdateGitIssue(ctx context.Context, id int64, name string, in contracts.GitIssue) error {
	path, err := issueRepoPath(name)
	if err != nil {
		return err
	}
	if in.Number <= 0 {
		return httpx.Invalid("Issue 编号不正确")
	}
	c, err := m.connection(ctx, id)
	if err != nil {
		return err
	}
	g, err := m.client(ctx, c)
	if err != nil {
		return err
	}
	body := map[string]string{"title": in.Title, "body": in.Body, "state": in.State}
	return gitFailure(g.do(ctx, http.MethodPatch, path+"/issues/"+url.PathEscape(fmt.Sprint(in.Number)), body, nil))
}
