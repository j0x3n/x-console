package aiagents_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type fakeLauncher struct {
	mu   sync.Mutex
	runs []contracts.LaunchCoding
}

func (f *fakeLauncher) Launch(_ context.Context, in contracts.LaunchCoding) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, in)
	return 41, nil
}

type fakeRunner struct {
	mu      sync.Mutex
	runs    []contracts.ToolRun
	session *auth.Session
	release chan struct{}
}

func (f *fakeRunner) RunTools(ctx context.Context, in contracts.ToolRun) (string, error) {
	f.mu.Lock()
	f.runs = append(f.runs, in)
	f.session = auth.FromContext(ctx)
	f.mu.Unlock()
	<-f.release
	return "拆成了 3 条清单。", nil
}

type comment struct {
	Body   string
	Author *string
}

func comments(t *testing.T, env *testutil.Env, key string) []comment {
	t.Helper()
	var out []comment
	env.MustDo(http.MethodGet, "/issues/"+key+"/comments", nil, &out)
	return out
}

func waitComments(t *testing.T, env *testutil.Env, key string, n int) []comment {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		list := comments(t, env, key)
		if len(list) >= n {
			return list
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %d comments, want %d: %+v", key, len(list), n, list)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type card struct {
	Key     string
	Status  string
	Members []struct{ Kind, ID string }
}

func newCard(t *testing.T, env *testutil.Env) card {
	t.Helper()
	var p struct{ ID int64 }
	env.MustDo(http.MethodPost, "/projects", map[string]any{"key": "AG", "name": "Agent 测试"}, &p)
	var c card
	env.MustDo(http.MethodPost, fmt.Sprintf("/projects/%d/issues", p.ID), map[string]any{"title": "修好登录", "description": "用户登录不了。"}, &c)
	var cl struct{ ID int64 }
	env.MustDo(http.MethodPost, "/issues/"+c.Key+"/checklists", map[string]any{"title": "步骤"}, &cl)
	env.MustDo(http.MethodPost, fmt.Sprintf("/issues/%s/checklists/%d/items", c.Key, cl.ID), map[string]any{"text": "加一个测试"}, nil)
	env.MustDo(http.MethodPost, "/issues/"+c.Key+"/comments", map[string]any{"body": "先看看 session 过期"}, nil)
	return c
}

func getCard(t *testing.T, env *testutil.Env, key string) card {
	t.Helper()
	var c card
	env.MustDo(http.MethodGet, "/issues/"+key, nil, &c)
	return c
}

func TestAssignCLIAgentAndFollowTask(t *testing.T) {
	env := testutil.New(t)
	launcher := &fakeLauncher{}
	module.Provide[contracts.Coding](env.App.Deps.Registry, contracts.CodingKey, launcher)
	c := newCard(t, env)
	var a agent
	env.MustDo(http.MethodPost, "/ai-agents", map[string]any{"name": "后端", "kind": "codex"}, &a)
	path := fmt.Sprintf("/ai-agents/%d/assign", a.ID)

	if s, _ := env.Do(http.MethodPost, path, map[string]any{"issueKey": c.Key}, nil); s != 400 {
		t.Fatalf("no repo: %d", s)
	}
	if s, _ := env.Do(http.MethodPost, path, map[string]any{"issueKey": "AG-99", "repoId": 1}, nil); s != 400 {
		t.Fatalf("unknown card: %d", s)
	}
	var out struct{ TaskID int64 }
	env.MustDo(http.MethodPost, path, map[string]any{"issueKey": c.Key, "repoId": 7, "baseBranch": "dev", "note": "顺便更新文档"}, &out)
	if out.TaskID != 41 || len(launcher.runs) != 1 {
		t.Fatalf("launch: %+v %+v", out, launcher.runs)
	}
	in := launcher.runs[0]
	if in.RepoID != 7 || in.IssueKey != c.Key || in.AIAgentID != a.ID || in.BaseBranch != "dev" ||
		!strings.Contains(in.Prompt, "加一个测试") || !strings.Contains(in.Prompt, "session 过期") || !strings.HasSuffix(in.Prompt, "顺便更新文档") {
		t.Fatalf("launch input: %+v", in)
	}
	if got := getCard(t, env, c.Key); len(got.Members) != 1 || got.Members[0].Kind != "agent" || got.Members[0].ID != fmt.Sprint(a.ID) {
		t.Fatalf("members: %+v", got.Members)
	}

	// The coding task moves on: comments as the agent, card to review.
	bus := env.App.Deps.Bus
	task := func(status string, extra map[string]any) map[string]any {
		ev := map[string]any{"id": 41, "issueKey": c.Key, "aiAgentId": a.ID, "status": status, "changedFiles": []map[string]any{}}
		for k, v := range extra {
			ev[k] = v
		}
		return ev
	}
	bus.Publish("coding_task.updated", task("running", nil))
	bus.Publish("coding_task.updated", task("running", nil)) // same status: no second comment
	bus.Publish("coding_task.updated", task("review", map[string]any{"changedFiles": []map[string]any{{"path": "a"}, {"path": "b"}}}))
	bus.Publish("coding_task.build", map[string]any{"taskId": 41, "status": "failed", "error": "第 1 步失败了。", "issueKey": c.Key, "aiAgentId": a.ID})
	bus.Publish("coding_task.updated", task("pr_opened", map[string]any{"prUrl": "https://git.example.com/team/x/pulls/3"}))
	// Tasks of no agent are not followed.
	bus.Publish("coding_task.updated", map[string]any{"id": 42, "issueKey": c.Key, "status": "failed"})
	list := waitComments(t, env, c.Key, 5)
	time.Sleep(100 * time.Millisecond)
	list = comments(t, env, c.Key)
	var bodies []string
	for _, cm := range list[1:] {
		if cm.Author == nil || *cm.Author != fmt.Sprintf("agent:%d", a.ID) {
			t.Fatalf("author: %+v", cm)
		}
		bodies = append(bodies, cm.Body)
	}
	all := strings.Join(bodies, "|")
	if len(bodies) != 4 || !strings.Contains(all, "开始处理") || !strings.Contains(all, "改了 2 个文件") ||
		!strings.Contains(all, "构建没通过") || !strings.Contains(all, "pulls/3") {
		t.Fatalf("comments: %q", bodies)
	}
	if got := getCard(t, env, c.Key); got.Status != "in_review" {
		t.Fatalf("card status: %s", got.Status)
	}

	// Disabled agents take no cards.
	env.MustDo(http.MethodPatch, fmt.Sprintf("/ai-agents/%d", a.ID), map[string]any{"enabled": false}, nil)
	if s, _ := env.Do(http.MethodPost, path, map[string]any{"issueKey": c.Key, "repoId": 7}, nil); s != http.StatusConflict {
		t.Fatalf("disabled: %d", s)
	}
}

func TestAssignBuiltinAgent(t *testing.T) {
	env := testutil.New(t)
	runner := &fakeRunner{release: make(chan struct{})}
	module.Provide[contracts.ToolRunner](env.App.Deps.Registry, contracts.ToolRunnerKey, runner)
	env.MustDo(http.MethodPost, "/ai/memories", map[string]any{"text": "仓库在 Forgejo 上"}, nil) // B61
	c := newCard(t, env)
	var a agent
	env.MustDo(http.MethodPost, "/ai-agents", map[string]any{"name": "整理员", "kind": "builtin", "model": "3:gpt-5-mini",
		"access": "read", "instructions": "只拆清单。"}, &a)
	path := fmt.Sprintf("/ai-agents/%d/assign", a.ID)
	env.MustDo(http.MethodPost, path, map[string]any{"issueKey": c.Key}, nil)
	// One job at a time (max parallel 1).
	waitFor(t, func() bool { runner.mu.Lock(); defer runner.mu.Unlock(); return len(runner.runs) == 1 })
	if s, _ := env.Do(http.MethodPost, path, map[string]any{"issueKey": c.Key}, nil); s != http.StatusConflict {
		t.Fatalf("busy: %d", s)
	}
	var got agent
	env.MustDo(http.MethodGet, fmt.Sprintf("/ai-agents/%d", a.ID), nil, &got)
	if got.RunningTasks != 1 {
		t.Fatalf("running: %+v", got)
	}
	run := runner.runs[0]
	if run.Model != "3:gpt-5-mini" || run.Access != "read" || run.Source != "ai_agent" || run.Ref != fmt.Sprint(a.ID) ||
		!strings.Contains(run.System, "只拆清单。") || !strings.Contains(run.System, "仓库在 Forgejo 上") || !strings.Contains(run.Prompt, "加一个测试") {
		t.Fatalf("run: %+v", run)
	}
	// The agent never has the user's elevation.
	if s := runner.session; s == nil || !s.ViaToken || s.Elevated() || s.Token.Access != "read" {
		t.Fatalf("session: %+v", s)
	}
	close(runner.release)
	list := waitComments(t, env, c.Key, 3)
	if !strings.Contains(list[1].Body, "开始处理") || list[2].Body != "拆成了 3 条清单。" || *list[2].Author != fmt.Sprintf("agent:%d", a.ID) {
		t.Fatalf("comments: %+v", list)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookMergedPR(t *testing.T) {
	env := testutil.New(t)
	git := newFakeGit(t, "/api/v1", "token")
	c := newCard(t, env)
	env.Elevate()
	var created struct {
		Connection    connection
		WebhookSecret string
	}
	env.MustDo(http.MethodPost, "/git-connections", map[string]any{"kind": "forgejo", "name": "fj", "baseUrl": git.URL, "token": secretToken}, &created)
	var a agent
	env.MustDo(http.MethodPost, "/ai-agents", map[string]any{"name": "后端", "kind": "codex"}, &a)
	prURL := "https://git.example.com/team/x/pulls/3"
	db := env.App.Deps.DB
	runner := env.Agent("server", nil, nil)
	res, err := db.Exec(`INSERT INTO coding_repos (agent_id, path, name, created_at) VALUES (?, '/srv/x', 'x', ?)`, runner, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	repoID, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO coding_tasks (repo_id, executor, prompt, created_at, updated_at, ai_agent_id, status, issue_key, pr_url)
		VALUES (?, 'codex', 'p', ?, ?, ?, 'pr_opened', ?, ?)`, repoID, time.Now(), time.Now(), a.ID, c.Key, prURL); err != nil {
		t.Fatal(err)
	}

	url := env.URL(created.Connection.WebhookPath)
	post := func(body string, headers map[string]string) int {
		req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req) // no session cookie
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	merged := fmt.Sprintf(`{"action":"closed","pull_request":{"html_url":%q,"merged":true}}`, prURL)
	if s := post(merged, map[string]string{"X-Forgejo-Event": "pull_request", "X-Forgejo-Signature": sign("wrong", []byte(merged))}); s != http.StatusUnauthorized {
		t.Fatalf("bad signature: %d", s)
	}
	if s := post(merged, map[string]string{"X-Forgejo-Event": "pull_request"}); s != http.StatusUnauthorized {
		t.Fatalf("no signature: %d", s)
	}
	if s := post(merged, map[string]string{"X-GitHub-Event": "pull_request", "X-Hub-Signature-256": "sha256=" + sign(created.WebhookSecret, []byte(merged))}); s != http.StatusNoContent {
		t.Fatalf("github style: %d", s)
	}
	if got := getCard(t, env, c.Key); got.Status != "done" {
		t.Fatalf("card after merge: %s", got.Status)
	}
	list := comments(t, env, c.Key)
	last := list[len(list)-1]
	if !strings.Contains(last.Body, "PR 合并了") || last.Author == nil || *last.Author != fmt.Sprintf("agent:%d", a.ID) {
		t.Fatalf("merge comment: %+v", last)
	}
	// Forgejo style signature, a closed but not merged PR: nothing happens.
	closed := strings.Replace(merged, `"merged":true`, `"merged":false`, 1)
	if s := post(closed, map[string]string{"X-Gitea-Event": "pull_request", "X-Gitea-Signature": sign(created.WebhookSecret, []byte(closed))}); s != http.StatusNoContent {
		t.Fatalf("closed: %d", s)
	}
	if s := post("{}", map[string]string{"X-Forgejo-Event": "push", "X-Forgejo-Signature": sign(created.WebhookSecret, []byte("{}"))}); s != http.StatusNoContent {
		t.Fatalf("push event: %d", s)
	}
	if s := post(merged, map[string]string{"X-Hub-Signature-256": "sha256=" + sign(created.WebhookSecret, []byte(merged))}); s != http.StatusNoContent {
		t.Fatalf("no event header: %d", s)
	}
	// An unknown connection answers like a bad signature.
	req, _ := http.NewRequest(http.MethodPost, env.URL("/hooks/git/999"), strings.NewReader(merged))
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown connection: %d", resp.StatusCode)
	}
}
