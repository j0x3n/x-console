package aiagents

import (
	"context"
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

const agentNotifySetting = "aiagents.notify"

func (m *Module) notifySettings(ctx context.Context) api.AiAgentNotify {
	x := api.AiAgentNotify{Decision: true, Done: true, Failed: true, PrOpened: true}
	_ = m.d.Settings.Get(ctx, agentNotifySetting, &x)
	return x
}
func (m *Module) GetAiAgentNotify(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, m.notifySettings(r.Context()))
}
func (m *Module) PutAiAgentNotify(w http.ResponseWriter, r *http.Request) {
	var x api.AiAgentNotify
	if err := httpx.Decode(r, &x); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err := m.d.Settings.Set(r.Context(), agentNotifySetting, x)
	m.d.Audit.Record(r.Context(), "ai_agent.notify", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, x)
}

func (m *Module) notifyRun(ctx context.Context, id int64, kind, body string, buttons []notify.Action) {
	c := m.notifySettings(ctx)
	enabled := map[string]bool{"received": c.Received, "started": c.Started, "decision": c.Decision, "pr_opened": c.PrOpened, "done": c.Done, "failed": c.Failed}
	if !enabled[kind] {
		return
	}
	if body == "" {
		body = map[string]string{"received": "任务已经排队", "started": "正在处理这张卡片", "decision": "任务在等你确认", "pr_opened": "代码已提交，PR 等你审查", "done": "任务已经完成", "failed": "任务未完成，请查看执行日志"}[kind]
	}
	var name, key string
	if err := m.d.DB.QueryRowContext(ctx, `SELECT a.name,r.issue_key FROM ai_agent_runs r JOIN ai_agents a ON a.id=r.agent_id WHERE r.id=?`, id).Scan(&name, &key); err != nil {
		return
	}
	labels := map[string]string{"received": "收到任务", "started": "开始任务", "decision": "需要你决定", "pr_opened": "已提 PR", "done": "完成任务", "failed": "失败或中断"}
	_, err := m.d.Notify.Send(ctx, notify.Notification{Kind: "ai_agent." + kind, Title: name + " · " + key + " " + labels[kind], Body: body, Link: "/today", Source: "aiagents", Actions: buttons, Data: map[string]any{"runId": id, "issueKey": key}})
	if err != nil {
		m.d.Log.Warn("aiagents: notification", "err", err)
	}
}
