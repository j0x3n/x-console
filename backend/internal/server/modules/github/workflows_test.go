package github_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

const workflowURL = "/github/repos/acme/app/workflows"
const dispatchURL = workflowURL + "/42/dispatches"
const rerunURL = "/github/repos/acme/app/runs/7/rerun-failed"

func addWorkflow(f *fakeGitHub, id int, content string) {
	path := fmt.Sprintf(".github/workflows/%d.yml", id)
	f.workflows = append(f.workflows, map[string]any{"id": id, "name": fmt.Sprintf("Workflow %d", id), "path": path})
	f.workflowFiles[path] = content
}

func TestWorkflowListInputsAndCache(t *testing.T) {
	env, gh := setup(t)
	gh.set(func(f *fakeGitHub) {
		addWorkflow(f, 1, "on: workflow_dispatch\n")
		addWorkflow(f, 2, "on: [push, workflow_dispatch]\n")
		addWorkflow(f, 3, `on:
  push:
  workflow_dispatch:
    inputs:
      title:
        description: 发布说明
        required: true
        default: hello
      stage:
        type: choice
        options: [staging, production]
        default: staging
      dryRun:
        type: boolean
        default: false
      count:
        type: number
        default: 3
      target:
        type: environment
`)
		addWorkflow(f, 4, "on: [push, pull_request]\n")
		addWorkflow(f, 5, "on:\n  push:\n")
		addWorkflow(f, 6, "on: push\n")
	})
	var out struct {
		Items []api.GitHubWorkflow `json:"items"`
	}
	env.MustDo(http.MethodGet, workflowURL, nil, &out)
	if len(out.Items) != 3 || out.Items[0].Id != 1 || out.Items[1].Id != 2 || out.Items[2].Id != 3 || out.Items[0].Inputs == nil {
		t.Fatalf("workflows: %+v", out)
	}
	inputs := out.Items[2].Inputs
	if len(inputs) != 5 {
		t.Fatalf("inputs: %+v", inputs)
	}
	for i, want := range []struct{ name, kind, def string }{{"title", "string", "hello"}, {"stage", "choice", "staging"}, {"dryRun", "boolean", "false"}, {"count", "number", "3"}, {"target", "environment", ""}} {
		v := inputs[i]
		if v.Name != want.name || string(v.Type) != want.kind || v.Options == nil || (i < 4 && (v.Default == nil || *v.Default != want.def)) || (i == 4 && v.Default != nil) {
			t.Fatalf("input %d: %+v", i, v)
		}
	}
	if !inputs[0].Required || inputs[0].Description != "发布说明" || inputs[1].Required || strings.Join(inputs[1].Options, ",") != "staging,production" {
		t.Fatalf("input metadata: %+v", inputs)
	}
	counts := func(wantLists, wantFiles int) {
		t.Helper()
		gh.set(func(f *fakeGitHub) {
			if f.workflowRequests != wantLists || f.contentRequests != wantFiles {
				t.Errorf("requests: lists=%d files=%d", f.workflowRequests, f.contentRequests)
			}
		})
	}
	counts(1, 6)
	env.MustDo(http.MethodGet, workflowURL, nil, &out)
	counts(1, 6)
	env.MustDo(http.MethodGet, workflowURL+"?refresh=true", nil, &out)
	counts(2, 12)
	future := time.Now().Add(6 * time.Minute)
	githubModule(t, env).SetNow(func() time.Time { return future })
	env.MustDo(http.MethodGet, workflowURL, nil, &out)
	counts(3, 18)
}

func TestWorkflowListSkipsBrokenAndDisabled(t *testing.T) {
	env, gh := setup(t)
	gh.set(func(f *fakeGitHub) {
		addWorkflow(f, 1, "on: workflow_dispatch\n")
		addWorkflow(f, 2, "on: [workflow_dispatch\n")
		addWorkflow(f, 3, "on: workflow_dispatch\n")
		f.workflows[2]["state"] = "disabled_manually"
		f.workflows = append(f.workflows, map[string]any{"id": 4, "name": "Gone", "path": ".github/workflows/gone.yml", "state": "active"})
		addWorkflow(f, 5, "on:\n  workflow_dispatch:\n    inputs:\n      when:\n        type: datetime\n")
	})
	var out struct {
		Items []api.GitHubWorkflow `json:"items"`
	}
	env.MustDo(http.MethodGet, workflowURL, nil, &out)
	if len(out.Items) != 2 || out.Items[0].Id != 1 || out.Items[1].Id != 5 {
		t.Fatalf("workflows: %+v", out.Items)
	}
	if in := out.Items[1].Inputs; len(in) != 1 || in[0].Type != api.String {
		t.Fatalf("unknown type should fall back to string: %+v", in)
	}
}

func TestWorkflowDispatchForwardAuditAndRefresh(t *testing.T) {
	env, gh := setup(t)
	value := strings.Repeat("参数", 150)
	body := map[string]any{"ref": "feature/test", "inputs": map[string]string{"message": value, "dryRun": "false"}}
	status, raw := env.Do(http.MethodPost, dispatchURL, body, nil)
	if status != 204 || len(raw) != 0 {
		t.Fatalf("dispatch: %d %s", status, raw)
	}
	gh.set(func(f *fakeGitHub) {
		if len(f.actionRequests) != 1 {
			t.Fatalf("requests: %+v", f.actionRequests)
		}
		v := f.actionRequests[0]
		if v.Method != "POST" || v.Path != "/repos/acme/app/actions/workflows/42/dispatches" || v.Body["ref"] != "feature/test" || v.Body["inputs"].(map[string]any)["message"] != value || v.Body["inputs"].(map[string]any)["dryRun"] != "false" {
			t.Errorf("forwarded: %+v", v)
		}
		f.runs[repo] = []map[string]any{run(7, 42, "Manual", "feature/test", "queued", "")}
	})
	var detail, result string
	if err := env.App.Deps.DB.QueryRow(`SELECT detail,result FROM audit_log WHERE action='github.workflow_dispatch'`).Scan(&detail, &result); err != nil {
		t.Fatal(err)
	}
	var audit struct {
		Repo       string
		WorkflowID int64 `json:"workflowId"`
		Ref        string
		Inputs     map[string]string
	}
	if err := json.Unmarshal([]byte(detail), &audit); err != nil {
		t.Fatal(err)
	}
	if result != "ok" || audit.Repo != repo || audit.WorkflowID != 42 || audit.Ref != "feature/test" || audit.Inputs["message"] != string([]rune(value)[:200]) {
		t.Fatalf("audit: %s %s", result, detail)
	}
	m := githubModule(t, env)
	if err := m.RefreshActionRuns(context.Background()); err != nil {
		t.Fatal(err)
	}
	var runs []api.GitHubRun
	env.MustDo(http.MethodGet, "/github/runs", nil, &runs)
	if len(runs) != 0 {
		t.Fatal("refresh ran before the delay")
	}
	future := time.Now().Add(31 * time.Second)
	m.SetNow(func() time.Time { return future })
	if err := m.RefreshActionRuns(context.Background()); err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodGet, "/github/runs", nil, &runs)
	if len(runs) != 1 || runs[0].Id != 7 {
		t.Fatalf("refreshed runs: %+v", runs)
	}
	before, _ := gh.counts()
	if err := m.RefreshActionRuns(context.Background()); err != nil {
		t.Fatal(err)
	}
	if after, _ := gh.counts(); after != before {
		t.Fatal("refresh was not one-shot")
	}
}

func TestWorkflowWritesRequireElevation(t *testing.T) {
	env := testutil.New(t)
	for _, path := range []string{dispatchURL, rerunURL} {
		status, raw := env.Do(http.MethodPost, path, map[string]any{"ref": "main", "inputs": map[string]string{}}, nil)
		if status != 403 || errCode(raw) != "elevation_required" {
			t.Fatalf("%s: %d %s", path, status, raw)
		}
	}
}

func TestWorkflowRepoRestrictionsAndAccountToken(t *testing.T) {
	env := testutil.New(t)
	env.Elevate()
	for _, path := range []string{workflowURL, dispatchURL, rerunURL} {
		method := http.MethodPost
		if path == workflowURL {
			method = http.MethodGet
		}
		if status, raw := env.Do(method, path, map[string]any{"ref": "main", "inputs": map[string]string{}}, nil); status != 404 {
			t.Fatalf("unwatched %s: %d %s", path, status, raw)
		}
	}
	fj := newFakeForge(t)
	forge := createConnection(t, env, map[string]any{"kind": "forgejo", "name": "Forge", "baseUrl": fj.srv.URL, "token": "fj-token"})
	env.MustDo(http.MethodPut, "/github/config", map[string]any{"watches": []api.RepoWatch{{ConnectionId: forge.ID, Repo: repo}}}, nil)
	for _, path := range []string{workflowURL, dispatchURL, rerunURL} {
		method := http.MethodPost
		if path == workflowURL {
			method = http.MethodGet
		}
		if status, raw := env.Do(method, path, map[string]any{"ref": "main", "inputs": map[string]string{}}, nil); status != 404 {
			t.Fatalf("Forgejo %s: %d %s", path, status, raw)
		}
	}
	gh := newFakeGitHub(t)
	gh.set(func(f *fakeGitHub) { f.token = "other-account-token"; addWorkflow(f, 42, "on: workflow_dispatch") })
	account := createConnection(t, env, map[string]any{"kind": "github", "name": "GitHub", "baseUrl": gh.URL(), "token": gh.token})
	env.MustDo(http.MethodPut, "/github/config", map[string]any{"watches": []api.RepoWatch{{ConnectionId: forge.ID, Repo: repo}, {ConnectionId: account.ID, Repo: repo}}}, nil)
	env.MustDo(http.MethodGet, workflowURL, nil, nil)
	if status, raw := env.Do(http.MethodPost, dispatchURL+fmt.Sprintf("?connectionId=%d", account.ID), map[string]any{"ref": "main", "inputs": map[string]string{}}, nil); status != 204 {
		t.Fatalf("account token: %d %s", status, raw)
	}
	if status, _ := env.Do(http.MethodGet, workflowURL+fmt.Sprintf("?connectionId=%d", forge.ID), nil, nil); status != 404 {
		t.Fatal("explicit Forgejo accepted")
	}
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/github/repos/acme/unwatched/workflows"},
		{http.MethodPost, "/github/repos/acme/unwatched/workflows/42/dispatches"},
		{http.MethodPost, "/github/repos/acme/unwatched/runs/7/rerun-failed"},
	} {
		if status, raw := env.Do(request.method, request.path, map[string]any{"ref": "main", "inputs": map[string]string{}}, nil); status != 404 {
			t.Fatalf("configured but unwatched: %d %s", status, raw)
		}
	}
}

func TestWorkflowUpstreamPermissionErrors(t *testing.T) {
	env, gh := setup(t)
	for _, upstream := range []int{403, 404} {
		gh.set(func(f *fakeGitHub) {
			f.actionStatus = upstream
			f.actionMessage = "Resource not accessible by personal access token"
		})
		for _, path := range []string{workflowURL, dispatchURL, rerunURL} {
			method := http.MethodPost
			if path == workflowURL {
				method = http.MethodGet
			}
			status, raw := env.Do(method, path, map[string]any{"ref": "main", "inputs": map[string]string{}}, nil)
			if status != 502 || errCode(raw) != "git_service_error" || !strings.Contains(string(raw), "Resource not accessible by personal access token") || !strings.Contains(string(raw), "经典令牌要 repo 和 workflow 权限，细粒度令牌要 Actions 读写") {
				t.Fatalf("upstream %d %s: %d %s", upstream, path, status, raw)
			}
		}
	}
}

func TestWorkflowRerunFailedForwardAndAudit(t *testing.T) {
	env, gh := setup(t)
	gh.set(func(f *fakeGitHub) { f.runs[repo] = []map[string]any{run(7, 42, "CI", "main", "completed", "failure")} })
	syncNow(t, env)
	status, raw := env.Do(http.MethodPost, rerunURL, nil, nil)
	if status != 204 || len(raw) != 0 {
		t.Fatalf("rerun: %d %s", status, raw)
	}
	gh.set(func(f *fakeGitHub) {
		if len(f.actionRequests) != 1 || f.actionRequests[0].Path != "/repos/acme/app/actions/runs/7/rerun-failed-jobs" || f.actionRequests[0].Method != "POST" {
			t.Errorf("rerun forwarded: %+v", f.actionRequests)
		}
	})
	var detail string
	if err := env.App.Deps.DB.QueryRow(`SELECT detail FROM audit_log WHERE action='github.rerun_failed'`).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	var audit struct {
		Repo  string
		RunID int64 `json:"runId"`
		Ref   string
	}
	if err := json.Unmarshal([]byte(detail), &audit); err != nil {
		t.Fatal(err)
	}
	if audit.Repo != repo || audit.RunID != 7 || audit.Ref != "main" {
		t.Fatalf("audit: %s", detail)
	}
}

func TestWorkflowRerunUncachedRunAndRefresh(t *testing.T) {
	env, gh := setup(t)
	gh.set(func(f *fakeGitHub) {
		f.runs[repo] = []map[string]any{run(7, 42, "CI", "feature/test", "completed", "failure")}
	})
	if status, raw := env.Do(http.MethodPost, rerunURL, nil, nil); status != 204 {
		t.Fatalf("uncached rerun: %d %s", status, raw)
	}
	var detail string
	if err := env.App.Deps.DB.QueryRow(`SELECT detail FROM audit_log WHERE action='github.rerun_failed'`).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, `"ref":"feature/test"`) {
		t.Fatalf("missing branch: %s", detail)
	}
	gh.set(func(f *fakeGitHub) { f.runs[repo] = []map[string]any{run(7, 42, "CI", "feature/test", "queued", "")} })
	future := time.Now().Add(31 * time.Second)
	m := githubModule(t, env)
	m.SetNow(func() time.Time { return future })
	if err := m.RefreshActionRuns(context.Background()); err != nil {
		t.Fatal(err)
	}
	var runs []api.GitHubRun
	env.MustDo(http.MethodGet, "/github/runs", nil, &runs)
	if len(runs) != 1 || runs[0].Status != "queued" {
		t.Fatalf("rerun refresh: %+v", runs)
	}
}

func TestWorkflowDispatchValidation(t *testing.T) {
	env, gh := setup(t)
	for _, body := range []any{
		map[string]any{"inputs": map[string]string{}},
		map[string]any{"ref": " ", "inputs": map[string]string{}},
		map[string]any{"ref": "main"},
		map[string]any{"ref": "main", "inputs": map[string]any{"dryRun": true}},
	} {
		if status, raw := env.Do(http.MethodPost, dispatchURL, body, nil); status != 400 {
			t.Fatalf("invalid body: %d %s", status, raw)
		}
	}
	gh.set(func(f *fakeGitHub) {
		if len(f.actionRequests) != 0 {
			t.Errorf("invalid dispatch forwarded: %+v", f.actionRequests)
		}
	})
}

func TestWorkflowRateLimitPreservesMessage(t *testing.T) {
	env, gh := setup(t)
	gh.set(func(f *fakeGitHub) { f.rateLimited = true })
	status, raw := env.Do(http.MethodPost, dispatchURL, map[string]any{"ref": "main", "inputs": map[string]string{}}, nil)
	if status != 502 || !strings.Contains(string(raw), "API rate limit exceeded") || !strings.Contains(string(raw), "Actions 读写") {
		t.Fatalf("rate limit: %d %s", status, raw)
	}
}
