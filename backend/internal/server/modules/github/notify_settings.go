package github

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const keyNotify = "github.notify"

var notifyEvents = map[string]string{"ci_started": "CI 开始", "ci_succeeded": "CI 成功", "ci_failed": "CI 失败", "ci_cancelled": "CI 取消", "ci_recovered": "CI 恢复", "push": "新提交", "pr_opened": "新 PR", "pr_merged": "PR 合并", "pr_closed": "PR 关闭", "pr_review": "PR 评审", "issue_opened": "新 Issue", "issue_assigned": "Issue 指派", "release": "发布版本"}

func defaultNotify() api.GitHubNotifySettings {
	return api.GitHubNotifySettings{Defaults: api.RepoNotify{Events: []string{"ci_failed", "ci_recovered", "pr_opened", "pr_merged", "pr_review", "issue_assigned", "release"}, CiBranches: api.Default}, Repos: []api.RepoNotifyOverride{}}
}
func (m *Module) notifySettings(ctx context.Context) (api.GitHubNotifySettings, error) {
	out := defaultNotify()
	if err := m.d.Settings.Get(ctx, keyNotify, &out); err != nil && err != settings.ErrNotSet {
		return out, err
	}
	if out.Repos == nil {
		out.Repos = []api.RepoNotifyOverride{}
	}
	if out.CiQuota == nil {
		on := true // B109: on unless turned off
		out.CiQuota = &on
	}
	return out, nil
}
func normalizeNotify(n api.RepoNotify) (api.RepoNotify, error) {
	if n.CiBranches != api.Default && n.CiBranches != api.All {
		return n, httpx.Invalid("CI 范围只能是 default 或 all")
	}
	seen := map[string]bool{}
	events := []string{}
	for _, e := range n.Events {
		if _, ok := notifyEvents[e]; !ok {
			return n, httpx.Invalid("不支持的仓库通知事件：" + e)
		}
		if !seen[e] {
			seen[e] = true
			events = append(events, e)
		}
	}
	sort.Strings(events)
	n.Events = events
	return n, nil
}
func effectiveNotify(s api.GitHubNotifySettings, k repoKey) (api.RepoNotify, bool) {
	for _, r := range s.Repos {
		if r.ConnectionId == k.ConnectionID && strings.EqualFold(r.Repo, k.Repo) {
			return r.Notify, true
		}
	}
	return s.Defaults, false
}
func (m *Module) GetGitHubNotify(w http.ResponseWriter, r *http.Request) {
	out, err := m.notifySettings(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, out)
}
func (m *Module) PutGitHubNotify(w http.ResponseWriter, r *http.Request) {
	var in api.GitHubNotifySettings
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defaults, err := normalizeNotify(in.Defaults)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in.Defaults = defaults
	if len(in.Repos) > 100 {
		httpx.Fail(w, r, httpx.Invalid("最多设置 100 个仓库通知"))
		return
	}
	if in.Repos == nil {
		in.Repos = []api.RepoNotifyOverride{}
	}
	if in.CiQuota == nil {
		// B109: the bell on one repository sends the whole settings without it.
		old, err := m.notifySettings(r.Context())
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		in.CiQuota = old.CiQuota
	}
	seen := map[repoKey]bool{}
	for i, override := range in.Repos {
		repo, err := normalizeRepo(override.Repo)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		k := repoKey{override.ConnectionId, strings.ToLower(repo)}
		if seen[k] {
			httpx.Fail(w, r, httpx.Invalid("仓库通知设置重复"))
			return
		}
		seen[k] = true
		if _, err := m.resolveRepo(r.Context(), ptr(k.ConnectionID), repo); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		n, err := normalizeNotify(override.Notify)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		in.Repos[i] = api.RepoNotifyOverride{ConnectionId: k.ConnectionID, Repo: repo, Notify: n}
	}
	err = m.d.Settings.Set(r.Context(), keyNotify, in)
	m.d.Audit.Record(r.Context(), "github.notify", "", map[string]any{"repos": len(in.Repos)}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("github.notify_changed", in)
	httpx.JSON(w, 200, in)
}
