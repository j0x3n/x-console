// Package assistant provides persisted Claude conversations and confirmed tools.
package assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/assistant/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/automations"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const (
	defaultModel = "claude-opus-5"
	modelKey     = "ai.model"
	apiKeyKey    = "ai.api_key"
)

type Module struct {
	d       *module.Deps
	ctx     context.Context
	mu      sync.Mutex
	running map[string]bool
	baseURL string // tests replace the Claude endpoint.
}

func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, ctx: context.Background(), running: make(map[string]bool)}
	module.Provide[brief.Polisher](d.Registry, brief.PolisherKey, m)
	module.Provide[automations.Asker](d.Registry, automations.AskerKey, m)
	return m, nil
}

func (m *Module) Name() string { return "assistant" }

func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

func (m *Module) Start(ctx context.Context) error { m.ctx = ctx; return nil }

func (m *Module) ListConversations(w http.ResponseWriter, r *http.Request) {
	rows, err := m.d.DB.QueryContext(r.Context(), `SELECT id,title,created_at,updated_at FROM ai_conversations ORDER BY updated_at DESC LIMIT 100`)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	out := []api.Conversation{}
	for rows.Next() {
		var v api.Conversation
		if err := rows.Scan(&v.Id, &v.Title, &v.CreatedAt, &v.UpdatedAt); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateConversation(w http.ResponseWriter, r *http.Request) {
	var body api.CreateConversation
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	title := "新对话"
	if body.Title != nil && strings.TrimSpace(*body.Title) != "" {
		title = strings.TrimSpace(*body.Title)
	}
	if len([]rune(title)) > 100 {
		httpx.Fail(w, r, httpx.Invalid("标题太长"))
		return
	}
	now := time.Now().UTC()
	v := api.Conversation{Id: uuid.NewString(), Title: title, CreatedAt: now, UpdatedAt: now}
	_, err := m.d.DB.ExecContext(r.Context(), `INSERT INTO ai_conversations(id,title,created_at,updated_at) VALUES(?,?,?,?)`, v.Id, v.Title, now, now)
	m.d.Audit.Record(r.Context(), "ai.conversation.create", v.Id, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, v)
}

func (m *Module) GetConversation(w http.ResponseWriter, r *http.Request, id string) {
	v, err := m.detail(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) detail(ctx context.Context, id string) (api.ConversationDetail, error) {
	v := api.ConversationDetail{Messages: []api.AIMessage{}, PendingActions: []api.PendingAction{}}
	err := m.d.DB.QueryRowContext(ctx, `SELECT id,title,created_at,updated_at FROM ai_conversations WHERE id=?`, id).Scan(&v.Conversation.Id, &v.Conversation.Title, &v.Conversation.CreatedAt, &v.Conversation.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return v, httpx.ErrNotFound
	}
	if err != nil {
		return v, err
	}
	rows, err := m.d.DB.QueryContext(ctx, `SELECT id,seq,role,content,created_at FROM ai_messages WHERE conversation_id=? ORDER BY seq`, id)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		var msg api.AIMessage
		var raw string
		if err := rows.Scan(&msg.Id, &msg.Seq, &msg.Role, &raw, &msg.CreatedAt); err != nil {
			rows.Close()
			return v, err
		}
		msg.ConversationId = id
		if err := json.Unmarshal([]byte(raw), &msg.Content); err != nil {
			rows.Close()
			return v, err
		}
		v.Messages = append(v.Messages, msg)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return v, err
	}
	rows, err = m.d.DB.QueryContext(ctx, `SELECT id,tool_use_id,action,input,status,result FROM ai_pending_actions WHERE conversation_id=? ORDER BY created_at`, id)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var p api.PendingAction
		var input string
		var result sql.NullString
		if err := rows.Scan(&p.Id, &p.ToolUseId, &p.Action, &input, &p.Status, &result); err != nil {
			return v, err
		}
		p.ConversationId = id
		if err := json.Unmarshal([]byte(input), &p.Input); err != nil {
			return v, err
		}
		if result.Valid {
			var data map[string]any
			if err := json.Unmarshal([]byte(result.String), &data); err != nil {
				return v, err
			}
			p.Result = &data
		}
		v.PendingActions = append(v.PendingActions, p)
	}
	return v, rows.Err()
}

func (m *Module) DeleteConversation(w http.ResponseWriter, r *http.Request, id string) {
	m.mu.Lock()
	if m.running[id] {
		m.mu.Unlock()
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	m.mu.Unlock()
	result, err := m.d.DB.ExecContext(r.Context(), `DELETE FROM ai_conversations WHERE id=?`, id)
	m.d.Audit.Record(r.Context(), "ai.conversation.delete", id, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) GetAISettings(w http.ResponseWriter, r *http.Request) {
	v, err := m.settings(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) settings(ctx context.Context) (api.AISettings, error) {
	v := api.AISettings{Model: defaultModel}
	if err := m.d.Settings.Get(ctx, modelKey, &v.Model); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return v, err
	}
	var err error
	v.Configured, err = m.d.Settings.Has(ctx, apiKeyKey)
	return v, err
}

func (m *Module) PutAISettings(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.UpdateAISettings
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	model := strings.TrimSpace(body.Model)
	if model == "" || len(model) > 120 {
		httpx.Fail(w, r, httpx.Invalid("模型名称无效"))
		return
	}
	if body.ApiKey != nil && *body.ApiKey != "" {
		if err := m.d.Settings.SetSecret(r.Context(), apiKeyKey, strings.TrimSpace(*body.ApiKey)); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if err := m.d.Settings.Set(r.Context(), modelKey, model); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(r.Context(), "ai.settings.update", modelKey, map[string]any{"keyChanged": body.ApiKey != nil && *body.ApiKey != ""}, nil)
	v, err := m.settings(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

var _ api.ServerInterface = (*Module)(nil)
var _ module.Starter = (*Module)(nil)
