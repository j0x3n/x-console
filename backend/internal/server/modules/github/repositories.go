package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

type ghCommit struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Author  *struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"author"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name string    `json:"name"`
			Date time.Time `json:"date"`
		} `json:"author"`
		Committer struct {
			Date time.Time `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

func pageQuery(c *restClient, n, page int) url.Values {
	key := "per_page"
	if c.forge == "forgejo" {
		key = "limit"
	}
	return url.Values{key: {strconv.Itoa(n)}, "page": {strconv.Itoa(page)}}
}
func (m *Module) fetchIssues(ctx context.Context, c *restClient, repo string) ([]ghIssue, error) {
	out := []ghIssue{}
	for page := 1; page <= 20; page++ {
		var batch []ghIssue
		q := pageQuery(c, 100, page)
		q.Set("state", "open")
		if c.forge == "forgejo" {
			q.Set("type", "issues")
		}
		more, err := c.getPage(ctx, "/repos/"+repo+"/issues", q, &batch)
		if err != nil {
			return nil, err
		}
		for _, i := range batch {
			if len(i.PullRequest) == 0 || string(i.PullRequest) == "null" {
				out = append(out, i)
			}
		}
		if !more && (c.forge != "forgejo" || len(batch) < 100) {
			break
		}
	}
	return out, nil
}
func (m *Module) fetchCommits(ctx context.Context, c *restClient, repo, branch string) ([]ghCommit, error) {
	out := []ghCommit{}
	q := pageQuery(c, 30, 1)
	q.Set("sha", branch)
	if c.forge == "forgejo" {
		q.Set("stat", "false")
		q.Set("files", "false")
		q.Set("verification", "false")
	}
	if err := c.get(ctx, "/repos/"+repo+"/commits", q, &out); err != nil {
		if statusOf(err) == 409 {
			return out, nil
		}
		return nil, err
	}
	return out, nil
}
func (m *Module) syncRepository(ctx context.Context, c *restClient, cfg config, k repoKey) error {
	var raw ghRepo
	info := api.WatchedRepo{ConnectionId: k.ConnectionID, Forge: api.Forge(cfg.Forge), Repo: k.Repo, ConnectionName: ptr(cfg.Name), Url: strings.TrimSuffix(c.base, "/api/v1") + "/" + k.Repo}
	if cfg.Forge == "github" {
		info.Url = "https://github.com/" + k.Repo
	}
	if err := c.get(ctx, "/repos/"+k.Repo, nil, &raw); err != nil {
		_ = m.cachedOne(ctx, k, "repo", "info", &info)
		info.SyncError = ptr(err.Error())
		_ = m.putObject(ctx, k, "repo", object("info", info))
		return err
	}
	info.DefaultBranch, info.Private, info.PushedAt = raw.DefaultBranch, raw.Private, raw.PushedAt
	if raw.HTMLURL != "" {
		info.Url = raw.HTMLURL
	}
	if raw.Description != "" {
		info.Description = ptr(raw.Description)
	}
	var errs []error
	pulls, err := m.recentPulls(ctx, c, k.Repo)
	if err != nil {
		errs = append(errs, err)
	} else {
		objects := []cacheObject{}
		for _, p := range pulls {
			review, checks := "none", "none"
			if p.State == "open" {
				review, err = m.reviewState(ctx, c, k.Repo, p)
				if err != nil {
					break
				}
				checks, err = m.checkState(ctx, c, k.Repo, p.Head.SHA)
				if err != nil {
					break
				}
				info.OpenPulls++
			}
			state := p.State
			if p.MergedAt != nil || p.Merged {
				state = "merged"
			}
			v := cachedPull{GitHubPull: api.GitHubPull{ConnectionId: ptr(k.ConnectionID), Forge: ptr(api.Forge(cfg.Forge)), Repo: k.Repo, Number: p.Number, Title: p.Title, Author: p.User.Login, Url: p.HTMLURL, HeadRef: p.Head.Ref, BaseRef: p.Base.Ref, Draft: p.Draft, State: api.GitHubPullState(state), ReviewState: api.GitHubReviewState(review), CheckState: api.GitHubCheckState(checks), IssueKeys: []string{}, CreatedAt: utc(p.CreatedAt), UpdatedAt: utc(p.UpdatedAt)}, HeadSHA: p.Head.SHA}
			m.linkRepositoryPull(ctx, k, &v)
			objects = append(objects, object(strconv.Itoa(p.Number), v))
			if p.State == "open" {
				if e := m.ciTransition(ctx, fmt.Sprintf("pr:%d:%s#%d", k.ConnectionID, k.Repo, p.Number), checks, notify.Notification{Kind: "github.ci_failed", Title: fmt.Sprintf("PR 检查失败：%s#%d", k.Repo, p.Number), Body: p.Title, Link: repoLink(k, "pulls"), Source: "github", Priority: notify.PriorityHigh}); e != nil {
					err = e
					break
				}
			}
		}
		if err != nil {
			errs = append(errs, err)
		} else if err = m.replaceObjects(ctx, k, "pull", objects); err != nil {
			errs = append(errs, err)
		}
	}
	issues, err := m.fetchIssues(ctx, c, k.Repo)
	if err != nil {
		errs = append(errs, err)
	} else {
		objects := []cacheObject{}
		for _, i := range issues {
			assignees := []string{}
			assigned := false
			for _, a := range i.Assignees {
				assignees = append(assignees, a.Login)
				assigned = assigned || strings.EqualFold(a.Login, cfg.Login)
			}
			labels := []string{}
			for _, l := range i.Labels {
				labels = append(labels, l.Name)
			}
			relation := "none"
			created := strings.EqualFold(i.User.Login, cfg.Login)
			if assigned && created {
				relation = "both"
			} else if assigned {
				relation = "assigned"
			} else if created {
				relation = "created"
			}
			v := cachedIssue{GitHubIssue: api.GitHubIssue{ConnectionId: ptr(k.ConnectionID), Forge: ptr(api.Forge(cfg.Forge)), Repo: k.Repo, Number: i.Number, Title: i.Title, Url: i.HTMLURL, Author: i.User.Login, Assignees: assignees, Labels: labels, Relation: api.GitHubIssueRelation(relation), CreatedAt: utc(i.CreatedAt), UpdatedAt: utc(i.UpdatedAt)}, State: "open"}
			objects = append(objects, object(strconv.Itoa(i.Number), v))
		}
		info.OpenIssues = len(objects)
		if err = m.replaceObjects(ctx, k, "issue", objects); err != nil {
			errs = append(errs, err)
		}
	}
	commits, err := m.fetchCommits(ctx, c, k.Repo, info.DefaultBranch)
	if err != nil {
		errs = append(errs, err)
	} else {
		objects := []cacheObject{}
		for _, v := range commits {
			author := v.Commit.Author.Name
			var avatar *string
			if v.Author != nil {
				if v.Author.Login != "" {
					author = v.Author.Login
				}
				if v.Author.AvatarURL != "" {
					avatar = ptr(v.Author.AvatarURL)
				}
			}
			at := v.Commit.Committer.Date
			if at.IsZero() {
				at = v.Commit.Author.Date
			}
			item := api.GitHubCommit{ConnectionId: k.ConnectionID, Forge: api.Forge(cfg.Forge), Repo: k.Repo, Sha: v.SHA, Message: v.Commit.Message, Author: author, AvatarUrl: avatar, Url: v.HTMLURL, CommittedAt: utc(at), CheckState: api.GitHubCheckStateNone}
			objects = append(objects, object(v.SHA, item))
		}
		if len(commits) > 0 {
			v := commits[0]
			author := v.Commit.Author.Name
			if v.Author != nil && v.Author.Login != "" {
				author = v.Author.Login
			}
			at := v.Commit.Committer.Date
			if at.IsZero() {
				at = v.Commit.Author.Date
			}
			info.LastCommit = &struct {
				At      time.Time `json:"at"`
				Author  string    `json:"author"`
				Message string    `json:"message"`
				Sha     string    `json:"sha"`
			}{utc(at), author, firstLine(v.Commit.Message), v.SHA}
		}
		if err = m.replaceObjects(ctx, k, "commit", objects); err != nil {
			errs = append(errs, err)
		}
	}
	runs, err := m.fetchRuns(ctx, c, k, info.DefaultBranch, commits)
	if err != nil {
		errs = append(errs, err)
	} else {
		objects := []cacheObject{}
		settled := map[int64]bool{}
		for _, v := range runs {
			if v.Status == "in_progress" && !v.Synthetic {
				jobs, e := m.fetchJobs(ctx, c, k, v.Id)
				if e != nil && !errors.Is(e, errJobsUnsupported) {
					errs = append(errs, e)
				}
				if e == nil {
					done, total, current := jobProgress(jobs)
					v.StepsDone = ptr(done)
					v.StepsTotal = ptr(total)
					if current != "" {
						v.CurrentStep = ptr(current)
					}
				}
			}
			if v.DefaultBranch && info.Ci == nil {
				info.Ci = &struct {
					Conclusion string     `json:"conclusion"`
					RunId      *int64     `json:"runId,omitempty"`
					Status     string     `json:"status"`
					UpdatedAt  *time.Time `json:"updatedAt,omitempty"`
				}{v.Conclusion, ptr(v.Id), v.Status, ptr(v.UpdatedAt)}
			}
			if v.DefaultBranch && v.Status == "completed" && !settled[v.WorkflowID] {
				state := runState(v.Conclusion)
				if state != "" {
					settled[v.WorkflowID] = true
					if e := m.ciTransition(ctx, fmt.Sprintf("run:%d:%s:%d", k.ConnectionID, k.Repo, v.WorkflowID), state, notify.Notification{Kind: "github.ci_failed", Title: fmt.Sprintf("%s 的 %s 失败了", k.Repo, v.Name), Body: "分支 " + v.Branch, Link: repoLink(k, "runs"), Source: "github", Priority: notify.PriorityHigh}); e != nil {
						errs = append(errs, e)
					}
				}
			}
			objects = append(objects, object(strconv.FormatInt(v.Id, 10), v))
		}
		if err = m.replaceObjects(ctx, k, "run", objects); err != nil {
			errs = append(errs, err)
		}
	}
	if err := m.putObject(ctx, k, "repo", object("info", info)); err != nil {
		return err
	}
	return errors.Join(errs...)
}
func repoLink(k repoKey, tab string) string {
	return "/github?" + url.Values{"repo": {fmt.Sprintf("%d:%s", k.ConnectionID, k.Repo)}, "tab": {tab}}.Encode()
}
func (m *Module) linkRepositoryPull(ctx context.Context, k repoKey, p *cachedPull) {
	m.linkPullScoped(ctx, k, p.Number, p.Title, p.HeadRef, p.Url)
	rows, err := m.cached(ctx, k, "link")
	if err != nil {
		return
	}
	for _, r := range rows {
		var l struct {
			Number    int64
			Kind, Ref string
		}
		if json.Unmarshal(r.Data, &l) != nil || l.Number != int64(p.Number) {
			continue
		}
		if l.Kind == "issue" {
			p.IssueKeys = append(p.IssueKeys, l.Ref)
		}
		if l.Kind == "coding_task" {
			if id, e := strconv.ParseInt(l.Ref, 10, 64); e == nil {
				p.CodingTaskId = ptr(id)
			}
		}
	}
}
func (m *Module) ListGitHubWatchedRepos(w http.ResponseWriter, r *http.Request) {
	keys, err := m.selected(r.Context(), nil, nil)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := []api.WatchedRepo{}
	for _, k := range keys {
		var v api.WatchedRepo
		if e := m.cachedOne(r.Context(), k, "repo", "info", &v); e != nil {
			cfg, e := m.accountConfig(r.Context(), k.ConnectionID)
			v = api.WatchedRepo{ConnectionId: k.ConnectionID, Repo: k.Repo, Forge: api.Forge(cfg.Forge), ConnectionName: ptr(cfg.Name)}
			if e != nil {
				v.Forge = api.Github
				v.SyncError = ptr(e.Error())
			}
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		af := a.Ci != nil && a.Ci.Conclusion == "failure"
		bf := b.Ci != nil && b.Ci.Conclusion == "failure"
		if af != bf {
			return af
		}
		if a.PushedAt != nil && b.PushedAt != nil {
			return a.PushedAt.After(*b.PushedAt)
		}
		return a.PushedAt != nil
	})
	httpx.JSON(w, 200, out)
}
func (m *Module) ListGitHubCommits(w http.ResponseWriter, r *http.Request, p api.ListGitHubCommitsParams) {
	keys, err := m.selected(r.Context(), p.ConnectionId, p.Repo)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := []api.GitHubCommit{}
	for _, k := range keys {
		rows, e := m.cached(r.Context(), k, "commit")
		if e != nil {
			httpx.Fail(w, r, e)
			return
		}
		runs, e := m.cached(r.Context(), k, "run")
		if e != nil {
			httpx.Fail(w, r, e)
			return
		}
		for _, row := range rows {
			var v api.GitHubCommit
			if e := json.Unmarshal(row.Data, &v); e != nil {
				httpx.Fail(w, r, e)
				return
			}
			latest := map[int64]cachedRun{}
			for _, rr := range runs {
				var run cachedRun
				_ = json.Unmarshal(rr.Data, &run)
				if run.HeadSha == nil || *run.HeadSha != v.Sha {
					continue
				}
				prev, ok := latest[run.WorkflowID]
				if !ok || run.UpdatedAt.After(prev.UpdatedAt) || run.UpdatedAt.Equal(prev.UpdatedAt) && run.Id > prev.Id {
					latest[run.WorkflowID] = run
				}
			}
			states := []string{}
			for _, run := range latest {
				state := runState(run.Conclusion)
				if run.Status != "completed" {
					state = "pending"
				}
				if state != "" {
					states = append(states, state)
				}
				if v.RunId == nil || run.Id > *v.RunId {
					v.RunId = ptr(run.Id)
				}
			}
			v.CheckState = api.GitHubCheckState(combineChecks(states))
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CommittedAt.Equal(out[j].CommittedAt) {
			return out[i].Sha > out[j].Sha
		}
		return out[i].CommittedAt.After(out[j].CommittedAt)
	})
	limit := int(httpx.Limit(p.Limit))
	if len(out) > limit {
		out = out[:limit]
	}
	httpx.JSON(w, 200, out)
}
