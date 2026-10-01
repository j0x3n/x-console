package aiagents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/api"
)

// gitAPI talks to GitHub or Forgejo (Gitea has the same API) with one token.
type gitAPI struct {
	kind  string // github, forgejo
	base  string // REST base: https://api.github.com or https://git.example.com/api/v1
	token string
	hc    *http.Client
}

const (
	kindGitHub  = "github"
	kindForgejo = "forgejo"
)

const defaultGitHubAPI = "https://api.github.com"

// apiBase turns the stored base_url into the REST base.
func apiBase(kind, baseURL string) string {
	if kind == kindForgejo {
		return strings.TrimRight(baseURL, "/") + "/api/v1"
	}
	return strings.TrimRight(baseURL, "/")
}

// normalizeBaseURL checks a user entered address. For Forgejo it keeps the
// site address: a pasted ".../api/v1" is cut off.
func normalizeBaseURL(kind, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if kind == kindGitHub {
			return defaultGitHubAPI, nil
		}
		return "", httpx.Invalid("Forgejo 要填站点地址，比如 https://git.example.com")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", httpx.Invalid("地址格式不对，要以 https:// 开头")
	}
	out := strings.TrimRight(u.Scheme+"://"+u.Host+u.Path, "/")
	if kind == kindForgejo {
		out = strings.TrimSuffix(out, "/api/v1")
	}
	return out, nil
}

// gitError is an error answer of the Git service.
type gitError struct {
	status int
	msg    string
}

func (e *gitError) Error() string { return fmt.Sprintf("Git 服务返回 %d：%s", e.status, e.msg) }

func (g *gitAPI) do(ctx context.Context, method, path string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if g.kind == kindGitHub {
		req.Header.Set("Authorization", "Bearer "+g.token)
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	} else {
		req.Header.Set("Authorization", "token "+g.token)
	}
	resp, err := g.hc.Do(req)
	if err != nil {
		return fmt.Errorf("连不上 Git 服务：%w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		msg := e.Message
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
			if len(msg) > 200 {
				msg = msg[:200]
			}
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return &gitError{status: resp.StatusCode, msg: msg}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("Git 服务的回答看不懂：%w", err)
	}
	return nil
}

// user returns the login of the token.
func (g *gitAPI) user(ctx context.Context) (string, error) {
	var u struct {
		Login    string `json:"login"`
		Username string `json:"username"`
	}
	if err := g.do(ctx, http.MethodGet, "/user", nil, &u); err != nil {
		return "", err
	}
	if u.Login == "" {
		u.Login = u.Username
	}
	return u.Login, nil
}

type remoteRepo struct {
	Owner struct {
		Login    string `json:"login"`
		Username string `json:"username"`
	} `json:"owner"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	CloneURL      string    `json:"clone_url"`
	HTMLURL       string    `json:"html_url"`
	DefaultBranch string    `json:"default_branch"`
	Private       bool      `json:"private"`
	Description   string    `json:"description"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (r remoteRepo) toAPI() api.RemoteRepo {
	owner := r.Owner.Login
	if owner == "" {
		owner = r.Owner.Username
	}
	out := api.RemoteRepo{Owner: owner, Name: r.Name, FullName: r.FullName, CloneUrl: r.CloneURL,
		DefaultBranch: r.DefaultBranch, Private: r.Private}
	if r.HTMLURL != "" {
		out.HtmlUrl = &r.HTMLURL
	}
	if r.Description != "" {
		out.Description = &r.Description
	}
	return out
}

// repos lists up to 100 repositories the token can reach, most recently
// updated first, whose name contains q.
func (g *gitAPI) repos(ctx context.Context, q string) ([]api.RemoteRepo, error) {
	var list []remoteRepo
	if g.kind == kindGitHub {
		if err := g.do(ctx, http.MethodGet, "/user/repos?per_page=100&sort=updated&affiliation=owner,collaborator,organization_member", nil, &list); err != nil {
			return nil, err
		}
	} else {
		var res struct {
			Data []remoteRepo `json:"data"`
		}
		path := "/repos/search?limit=50&sort=updated&order=desc&q=" + url.QueryEscape(q)
		if err := g.do(ctx, http.MethodGet, path, nil, &res); err != nil {
			return nil, err
		}
		list = res.Data
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
	q = strings.ToLower(strings.TrimSpace(q))
	out := []api.RemoteRepo{}
	for _, r := range list {
		if q != "" && !strings.Contains(strings.ToLower(r.FullName), q) {
			continue
		}
		out = append(out, r.toAPI())
		if len(out) == 100 {
			break
		}
	}
	return out, nil
}

// createPR opens a pull request. in.Repo is "owner/name".
func (g *gitAPI) createPR(ctx context.Context, in contracts.CreatePR) (string, int, error) {
	owner, name, ok := strings.Cut(in.Repo, "/")
	if !ok || owner == "" || name == "" {
		return "", 0, httpx.Invalid("仓库要写成 owner/name")
	}
	body := map[string]any{"title": in.Title, "head": in.Head, "base": in.Base, "body": in.Body}
	if g.kind == kindGitHub {
		body["draft"] = in.Draft
	}
	var out struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name) + "/pulls"
	if err := g.do(ctx, http.MethodPost, path, body, &out); err != nil {
		return "", 0, err
	}
	if out.HTMLURL == "" {
		return "", 0, fmt.Errorf("Git 服务没有返回 PR 地址（编号 %s）", strconv.Itoa(out.Number))
	}
	return out.HTMLURL, out.Number, nil
}
