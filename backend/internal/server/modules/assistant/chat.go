package assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/google/uuid"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/assistant/api"
)

func (m *Module) SendMessage(w http.ResponseWriter, r *http.Request, id string) {
	var body api.SendMessage
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" || len([]rune(text)) > 20000 {
		httpx.Fail(w, r, httpx.Invalid("消息内容无效"))
		return
	}
	configured, err := m.d.Settings.Has(r.Context(), apiKeyKey)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !configured {
		httpx.Fail(w, r, httpx.ErrIntegrationMissing)
		return
	}
	var pending int
	if err := m.d.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM ai_pending_actions WHERE conversation_id=? AND status IN ('pending','approved')`, id).Scan(&pending); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if pending != 0 {
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	m.mu.Lock()
	if m.running[id] {
		m.mu.Unlock()
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	m.running[id] = true
	m.mu.Unlock()
	defer func() {
		if err != nil {
			m.finish(id)
		}
	}()
	if _, err = m.appendMessage(r.Context(), id, "user", []map[string]any{{"type": "text", "text": text}}); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(r.Context(), "ai.message.send", id, nil, nil)
	go m.respond(id)
	httpx.JSON(w, http.StatusAccepted, map[string]any{"conversationId": id})
}

func (m *Module) appendMessage(ctx context.Context, id, role string, blocks []map[string]any) (api.AIMessage, error) {
	msg := api.AIMessage{Id: uuid.NewString(), ConversationId: id, Role: api.AIMessageRole(role), Content: blocks, CreatedAt: time.Now().UTC()}
	raw, err := json.Marshal(blocks)
	if err != nil {
		return msg, err
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return msg, err
	}
	defer tx.Rollback()
	var seq int
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM ai_messages WHERE conversation_id=?`, id).Scan(&seq)
	if err != nil {
		return msg, err
	}
	msg.Seq = seq
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_messages(id,conversation_id,seq,role,content,created_at) VALUES(?,?,?,?,?,?)`, msg.Id, id, seq, role, string(raw), msg.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			return msg, httpx.ErrNotFound
		}
		return msg, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE ai_conversations SET updated_at=? WHERE id=?`, msg.CreatedAt, id)
	if err != nil {
		return msg, err
	}
	return msg, tx.Commit()
}

func (m *Module) finish(id string) {
	m.mu.Lock()
	delete(m.running, id)
	m.mu.Unlock()
}

func (m *Module) respond(id string) {
	defer m.finish(id)
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
	defer cancel()
	if err := m.generate(ctx, id); err != nil {
		m.d.Log.Error("AI reply failed", "conversation", id, "err", err)
		m.d.Bus.Publish("ai.error", map[string]any{"conversationId": id, "message": "AI 回复失败，请稍后重试"})
	}
}

func (m *Module) generate(ctx context.Context, id string) error { return m.generateDepth(ctx, id, 0) }

func (m *Module) generateDepth(ctx context.Context, id string, depth int) error {
	if depth >= 12 {
		return fmt.Errorf("too many AI tool rounds")
	}
	var key string
	if err := m.d.Settings.Get(ctx, apiKeyKey, &key); err != nil {
		return err
	}
	cfg, err := m.settings(ctx)
	if err != nil {
		return err
	}
	detail, err := m.detail(ctx, id)
	if err != nil {
		return err
	}
	messages := make([]anthropic.MessageParam, 0, len(detail.Messages))
	for _, saved := range detail.Messages {
		blocks := make([]anthropic.ContentBlockParamUnion, 0, len(saved.Content))
		for _, block := range saved.Content {
			raw, err := json.Marshal(block)
			if err != nil {
				return err
			}
			var param anthropic.ContentBlockParamUnion
			if err := json.Unmarshal(raw, &param); err != nil {
				return err
			}
			blocks = append(blocks, param)
		}
		if saved.Role == "assistant" {
			messages = append(messages, anthropic.NewAssistantMessage(blocks...))
		} else {
			messages = append(messages, anthropic.NewUserMessage(blocks...))
		}
	}
	tools, err := m.tools()
	if err != nil {
		return err
	}
	system := m.systemPrompt(ctx)
	params := anthropic.MessageNewParams{Model: anthropic.Model(cfg.Model), MaxTokens: 8192, Messages: messages, Tools: tools, Thinking: anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}, System: []anthropic.TextBlockParam{{Text: system}}}
	opts := []option.RequestOption{option.WithAPIKey(key), option.WithHeader("anthropic-beta", "server-side-fallback-2026-07-01"), option.WithJSONSet("fallbacks", "default"), option.WithHTTPClient(&http.Client{Timeout: 10 * time.Minute})}
	if m.baseURL != "" {
		opts = append(opts, option.WithBaseURL(m.baseURL))
	}
	client := anthropic.NewClient(opts...)
	stream := client.Messages.NewStreaming(ctx, params)
	defer stream.Close()
	var reply anthropic.Message
	for stream.Next() {
		event := stream.Current()
		if err := reply.Accumulate(event); err != nil {
			return err
		}
		if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" && event.Delta.Text != "" {
			m.d.Bus.Publish("ai.delta", map[string]any{"conversationId": id, "text": event.Delta.Text})
		}
	}
	if err := stream.Err(); err != nil {
		return err
	}
	if len(reply.Content) == 0 {
		return fmt.Errorf("empty Claude reply")
	}
	var blocks []map[string]any
	if err := json.Unmarshal([]byte(reply.RawJSON()), &struct {
		Content *[]map[string]any `json:"content"`
	}{Content: &blocks}); err != nil {
		return err
	}
	if len(blocks) == 0 {
		return fmt.Errorf("empty Claude content")
	}
	msg, err := m.appendMessage(ctx, id, "assistant", blocks)
	if err != nil {
		return err
	}
	m.d.Bus.Publish("ai.message_done", map[string]any{"conversationId": id, "message": msg})
	for _, block := range reply.Content {
		if block.Type != "tool_use" {
			continue
		}
		name := strings.ReplaceAll(block.Name, "__", ".")
		action, ok := m.d.Actions.Get(name)
		if !ok {
			return fmt.Errorf("unknown tool %q", name)
		}
		if action.Effect != actions.Read {
			if err := m.queueAction(ctx, id, block.ID, name, block.Input); err != nil {
				return err
			}
			continue
		}
		result, runErr := m.d.Actions.Run(ctx, name, block.Input)
		m.d.Audit.Record(ctx, "ai.action.read", name, nil, runErr)
		m.d.Bus.Publish("ai.action_result", map[string]any{"conversationId": id, "toolUseId": block.ID, "result": result, "error": errorText(runErr)})
		if err := m.appendToolResult(ctx, id, block.ID, result, runErr); err != nil {
			return err
		}
	}
	if reply.StopReason == "tool_use" {
		var pending int
		if err := m.d.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_pending_actions WHERE conversation_id=? AND status='pending'`, id).Scan(&pending); err != nil {
			return err
		}
		if pending == 0 {
			return m.generateDepth(ctx, id, depth+1)
		}
	}
	return nil
}

func (m *Module) tools() ([]anthropic.ToolUnionParam, error) {
	out := make([]anthropic.ToolUnionParam, 0)
	for _, a := range m.d.Actions.List() {
		var schema anthropic.ToolInputSchemaParam
		if err := json.Unmarshal(a.Input, &schema); err != nil {
			return nil, err
		}
		tool := anthropic.ToolUnionParamOfTool(schema, strings.ReplaceAll(a.Name, ".", "__"))
		tool.OfTool.Description = anthropic.String(a.Description)
		out = append(out, tool)
	}
	return out, nil
}

func (m *Module) systemPrompt(ctx context.Context) string {
	agents, err := m.d.Agents.List(ctx)
	count := 0
	if err == nil {
		count = len(agents)
	}
	return fmt.Sprintf("你是 X Console 的助手。用户是面板的唯一主人。按动作说明使用工具。修改数据前等待用户确认。已配对设备：%d 台。\n当前时间：%s。时区：%s。", count, time.Now().In(m.d.Config.Location).Format(time.RFC3339), m.d.Config.Location)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (m *Module) queueAction(ctx context.Context, id, toolID, name string, input json.RawMessage) error {
	actionID := uuid.NewString()
	_, err := m.d.DB.ExecContext(ctx, `INSERT INTO ai_pending_actions(id,conversation_id,tool_use_id,action,input,status,created_at) VALUES(?,?,?,?,?,'pending',?)`, actionID, id, toolID, name, string(input), time.Now().UTC())
	if err != nil {
		return err
	}
	m.d.Bus.Publish("ai.action_pending", map[string]any{"conversationId": id, "id": actionID, "toolUseId": toolID, "action": name, "input": json.RawMessage(input)})
	return nil
}

func (m *Module) appendToolResult(ctx context.Context, id, toolID string, result any, runErr error) error {
	content := result
	if runErr != nil {
		content = map[string]any{"error": runErr.Error()}
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return err
	}
	_, err = m.appendMessage(ctx, id, "user", []map[string]any{{"type": "tool_result", "tool_use_id": toolID, "content": string(raw), "is_error": runErr != nil}})
	return err
}

func (m *Module) ApproveAction(w http.ResponseWriter, r *http.Request, id string) {
	m.decideAction(w, r, id, true)
}
func (m *Module) RejectAction(w http.ResponseWriter, r *http.Request, id string) {
	m.decideAction(w, r, id, false)
}

func (m *Module) decideAction(w http.ResponseWriter, r *http.Request, id string, approved bool) {
	var conversationID, toolID, name, input, status string
	err := m.d.DB.QueryRowContext(r.Context(), `SELECT conversation_id,tool_use_id,action,input,status FROM ai_pending_actions WHERE id=?`, id).Scan(&conversationID, &toolID, &name, &input, &status)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if status != "pending" {
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	action, ok := m.d.Actions.Get(name)
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if approved && action.Effect == actions.Dangerous {
		if err := auth.RequireElevated(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	decision := "rejected"
	if approved {
		decision = "approved"
	}
	res, err := m.d.DB.ExecContext(r.Context(), `UPDATE ai_pending_actions SET status=? WHERE id=? AND status='pending'`, decision, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	ctx := context.WithoutCancel(r.Context())
	go m.completeAction(ctx, id, conversationID, toolID, name, json.RawMessage(input), approved)
	httpx.JSON(w, http.StatusAccepted, map[string]any{"id": id, "status": decision})
}

func (m *Module) completeAction(ctx context.Context, id, conversationID, toolID, name string, input json.RawMessage, approved bool) {
	var result any
	var err error
	if approved {
		result, err = m.d.Actions.Run(ctx, name, input)
	} else {
		err = errors.New("用户拒绝了")
	}
	status := "done"
	if !approved {
		status = "rejected"
	} else if err != nil {
		status = "failed"
	}
	raw, _ := json.Marshal(map[string]any{"result": result, "error": errorText(err)})
	_, dbErr := m.d.DB.ExecContext(ctx, `UPDATE ai_pending_actions SET status=?,result=? WHERE id=?`, status, string(raw), id)
	m.d.Audit.Record(ctx, "ai.action."+status, name, map[string]any{"pendingActionId": id}, err)
	if dbErr != nil {
		m.d.Log.Error("save AI action result", "err", dbErr)
		return
	}
	if err := m.appendToolResult(ctx, conversationID, toolID, result, err); err != nil {
		m.d.Log.Error("save AI tool result", "err", err)
		return
	}
	m.d.Bus.Publish("ai.action_result", map[string]any{"conversationId": conversationID, "id": id, "status": status})
	var pending int
	if err := m.d.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_pending_actions WHERE conversation_id=? AND status IN ('pending','approved')`, conversationID).Scan(&pending); err != nil || pending != 0 {
		return
	}
	m.mu.Lock()
	if m.running[conversationID] {
		m.mu.Unlock()
		return
	}
	m.running[conversationID] = true
	m.mu.Unlock()
	go m.respond(conversationID)
}
