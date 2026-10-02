package github

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
)

func (m *Module) ReceiveGitWebhook(ctx context.Context, hook contracts.GitWebhook) error {
	var ev struct {
		Action     string `json:"action"`
		Ref        string `json:"ref"`
		Deleted    bool   `json:"deleted"`
		Repository struct {
			FullName      string `json:"full_name"`
			DefaultBranch string `json:"default_branch"`
		} `json:"repository"`
		PullRequest ghPull        `json:"pull_request"`
		Review      ghReview      `json:"review"`
		Issue       ghIssue       `json:"issue"`
		Assignee    ghUser        `json:"assignee"`
		WorkflowRun ghRun         `json:"workflow_run"`
		Release     remoteRelease `json:"release"`
		Commits     []struct {
			ID      string `json:"id"`
			SHA     string `json:"sha"`
			Message string `json:"message"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(hook.Body, &ev); err != nil {
		return httpx.Invalid("请求体不是 JSON")
	}
	if ev.Repository.FullName == "" {
		return nil
	}
	repo, err := normalizeRepo(ev.Repository.FullName)
	if err != nil {
		return err
	}
	keys, err := m.selected(ctx, &hook.ConnectionID, &repo)
	if err != nil {
		if err == httpx.ErrIntegrationMissing {
			return nil
		}
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	k := keys[0]
	cfg, err := m.accountConfig(ctx, k.ConnectionID)
	if err != nil {
		return err
	}
	branch := ev.Repository.DefaultBranch
	if branch == "" {
		var info api.WatchedRepo
		if err := m.cachedOne(ctx, k, "repo", "info", &info); err == nil {
			branch = info.DefaultBranch
		}
	}
	events := []repoEvent{}
	var settled *cachedRun
	switch hook.Event {
	case "workflow_run":
		r := ev.WorkflowRun
		if r.ID <= 0 {
			return httpx.Invalid("运行编号不对")
		}
		status, conclusion := normalizedStatus(r.Status, r.Conclusion)
		v := cachedRun{GitHubRun: api.GitHubRun{Id: r.ID, Name: r.Name, Branch: r.HeadBranch, HeadSha: ptr(r.HeadSHA), Status: status, Conclusion: conclusion, DefaultBranch: branch != "" && branch == r.HeadBranch, CreatedAt: utc(r.CreatedAt)}, WorkflowID: r.WorkflowID, Attempt: max(1, r.RunAttempt)}
		events = append(events, runEvents(v, nil)...)
		settled = &v
	case "push":
		if branch == "" || ev.Ref != "refs/heads/"+branch || ev.Deleted {
			return nil
		}
		shas := []string{}
		message := ""
		for _, c := range ev.Commits {
			sha := c.ID
			if sha == "" {
				sha = c.SHA
			}
			if sha != "" {
				shas = append(shas, sha)
				message = c.Message
			}
		}
		events = append(events, repoEvent{Kind: "push", State: "default", Commits: shas, Message: message, Default: true, Tab: "commits"})
	case "pull_request", "pull_request_review":
		p := ev.PullRequest
		if p.Number <= 0 {
			return httpx.Invalid("PR 编号不对")
		}
		e := repoEvent{Object: strconv.Itoa(p.Number), Body: p.Title, Tab: "pulls"}
		switch {
		case hook.Event == "pull_request_review" && ev.Action == "submitted":
			state, err := m.reviewState(ctx, m.client(cfg), k.Repo, p)
			if err != nil {
				return err
			}
			if state == "approved" || state == "changes_requested" {
				e.Kind, e.State = "pr_review", state
			}
		case ev.Action == "opened" || ev.Action == "reopened":
			e.Kind, e.State = "pr_opened", "open"
		case ev.Action == "closed":
			e.Kind, e.State = "pr_closed", "closed"
			if p.Merged || p.MergedAt != nil {
				e.Kind, e.State = "pr_merged", "merged"
			}
		}
		if e.Kind != "" {
			events = append(events, e)
		}
	case "issues":
		i := ev.Issue
		if i.Number <= 0 {
			return httpx.Invalid("Issue 编号不对")
		}
		e := repoEvent{Object: strconv.Itoa(i.Number), Body: i.Title, Tab: "issues"}
		if ev.Action == "opened" || ev.Action == "reopened" {
			e.Kind, e.State = "issue_opened", "open"
			events = append(events, e)
		}
		assigned := strings.EqualFold(ev.Assignee.Login, cfg.Login)
		if ev.Action == "opened" {
			for _, a := range i.Assignees {
				assigned = assigned || strings.EqualFold(a.Login, cfg.Login)
			}
		}
		if cfg.Login != "" && assigned && (ev.Action == "assigned" || ev.Action == "opened") {
			e.Kind, e.State = "issue_assigned", strings.ToLower(cfg.Login)
			events = append(events, e)
		}
	case "release":
		r := ev.Release
		if (ev.Action == "published" || ev.Action == "released") && r.Tag != "" && !r.Draft && !r.Prerelease {
			events = append(events, repoEvent{Kind: "release", Object: r.Tag, State: "published", Body: r.Tag + " · " + r.Name, Tab: "issues"})
		}
	default:
		return nil
	}
	return m.processEvents(ctx, k, hook.DeliveryID, func(tx *sql.Tx) ([]repoEvent, error) {
		if settled != nil {
			recovered, err := m.settleRun(ctx, tx, k, *settled, true)
			if err != nil {
				return nil, fmt.Errorf("记录 CI 状态：%w", err)
			}
			events = append(events, recovered...)
		}
		return events, nil
	})
}
