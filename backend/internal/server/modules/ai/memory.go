package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// B61: short notes about the user that the panel AI keeps across
// conversations. Agents and machine conversations read them; only the panel
// AI and the settings page change them; remote AI (MCP) never sees them.

const (
	memoryEnabledKey = "ai.memory_enabled"
	memoryLimitChars = 4000
	memoryMaxChars   = 500
)

var _ contracts.Memories = (*Module)(nil)

// memoryRules tell the panel AI when to write memory. Machine conversations
// and agents only read it, so they do not get these.
const memoryRules = `## 记忆
用户明确要求“记住……”，或者说了以后长期适用的偏好、习惯、设备情况时，用 memory__save 存一条简短的中文。
不存一次性的事，不存密码、令牌、验证码，不存隐藏笔记和隐藏文件里的内容。
存之前先看下面已有的记忆，有相近的就用 memory__update 改那一条。用户让你忘掉时用 memory__delete。`

func (m *Module) memoryEnabled(ctx context.Context) bool {
	var on bool
	err := m.d.Settings.Get(ctx, memoryEnabledKey, &on)
	if errors.Is(err, settings.ErrNotSet) {
		return true
	}
	return err == nil && on
}

func (m *Module) listMemories(ctx context.Context) ([]api.AiMemory, error) {
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id,text,source,created_at,updated_at FROM ai_memories ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []api.AiMemory{}
	for rows.Next() {
		var x api.AiMemory
		var source string
		if err := rows.Scan(&x.Id, &x.Text, &source, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return nil, err
		}
		x.Source = api.AiMemorySource(source)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (m *Module) memories(ctx context.Context) (api.AiMemories, error) {
	items, err := m.listMemories(ctx)
	if err != nil {
		return api.AiMemories{}, err
	}
	used := 0
	for _, x := range items {
		used += utf8.RuneCountInString(x.Text)
	}
	return api.AiMemories{Enabled: m.memoryEnabled(ctx), Items: items, UsedChars: used, LimitChars: memoryLimitChars}, nil
}

func cleanMemory(text string) (string, error) {
	text = strings.TrimSpace(text)
	if n := utf8.RuneCountInString(text); n == 0 || n > memoryMaxChars {
		return "", httpx.Invalid("每条记忆 1 到 500 字")
	}
	return text, nil
}

// fits checks the total length once text replaces the memory skip (0 for a
// new one). It runs inside the write transaction.
func memoryFits(ctx context.Context, tx *sql.Tx, skip int64, text string) error {
	var used sql.NullInt64
	if err := tx.QueryRowContext(ctx, "SELECT sum(length(text)) FROM ai_memories WHERE id<>?", skip).Scan(&used); err != nil {
		return err
	}
	if int(used.Int64)+utf8.RuneCountInString(text) > memoryLimitChars {
		return httpx.Invalid("记忆总共最多 4000 字，先删掉一些")
	}
	return nil
}

func (m *Module) getMemory(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64) (api.AiMemory, error) {
	var x api.AiMemory
	var source string
	err := q.QueryRowContext(ctx, "SELECT id,text,source,created_at,updated_at FROM ai_memories WHERE id=?", id).
		Scan(&x.Id, &x.Text, &source, &x.CreatedAt, &x.UpdatedAt)
	x.Source = api.AiMemorySource(source)
	return x, notFound(err)
}

func (m *Module) addMemory(ctx context.Context, text, source string) (out api.AiMemory, err error) {
	defer func() {
		m.d.Audit.Record(ctx, "ai.memory.create", strconv.FormatInt(out.Id, 10), map[string]any{"source": source}, err)
	}()
	if text, err = cleanMemory(text); err != nil {
		return out, err
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = memoryFits(ctx, tx, 0, text); err != nil {
		return out, err
	}
	now := m.now().UTC()
	var id int64
	if err = tx.QueryRowContext(ctx, "INSERT INTO ai_memories(text,source,created_at,updated_at) VALUES(?,?,?,?) RETURNING id",
		text, source, now, now).Scan(&id); err != nil {
		return out, err
	}
	if out, err = m.getMemory(ctx, tx, id); err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	m.d.Bus.Publish("ai.memory_changed", map[string]any{"id": id})
	return out, nil
}

func (m *Module) updateMemory(ctx context.Context, id int64, text string) (out api.AiMemory, err error) {
	defer func() { m.d.Audit.Record(ctx, "ai.memory.update", strconv.FormatInt(id, 10), nil, err) }()
	if text, err = cleanMemory(text); err != nil {
		return out, err
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err = m.getMemory(ctx, tx, id); err != nil {
		return out, err
	}
	if err = memoryFits(ctx, tx, id, text); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE ai_memories SET text=?,updated_at=? WHERE id=?", text, m.now().UTC(), id); err != nil {
		return out, err
	}
	if out, err = m.getMemory(ctx, tx, id); err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	m.d.Bus.Publish("ai.memory_changed", map[string]any{"id": id})
	return out, nil
}

func (m *Module) deleteMemory(ctx context.Context, id int64) (err error) {
	defer func() { m.d.Audit.Record(ctx, "ai.memory.delete", strconv.FormatInt(id, 10), nil, err) }()
	res, err := m.d.DB.ExecContext(ctx, "DELETE FROM ai_memories WHERE id=?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return httpx.ErrNotFound
	}
	m.d.Bus.Publish("ai.memory_changed", map[string]any{"id": id})
	return nil
}

// Prompt implements contracts.Memories.
func (m *Module) Prompt(ctx context.Context) string {
	if !m.memoryEnabled(ctx) {
		return ""
	}
	items, err := m.listMemories(ctx)
	if err != nil || len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## 关于用户的记忆（只读参考，可能过时，和用户这次说的冲突时以用户为准）\n")
	for _, x := range items {
		b.WriteString("- [")
		b.WriteString(strconv.FormatInt(x.Id, 10))
		b.WriteString("] ")
		b.WriteString(strings.ReplaceAll(x.Text, "\n", " "))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// ---- HTTP ----

func (m *Module) ListAiMemories(w http.ResponseWriter, r *http.Request) {
	out, err := m.memories(r.Context())
	if m.fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateAiMemory(w http.ResponseWriter, r *http.Request) {
	var body api.AiMemoryInput
	if m.fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	out, err := m.addMemory(r.Context(), body.Text, "user")
	if m.fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) UpdateAiMemory(w http.ResponseWriter, r *http.Request, id int64) {
	var body api.AiMemoryInput
	if m.fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	out, err := m.updateMemory(r.Context(), id, body.Text)
	if m.fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) DeleteAiMemory(w http.ResponseWriter, r *http.Request, id int64) {
	if m.fail(w, r, m.deleteMemory(r.Context(), id)) {
		return
	}
	httpx.NoContent(w)
}

func (m *Module) SetAiMemoryEnabled(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if m.fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	err := m.d.Settings.Set(r.Context(), memoryEnabledKey, body.Enabled)
	m.d.Audit.Record(r.Context(), "ai.memory.enabled", "", map[string]any{"enabled": body.Enabled}, err)
	if m.fail(w, r, err) {
		return
	}
	m.d.Bus.Publish("ai.memory_changed", map[string]any{"enabled": body.Enabled})
	out, err := m.memories(r.Context())
	if m.fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// ---- actions for the panel AI ----

func (m *Module) registerMemoryActions() {
	decode := func(raw json.RawMessage, v any) error {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(v); err != nil {
			return httpx.Invalid("参数不对：" + err.Error())
		}
		return nil
	}
	m.d.Actions.Register(actions.Action{
		Name: "memory.save", Title: "记住", PanelOnly: true, Effect: actions.Write,
		Description: "Save one short memory about the user that later conversations should know. Only when the user asks to remember something or states a lasting preference.",
		Input:       actions.Schema(`{"type":"object","properties":{"text":{"type":"string","maxLength":500}},"required":["text"],"additionalProperties":false}`),
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Text string `json:"text"`
			}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return m.addMemory(ctx, in.Text, "ai")
		},
	})
	m.d.Actions.Register(actions.Action{
		Name: "memory.update", Title: "修改记忆", PanelOnly: true, Effect: actions.Write,
		Description: "Replace the text of one memory, by the id shown in brackets in the memory list.",
		Input:       actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"},"text":{"type":"string","maxLength":500}},"required":["id","text"],"additionalProperties":false}`),
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				ID   int64  `json:"id"`
				Text string `json:"text"`
			}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return m.updateMemory(ctx, in.ID, in.Text)
		},
	})
	m.d.Actions.Register(actions.Action{
		Name: "memory.delete", Title: "删除记忆", PanelOnly: true, Effect: actions.Write, Destructive: true,
		Description: "Delete one memory by its id.",
		Input:       actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"],"additionalProperties":false}`),
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				ID int64 `json:"id"`
			}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			if err := m.deleteMemory(ctx, in.ID); err != nil {
				return nil, err
			}
			return map[string]any{"deleted": in.ID}, nil
		},
	})
}
