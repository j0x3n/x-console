package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/db"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

func conversation(row db.AiConversation) api.Conversation {
	c := api.Conversation{Id: row.ID, Title: row.Title, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.HostID != nil {
		c.HostId = row.HostID
	}
	return c
}

func (m *Module) ListAiConversations(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListConversations(r.Context())
	if m.fail(w, r, err) {
		return
	}
	out := make([]api.Conversation, 0, len(rows))
	for _, row := range rows {
		out = append(out, conversation(row))
	}
	httpx.JSON(w, 200, out)
}
func (m *Module) CreateAiConversation(w http.ResponseWriter, r *http.Request) {
	var body api.CreateAiConversationJSONRequestBody
	if r.ContentLength > 0 && m.fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	title := ""
	if body.Title != nil {
		title = strings.TrimSpace(*body.Title)
	}
	if chars := []rune(title); len(chars) > 100 {
		title = string(chars[:100])
	}
	now := time.Now().UTC()
	row, err := m.q.CreateConversation(r.Context(), db.CreateConversationParams{Title: title, CreatedAt: now, UpdatedAt: now})
	if m.fail(w, r, err) {
		return
	}
	m.d.Audit.Record(r.Context(), "ai.conversation.create", strconv.FormatInt(row.ID, 10), nil, nil)
	httpx.JSON(w, 201, conversation(row))
}
func (m *Module) DeleteAiConversation(w http.ResponseWriter, r *http.Request, id api.ConversationId) {
	m.stop(id)
	attachments, err := m.attachmentIDs(r.Context(), "SELECT id FROM ai_attachments WHERE conversation_id=?", id)
	if m.fail(w, r, err) {
		return
	}
	n, err := m.q.DeleteConversation(r.Context(), id)
	if m.fail(w, r, err) {
		return
	}
	if n == 0 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	m.deleteAttachmentFiles(r.Context(), attachments) // rows went with the conversation
	m.mu.Lock()
	delete(m.permissions, id)
	m.mu.Unlock()
	m.d.Audit.Record(r.Context(), "ai.conversation.delete", strconv.FormatInt(id, 10), nil, nil)
	httpx.NoContent(w)
}
func (m *Module) GetAiConversation(w http.ResponseWriter, r *http.Request, id api.ConversationId) {
	row, err := m.q.GetConversation(r.Context(), id)
	if m.fail(w, r, notFound(err)) {
		return
	}
	messages, err := m.messages(r.Context(), id)
	if m.fail(w, r, err) {
		return
	}
	pending, err := m.pendingActions(r.Context(), id)
	if m.fail(w, r, err) {
		return
	}
	m.mu.Lock()
	_, running := m.running[id]
	m.mu.Unlock()
	httpx.JSON(w, 200, api.ConversationDetail{Conversation: m.conversation(row), Messages: messages, PendingActions: pending, Running: running})
}
func (m *Module) SendAiMessage(w http.ResponseWriter, r *http.Request, id api.ConversationId) {
	ctx := r.Context()
	var body api.SendAiMessageJSONRequestBody
	if m.fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" || len(text) > 20000 {
		httpx.Fail(w, r, httpx.Invalid("消息不能为空，最多 20000 字符"))
		return
	}
	row, err := m.q.GetConversation(ctx, id)
	if m.fail(w, r, notFound(err)) {
		return
	}
	var waiting int
	if err := m.d.DB.QueryRowContext(ctx, "SELECT count(*) FROM ai_pending_actions WHERE conversation_id=? AND status IN ('pending','approved')", id).Scan(&waiting); m.fail(w, r, err) {
		return
	}
	if waiting > 0 {
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	if _, err := m.resolveLLM(ctx, "agent"); errors.Is(err, llm.ErrNotConfigured) {
		httpx.Fail(w, r, httpx.NewError(409, "ai_not_configured", "请先配置 Agent 模型"))
		return
	} else if m.fail(w, r, err) {
		return
	}
	var attachments []attachmentRow
	if body.AttachmentIds != nil && len(*body.AttachmentIds) > 0 {
		attachments, err = m.claimAttachments(ctx, id, *body.AttachmentIds)
		if m.fail(w, r, err) {
			return
		}
		for _, a := range attachments {
			if a.kind != "image" {
				continue
			}
			sees, err := m.modelSeesImages(ctx)
			if m.fail(w, r, err) {
				return
			}
			if !sees {
				httpx.Fail(w, r, httpx.NewError(400, "model_without_images", "当前 Agent 模型不支持图片，换一个能看图的模型再发"))
				return
			}
			break
		}
	}
	m.mu.Lock()
	if _, exists := m.running[id]; exists {
		m.mu.Unlock()
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	worker, cancel := context.WithCancel(context.Background())
	state := &generation{cancel: cancel}
	m.running[id] = state
	m.mu.Unlock()
	blocks := append([]map[string]any{{"type": "text", "text": text}}, attachmentBlocks(attachments)...)
	if body.Context != nil && body.Context.Path != nil && strings.HasPrefix(*body.Context.Path, "/") {
		title := ""
		if body.Context.Title != nil {
			title = *body.Context.Title
		}
		blocks = append(blocks, map[string]any{"type": "text", "text": "当前页面: " + title + " " + *body.Context.Path, "context": true})
	}
	blocks = append(blocks, map[string]any{"type": "text", "text": m.turnContext(ctx, row.HostID), "context": true})
	if err := m.saveMessage(ctx, id, "user", blocks); err != nil {
		m.stop(id)
		httpx.Fail(w, r, err)
		return
	}
	if row.HostID != nil {
		m.touchPermission(id)
	}
	if err := m.setTitle(ctx, id, text); err != nil {
		m.stop(id)
		httpx.Fail(w, r, err)
		return
	}
	session := auth.FromContext(ctx)
	go m.run(worker, id, session, state)
	httpx.JSON(w, 202, nil)
}
func (m *Module) StopAiReply(w http.ResponseWriter, r *http.Request, id api.ConversationId) {
	if _, err := m.q.GetConversation(r.Context(), id); m.fail(w, r, notFound(err)) {
		return
	}
	m.stop(id)
	_, err := m.d.DB.ExecContext(r.Context(), "UPDATE ai_pending_actions SET status='rejected',result=? WHERE conversation_id=? AND status IN ('pending','approved')", `"用户已停止生成"`, id)
	if m.fail(w, r, err) {
		return
	}
	if m.fail(w, r, m.completeAfterDecisions(r.Context(), id, false)) {
		return
	}
	m.d.Bus.Publish("ai.message_done", map[string]any{"conversationId": id})
	httpx.NoContent(w)
}
func (m *Module) stop(id int64) {
	m.mu.Lock()
	state := m.running[id]
	delete(m.running, id)
	m.mu.Unlock()
	if state != nil {
		state.cancel()
	}
}
func (m *Module) apiKey(ctx context.Context) (string, error) {
	var key string
	err := m.d.Settings.Get(ctx, keySetting, &key)
	if errors.Is(err, settings.ErrNotSet) {
		return "", nil
	}
	return key, err
}
func (m *Module) ListAiTools(w http.ResponseWriter, r *http.Request) {
	out := []api.Tool{}
	for _, a := range m.d.Actions.List(r.Context()) {
		out = append(out, api.Tool{Name: strings.ReplaceAll(a.Name, ".", "__"), Action: a.Name, Title: a.Title, Effect: api.ToolEffect(a.Effect)})
	}
	httpx.JSON(w, 200, out)
}

func (m *Module) setTitle(ctx context.Context, id int64, text string) error {
	chars := []rune(text)
	if len(chars) > 60 {
		chars = chars[:60]
	}
	_, err := m.d.DB.ExecContext(ctx, "UPDATE ai_conversations SET title=CASE WHEN title='' THEN ? ELSE title END,updated_at=? WHERE id=?", string(chars), time.Now().UTC(), id)
	return err
}
func (m *Module) saveMessage(ctx context.Context, id int64, role string, blocks any) error {
	raw, err := json.Marshal(blocks)
	if err != nil {
		return err
	}
	_, err = m.d.DB.ExecContext(ctx, "INSERT INTO ai_messages(conversation_id,seq,role,content,created_at) VALUES(?,(SELECT COALESCE(max(seq),0)+1 FROM ai_messages WHERE conversation_id=?),?,?,?)", id, id, role, string(raw), time.Now().UTC())
	if err == nil {
		m.d.Bus.Publish("ai.message_saved", map[string]any{"conversationId": id})
	}
	return err
}
func (m *Module) messages(ctx context.Context, id int64) ([]api.Message, error) {
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id,seq,role,content,created_at FROM ai_messages WHERE conversation_id=? ORDER BY seq", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []api.Message{}
	for rows.Next() {
		var msg api.Message
		var raw string
		if err = rows.Scan(&msg.Id, &msg.Seq, &msg.Role, &raw, &msg.CreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &msg.Content); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}
func (m *Module) pendingActions(ctx context.Context, id int64) ([]api.PendingAction, error) {
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id,tool_use_id,action,input,status,result,effect FROM ai_pending_actions WHERE conversation_id=? ORDER BY id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []api.PendingAction{}
	for rows.Next() {
		p := api.PendingAction{ConversationId: id}
		var input string
		var result sql.NullString
		var effect sql.NullString
		if err = rows.Scan(&p.Id, &p.ToolUseId, &p.Action, &input, &p.Status, &result, &effect); err != nil {
			return nil, err
		}
		if effect.Valid {
			value := api.PendingActionEffect(effect.String)
			p.Effect = &value
		}
		if err = json.Unmarshal([]byte(input), &p.Input); err != nil {
			return nil, err
		}
		if result.Valid {
			_ = json.Unmarshal([]byte(result.String), &p.Result)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
