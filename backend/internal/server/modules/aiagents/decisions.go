package aiagents

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

type decisionReply struct {
	session *auth.Session
	approve bool
	answer  string
}

func (m *Module) ReceiveCodingQuestion(ctx context.Context, task int64, q contracts.ToolQuestion) error {
	var run int64
	if err := m.d.DB.QueryRowContext(ctx, "SELECT id FROM ai_agent_runs WHERE task_id=?", task).Scan(&run); err != nil {
		return err
	}
	_, err := m.createDecision(ctx, run, "question", q, false, nil)
	return err
}

func decisionID(run, id int64) string { return fmt.Sprintf("run:%d:%d", run, id) }

func (m *Module) createDecision(ctx context.Context, runID int64, kind string, q contracts.ToolQuestion, dangerous bool, ch chan decisionReply) (int64, error) {
	q.Title = strings.TrimSpace(q.Title)
	if q.Title == "" || len(q.Title) > 500 || len(q.Detail) > 65536 || len(q.Options) > 20 {
		return 0, httpx.Invalid("问题太长或没有标题")
	}
	options, _ := json.Marshal(q.Options)
	m.decisionMu.Lock()
	defer m.decisionMu.Unlock()
	var id int64
	err := m.d.DB.QueryRowContext(ctx, `INSERT INTO ai_agent_decisions(run_id,kind,title,detail,options,dangerous,created_at) VALUES(?,?,?,?,?,?,?) RETURNING id`, runID, kind, q.Title, q.Detail, string(options), dangerous, m.now()).Scan(&id)
	if err != nil {
		return 0, err
	}
	if ch != nil {
		m.decisions[id] = ch
	}
	m.setRunStatus(ctx, runID, "waiting", q.Title)
	m.appendRunEvent(ctx, runID, "decision", q.Title, "", nil)
	m.d.Bus.Publish("ai_agent.decision", map[string]any{"id": decisionID(runID, id)})
	buttons := []notify.Action{{ID: "open", Label: "打开"}}
	if kind == "permission" {
		buttons = []notify.Action{{ID: "ai_agent.approve:" + decisionID(runID, id), Label: "批准"}, {ID: "ai_agent.reject:" + decisionID(runID, id), Label: "拒绝"}}
	}
	m.notifyRun(ctx, runID, "decision", q.Title, buttons)
	return id, nil
}

func (o *runObserver) waitDecision(ctx context.Context, kind string, q contracts.ToolQuestion, dangerous bool) (decisionReply, error) {
	ch := make(chan decisionReply, 1)
	id, err := o.m.createDecision(ctx, o.id, kind, q, dangerous, ch)
	if err != nil {
		return decisionReply{}, err
	}
	defer func() { o.m.decisionMu.Lock(); delete(o.m.decisions, id); o.m.decisionMu.Unlock() }()
	select {
	case reply := <-ch:
		o.m.setRunStatus(ctx, o.id, "running", "")
		return reply, nil
	case <-ctx.Done():
		_, _ = o.m.d.DB.ExecContext(context.WithoutCancel(ctx), "UPDATE ai_agent_decisions SET status='canceled' WHERE id=? AND status='pending'", id)
		o.m.d.Bus.Publish("ai_agent.decision", map[string]any{"id": decisionID(o.id, id)})
		return decisionReply{}, ctx.Err()
	}
}

func (o *runObserver) ConfirmTool(ctx context.Context, name string, input json.RawMessage, dangerous bool) (context.Context, bool, error) {
	r, err := o.waitDecision(ctx, "permission", contracts.ToolQuestion{Title: "确认操作：" + name, Detail: string(input)}, dangerous)
	if err != nil {
		return ctx, false, err
	}
	if r.session != nil {
		ctx = auth.WithSession(ctx, r.session)
	}
	return ctx, r.approve, nil
}
func (o *runObserver) AskUser(ctx context.Context, q contracts.ToolQuestion) (string, error) {
	r, err := o.waitDecision(ctx, "question", q, false)
	return r.answer, err
}

func (m *Module) ListAiAgentDecisions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := m.d.DB.QueryContext(ctx, `SELECT d.id,d.run_id,r.agent_id,a.name,r.issue_key,d.kind,d.title,d.detail,d.options,d.created_at FROM ai_agent_decisions d JOIN ai_agent_runs r ON r.id=d.run_id JOIN ai_agents a ON a.id=r.agent_id WHERE d.status='pending' ORDER BY d.id`)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := []api.AiAgentDecision{}
	for rows.Next() {
		var x api.AiAgentDecision
		var id, run, agent int64
		var name, key, detail, options string
		if err = rows.Scan(&id, &run, &agent, &name, &key, &x.Kind, &x.Title, &detail, &options, &x.CreatedAt); err != nil {
			break
		}
		x.Id = decisionID(run, id)
		x.RunId = &run
		x.AgentId = &agent
		x.AgentName = &name
		x.IssueKey = &key
		x.Detail = &detail
		var opts []string
		_ = json.Unmarshal([]byte(options), &opts)
		if len(opts) > 0 {
			x.Options = &opts
		}
		out = append(out, x)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err = m.d.DB.QueryContext(ctx, `SELECT p.id,p.conversation_id,p.action,p.input,c.created_at FROM ai_pending_actions p JOIN ai_conversations c ON c.id=p.conversation_id WHERE p.status='pending' ORDER BY p.id`)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var x api.AiAgentDecision
		var id, conversation int64
		var detail string
		if err = rows.Scan(&id, &conversation, &x.Title, &detail, &x.CreatedAt); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		x.Id = fmt.Sprintf("action:%d", id)
		x.ConversationId = &conversation
		x.Kind = api.Permission
		x.Detail = &detail
		out = append(out, x)
	}
	if err = rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, out)
}

func (m *Module) AnswerAiAgentDecision(w http.ResponseWriter, r *http.Request, id string) {
	var body api.AnswerAiAgentDecisionJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err := m.answerDecision(r.Context(), id, body)
	m.d.Audit.Record(r.Context(), "ai_agent.decision", id, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) answerDecision(ctx context.Context, key string, body api.AnswerAiAgentDecisionJSONRequestBody) error {
	if strings.HasPrefix(key, "action:") {
		id, err := strconv.ParseInt(strings.TrimPrefix(key, "action:"), 10, 64)
		if err != nil || body.Approve == nil {
			return httpx.Invalid("请选择批准或拒绝")
		}
		a, ok := module.Lookup[contracts.AssistantDecisions](m.d.Registry, contracts.AssistantDecisionsKey)
		if !ok {
			return httpx.ErrNotLive
		}
		err = a.AnswerAssistantDecision(ctx, id, *body.Approve)
		if err == nil {
			m.d.Bus.Publish("ai_agent.decision", map[string]any{"id": key})
		}
		return err
	}
	var run, id int64
	if n, _ := fmt.Sscanf(key, "run:%d:%d", &run, &id); n != 2 || key != decisionID(run, id) {
		return httpx.ErrNotFound
	}
	m.decisionMu.Lock()
	defer m.decisionMu.Unlock()
	var kind, status string
	var dangerous bool
	var task sql.NullInt64
	err := m.d.DB.QueryRowContext(ctx, `SELECT d.kind,d.status,d.dangerous,r.task_id FROM ai_agent_decisions d JOIN ai_agent_runs r ON r.id=d.run_id WHERE d.id=? AND d.run_id=?`, id, run).Scan(&kind, &status, &dangerous, &task)
	if err == sql.ErrNoRows {
		return httpx.ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "pending" {
		return httpx.ErrConflict
	}
	reply := decisionReply{session: auth.FromContext(ctx)}
	state := "answered"
	if kind == "permission" {
		if body.Approve == nil {
			return httpx.Invalid("请选择批准或拒绝")
		}
		reply.approve = *body.Approve
		state = "rejected"
		if reply.approve {
			state = "approved"
			if dangerous {
				if err := auth.RequireElevated(ctx); err != nil {
					return err
				}
			}
		}
	} else {
		if body.Answer == nil || strings.TrimSpace(*body.Answer) == "" || len(*body.Answer) > 65536 {
			return httpx.Invalid("请填写回答")
		}
		reply.answer = strings.TrimSpace(*body.Answer)
	}
	ch := m.decisions[id]
	if ch == nil && !task.Valid {
		return httpx.ErrConflict
	}
	if task.Valid {
		c, ok := module.Lookup[contracts.CodingControl](m.d.Registry, contracts.CodingControlKey)
		if !ok {
			return httpx.ErrNotLive
		}
		if err = c.ResumeCoding(ctx, task.Int64, reply.answer); err != nil {
			return err
		}
	}
	if _, err = m.d.DB.ExecContext(ctx, "UPDATE ai_agent_decisions SET status=?,answer=? WHERE id=? AND status='pending'", state, reply.answer, id); err != nil {
		return err
	}
	if ch != nil {
		ch <- reply
	}
	m.d.Bus.Publish("ai_agent.decision", map[string]any{"id": key})
	return nil
}

func (m *Module) notificationAction(ctx context.Context, id string) error {
	a, key, ok := strings.Cut(id, ":")
	if !ok {
		return httpx.ErrNotFound
	}
	approve := a == "ai_agent.approve"
	if !approve && a != "ai_agent.reject" {
		return httpx.ErrNotFound
	}
	return m.answerDecision(ctx, key, api.AnswerAiAgentDecisionJSONRequestBody{Approve: &approve})
}
