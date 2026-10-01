package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func (m *Module) hostExists(ctx context.Context, hostID string) error {
	if strings.HasPrefix(hostID, "ssh:") {
		id, err := strconv.ParseInt(strings.TrimPrefix(hostID, "ssh:"), 10, 64)
		if err != nil || id <= 0 {
			return httpx.ErrNotFound
		}
		var exists int
		err = m.d.DB.QueryRowContext(ctx, "SELECT 1 FROM ssh_hosts WHERE id=?", id).Scan(&exists)
		return notFound(err)
	}
	_, err := m.d.Agents.Get(ctx, hostID)
	return err
}

func (m *Module) permission(id int64) api.HostAgentPermission {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.permissions[id]
	if !ok {
		return api.Confirm
	}
	if m.now().Sub(p.lastMessage) >= 2*time.Hour {
		delete(m.permissions, id)
		return api.Confirm
	}
	return p.mode
}

func (m *Module) touchPermission(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.permissions[id]; ok {
		if m.now().Sub(p.lastMessage) >= 2*time.Hour {
			delete(m.permissions, id)
		} else {
			p.lastMessage = m.now()
			m.permissions[id] = p
		}
	}
}

func (m *Module) conversation(row db.AiConversation) api.Conversation {
	c := conversation(row)
	if row.HostID != nil {
		mode := m.permission(row.ID)
		c.Permission = &mode
	}
	return c
}

// hostSystem is the fixed system prompt of a machine conversation: what the
// machine is. Whether it is online and its current load go into hostState.
func (m *Module) hostSystem(ctx context.Context, hostID string) string {
	base := m.system(ctx) + "\n你只能通过给你的机器工具操作当前这台机器。先看再改；改文件前说明原因；每一步都填写 reason。"
	a, err := m.d.Agents.Get(ctx, hostID)
	if err == nil {
		base += fmt.Sprintf("\n当前机器：名称 %s，主机名 %s，系统 %s，架构 %s，能力 %v。", a.Name, a.Hostname, a.Os, a.Arch, a.Capabilities)
		if a.Online && a.Has(protocol.CapSystemInfo) {
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			var info protocol.SystemInfo
			if m.d.Agents.Call(callCtx, a.ID, protocol.MethodSystemInfo, nil, &info) == nil {
				base += fmt.Sprintf(" CPU %d 核，内存总量 %d 字节。", info.CPUCores, info.MemoryTotal)
			}
			cancel()
		}
	} else {
		base += "\n当前机器 ID：" + hostID + "。若机器离线，说明操作无法执行。"
	}
	// B61: the machine agent reads the memory but has no tool to change it.
	if mem := m.Prompt(ctx); mem != "" {
		base += "\n\n" + mem
	}
	return base
}

// hostState is the part of the machine that changes: online and recent load.
func (m *Module) hostState(ctx context.Context, hostID string) string {
	text := ""
	if a, err := m.d.Agents.Get(ctx, hostID); err == nil {
		text = fmt.Sprintf("机器在线：%t。", a.Online)
	}
	var cpu float64
	var memUsed, memTotal uint64
	var diskJSON string
	if m.d.DB.QueryRowContext(ctx, "SELECT cpu,mem_used,mem_total,disk_json FROM host_metrics_1m WHERE host_id=? ORDER BY at DESC LIMIT 1", hostID).Scan(&cpu, &memUsed, &memTotal, &diskJSON) == nil {
		var disks []protocol.DiskUsage
		_ = json.Unmarshal([]byte(diskJSON), &disks)
		text += fmt.Sprintf("最近指标：CPU %.1f%%，内存 %d/%d 字节，磁盘 %v。", cpu, memUsed, memTotal, disks)
	}
	return text
}

func (m *Module) ListHostAgentConversations(w http.ResponseWriter, r *http.Request, hostID string) {
	ctx := r.Context()
	if m.fail(w, r, m.hostExists(ctx, hostID)) {
		return
	}
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id,title,created_at,updated_at,host_id FROM ai_conversations WHERE host_id=? ORDER BY updated_at DESC,id DESC LIMIT 50", hostID)
	if m.fail(w, r, err) {
		return
	}
	defer rows.Close()
	out := []api.Conversation{}
	for rows.Next() {
		var row db.AiConversation
		if err = rows.Scan(&row.ID, &row.Title, &row.CreatedAt, &row.UpdatedAt, &row.HostID); err != nil {
			break
		}
		out = append(out, m.conversation(row))
	}
	if err == nil {
		err = rows.Err()
	}
	if m.fail(w, r, err) {
		return
	}
	httpx.JSON(w, 200, out)
}

func (m *Module) CreateHostAgentConversation(w http.ResponseWriter, r *http.Request, hostID string) {
	ctx := r.Context()
	if m.fail(w, r, m.hostExists(ctx, hostID)) {
		return
	}
	var body api.CreateHostAgentConversationJSONRequestBody
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
	now := m.now().UTC()
	var row db.AiConversation
	err := m.d.DB.QueryRowContext(ctx, "INSERT INTO ai_conversations(title,created_at,updated_at,host_id) VALUES(?,?,?,?) RETURNING id,title,created_at,updated_at,host_id", title, now, now, hostID).Scan(&row.ID, &row.Title, &row.CreatedAt, &row.UpdatedAt, &row.HostID)
	if m.fail(w, r, err) {
		return
	}
	m.d.Audit.Record(ctx, "ai.conversation.create", strconv.FormatInt(row.ID, 10), map[string]any{"hostId": hostID}, nil)
	httpx.JSON(w, 201, m.conversation(row))
}

func (m *Module) SetAiConversationPermission(w http.ResponseWriter, r *http.Request, id api.ConversationId) {
	ctx := r.Context()
	var body api.SetAiConversationPermissionJSONRequestBody
	if m.fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if !body.Mode.Valid() {
		httpx.Fail(w, r, httpx.Invalid("权限模式无效"))
		return
	}
	row, err := m.q.GetConversation(ctx, id)
	if m.fail(w, r, notFound(err)) {
		return
	}
	if row.HostID == nil {
		httpx.Fail(w, r, httpx.NewError(400, "not_host_conversation", "这不是机器 Agent 会话"))
		return
	}
	if body.Mode == api.AllAuto && m.fail(w, r, auth.RequireElevated(ctx)) {
		return
	}
	m.mu.Lock()
	if body.Mode == api.Confirm {
		delete(m.permissions, id)
	} else {
		m.permissions[id] = hostPermission{mode: body.Mode, lastMessage: m.now()}
	}
	m.mu.Unlock()
	m.d.Bus.Publish("ai.permission_changed", map[string]any{"conversationId": id, "mode": body.Mode})
	m.d.Audit.Record(ctx, "ai.host_agent.permission", *row.HostID, map[string]any{"conversationId": id, "mode": body.Mode}, nil)
	httpx.JSON(w, 200, map[string]any{"mode": body.Mode})
}
