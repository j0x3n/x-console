package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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

func (m *Module) permission(ctx context.Context, id int64) (api.HostAgentPermission, error) {
	if err := m.expireHostPermission(ctx, id); err != nil {
		return api.Confirm, err
	}
	row, err := m.q.GetConversation(ctx, id)
	return m.effectiveHostPermission(row), err
}

func (m *Module) effectiveHostPermission(row db.AiConversation) api.HostAgentPermission {
	mode := api.HostAgentPermission(row.HostPermission)
	if !mode.Valid() || mode == api.AllAuto && (row.HostPermissionUntil == nil || !row.HostPermissionUntil.After(m.now())) {
		return api.Confirm
	}
	return mode
}

func (m *Module) expireHostPermission(ctx context.Context, id int64) error {
	_, err := m.d.DB.ExecContext(ctx, `UPDATE ai_conversations SET host_permission='confirm',host_permission_until=NULL WHERE id=? AND host_permission='all_auto' AND (host_permission_until IS NULL OR host_permission_until<=?)`, id, m.now().UTC())
	return err
}

func (m *Module) permissionUntil(mode api.HostAgentPermission) *time.Time {
	if mode != api.AllAuto {
		return nil
	}
	until := m.now().UTC().Add(2 * time.Hour)
	return &until
}

func (m *Module) setHostPermission(ctx context.Context, id int64, mode api.HostAgentPermission) error {
	_, err := m.d.DB.ExecContext(ctx, `UPDATE ai_conversations SET host_permission=?,host_permission_until=? WHERE id=?`, mode, m.permissionUntil(mode), id)
	return err
}

// 只有尚未过期的全部自动模式能续期。
func (m *Module) touchPermission(ctx context.Context, id int64) error {
	now := m.now().UTC()
	_, err := m.d.DB.ExecContext(ctx, `UPDATE ai_conversations SET
 host_permission=CASE WHEN host_permission_until>? THEN 'all_auto' ELSE 'confirm' END,
 host_permission_until=CASE WHEN host_permission_until>? THEN ? ELSE NULL END
 WHERE id=? AND host_permission='all_auto'`, now, now, now.Add(2*time.Hour), id)
	return err
}

func (m *Module) conversation(row db.AiConversation) api.Conversation {
	c := conversation(row)
	if row.HostID != nil {
		mode := m.effectiveHostPermission(row)
		c.Permission = &mode
	} else {
		m.panelFields(&c, row)
	}
	return c
}

// hostSystem 把记忆放在固定规则和机器 id 末尾。
func (m *Module) hostSystem(ctx context.Context, hostID string) string {
	base := m.system(ctx) + "\n你只能通过给你的机器工具操作当前这台机器。先看再改。"
	base += "\n当前机器 ID：" + hostID + "。机器信息附在每条用户消息后。若机器离线，说明操作无法执行。"
	if mem := m.Prompt(ctx); mem != "" {
		base += "\n\n" + mem
	}
	return base
}

// hostState 的结果随用户消息存下，不再改变后续请求的 system。
func (m *Module) hostState(ctx context.Context, hostID string) string {
	base := ""
	a, err := m.d.Agents.Get(ctx, hostID)
	if err == nil {
		capabilities := append([]string(nil), a.Capabilities...)
		sort.Strings(capabilities)
		base += fmt.Sprintf("\n当前机器：名称 %s，主机名 %s，系统 %s，架构 %s，能力 %v。机器在线：%t。", a.Name, a.Hostname, a.Os, a.Arch, capabilities, a.Online)
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
	var cpu float64
	var memUsed, memTotal uint64
	var diskJSON string
	if m.d.DB.QueryRowContext(ctx, "SELECT cpu,mem_used,mem_total,disk_json FROM host_metrics_1m WHERE host_id=? ORDER BY at DESC LIMIT 1", hostID).Scan(&cpu, &memUsed, &memTotal, &diskJSON) == nil {
		var disks []protocol.DiskUsage
		_ = json.Unmarshal([]byte(diskJSON), &disks)
		base += fmt.Sprintf("最近指标：CPU %.1f%%，内存 %d/%d 字节，磁盘 %v。", cpu, memUsed, memTotal, disks)
	}
	return base
}

func (m *Module) ListHostAgentConversations(w http.ResponseWriter, r *http.Request, hostID string) {
	ctx := r.Context()
	if m.fail(w, r, m.hostExists(ctx, hostID)) {
		return
	}
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id,title,created_at,updated_at,host_id,host_permission,host_permission_until FROM ai_conversations WHERE host_id=? ORDER BY updated_at DESC,id DESC LIMIT 50", hostID)
	if m.fail(w, r, err) {
		return
	}
	defer rows.Close()
	out := []api.Conversation{}
	for rows.Next() {
		var row db.AiConversation
		if err = rows.Scan(&row.ID, &row.Title, &row.CreatedAt, &row.UpdatedAt, &row.HostID, &row.HostPermission, &row.HostPermissionUntil); err != nil {
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
	mode := api.Confirm
	if body.Mode != nil {
		mode = *body.Mode
	}
	if !mode.Valid() {
		httpx.Fail(w, r, httpx.Invalid("权限模式无效"))
		return
	}
	if mode == api.AllAuto && m.fail(w, r, auth.RequireElevated(ctx)) {
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
	err := m.d.DB.QueryRowContext(ctx, "INSERT INTO ai_conversations(title,created_at,updated_at,host_id,host_permission,host_permission_until) VALUES(?,?,?,?,?,?) RETURNING id,title,created_at,updated_at,host_id,host_permission,host_permission_until", title, now, now, hostID, mode, m.permissionUntil(mode)).Scan(&row.ID, &row.Title, &row.CreatedAt, &row.UpdatedAt, &row.HostID, &row.HostPermission, &row.HostPermissionUntil)
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
	if m.fail(w, r, m.setHostPermission(ctx, id, body.Mode)) {
		return
	}
	m.d.Bus.Publish("ai.permission_changed", map[string]any{"conversationId": id, "mode": body.Mode})
	m.d.Audit.Record(ctx, "ai.host_agent.permission", *row.HostID, map[string]any{"conversationId": id, "mode": body.Mode}, nil)
	httpx.JSON(w, 200, map[string]any{"mode": body.Mode})
}
