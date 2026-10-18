package github

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
)

type workflowsEntry struct {
	At    time.Time
	Items []api.GitHubWorkflow
}

type workflowsCache struct {
	mu      sync.Mutex
	entries map[string]workflowsEntry
}

// githubRepo resolves only watched GitHub repositories. A same-name Forgejo
// repository cannot select the credentials used for GitHub Actions.
func (m *Module) githubRepo(ctx context.Context, owner, repo string, connection *int64) (repoKey, config, error) {
	full := owner + "/" + repo
	if !repoRe.MatchString(full) || owner == "." || owner == ".." || repo == "." || repo == ".." {
		return repoKey{}, config{}, httpx.ErrNotFound
	}
	stored, err := m.loadConfig(ctx)
	if err != nil {
		return repoKey{}, config{}, err
	}
	var key repoKey
	var cfg config
	found := false
	for _, watch := range stored.Watches {
		if !strings.EqualFold(watch.Repo, full) || connection != nil && watch.ConnectionId != *connection {
			continue
		}
		if watch.ConnectionId != 0 {
			accounts, ok := module.Lookup[contracts.GitAccounts](m.d.Registry, contracts.GitAccountsKey)
			if !ok {
				return repoKey{}, config{}, httpx.ErrNotFound
			}
			account, err := accounts.Account(ctx, watch.ConnectionId)
			if errors.Is(err, httpx.ErrNotFound) {
				continue
			}
			if err != nil {
				return repoKey{}, config{}, err
			}
			if account.Kind != "github" {
				continue
			}
		}
		candidate, err := m.accountConfig(ctx, watch.ConnectionId)
		if errors.Is(err, httpx.ErrNotFound) {
			continue
		}
		if err != nil {
			return repoKey{}, config{}, err
		}
		if candidate.Forge != "github" {
			continue
		}
		if found {
			return repoKey{}, config{}, httpx.Invalid("这个仓库属于多个账号，请指定 connectionId")
		}
		key, cfg, found = repoKey{watch.ConnectionId, watch.Repo}, candidate, true
	}
	if !found {
		return repoKey{}, config{}, httpx.ErrNotFound
	}
	return key, cfg, nil
}

func workflowError(err error) error {
	if err == nil {
		return nil
	}
	var upstream *apiError
	if errors.As(err, &upstream) {
		message := upstream.Error()
		if upstream.Status == 403 || upstream.Status == 404 {
			message = upstream.Message + "。经典令牌要 repo 和 workflow 权限，细粒度令牌要 Actions 读写。"
		}
		return httpx.NewError(502, "git_service_error", message)
	}
	var limited *rateLimitError
	if errors.As(err, &limited) {
		if limited.Response != nil {
			return workflowError(limited.Response)
		}
		return httpx.NewError(502, "git_service_error", err.Error())
	}
	return err
}

func (m *Module) ListGitHubRepoWorkflows(w http.ResponseWriter, r *http.Request, owner, repo string, p api.ListGitHubRepoWorkflowsParams) {
	k, cfg, err := m.githubRepo(r.Context(), owner, repo, p.ConnectionId)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := m.fetchWorkflows(r.Context(), m.client(cfg), k, p.Refresh != nil && *p.Refresh)
	if err != nil {
		httpx.Fail(w, r, workflowError(err))
		return
	}
	httpx.JSON(w, 200, struct {
		Items []api.GitHubWorkflow `json:"items"`
	}{items})
}

func (m *Module) fetchWorkflows(ctx context.Context, c *restClient, k repoKey, refresh bool) ([]api.GitHubWorkflow, error) {
	m.workflows.mu.Lock()
	defer m.workflows.mu.Unlock()
	key := fmt.Sprintf("%d:%s:%s:%s", k.ConnectionID, k.Repo, c.base, c.token)
	if e, ok := m.workflows.entries[key]; ok && !refresh && m.now().Sub(e.At) < 5*time.Minute {
		return e.Items, nil
	}
	items := []api.GitHubWorkflow{}
	for page := 1; ; page++ {
		var batch struct {
			Workflows []struct {
				ID    int64  `json:"id"`
				Name  string `json:"name"`
				Path  string `json:"path"`
				State string `json:"state"`
			} `json:"workflows"`
		}
		more, err := c.getPage(ctx, "/repos/"+k.Repo+"/actions/workflows", pageQuery(c, 100, page), &batch)
		if err != nil {
			return nil, err
		}
		for _, workflow := range batch.Workflows {
			// Disabled workflows cannot be dispatched.
			if workflow.State != "" && workflow.State != "active" {
				continue
			}
			var file struct {
				Encoding string `json:"encoding"`
				Content  string `json:"content"`
			}
			if err := c.get(ctx, "/repos/"+k.Repo+"/contents/"+escapeWorkflowPath(workflow.Path), nil, &file); err != nil {
				// The file is gone from the default branch: GitHub cannot run it either.
				var upstream *apiError
				if errors.As(err, &upstream) && upstream.Status == http.StatusNotFound {
					continue
				}
				return nil, err
			}
			if file.Encoding != "base64" {
				return nil, httpx.NewError(502, "git_service_error", "GitHub 返回的工作流文件不是 base64 编码")
			}
			raw, err := base64.StdEncoding.DecodeString(file.Content)
			if err != nil {
				return nil, httpx.NewError(502, "git_service_error", "GitHub 返回的工作流文件编码不正确")
			}
			// A broken file only hides that workflow, not the whole list.
			inputs, dispatch, err := workflowInputs(raw)
			if err == nil && dispatch {
				items = append(items, api.GitHubWorkflow{Id: workflow.ID, Name: workflow.Name, Path: workflow.Path, Inputs: inputs})
			}
		}
		if !more {
			break
		}
	}
	if m.workflows.entries == nil || len(m.workflows.entries) >= 200 {
		m.workflows.entries = map[string]workflowsEntry{}
	}
	m.workflows.entries[key] = workflowsEntry{At: m.now(), Items: items}
	return items, nil
}

func escapeWorkflowPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func workflowInputs(raw []byte) ([]api.GitHubWorkflowInput, bool, error) {
	var workflow struct {
		On yaml.Node `yaml:"on"`
	}
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		return nil, false, err
	}
	inputs := []api.GitHubWorkflowInput{}
	on := &workflow.On
	if on.Kind == yaml.AliasNode {
		on = on.Alias
	}
	switch on.Kind {
	case yaml.ScalarNode:
		return inputs, on.Value == "workflow_dispatch", nil
	case yaml.SequenceNode:
		for _, event := range on.Content {
			if event.Value == "workflow_dispatch" {
				return inputs, true, nil
			}
		}
	case yaml.MappingNode:
		for i := 0; i < len(on.Content); i += 2 {
			if on.Content[i].Value != "workflow_dispatch" {
				continue
			}
			var dispatch struct {
				Inputs yaml.Node `yaml:"inputs"`
			}
			if err := on.Content[i+1].Decode(&dispatch); err != nil {
				return nil, false, err
			}
			definitions := &dispatch.Inputs
			if definitions.Kind == yaml.AliasNode {
				definitions = definitions.Alias
			}
			if definitions.Kind != 0 && definitions.Tag != "!!null" && definitions.Kind != yaml.MappingNode {
				return nil, false, fmt.Errorf("inputs must be a mapping")
			}
			for j := 0; j < len(definitions.Content); j += 2 {
				var def struct {
					Description string                      `yaml:"description"`
					Type        api.GitHubWorkflowInputType `yaml:"type"`
					Required    bool                        `yaml:"required"`
					Default     yaml.Node                   `yaml:"default"`
					Options     []string                    `yaml:"options"`
				}
				if err := definitions.Content[j+1].Decode(&def); err != nil {
					return nil, false, err
				}
				if def.Type == "" {
					def.Type = api.String
				}
				// Types GitHub adds later are shown as a text field.
				if !def.Type.Valid() {
					def.Type = api.String
				}
				input := api.GitHubWorkflowInput{Name: definitions.Content[j].Value, Description: def.Description, Type: def.Type, Required: def.Required, Options: def.Options}
				if input.Options == nil {
					input.Options = []string{}
				}
				if def.Default.Kind != 0 {
					if def.Default.Kind != yaml.ScalarNode {
						return nil, false, fmt.Errorf("default must be a scalar")
					}
					var value any
					if err := def.Default.Decode(&value); err != nil {
						return nil, false, err
					}
					if value != nil {
						input.Default = ptr(fmt.Sprint(value))
					}
				}
				inputs = append(inputs, input)
			}
			return inputs, true, nil
		}
	}
	return inputs, false, nil
}

func auditWorkflowInputs(inputs map[string]string) map[string]string {
	result := map[string]string{}
	for name, value := range inputs {
		runes := []rune(value)
		result[name] = string(runes[:min(len(runes), 200)])
	}
	return result
}

func (m *Module) DispatchGitHubRepoWorkflow(w http.ResponseWriter, r *http.Request, owner, repo string, workflowID int64, p api.DispatchGitHubRepoWorkflowParams) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.GitHubWorkflowDispatch
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err := m.dispatchWorkflow(r.Context(), owner, repo, workflowID, p.ConnectionId, body)
	m.d.Audit.Record(r.Context(), "github.workflow_dispatch", owner+"/"+repo, map[string]any{"repo": owner + "/" + repo, "connectionId": p.ConnectionId, "workflowId": workflowID, "ref": body.Ref, "inputs": auditWorkflowInputs(body.Inputs)}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) dispatchWorkflow(ctx context.Context, owner, repo string, workflowID int64, connection *int64, body api.GitHubWorkflowDispatch) error {
	k, cfg, err := m.githubRepo(ctx, owner, repo, connection)
	if err != nil {
		return err
	}
	if workflowID <= 0 || strings.TrimSpace(body.Ref) == "" || body.Inputs == nil {
		return httpx.Invalid("工作流编号、分支和输入参数不能为空")
	}
	if err := m.client(cfg).post(ctx, fmt.Sprintf("/repos/%s/actions/workflows/%d/dispatches", k.Repo, workflowID), body, nil); err != nil {
		return workflowError(err)
	}
	m.queueActionRuns(k)
	return nil
}

func (m *Module) RerunGitHubRepoFailedJobs(w http.ResponseWriter, r *http.Request, owner, repo string, runID int64, p api.RerunGitHubRepoFailedJobsParams) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ref, err := m.rerunFailed(r.Context(), owner, repo, runID, p.ConnectionId)
	m.d.Audit.Record(r.Context(), "github.rerun_failed", owner+"/"+repo, map[string]any{"repo": owner + "/" + repo, "connectionId": p.ConnectionId, "runId": runID, "ref": ref, "inputs": map[string]string{}}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) rerunFailed(ctx context.Context, owner, repo string, runID int64, connection *int64) (string, error) {
	k, cfg, err := m.githubRepo(ctx, owner, repo, connection)
	if err != nil {
		return "", err
	}
	if runID <= 0 {
		return "", httpx.Invalid("运行编号不正确")
	}
	var run cachedRun
	err = m.cachedOne(ctx, k, "run", strconv.FormatInt(runID, 10), &run)
	if err != nil && !errors.Is(err, httpx.ErrNotFound) {
		return "", err
	}
	client := m.client(cfg)
	if errors.Is(err, httpx.ErrNotFound) {
		var remote ghRun
		if err := client.get(ctx, fmt.Sprintf("/repos/%s/actions/runs/%d", k.Repo, runID), nil, &remote); err != nil {
			return "", workflowError(err)
		}
		run.Branch = remote.HeadBranch
	}
	if err := client.post(ctx, fmt.Sprintf("/repos/%s/actions/runs/%d/rerun-failed-jobs", k.Repo, runID), nil, nil); err != nil {
		return run.Branch, workflowError(err)
	}
	m.queueActionRuns(k)
	return run.Branch, nil
}

type actionRunsRefresh struct {
	mu      sync.Mutex
	pending map[repoKey]time.Time
}

func (m *Module) queueActionRuns(k repoKey) {
	m.actionRuns.mu.Lock()
	defer m.actionRuns.mu.Unlock()
	if m.actionRuns.pending == nil {
		m.actionRuns.pending = map[repoKey]time.Time{}
	}
	// Keep the first deadline when several workflows are dispatched together.
	if _, exists := m.actionRuns.pending[k]; !exists {
		m.actionRuns.pending[k] = m.now().Add(30 * time.Second)
	}
}

// Refresh through the scheduler so shutdown cancels and waits for the work.
// Recheck the watch and credentials at execution time.
func (m *Module) refreshActionRuns(ctx context.Context) error {
	m.actionRuns.mu.Lock()
	keys := []repoKey{}
	for k, at := range m.actionRuns.pending {
		if !m.now().Before(at) {
			keys = append(keys, k)
			delete(m.actionRuns.pending, k)
		}
	}
	m.actionRuns.mu.Unlock()
	if len(keys) == 0 {
		return nil
	}
	m.syncMu.Lock()
	defer m.syncMu.Unlock()
	var errs []error
	for _, k := range keys {
		owner, repo, _ := strings.Cut(k.Repo, "/")
		_, cfg, err := m.githubRepo(ctx, owner, repo, &k.ConnectionID)
		if errors.Is(err, httpx.ErrNotFound) {
			continue
		}
		if err == nil {
			err = m.refreshRepoRuns(ctx, m.client(cfg), k)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Module) refreshRepoRuns(ctx context.Context, c *restClient, k repoKey) error {
	var info api.WatchedRepo
	err := m.cachedOne(ctx, k, "repo", "info", &info)
	if err != nil && !errors.Is(err, httpx.ErrNotFound) {
		return err
	}
	if info.DefaultBranch == "" {
		var raw ghRepo
		if err := c.get(ctx, "/repos/"+k.Repo, nil, &raw); err != nil {
			return err
		}
		info.DefaultBranch = raw.DefaultBranch
	}
	runs, err := m.fetchRuns(ctx, c, k, info.DefaultBranch, nil)
	if err != nil {
		return err
	}
	objects := make([]cacheObject, 0, len(runs))
	for _, run := range runs {
		objects = append(objects, object(strconv.FormatInt(run.Id, 10), run))
	}
	if err := m.saveCompared(ctx, k, "run", objects); err != nil {
		return err
	}
	m.d.Bus.Publish("github.synced", map[string]any{"connectionId": k.ConnectionID, "repo": k.Repo, "at": m.now()})
	return nil
}
