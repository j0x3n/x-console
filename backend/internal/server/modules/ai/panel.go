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
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/db"
)

// B60: the panel AI's permission level, model and effort per conversation,
// and machines only through an agent bound to them.

const (
	permManual = "manual"
	permWrite  = "write"
	permAll    = "all"

	defaultPermissionSetting = "ai.default_permission"
	// panelAllFor is how long "all" lasts without a new message.
	panelAllFor = 2 * time.Hour
)

// panelHostActions are the machine actions the panel AI no longer gets
// directly; it asks a bound agent with agents.operate_host instead.
var panelHostActions = map[string]bool{
	"hosts.exec": true, "hosts.service_action": true, "hosts.power": true,
	"hosts.clipboard_set": true, "hosts.open": true,
}

// panelPermission is the effective level of a panel conversation.
func (m *Module) panelPermission(row db.AiConversation) (string, *time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if last, ok := m.panelAll[row.ID]; ok {
		if m.now().Sub(last) < panelAllFor {
			until := last.Add(panelAllFor)
			return permAll, &until
		}
		delete(m.panelAll, row.ID)
	}
	if row.Permission == permWrite {
		return permWrite, nil
	}
	return permManual, nil
}

// touchPanel keeps "all" alive while the conversation is in use.
func (m *Module) touchPanel(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if last, ok := m.panelAll[id]; ok {
		if m.now().Sub(last) >= panelAllFor {
			delete(m.panelAll, id)
		} else {
			m.panelAll[id] = m.now()
		}
	}
}

// panelFields fills the B60 fields of a panel conversation.
func (m *Module) panelFields(c *api.Conversation, row db.AiConversation) {
	perm, until := m.panelPermission(row)
	p := api.AiPermission(perm)
	c.PanelPermission = &p
	c.PanelPermissionUntil = until
	model, effort := row.Model, row.Effort
	c.Model, c.Effort = &model, &effort
}

// panelConfirm says whether a panel action waits for the user.
func panelConfirm(perm string, a actions.Action, name string) bool {
	deletes := a.Destructive || strings.HasSuffix(name, ".delete")
	switch perm {
	case permAll:
		return false
	case permWrite:
		return a.Effect == actions.Dangerous || deletes
	default:
		return a.Effect != actions.Read
	}
}

type effortOverrideKey struct{}

// withEffort makes resolveLLM use this effort for the agent model.
func withEffort(ctx context.Context, effort api.ReasoningEffort) context.Context {
	return context.WithValue(ctx, effortOverrideKey{}, effort)
}

func effortOverride(ctx context.Context) (api.ReasoningEffort, bool) {
	e, ok := ctx.Value(effortOverrideKey{}).(api.ReasoningEffort)
	return e, ok
}

func validEffort(e string) bool {
	switch api.ReasoningEffort(e) {
	case api.Off, api.Low, api.Medium, api.High:
		return true
	}
	return false
}

func parseModel(s string) (api.ModelRef, bool) {
	provider, model, ok := strings.Cut(s, ":")
	id, err := strconv.ParseInt(provider, 10, 64)
	if !ok || err != nil || id <= 0 || model == "" {
		return api.ModelRef{}, false
	}
	return api.ModelRef{ProviderId: id, Model: model}, true
}

// convContext applies the conversation's own model and effort.
func convContext(ctx context.Context, row db.AiConversation) context.Context {
	if ref, ok := parseModel(row.Model); ok {
		ctx = withModel(ctx, ref)
	}
	if validEffort(row.Effort) {
		ctx = withEffort(ctx, api.ReasoningEffort(row.Effort))
	}
	return ctx
}

// checkPanelSettings validates a model and an effort chosen for a conversation.
func (m *Module) checkPanelSettings(ctx context.Context, model, effort *string) error {
	if model != nil && *model != "" {
		ref, ok := parseModel(*model)
		if !ok {
			return httpx.Invalid("模型格式不对，应该是 <供应商 id>:<模型 id>")
		}
		if _, err := m.provider(ctx, ref.ProviderId); err != nil {
			if errors.Is(err, httpx.ErrNotFound) {
				return httpx.Invalid("这个模型的供应商不存在")
			}
			return err
		}
	}
	if effort != nil && *effort != "" && !validEffort(*effort) {
		return httpx.Invalid("思考程度只能是 off、low、medium、high")
	}
	return nil
}

// setPanelPermission stores manual and write, and keeps all in memory.
func (m *Module) setPanelPermission(ctx context.Context, id int64, perm string) error {
	switch perm {
	case permAll:
		if err := auth.RequireElevated(ctx); err != nil {
			return err
		}
		m.mu.Lock()
		m.panelAll[id] = m.now()
		m.mu.Unlock()
		return nil
	case permManual, permWrite:
		m.mu.Lock()
		delete(m.panelAll, id)
		m.mu.Unlock()
		_, err := m.d.DB.ExecContext(ctx, "UPDATE ai_conversations SET permission=? WHERE id=?", perm, id)
		return err
	}
	return httpx.Invalid("权限只能是 manual、write 或 all")
}

// SetAiConversationSettings is PATCH /ai/conversations/{id}/settings.
func (m *Module) SetAiConversationSettings(w http.ResponseWriter, r *http.Request, id api.ConversationId) {
	ctx := r.Context()
	var body api.AiConversationSettings
	if m.fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	row, err := m.q.GetConversation(ctx, id)
	if m.fail(w, r, notFound(err)) {
		return
	}
	if row.HostID != nil {
		httpx.Fail(w, r, httpx.NewError(400, "host_conversation", "机器会话的权限在机器的 Agent 标签里改"))
		return
	}
	if m.fail(w, r, m.checkPanelSettings(ctx, body.Model, body.Effort)) {
		return
	}
	if body.Permission != nil {
		if m.fail(w, r, m.setPanelPermission(ctx, id, string(*body.Permission))) {
			return
		}
	}
	if body.Model != nil {
		if _, err := m.d.DB.ExecContext(ctx, "UPDATE ai_conversations SET model=? WHERE id=?", *body.Model, id); m.fail(w, r, err) {
			return
		}
	}
	if body.Effort != nil {
		if _, err := m.d.DB.ExecContext(ctx, "UPDATE ai_conversations SET effort=? WHERE id=?", *body.Effort, id); m.fail(w, r, err) {
			return
		}
	}
	row, err = m.q.GetConversation(ctx, id)
	if m.fail(w, r, err) {
		return
	}
	out := m.conversation(row)
	m.d.Audit.Record(ctx, "ai.conversation.settings", strconv.FormatInt(id, 10), map[string]any{
		"permission": out.PanelPermission, "model": row.Model, "effort": row.Effort}, nil)
	m.d.Bus.Publish("ai.permission_changed", map[string]any{"conversationId": id})
	httpx.JSON(w, http.StatusOK, out)
}

// ---- agents.operate_host ----

type panelCtxKey struct{}

type panelInfo struct {
	conversationID int64
	permission     string
}

// withPanel tells actions which panel conversation runs them.
func withPanel(ctx context.Context, id int64, perm string) context.Context {
	return context.WithValue(ctx, panelCtxKey{}, panelInfo{conversationID: id, permission: perm})
}

func panelOf(ctx context.Context) (panelInfo, bool) {
	p, ok := ctx.Value(panelCtxKey{}).(panelInfo)
	return p, ok
}

// hostModeFor maps the panel level to the machine conversation's mode.
func hostModeFor(perm string) api.HostAgentPermission {
	switch perm {
	case permAll:
		return api.AllAuto
	case permWrite:
		return api.ReadAuto
	}
	return api.Confirm
}

const noBoundAgent = "这台机器没有绑定 Agent，不能操作。可以在 Agent 页面给一个 Agent 勾选这台机器"

func (m *Module) registerOperateHost() {
	m.d.Actions.Register(actions.Action{
		Name: "agents.operate_host", Title: "让 Agent 操作机器", PanelOnly: true, Effect: actions.Dangerous,
		Description: "Ask the agent bound to a machine to do something on it: run commands, open apps, manage services. " +
			"host is the machine id or name, request says what to do in plain words. Returns the agent's reply and the commands it ran. " +
			"Fails when no agent is bound to the machine.",
		Input: actions.Schema(`{"type":"object","properties":{"host":{"type":"string"},"request":{"type":"string","maxLength":4000}},"required":["host","request"],"additionalProperties":false}`),
		Run:   m.operateHost,
	})
}

func (m *Module) findHost(ctx context.Context, ref string) (contracts.HostSummary, error) {
	hosts, ok := module.Lookup[contracts.Hosts](m.d.Registry, contracts.HostsKey)
	if !ok {
		return contracts.HostSummary{}, errors.New("服务器模块没有启用")
	}
	list, err := hosts.Summaries(ctx)
	if err != nil {
		return contracts.HostSummary{}, err
	}
	ref = strings.TrimSpace(ref)
	for _, h := range list {
		if h.ID == ref {
			return h, nil
		}
	}
	for _, h := range list {
		if strings.EqualFold(h.Name, ref) {
			return h, nil
		}
	}
	return contracts.HostSummary{}, fmt.Errorf("没有这台机器：%s", ref)
}

func hostLink(h contracts.HostSummary, conversationID int64) string {
	c := strconv.FormatInt(conversationID, 10)
	if h.Kind == "desktop" {
		return "/pc/agent?c=" + c
	}
	return "/servers/" + h.ID + "/agent?c=" + c
}

func (m *Module) operateHost(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Host    string `json:"host"`
		Request string `json:"request"`
	}
	if err := json.Unmarshal(raw, &in); err != nil || strings.TrimSpace(in.Request) == "" {
		return nil, httpx.Invalid("要说明在哪台机器上做什么")
	}
	host, err := m.findHost(ctx, in.Host)
	if err != nil {
		return nil, err
	}
	agents, ok := module.Lookup[contracts.AIAgents](m.d.Registry, contracts.AIAgentsKey)
	if !ok {
		return nil, errors.New(noBoundAgent)
	}
	bound, err := agents.ForHost(ctx, host.ID)
	if err != nil {
		return nil, err
	}
	if len(bound) == 0 {
		return nil, errors.New(noBoundAgent)
	}
	panel, _ := panelOf(ctx)
	agent := bound[0]
	m.mu.Lock()
	if last, ok := m.lastAgent[panel.conversationID]; ok && panel.conversationID != 0 {
		for _, a := range bound {
			if a.ID == last {
				agent = a
			}
		}
	}
	if panel.conversationID != 0 {
		m.lastAgent[panel.conversationID] = agent.ID
	}
	m.mu.Unlock()

	// A machine conversation of B33, run by this agent.
	now := m.now().UTC()
	title := agent.Name + "：" + truncateRunes(strings.TrimSpace(in.Request), 60)
	var row db.AiConversation
	err = m.d.DB.QueryRowContext(ctx, "INSERT INTO ai_conversations(title,created_at,updated_at,host_id) VALUES(?,?,?,?) RETURNING id,title,created_at,updated_at,host_id",
		title, now, now, host.ID).Scan(&row.ID, &row.Title, &row.CreatedAt, &row.UpdatedAt, &row.HostID)
	if err != nil {
		return nil, err
	}
	mode := hostModeFor(panel.permission)
	if mode != api.Confirm {
		m.mu.Lock()
		m.permissions[row.ID] = hostPermission{mode: mode, lastMessage: m.now()}
		m.mu.Unlock()
	}
	text := in.Request
	if instructions := strings.TrimSpace(agent.Instructions); instructions != "" {
		text = fmt.Sprintf("（你是 Agent“%s”，固定说明：%s）\n\n%s", agent.Name, instructions, in.Request)
	}
	blocks := []map[string]any{{"type": "text", "text": text}, {"type": "text", "text": m.turnContext(ctx, &host.ID), "context": true}}
	if err := m.saveMessage(ctx, row.ID, "user", blocks); err != nil {
		return nil, err
	}
	m.d.Audit.Record(ctx, "ai.operate_host", host.ID, map[string]any{"agentId": agent.ID, "conversationId": row.ID}, nil)

	work := context.WithoutCancel(ctx)
	if agent.Kind == "builtin" {
		if ref, ok := parseModel(agent.Model); ok {
			work = withModel(work, ref)
		}
	}
	worker, cancel := context.WithCancel(work)
	m.mu.Lock()
	m.running[row.ID] = &generation{cancel: cancel}
	m.mu.Unlock()
	genErr := m.generate(worker, worker, row.ID)
	cancel()
	m.mu.Lock()
	delete(m.running, row.ID)
	m.mu.Unlock()
	m.d.Bus.Publish("ai.message_done", map[string]any{"conversationId": row.ID})

	out := map[string]any{"agent": agent.Name, "agentId": agent.ID, "host": host.Name, "hostId": host.ID,
		"conversationId": row.ID, "link": hostLink(host, row.ID)}
	if len(bound) > 1 {
		out["note"] = fmt.Sprintf("这台机器绑定了 %d 个 Agent，用的是“%s”", len(bound), agent.Name)
	}
	reply, commands, waiting, err := m.hostOutcome(work, row.ID)
	if err != nil {
		return nil, err
	}
	out["reply"], out["commands"] = reply, commands
	if waiting > 0 {
		out["waiting"] = waiting
		out["reply"] = strings.TrimSpace(reply + "\n（有 " + strconv.Itoa(waiting) + " 个操作在等你在机器会话里确认）")
	}
	if genErr != nil {
		out["error"] = genErr.Error()
	}
	return out, nil
}

// hostOutcome reads what a machine conversation ended with: the last reply,
// the commands that ran, and how many wait for confirmation.
func (m *Module) hostOutcome(ctx context.Context, id int64) (string, []string, int, error) {
	var content string
	err := m.d.DB.QueryRowContext(ctx, "SELECT content FROM ai_messages WHERE conversation_id=? AND role='assistant' ORDER BY seq DESC LIMIT 1", id).Scan(&content)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", nil, 0, err
	}
	var blocks []map[string]any
	_ = json.Unmarshal([]byte(content), &blocks)
	var reply []string
	for _, b := range blocks {
		if b["type"] == "text" {
			if t, ok := b["text"].(string); ok && strings.TrimSpace(t) != "" {
				reply = append(reply, strings.TrimSpace(t))
			}
		}
	}
	rows, err := m.d.DB.QueryContext(ctx, "SELECT input,status FROM ai_pending_actions WHERE conversation_id=? ORDER BY id", id)
	if err != nil {
		return "", nil, 0, err
	}
	defer rows.Close()
	commands := []string{}
	waiting := 0
	for rows.Next() {
		var input, status string
		if err := rows.Scan(&input, &status); err != nil {
			return "", nil, 0, err
		}
		if status == "pending" {
			waiting++
			continue
		}
		var args map[string]any
		_ = json.Unmarshal([]byte(input), &args)
		if c, ok := args["command"].(string); ok && status == "done" {
			commands = append(commands, c)
		}
	}
	return strings.Join(reply, "\n"), commands, waiting, rows.Err()
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
