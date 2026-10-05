package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
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

func repoInput(in *[]string) []string {
	if in == nil {
		return []string{}
	}
	return *in
}

func configToAPI(cfg config) api.GitHubConfig {
	out := api.GitHubConfig{HasToken: cfg.Token != "", Token: maskToken(cfg.Token), Repos: cfg.Repos, ApiUrl: cfg.APIURL, Watches: &cfg.Watches}
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
	m.writeConfig(w, r, cfg)
}

// writeConfig answers the config with the free CI minutes (B109).
func (m *Module) writeConfig(w http.ResponseWriter, r *http.Request, cfg config) {
	out := configToAPI(cfg)
	n, err := m.ciIncludedMinutes(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out.CiIncludedMinutes = &n
	httpx.JSON(w, http.StatusOK, out)
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
		"repos": len(repoInput(body.Repos)), "tokenChanged": body.Token != nil && *body.Token != "" || body.ClearToken != nil && *body.ClearToken,
	}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.writeConfig(w, r, cfg)
}

func (m *Module) saveConfig(ctx context.Context, in api.GitHubConfigInput) (config, error) {
	old, err := m.loadConfig(ctx)
	if err != nil {
		return config{}, err
	}
	if in.CiIncludedMinutes != nil {
		if err := m.setCIIncludedMinutes(ctx, *in.CiIncludedMinutes); err != nil {
			return config{}, err
		}
		if in.Watches == nil && in.Repos == nil && in.Token == nil && in.ClearToken == nil && in.ApiUrl == nil && in.ConnectionId == nil {
			return old, nil // only the free minutes changed
		}
	}
	if in.Watches != nil {
		watches, err := m.normalizeWatches(ctx, *in.Watches)
		if err != nil {
			return config{}, err
		}
		if err := m.d.Settings.Set(ctx, keyWatches, watches); err != nil {
			return config{}, err
		}
		return m.loadConfig(ctx)
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
	repos, err := normalizeRepos(repoInput(in.Repos))
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
	watches := []api.RepoWatch{}
	for _, repo := range repos {
		watches = append(watches, api.RepoWatch{ConnectionId: old.ConnectionID, Repo: repo})
	}
	if err := m.d.Settings.Set(ctx, keyWatches, watches); err != nil {
		return config{}, err
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
	out := api.GitHubStatus{Configured: cfg.configured(), Syncing: m.isSyncing(), RepoCount: len(cfg.Watches)}
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
	rate := m.rate
	for _, watch := range cfg.Watches {
		ac, e := m.accountConfig(ctx, watch.ConnectionId)
		if e == nil && ac.Forge == "github" {
			rate = m.accountState(ac).rate
			break
		}
	}
	if remaining, limit, reset := rate.info(); remaining >= 0 {
		out.RateLimitRemaining = &remaining
		if limit > 0 {
			out.RateLimitLimit = &limit
		}
		if !reset.IsZero() {
			out.RateLimitResetAt = &reset
		}
	}
	if cfg.configured() {
		seconds := 60
		remaining, limit, reset := rate.info()
		if remaining >= 0 && limit > 0 && remaining < limit/10 && reset.After(m.now()) {
			seconds = int(slowSyncInterval / time.Second)
		}
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

func (m *Module) ListGitHubPulls(w http.ResponseWriter, r *http.Request, p api.ListGitHubPullsParams) {
	out, err := m.listPullsSelected(r.Context(), p.ConnectionId, p.Repo)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, out)
}
func (m *Module) listPulls(ctx context.Context, repo string) ([]api.GitHubPull, error) {
	return m.listPullsSelected(ctx, nil, &repo)
}
func (m *Module) listPullsSelected(ctx context.Context, connection *int64, repo *string) ([]api.GitHubPull, error) {
	keys, err := m.selected(ctx, connection, repo)
	if err != nil {
		return nil, err
	}
	out := []api.GitHubPull{}
	for _, k := range keys {
		rows, e := m.cached(ctx, k, "pull")
		if e != nil {
			return nil, e
		}
		for _, row := range rows {
			var v cachedPull
			if e := json.Unmarshal(row.Data, &v); e != nil {
				return nil, e
			}
			out = append(out, v.GitHubPull)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Number > out[j].Number
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
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

func (m *Module) ListGitHubRuns(w http.ResponseWriter, r *http.Request, p api.ListGitHubRunsParams) {
	keys, err := m.selected(r.Context(), p.ConnectionId, p.Repo)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := []api.GitHubRun{}
	for _, k := range keys {
		rows, e := m.cached(r.Context(), k, "run")
		if e != nil {
			httpx.Fail(w, r, e)
			return
		}
		for _, row := range rows {
			var v cachedRun
			if e := json.Unmarshal(row.Data, &v); e != nil {
				httpx.Fail(w, r, e)
				return
			}
			out = append(out, v.GitHubRun)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].Id > out[j].Id
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	limit := int(httpx.Limit(p.Limit))
	if len(out) > limit {
		out = out[:limit]
	}
	httpx.JSON(w, 200, out)
}
func (m *Module) ListGitHubIssues(w http.ResponseWriter, r *http.Request, p api.ListGitHubIssuesParams) {
	keys, err := m.selected(r.Context(), p.ConnectionId, p.Repo)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := []api.GitHubIssue{}
	for _, k := range keys {
		rows, e := m.cached(r.Context(), k, "issue")
		if e != nil {
			httpx.Fail(w, r, e)
			return
		}
		for _, row := range rows {
			var v cachedIssue
			if e := json.Unmarshal(row.Data, &v); e != nil {
				httpx.Fail(w, r, e)
				return
			}
			if v.Relation != "none" && v.State == "open" {
				out = append(out, v.GitHubIssue)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	httpx.JSON(w, 200, out)
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
