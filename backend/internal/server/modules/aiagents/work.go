package aiagents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/api"
)

// B47 step 4: cards assigned to agents. A CLI agent gets a coding task; the
// module then follows the task's events and writes comments on the card
// as the agent. A built-in agent works on the card with the actions its
// access level allows and comments the result.

func author(id int64) string { return "agent:" + strconv.FormatInt(id, 10) }

func (m *Module) issueWork() (contracts.IssueWork, error) {
	w, ok := module.Lookup[contracts.IssueWork](m.d.Registry, contracts.IssueWorkKey)
	if !ok {
		return nil, httpx.NewError(http.StatusNotImplemented, "feature_unavailable", "项目模块没有启用")
	}
	return w, nil
}

// briefText is the part of the prompt the card's title and description do
// not cover: open checklist items and recent comments.
func briefText(b contracts.IssueBrief) string {
	var sb strings.Builder
	if len(b.OpenItems) > 0 {
		sb.WriteString("未完成的清单：\n")
		for _, it := range b.OpenItems {
			sb.WriteString("- " + it + "\n")
		}
		sb.WriteString("\n")
	}
	if len(b.Comments) > 0 {
		sb.WriteString("最近的评论：\n")
		for _, c := range b.Comments {
			sb.WriteString("- " + strings.ReplaceAll(strings.TrimSpace(c), "\n", "\n  ") + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m *Module) AssignAiAgent(w http.ResponseWriter, r *http.Request, id int64) {
	var body api.AssignAiAgentJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	taskID, err := m.assign(ctx, id, body)
	m.d.Audit.Record(ctx, "ai_agent.assign", strconv.FormatInt(id, 10), map[string]any{"issueKey": body.IssueKey, "taskId": taskID}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := map[string]any{}
	if taskID != 0 {
		out["taskId"] = taskID
	}
	httpx.JSON(w, http.StatusAccepted, out)
}

func (m *Module) assign(ctx context.Context, id int64, body api.AssignAiAgentJSONRequestBody) (int64, error) {
	a, err := m.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	if !a.Enabled {
		return 0, httpx.NewError(http.StatusConflict, "conflict", "Agent “"+a.Name+"” 已停用")
	}
	if a.OverBudget {
		return 0, httpx.NewError(http.StatusConflict, "conflict", "Agent “"+a.Name+"” 本月费用已经到预算了")
	}
	work, err := m.issueWork()
	if err != nil {
		return 0, err
	}
	brief, err := work.Brief(ctx, strings.TrimSpace(body.IssueKey))
	var he *httpx.Error
	if errors.As(err, &he) && he.Status == http.StatusNotFound {
		return 0, httpx.Invalid("卡片 " + body.IssueKey + " 不存在")
	}
	if err != nil {
		return 0, err
	}
	note := ""
	if body.Note != nil {
		note = strings.TrimSpace(*body.Note)
	}
	if a.Kind == kindBuiltin {
		return 0, m.startBuiltin(ctx, a, brief, note)
	}
	if body.RepoId == nil {
		boards, ok := module.Lookup[contracts.BoardGit](m.d.Registry, contracts.BoardGitKey)
		if !ok {
			return 0, httpx.ErrNotLive
		}
		repo, err := boards.RepositoryForIssue(ctx, brief.Key)
		if err != nil {
			return 0, err
		}
		control, ok := module.Lookup[contracts.CodingControl](m.d.Registry, contracts.CodingControlKey)
		if !ok {
			return 0, httpx.ErrNotLive
		}
		runner := a.RunnerAgentID
		if body.RunnerAgentId != nil {
			runner = *body.RunnerAgentId
		}
		id, err := control.ResolveBoardRepo(ctx, a.ID, runner, repo)
		if err != nil {
			return 0, err
		}
		body.RepoId = &id
	}
	launcher, ok := module.Lookup[contracts.Coding](m.d.Registry, contracts.CodingKey)
	if !ok {
		return 0, httpx.NewError(http.StatusNotImplemented, "feature_unavailable", "编码任务模块没有启用")
	}
	in := contracts.LaunchCoding{RepoID: *body.RepoId, IssueKey: brief.Key, Prompt: strings.TrimSpace(briefText(brief) + note),
		AIAgentID: a.ID}
	runID, err := m.createRun(ctx, a, brief, nil)
	if err != nil {
		return 0, err
	}
	in.RunID = runID
	in.OpenPR = body.OpenPr == nil || *body.OpenPr
	if body.BaseBranch != nil {
		in.BaseBranch = strings.TrimSpace(*body.BaseBranch)
	}
	if body.RunnerAgentId != nil {
		in.AgentID = strings.TrimSpace(*body.RunnerAgentId)
	}
	taskID, err := launcher.Launch(ctx, in)
	if err != nil {
		m.finishRun(ctx, runID, "failed", err.Error())
		return 0, err
	}
	_, err = m.d.DB.ExecContext(ctx, "UPDATE ai_agent_runs SET task_id=?,open_pr=? WHERE id=?", taskID, in.OpenPR, runID)
	if err != nil {
		return 0, err
	}
	if err := work.AddMember(ctx, brief.Key, "agent", strconv.FormatInt(a.ID, 10)); err != nil {
		slog.Warn("aiagents: add member", "issue", brief.Key, "err", err)
	}
	return taskID, nil
}

// ---- built-in agents ----

func (m *Module) busy(id int64) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running[id]
}

func (m *Module) startBuiltin(ctx context.Context, a contracts.AIAgent, brief contracts.IssueBrief, note string) error {
	runner, ok := module.Lookup[contracts.ToolRunner](m.d.Registry, contracts.ToolRunnerKey)
	if !ok {
		return httpx.NewError(http.StatusNotImplemented, "feature_unavailable", "AI 模块没有启用")
	}
	row, err := m.agentRow(ctx, a.ID)
	if err != nil {
		return err
	}
	base := m.base()
	if base == nil {
		return errors.New("aiagents not started")
	}
	m.mu.Lock()
	if m.running[a.ID] >= max(a.MaxParallel, 1) {
		m.mu.Unlock()
		return httpx.NewError(http.StatusConflict, "conflict", "Agent “"+a.Name+"” 正在忙，稍后再分配")
	}
	m.running[a.ID]++
	m.mu.Unlock()
	work, _ := m.issueWork()
	if err := work.AddMember(ctx, brief.Key, "agent", strconv.FormatInt(a.ID, 10)); err != nil {
		slog.Warn("aiagents: add member", "issue", brief.Key, "err", err)
	}
	m.comment(ctx, brief.Key, a.ID, "开始处理这张卡片。")
	m.publishAgent(ctx, a.ID)
	// The agent never acts with the user's elevation: its own session, like
	// an API token with the agent's access level.
	session := &auth.Session{ID: "ai_agent:" + strconv.FormatInt(a.ID, 10), Username: author(a.ID), ViaToken: true,
		Token: &auth.TokenInfo{Name: a.Name, Access: row.Access}}
	runCtx := auth.WithSession(base, session)
	runID, err := m.createRun(ctx, a, brief, nil)
	if err != nil {
		m.mu.Lock()
		m.running[a.ID]--
		m.mu.Unlock()
		return err
	}
	runCtx, cancelRun := context.WithCancel(runCtx)
	m.runMu.Lock()
	m.activeRuns[runID] = cancelRun
	m.runMu.Unlock()
	m.setRunStatus(base, runID, "running", "")
	system := fmt.Sprintf("你是 X Console 里的 Agent“%s”。用户是这个面板的唯一主人。\n%s\n\n"+
		"你在处理卡片 %s。用工具读取和修改数据，先读再改，不要重复创建。不要删除东西，除非卡片里明确要求。"+
		"完成后用简短的中文说明做了什么，这段话会作为你的评论贴在卡片上。", a.Name, row.Instructions, brief.Key)
	// B61: agents read the AI memory; their tools cannot change it.
	if mem, ok := module.Lookup[contracts.Memories](m.d.Registry, contracts.MemoriesKey); ok {
		if text := mem.Prompt(base); text != "" {
			system += "\n\n" + text
		}
	}
	prompt := fmt.Sprintf("卡片 %s：%s\n\n%s\n\n%s%s", brief.Key, brief.Title, brief.Description, briefText(brief), note)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancelRun()
		defer func() { m.runMu.Lock(); delete(m.activeRuns, runID); m.runMu.Unlock() }()
		defer func() {
			m.mu.Lock()
			m.running[a.ID]--
			m.mu.Unlock()
			m.publishAgent(base, a.ID)
		}()
		text, err := runner.RunTools(runCtx, contracts.ToolRun{Model: row.Model, System: system, Prompt: prompt,
			Access: row.Access, Source: "ai_agent", Ref: strconv.FormatInt(a.ID, 10), Observer: &runObserver{m: m, id: runID}, Decider: &runObserver{m: m, id: runID}})
		if base.Err() != nil {
			return
		}
		if err != nil {
			status := "failed"
			if runCtx.Err() != nil {
				status = "canceled"
			}
			m.finishRun(base, runID, status, err.Error())
			m.comment(base, brief.Key, a.ID, "没做完："+err.Error())
			return
		}
		if text == "" {
			text = "做完了。"
		}
		m.comment(base, brief.Key, a.ID, text)
		m.appendRunEvent(base, runID, "text", text, "", nil)
		m.finishRun(base, runID, "done", text)
	}()
	return nil
}

func (m *Module) comment(ctx context.Context, key string, agentID int64, body string) {
	work, err := m.issueWork()
	if err != nil {
		return
	}
	if err := work.Comment(ctx, key, author(agentID), body); err != nil {
		slog.Warn("aiagents: comment", "issue", key, "err", err)
	}
}

func (m *Module) publishAgent(ctx context.Context, id int64) {
	if a, err := m.agent(ctx, id); err == nil {
		m.d.Bus.Publish("ai_agent.changed", a)
	}
}

// ---- following coding tasks ----

// taskEvent is the part of a coding task event this module reads.
type taskEvent struct {
	ID           int64      `json:"id"`
	IssueKey     *string    `json:"issueKey"`
	AiAgentID    *int64     `json:"aiAgentId"`
	Status       string     `json:"status"`
	Error        string     `json:"error"`
	PrURL        string     `json:"prUrl"`
	ChangedFiles []struct{} `json:"changedFiles"`
}

type buildEvent struct {
	TaskID    int64  `json:"taskId"`
	Status    string `json:"status"`
	Error     string `json:"error"`
	IssueKey  string `json:"issueKey"`
	AiAgentID *int64 `json:"aiAgentId"`
}

func decodeEvent(data any, out any) bool {
	raw, err := json.Marshal(data)
	return err == nil && json.Unmarshal(raw, out) == nil
}

// follow comments on cards as their agents' tasks move on.
func (m *Module) follow(ctx context.Context) {
	events, cancel := m.d.Bus.Subscribe("coding_task.", 128)
	defer cancel()
	last := map[int64]string{}
	lastError := map[int64]string{}
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			switch ev.Topic {
			case "coding_task.updated", "coding_task.created":
				var t taskEvent
				if !decodeEvent(ev.Data, &t) || t.AiAgentID == nil || t.IssueKey == nil || *t.IssueKey == "" {
					continue
				}
				prev := last[t.ID]
				if prev == t.Status && lastError[t.ID] == t.Error {
					continue
				}
				last[t.ID] = t.Status
				lastError[t.ID] = t.Error
				m.onTask(ctx, t, prev)
			case "coding_task.build":
				var b buildEvent
				if !decodeEvent(ev.Data, &b) || b.AiAgentID == nil || b.IssueKey == "" {
					continue
				}
				link := fmt.Sprintf("[编码任务 #%d](/coding/%d)", b.TaskID, b.TaskID)
				switch b.Status {
				case "passed":
					m.comment(ctx, b.IssueKey, *b.AiAgentID, "构建通过了。"+link)
				case "failed":
					m.comment(ctx, b.IssueKey, *b.AiAgentID, "构建没通过："+b.Error+" "+link)
					var runID int64
					if err := m.d.DB.QueryRowContext(ctx, "SELECT id FROM ai_agent_runs WHERE task_id=?", b.TaskID).Scan(&runID); err == nil {
						m.appendRunEvent(ctx, runID, "error", "构建没通过："+b.Error, "", nil)
						m.notifyRun(ctx, runID, "failed", "构建没通过："+b.Error, nil)
					}
				}
			}
		}
	}
}

func (m *Module) onTask(ctx context.Context, t taskEvent, prev string) {
	m.followRunTask(ctx, t)
	key, agentID := *t.IssueKey, *t.AiAgentID
	link := fmt.Sprintf("[编码任务 #%d](/coding/%d)", t.ID, t.ID)
	switch t.Status {
	case "running":
		if prev == "review" {
			m.comment(ctx, key, agentID, "按构建日志再改一次。"+link)
		} else {
			m.comment(ctx, key, agentID, "开始处理。"+link)
		}
	case "review":
		m.comment(ctx, key, agentID, fmt.Sprintf("改完了，改了 %d 个文件，等你看。%s", len(t.ChangedFiles), link))
		if issues, ok := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey); ok {
			if err := issues.SetStatus(ctx, key, "in_review"); err != nil {
				slog.Warn("aiagents: move card", "issue", key, "err", err)
			}
		}
	case "failed":
		m.comment(ctx, key, agentID, "失败了："+t.Error+" "+link)
	case "pr_opened":
		m.comment(ctx, key, agentID, "开了 PR："+t.PrURL)
	}
}
