package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

const systemPrompt = `你是 X Console 助手。用户是这个面板的唯一主人。可用工具由面板提供。
先读现有数据再改，避免重复创建。可直接执行读取、新建和修改。删除和高危操作按面板确认流程处理。
排期按用户时区从今天开始，跳过周末，除非用户另有要求。完成后用简短中文列出改动，并附面板内路径。隐藏内容不可访问。`

func (m *Module) tools() []llm.Tool {
	all := m.d.Actions.List()
	out := make([]llm.Tool, 0, len(all))
	for _, a := range all {
		if !json.Valid(a.Input) {
			continue
		}
		out = append(out, llm.Tool{Name: strings.ReplaceAll(a.Name, ".", "__"), Description: a.Description, Parameters: a.Input})
	}
	return out
}
func (m *Module) history(ctx context.Context, id int64) ([]llm.Message, error) {
	rows, err := m.d.DB.QueryContext(ctx, "SELECT role,content FROM ai_messages WHERE conversation_id=? ORDER BY seq", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []llm.Message{}
	for rows.Next() {
		var role, content string
		if err = rows.Scan(&role, &content); err != nil {
			return nil, err
		}
		var blocks []map[string]any
		if err = json.Unmarshal([]byte(content), &blocks); err != nil {
			return nil, err
		}
		msg := llm.Message{Role: role}
		for _, block := range blocks {
			switch block["type"] {
			case "text":
				if value, ok := block["text"].(string); ok {
					msg.Content += value + "\n"
				}
			case "tool_use":
				input, _ := json.Marshal(block["input"])
				msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{ID: stringValue(block["id"]), Name: stringValue(block["name"]), Arguments: input})
			case "tool_calls":
				for _, value := range listValue(block["calls"]) {
					call := mapValue(value)
					input, _ := json.Marshal(call["arguments"])
					msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{ID: stringValue(call["id"]), Name: stringValue(call["name"]), Arguments: input})
				}
			case "tool_result":
				value := block["content"]
				content := stringValue(value)
				if content == "" {
					raw, _ := json.Marshal(value)
					content = string(raw)
				}
				out = append(out, llm.Message{Role: "tool", ToolCallID: stringValue(block["tool_use_id"]), Content: content})
			}
		}
		if msg.Content != "" || len(msg.ToolCalls) > 0 {
			out = append(out, msg)
		}
	}
	return out, rows.Err()
}
func stringValue(v any) string      { value, _ := v.(string); return value }
func listValue(v any) []any         { value, _ := v.([]any); return value }
func mapValue(v any) map[string]any { value, _ := v.(map[string]any); return value }
func (m *Module) system(ctx context.Context) string {
	devices, _ := m.d.Agents.List(ctx)
	summary := fmt.Sprintf("当前时间：%s。时区：%s。已连接设备：%d。", time.Now().In(m.d.Config.Location).Format(time.RFC3339), m.d.Config.Location, len(devices))
	return systemPrompt + "\n" + summary
}
func (m *Module) run(ctx context.Context, id int64, session *auth.Session, state *generation) {
	defer func() {
		m.mu.Lock()
		if m.running[id] == state {
			delete(m.running, id)
		}
		m.mu.Unlock()
		m.d.Bus.Publish("ai.message_done", map[string]any{"conversationId": id})
	}()
	if session != nil {
		ctx = auth.WithSession(ctx, session)
	}
	ctx = auth.WithoutVault(ctx)
	err := m.generate(ctx, id)
	if err != nil && !errors.Is(err, context.Canceled) {
		m.d.Log.Error("ai generation failed", "conversation", id, "err", err)
		m.d.Bus.Publish("ai.error", map[string]any{"conversationId": id, "message": err.Error()})
	}
}
func (m *Module) generate(ctx context.Context, id int64) error {
	settings, err := m.modelSettings(ctx)
	if err != nil {
		return err
	}
	for turn := 0; turn < 20; turn++ {
		history, err := m.history(ctx, id)
		if err != nil {
			return err
		}
		stream, err := m.llm.Stream(ctx, llm.Request{Purpose: "agent", System: m.system(ctx), Messages: history, Tools: m.tools()})
		if err != nil {
			return err
		}
		for stream.Next() {
			if delta := stream.Current().Text; delta != "" {
				m.d.Bus.Publish("ai.delta", map[string]any{"conversationId": id, "text": delta})
			}
		}
		err = stream.Err()
		message := stream.Result()
		_ = stream.Close()
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		blocks := []map[string]any{}
		if message.Text != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": message.Text})
		}
		for _, call := range message.ToolCalls {
			var input any
			if json.Unmarshal(call.Arguments, &input) != nil {
				input = map[string]any{}
			}
			blocks = append(blocks, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": input})
		}
		if err = m.saveMessage(ctx, id, "assistant", blocks); err != nil {
			return err
		}
		results := []map[string]any{}
		pending := false
		for _, use := range message.ToolCalls {
			name := strings.ReplaceAll(use.Name, "__", ".")
			action, found := m.d.Actions.Get(name)
			input := use.Arguments
			if len(input) == 0 {
				input = []byte("{}")
			}
			if !found {
				results = append(results, toolResult(use.ID, "未知动作", true))
				continue
			}
			confirm := action.Effect == actions.Dangerous || action.Destructive || settings.ConfirmAllWrites && action.Effect == actions.Write || strings.HasSuffix(name, ".delete")
			if confirm {
				p, err := m.recordAction(ctx, id, use.ID, name, input, "pending", nil)
				if err != nil {
					return err
				}
				m.d.Bus.Publish("ai.action_pending", map[string]any{"conversationId": id, "action": p})
				pending = true
				continue
			}
			result, err := m.runAction(ctx, action, input)
			state := "done"
			if err != nil {
				state = "failed"
				result = err.Error()
			}
			if _, e := m.recordAction(ctx, id, use.ID, name, input, state, result); e != nil {
				return e
			}
			results = append(results, toolResult(use.ID, result, err != nil))
		}
		if pending {
			return nil
		}
		if len(results) == 0 {
			return nil
		}
		if err = m.saveMessage(ctx, id, "user", results); err != nil {
			return err
		}
	}
	return fmt.Errorf("助手调用动作次数超过限制")
}
func toolResult(id string, result any, isError bool) map[string]any {
	content := result
	if _, ok := result.(string); !ok {
		raw, _ := json.Marshal(result)
		content = string(raw)
	}
	return map[string]any{"type": "tool_result", "tool_use_id": id, "content": content, "is_error": isError}
}
func (m *Module) runAction(ctx context.Context, a actions.Action, input json.RawMessage) (any, error) {
	return a.Run(auth.WithoutVault(ctx), input)
}
func (m *Module) recordAction(ctx context.Context, conversationID int64, toolID, name string, input json.RawMessage, status string, result any) (api.PendingAction, error) {
	if len(input) == 0 {
		input = []byte("{}")
	}
	raw, _ := json.Marshal(result)
	var id int64
	err := m.d.DB.QueryRowContext(ctx, "INSERT INTO ai_pending_actions(conversation_id,tool_use_id,action,input,status,result) VALUES(?,?,?,?,?,?) RETURNING id", conversationID, toolID, name, string(input), status, string(raw)).Scan(&id)
	if err != nil {
		return api.PendingAction{}, err
	}
	var in map[string]any
	_ = json.Unmarshal(input, &in)
	return api.PendingAction{Id: id, ConversationId: conversationID, ToolUseId: toolID, Action: name, Input: in, Status: api.PendingActionStatus(status), Result: result}, nil
}
func (m *Module) decide(w http.ResponseWriter, r *http.Request, actionID int64, approve bool) {
	ctx := r.Context()
	var conversationID int64
	var toolID, name, input, status string
	err := m.d.DB.QueryRowContext(ctx, "SELECT conversation_id,tool_use_id,action,input,status FROM ai_pending_actions WHERE id=?", actionID).Scan(&conversationID, &toolID, &name, &input, &status)
	if m.fail(w, r, notFound(err)) {
		return
	}
	if status != "pending" {
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	a, found := m.d.Actions.Get(name)
	if !found {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if approve && a.Effect == actions.Dangerous && m.fail(w, r, auth.RequireElevated(ctx)) {
		return
	}
	workCtx := context.WithoutCancel(ctx)
	state := "rejected"
	if approve {
		state = "approved"
	}
	updated, e := m.d.DB.ExecContext(workCtx, "UPDATE ai_pending_actions SET status=? WHERE id=? AND status='pending'", state, actionID)
	if m.fail(w, r, e) {
		return
	}
	n, _ := updated.RowsAffected()
	if n == 0 {
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	result := any("用户拒绝了")
	next := "rejected"
	if approve {
		result, err = m.runAction(workCtx, a, json.RawMessage(input))
		next = "done"
		if err != nil {
			result = err.Error()
			next = "failed"
		}
	}
	raw, _ := json.Marshal(result)
	_, e = m.d.DB.ExecContext(workCtx, "UPDATE ai_pending_actions SET status=?,result=? WHERE id=?", next, string(raw), actionID)
	if m.fail(w, r, e) {
		return
	}
	m.stop(conversationID)
	m.d.Bus.Publish("ai.message_saved", map[string]any{"conversationId": conversationID})
	m.d.Audit.Record(workCtx, "ai.action."+next, strconv.FormatInt(actionID, 10), map[string]any{"action": name}, err)
	if e = m.resumeAfterDecisions(workCtx, conversationID); m.fail(w, r, e) {
		return
	}
	httpx.NoContent(w)
}
func (m *Module) ApproveAiAction(w http.ResponseWriter, r *http.Request, id api.ActionId) {
	m.decide(w, r, id, true)
}
func (m *Module) RejectAiAction(w http.ResponseWriter, r *http.Request, id api.ActionId) {
	m.decide(w, r, id, false)
}

func (m *Module) resumeAfterDecisions(ctx context.Context, id int64) error {
	return m.completeAfterDecisions(ctx, id, true)
}
func (m *Module) completeAfterDecisions(ctx context.Context, id int64, resume bool) error {
	var waiting int
	if err := m.d.DB.QueryRowContext(ctx, "SELECT count(*) FROM ai_pending_actions WHERE conversation_id=? AND status='pending'", id).Scan(&waiting); err != nil {
		return err
	}
	if waiting > 0 {
		return nil
	}
	var content string
	var seq int
	if err := m.d.DB.QueryRowContext(ctx, "SELECT seq,content FROM ai_messages WHERE conversation_id=? AND role='assistant' ORDER BY seq DESC LIMIT 1", id).Scan(&seq, &content); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	var later int
	if err := m.d.DB.QueryRowContext(ctx, "SELECT count(*) FROM ai_messages WHERE conversation_id=? AND seq>?", id, seq).Scan(&later); err != nil {
		return err
	}
	if later > 0 {
		return nil
	}
	var blocks []struct{ Type, ID string }
	if err := json.Unmarshal([]byte(content), &blocks); err != nil {
		return err
	}
	results := []map[string]any{}
	for _, block := range blocks {
		if block.Type != "tool_use" {
			continue
		}
		var status, result string
		err := m.d.DB.QueryRowContext(ctx, "SELECT status,result FROM ai_pending_actions WHERE conversation_id=? AND tool_use_id=?", id, block.ID).Scan(&status, &result)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		var value any
		_ = json.Unmarshal([]byte(result), &value)
		results = append(results, toolResult(block.ID, value, status == "failed" || status == "rejected"))
	}
	if len(results) == 0 {
		return nil
	}
	if err := m.saveMessage(ctx, id, "user", results); err != nil {
		return err
	}
	if !resume {
		return nil
	}
	if _, err := m.resolveLLM(ctx, "agent"); err != nil {
		return err
	}
	m.mu.Lock()
	if _, running := m.running[id]; running {
		m.mu.Unlock()
		return nil
	}
	worker, cancel := context.WithCancel(context.Background())
	state := &generation{cancel: cancel}
	m.running[id] = state
	m.mu.Unlock()
	go m.run(worker, id, auth.FromContext(ctx), state)
	return nil
}
