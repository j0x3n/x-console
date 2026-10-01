package github

import (
	"bytes"
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
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

func configToAPI(cfg config) api.GitHubConfig {
	out := api.GitHubConfig{HasToken: cfg.Token != "", Token: maskToken(cfg.Token), Repos: cfg.Repos, ApiUrl: cfg.APIURL}
	if cfg.Login != "" && cfg.Token != "" {
		out.Login = &cfg.Login
	}
	if cfg.ConnectionID != 0 {
		out.ConnectionId = &cfg.ConnectionID
	}
	return out
}

func (m *Module) GetGitHubConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := m.loadConfig(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, configToAPI(cfg))
}

// PutGitHubConfig saves the config. Changing the token or the API address
// can send the token elsewhere, so that needs elevation.
func (m *Module) PutGitHubConfig(w http.ResponseWriter, r *http.Request) {
	var body api.GitHubConfigInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cfg, err := m.saveConfig(r.Context(), body)
	m.d.Audit.Record(r.Context(), "github.config", "", map[string]any{
		"repos": len(body.Repos), "tokenChanged": body.Token != nil && *body.Token != "" || body.ClearToken != nil && *body.ClearToken,
	}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, configToAPI(cfg))
}

func (m *Module) saveConfig(ctx context.Context, in api.GitHubConfigInput) (config, error) {
	old, err := m.loadConfig(ctx)
	if err != nil {
		return config{}, err
	}
	apiURL := old.APIURL
	if in.ApiUrl != nil {
		if apiURL, err = normalizeAPIURL(*in.ApiUrl); err != nil {
			return config{}, err
		}
	}
	token := old.Token
	newToken := in.Token != nil && strings.TrimSpace(*in.Token) != ""
	clear := in.ClearToken != nil && *in.ClearToken
	if newToken || clear || apiURL != old.APIURL {
		if err := auth.RequireElevated(ctx); err != nil {
			return config{}, err
		}
	}
	switch {
	case clear:
		token = ""
	case newToken:
		token = strings.TrimSpace(*in.Token)
	}
	repos, err := normalizeRepos(in.Repos)
	if err != nil {
		return config{}, err
	}
	if in.ConnectionId != nil && *in.ConnectionId != old.ConnectionID {
		if err := m.setConnection(ctx, *in.ConnectionId); err != nil {
			return config{}, err
		}
		if err := m.d.Settings.Delete(ctx, keyLogin); err != nil {
			return config{}, err
		}
		m.etag.clear()
		old.ConnectionID = *in.ConnectionId
	}
	if old.ConnectionID != 0 {
		// The token belongs to the Git account; only the repos are ours.
		if err := m.d.Settings.Set(ctx, keyRepos, repos); err != nil {
			return config{}, err
		}
		return m.loadConfig(ctx)
	}
	if token == "" {
		if err := m.d.Settings.Delete(ctx, keyToken); err != nil {
			return config{}, err
		}
	} else if token != old.Token {
		if err := m.d.Settings.SetSecret(ctx, keyToken, token); err != nil {
			return config{}, err
		}
	}
	if err := m.d.Settings.Set(ctx, keyRepos, repos); err != nil {
		return config{}, err
	}
	if err := m.d.Settings.Set(ctx, keyAPIURL, apiURL); err != nil {
		return config{}, err
	}
	if token != old.Token || apiURL != old.APIURL {
		// Another account: forget who we were and what we cached.
		if err := m.d.Settings.Delete(ctx, keyLogin); err != nil {
			return config{}, err
		}
		m.etag.clear()
	}
	return m.loadConfig(ctx)
}

// TestGitHub checks the stored token, or the form values in the body.
func (m *Module) TestGitHub(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var cfg config
	if len(bytes.TrimSpace(raw)) == 0 {
		if cfg, err = m.requireConfigured(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	} else {
		if err := auth.RequireElevated(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		var body api.GitHubTestInput
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			httpx.Fail(w, r, httpx.Invalid("请求体格式不正确: "+err.Error()))
			return
		}
		if cfg, err = m.loadConfig(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if body.Token != nil && strings.TrimSpace(*body.Token) != "" {
			cfg.Token = strings.TrimSpace(*body.Token)
		}
		if body.ApiUrl != nil {
			if cfg.APIURL, err = normalizeAPIURL(*body.ApiUrl); err != nil {
				httpx.Fail(w, r, err)
				return
			}
		}
		if cfg.Token == "" {
			httpx.Fail(w, r, httpx.Invalid("请填写令牌"))
			return
		}
	}
	httpx.JSON(w, http.StatusOK, m.test(r.Context(), cfg))
}

func (m *Module) test(ctx context.Context, cfg config) api.GitHubTestResult {
	c := m.client(cfg)
	c.etag = nil // always ask GitHub
	var me ghUser
	if err := c.get(ctx, "/user", nil, &me); err != nil {
		msg := err.Error()
		return api.GitHubTestResult{Ok: false, Message: &msg}
	}
	out := api.GitHubTestResult{Ok: true, Login: &me.Login}
	if remaining, _ := m.rate.snapshot(); remaining >= 0 {
		out.RateLimitRemaining = &remaining
	}
	return out
}

func (m *Module) status(ctx context.Context) (api.GitHubStatus, error) {
	cfg, err := m.loadConfig(ctx)
	if err != nil {
		return api.GitHubStatus{}, err
	}
	out := api.GitHubStatus{Configured: cfg.configured(), Syncing: m.isSyncing(), RepoCount: len(cfg.Repos)}
	if cfg.configured() && cfg.Login != "" {
		out.Login = &cfg.Login
	}
	var last lastSync
	if err := m.d.Settings.Get(ctx, keyLastSync, &last); err == nil {
		out.LastSyncAt = &last.At
		if last.Error != "" {
			out.LastError = &last.Error
		}
	} else if !errors.Is(err, settings.ErrNotSet) {
		return out, err
	}
	if remaining, limit, reset := m.rate.info(); remaining >= 0 {
		out.RateLimitRemaining = &remaining
		if limit > 0 {
			out.RateLimitLimit = &limit
		}
		if !reset.IsZero() {
			out.RateLimitResetAt = &reset
		}
	}
	if cfg.configured() {
		seconds := int(m.currentSyncInterval() / time.Second)
		out.SyncIntervalSeconds = &seconds
	}
	return out, nil
}

func (m *Module) GetGitHubStatus(w http.ResponseWriter, r *http.Request) {
	out, err := m.status(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// SyncGitHub syncs now. A failed sync still answers 200 with the error in
// the status; only a missing setup is an error.
func (m *Module) SyncGitHub(w http.ResponseWriter, r *http.Request) {
	if err := m.sync(r.Context()); errors.Is(err, httpx.ErrIntegrationMissing) {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.status(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) ListGitHubPulls(w http.ResponseWriter, r *http.Request, params api.ListGitHubPullsParams) {
	var repo string
	if params.Repo != nil {
		repo = *params.Repo
	}
	out, err := m.listPulls(r.Context(), repo)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) listPulls(ctx context.Context, repo string) ([]api.GitHubPull, error) {
	if _, err := m.requireConfigured(ctx); err != nil {
		return nil, err
	}
	var rows []db.GithubPull
	var err error
	if repo != "" {
		rows, err = m.q.ListPullsByRepo(ctx, repo)
	} else {
		rows, err = m.q.ListPulls(ctx)
	}
	if err != nil {
		return nil, err
	}
	links, err := m.q.ListLinks(ctx)
	if err != nil {
		return nil, err
	}
	type pullKey struct {
		repo   string
		number int64
	}
	byPull := map[pullKey][]db.GithubLink{}
	for _, l := range links {
		k := pullKey{l.Repo, l.Number}
		byPull[k] = append(byPull[k], l)
	}
	out := make([]api.GitHubPull, len(rows))
	for i, p := range rows {
		out[i] = pullToAPI(p, byPull[pullKey{p.Repo, p.Number}])
	}
	return out, nil
}

func pullToAPI(p db.GithubPull, links []db.GithubLink) api.GitHubPull {
	out := api.GitHubPull{
		Repo: p.Repo, Number: int(p.Number), Title: p.Title, Author: p.Author, Url: p.Url, HeadRef: p.HeadRef,
		BaseRef: p.BaseRef, Draft: p.Draft, State: api.GitHubPullState(p.State), ReviewState: api.GitHubReviewState(p.ReviewState),
		CheckState: api.GitHubCheckState(p.CheckState), IssueKeys: []string{}, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
	for _, l := range links {
		switch l.Kind {
		case "issue":
			out.IssueKeys = append(out.IssueKeys, l.Ref)
		case "coding_task":
			if id, err := strconv.ParseInt(l.Ref, 10, 64); err == nil {
				out.CodingTaskId = &id
			}
		}
	}
	return out
}

func (m *Module) ListGitHubRuns(w http.ResponseWriter, r *http.Request, params api.ListGitHubRunsParams) {
	ctx := r.Context()
	if _, err := m.requireConfigured(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	limit := httpx.Limit(params.Limit)
	var rows []db.GithubRun
	var err error
	if params.Repo != nil && *params.Repo != "" {
		rows, err = m.q.ListRunsByRepo(ctx, db.ListRunsByRepoParams{Repo: *params.Repo, Limit: limit})
	} else {
		rows, err = m.q.ListRuns(ctx, limit)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.GitHubRun, len(rows))
	for i, run := range rows {
		out[i] = api.GitHubRun{
			Id: run.ID, Repo: run.Repo, Name: run.Name, Branch: run.Branch, Event: run.Event, Status: run.Status,
			Conclusion: run.Conclusion, Url: run.Url, DefaultBranch: run.DefaultBranch, CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) ListGitHubIssues(w http.ResponseWriter, r *http.Request, _ api.ListGitHubIssuesParams) {
	ctx := r.Context()
	if _, err := m.requireConfigured(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err := m.q.ListIssues(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.GitHubIssue, len(rows))
	for i, is := range rows {
		item := api.GitHubIssue{
			Repo: is.Repo, Number: int(is.Number), Title: is.Title, Url: is.Url, Author: is.Author,
			Assignees: []string{}, Labels: []string{}, Relation: api.GitHubIssueRelation(is.Relation),
			CreatedAt: is.CreatedAt, UpdatedAt: is.UpdatedAt,
		}
		_ = json.Unmarshal([]byte(is.Assignees), &item.Assignees)
		_ = json.Unmarshal([]byte(is.Labels), &item.Labels)
		out[i] = item
	}
	httpx.JSON(w, http.StatusOK, out)
}

// setConnection picks the Git account the module uses (B62). 0 goes back to
// the old token setting.
func (m *Module) setConnection(ctx context.Context, id int64) error {
	if id == 0 {
		return m.d.Settings.Delete(ctx, keyConnectionID)
	}
	accounts, ok := module.Lookup[contracts.GitAccounts](m.d.Registry, contracts.GitAccountsKey)
	if !ok {
		return httpx.NewError(http.StatusNotImplemented, "feature_unavailable", "Git 账号功能没有启用")
	}
	a, err := accounts.Account(ctx, id)
	if errors.Is(err, httpx.ErrNotFound) {
		return httpx.Invalid("没有这个 Git 账号")
	}
	if err != nil {
		return err
	}
	if a.Kind != "github" {
		return httpx.Invalid("GitHub 页面只能用 GitHub 账号")
	}
	return m.d.Settings.Set(ctx, keyConnectionID, id)
}
