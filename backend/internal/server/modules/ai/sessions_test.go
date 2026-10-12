package ai_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestAiSessionsSourcesStatusSortAndLimit(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	now := time.Now().UTC()
	var chat api.Conversation
	env.MustDo("POST", "/ai/conversations", map[string]any{"title": "面板对话"}, &chat)
	hostID := env.Agent("server", nil, nil)
	var host api.Conversation
	env.MustDo("POST", "/ai/host-agent/"+hostID+"/conversations", map[string]any{"title": "服务器对话"}, &host)
	_, err := env.App.Deps.DB.ExecContext(ctx, `INSERT INTO ai_pending_actions(conversation_id,tool_use_id,action,input,status) VALUES(?, 'pending-1', 'test.write', '{}', 'pending'),(?, 'done-1', 'test.write', '{}', 'done')`, host.Id, host.Id)
	if err != nil {
		t.Fatal(err)
	}
	var agentID int64
	err = env.App.Deps.DB.QueryRowContext(ctx, `INSERT INTO ai_agents(name,kind,avatar,color,created_at,updated_at) VALUES('测试 Agent','builtin','bot','blue',?,?) RETURNING id`, now, now).Scan(&agentID)
	if err != nil {
		t.Fatal(err)
	}
	statuses := []string{"queued", "running", "waiting", "pr_opened", "done", "failed", "canceled"}
	wants := []string{"working", "working", "waiting", "done", "done", "failed", "canceled"}
	for i, status := range statuses {
		_, err = env.App.Deps.DB.ExecContext(ctx, `INSERT INTO ai_agent_runs(agent_id,issue_key,issue_title,kind,status,summary,created_at) VALUES(?,'XC-1','任务标题','builtin',?,'执行摘要',?)`, agentID, status, now.Add(-time.Duration(i+1)*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
	}
	var sessions []api.AiSession
	env.MustDo("GET", "/ai/sessions", nil, &sessions)
	if len(sessions) != 9 {
		t.Fatalf("会话数：%d", len(sessions))
	}
	byID := map[string]api.AiSession{}
	for i, s := range sessions {
		byID[s.Id] = s
		if i > 0 {
			prev := sessions[i-1]
			active := func(status api.AiSessionStatus) bool { return status == "working" || status == "waiting" }
			if !active(prev.Status) && active(s.Status) || active(prev.Status) == active(s.Status) && prev.UpdatedAt.Before(s.UpdatedAt) {
				t.Fatalf("排序：%+v", sessions)
			}
		}
	}
	if s := byID[fmt.Sprintf("chat:%d", chat.Id)]; s.Kind != "chat" || s.SourceName != "" || s.Status != "idle" || s.HostId != nil {
		t.Fatalf("面板：%+v", s)
	}
	if s := byID[fmt.Sprintf("host:%d", host.Id)]; s.Kind != "host" || s.HostId == nil || *s.HostId != hostID || s.SourceName == "" || s.Status != "waiting" || s.PendingCount != 1 {
		t.Fatalf("服务器：%+v", s)
	}
	for i, want := range wants {
		s := byID[fmt.Sprintf("run:%d", i+1)]
		if string(s.Status) != want || s.Kind != "agent" || s.Title != "XC-1 任务标题" || s.AgentId == nil || *s.AgentId != agentID || s.AgentAvatar == nil || *s.AgentAvatar != "bot" {
			t.Fatalf("执行记录：%+v，期望 %s", s, want)
		}
	}
	env.MustDo("GET", "/ai/sessions?limit=2", nil, &sessions)
	if len(sessions) != 2 {
		t.Fatalf("limit：%d", len(sessions))
	}
	if status, _ := env.Do("GET", "/ai/sessions?limit=bad", nil, nil); status != 400 {
		t.Fatalf("参数校验：%d", status)
	}
	if _, err := env.App.Deps.DB.ExecContext(ctx, "DELETE FROM agents WHERE id=?", hostID); err != nil {
		t.Fatal(err)
	}
	env.MustDo("GET", "/ai/sessions", nil, &sessions)
	for _, s := range sessions {
		if s.RefId == host.Id && s.Kind == "host" && s.SourceName != hostID {
			t.Fatalf("删除后的机器名：%s", s.SourceName)
		}
	}
	env.MustDo("POST", "/vault/setup", map[string]any{"password": "hidden-password"}, nil)
	env.MustDo("PUT", "/vault/modules", map[string]any{"hidden": []string{"coding", "servers"}}, nil)
	env.MustDo("POST", "/vault/lock", nil, nil)
	env.MustDo("GET", "/ai/sessions", nil, &sessions)
	if len(sessions) != 1 || sessions[0].Kind != "chat" {
		t.Fatalf("锁定时泄露隐藏模块：%+v", sessions)
	}
	env.MustDo("POST", "/vault/unlock", map[string]any{"password": "hidden-password"}, nil)
	env.MustDo("GET", "/ai/sessions", nil, &sessions)
	if len(sessions) != 9 {
		t.Fatalf("解锁后会话数：%d", len(sessions))
	}
	if _, err := env.App.Deps.DB.ExecContext(ctx, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<305) INSERT INTO ai_conversations(title,created_at,updated_at) SELECT '旧对话',?,? FROM n`, now.Add(-24*time.Hour), now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	env.MustDo("GET", "/ai/sessions", nil, &sessions)
	if len(sessions) != 100 {
		t.Fatalf("默认 limit：%d", len(sessions))
	}
	env.MustDo("GET", "/ai/sessions?limit=999", nil, &sessions)
	if len(sessions) != 300 {
		t.Fatalf("最大 limit：%d", len(sessions))
	}
}

type sessionCodingControl struct {
	contracts.CodingControl
	at time.Time
}

func (s sessionCodingControl) CodingTasks(context.Context, []int64) (map[int64]contracts.CodingTaskInfo, error) {
	return map[int64]contracts.CodingTaskInfo{42: {Status: "review", Error: "等待你审查", StartedAt: &s.at}}, nil
}

func TestAiSessionRunUsesTaskStateEventsAndDecisions(t *testing.T) {
	env := testutil.New(t)
	now := time.Now().UTC()
	module.Provide[contracts.CodingControl](env.App.Deps.Registry, contracts.CodingControlKey, sessionCodingControl{at: now.Add(time.Minute)})
	var agentID, runID int64
	if err := env.App.Deps.DB.QueryRow(`INSERT INTO ai_agents(name,kind,created_at,updated_at) VALUES('Agent','codex',?,?) RETURNING id`, now, now).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	if err := env.App.Deps.DB.QueryRow(`INSERT INTO ai_agent_runs(agent_id,kind,status,task_id,summary,created_at) VALUES(?,'codex','running',42,'原始摘要',?) RETURNING id`, agentID, now).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	changed := now.Add(2 * time.Minute)
	if _, err := env.App.Deps.DB.Exec(`INSERT INTO ai_agent_run_events(run_id,seq,at,kind,text) VALUES(?,1,?,'status','waiting')`, runID, changed); err != nil {
		t.Fatal(err)
	}
	if _, err := env.App.Deps.DB.Exec(`INSERT INTO ai_agent_decisions(run_id,kind,title,status,created_at) VALUES(?,'question','问题','pending',?),(?,'question','已回答','answered',?)`, runID, changed, runID, changed); err != nil {
		t.Fatal(err)
	}
	var sessions []api.AiSession
	env.MustDo("GET", "/ai/sessions", nil, &sessions)
	if len(sessions) != 1 {
		t.Fatalf("会话数：%d", len(sessions))
	}
	s := sessions[0]
	if s.Status != "waiting" || s.Title != "等待你审查" || s.PendingCount != 1 || s.TaskId == nil || *s.TaskId != 42 || !s.UpdatedAt.Equal(changed) {
		t.Fatalf("实时执行状态：%+v", s)
	}
}

func TestAiSessionWorkingAndStartedEvent(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
			return
		}
		close(entered)
		<-release
		streamText(w, "完成")
	}))
	defer fake.Close()
	defer close(release)
	env := testutil.New(t)
	env.Elevate()
	var provider api.AiProvider
	env.MustDo("POST", "/ai/providers", map[string]any{"name": "测试", "baseUrl": fake.URL + "/v1", "apiKey": "fake"}, &provider)
	env.MustDo("POST", fmt.Sprintf("/ai/providers/%d/models", provider.Id), nil, nil)
	env.MustDo("PUT", "/ai/model-settings", map[string]any{"agent": map[string]any{"providerId": provider.Id, "model": "test-model"}}, nil)
	var c api.Conversation
	env.MustDo("POST", "/ai/conversations", map[string]any{}, &c)
	events, cancel := env.App.Deps.Bus.Subscribe("ai.message_started", 1)
	defer cancel()
	env.MustDo("POST", fmt.Sprintf("/ai/conversations/%d/messages", c.Id), map[string]any{"text": "检查状态"}, nil)
	select {
	case event := <-events:
		if event.Data.(map[string]any)["conversationId"] != c.Id {
			t.Fatalf("事件：%+v", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("没有生成开始事件")
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("没有请求模型")
	}
	var sessions []api.AiSession
	env.MustDo("GET", "/ai/sessions", nil, &sessions)
	if len(sessions) != 1 || sessions[0].Status != "working" {
		t.Fatalf("生成状态：%+v", sessions)
	}
}
