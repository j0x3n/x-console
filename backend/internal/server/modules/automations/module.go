// Package automations runs saved rules in response to events and schedules.
package automations

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/automations/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/scheduler"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Asker is supplied by the AI assistant without making this module depend on it.
type Asker interface {
	Ask(context.Context, string) (string, error)
}

const AskerKey = "ai.asker"

type Module struct {
	d    *module.Deps
	mu   sync.Mutex
	jobs map[string]scheduler.EntryID
	ctx  context.Context
}

func New(d *module.Deps) (module.Module, error) {
	return &Module{d: d, jobs: map[string]scheduler.EntryID{}, ctx: context.Background()}, nil
}
func (m *Module) Name() string { return "automations" }
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
	r.Post("/hooks/{token}", m.webhook)
}
func (m *Module) PublicPaths() []string { return []string{"/hooks"} }

func (m *Module) Start(ctx context.Context) error {
	m.ctx = ctx
	rules, err := m.list(ctx)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if err := m.register(rule); err != nil {
			return err
		}
	}
	ch, cancel := m.d.Bus.Subscribe("", 256)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				if strings.HasPrefix(ev.Topic, "automation.") || strings.HasPrefix(ev.Topic, "ai.") {
					continue
				}
				m.handleEvent(ctx, ev)
			}
		}
	}()
	return nil
}

func (m *Module) ListAutomations(w http.ResponseWriter, r *http.Request) {
	v, err := m.list(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}
func (m *Module) list(ctx context.Context) ([]api.Automation, error) {
	rows, err := m.d.DB.QueryContext(ctx, `SELECT id,name,enabled,trigger,conditions,actions,cooldown_seconds,dangerous_authorized,created_at,updated_at FROM automations ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	out := []api.Automation{}
	for rows.Next() {
		v, err := scanRule(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range out {
		if err := m.withWebhook(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

type scanner interface{ Scan(dest ...any) error }

func scanRule(row scanner) (api.Automation, error) {
	var v api.Automation
	var enabled, dangerous int
	var trigger, conditions, steps string
	err := row.Scan(&v.Id, &v.Name, &enabled, &trigger, &conditions, &steps, &v.CooldownSeconds, &dangerous, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return v, err
	}
	v.Enabled, v.DangerousAuthorized = enabled != 0, dangerous != 0
	if err := json.Unmarshal([]byte(trigger), &v.Trigger); err != nil {
		return v, err
	}
	if err := json.Unmarshal([]byte(conditions), &v.Conditions); err != nil {
		return v, err
	}
	if err := json.Unmarshal([]byte(steps), &v.Actions); err != nil {
		return v, err
	}
	return v, nil
}
func (m *Module) rule(ctx context.Context, id string) (api.Automation, error) {
	v, err := scanRule(m.d.DB.QueryRowContext(ctx, `SELECT id,name,enabled,trigger,conditions,actions,cooldown_seconds,dangerous_authorized,created_at,updated_at FROM automations WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return v, httpx.ErrNotFound
	}
	if err != nil {
		return v, err
	}
	return v, m.withWebhook(ctx, &v)
}

func (m *Module) withWebhook(ctx context.Context, v *api.Automation) error {
	if v.Trigger.Type != "webhook" {
		return nil
	}
	var token string
	if err := m.d.Settings.Get(ctx, "automation.webhook."+v.Id, &token); err != nil {
		if errors.Is(err, settings.ErrNotSet) {
			return nil
		}
		return err
	}
	path := "/api/v1/hooks/" + token
	v.WebhookUrl = &path
	return nil
}
func (m *Module) GetAutomation(w http.ResponseWriter, r *http.Request, id string) {
	v, err := m.rule(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) validate(ctx context.Context, body api.AutomationInput) (bool, error) {
	if strings.TrimSpace(body.Name) == "" || len([]rune(body.Name)) > 100 {
		return false, httpx.Invalid("规则名称无效")
	}
	if body.CooldownSeconds < 0 || body.CooldownSeconds > 86400*365 {
		return false, httpx.Invalid("冷却时间无效")
	}
	if len(body.Actions) == 0 || len(body.Actions) > 20 {
		return false, httpx.Invalid("动作数量无效")
	}
	switch body.Trigger.Type {
	case "schedule":
		if body.Trigger.Cron == nil {
			return false, httpx.Invalid("缺少 cron 表达式")
		}
		if _, err := m.d.Scheduler.Next(*body.Trigger.Cron, time.Now()); err != nil {
			return false, httpx.Invalid("cron 表达式无效")
		}
	case "event", "metric":
		if body.Trigger.Type == "metric" {
			if body.Trigger.Topic == nil {
				topic := "host.metrics"
				body.Trigger.Topic = &topic
			}
		}
		if body.Trigger.Topic == nil || strings.TrimSpace(*body.Trigger.Topic) == "" {
			return false, httpx.Invalid("缺少事件主题")
		}
	case "ha_state":
		if body.Trigger.EntityId == nil || *body.Trigger.EntityId == "" || body.Trigger.State == nil {
			return false, httpx.Invalid("缺少 HA 实体或状态")
		}
	case "webhook":
	default:
		return false, httpx.Invalid("触发器类型无效")
	}
	for _, c := range body.Conditions {
		if c.Field == "" || !validOp(string(c.Op)) {
			return false, httpx.Invalid("条件无效")
		}
	}
	dangerous := false
	for _, step := range body.Actions {
		if step.Action == "ai.ask" {
			continue
		}
		a, ok := m.d.Actions.Get(step.Action)
		if !ok {
			return false, httpx.Invalid("动作不存在: " + step.Action)
		}
		if a.Effect == actions.Dangerous {
			dangerous = true
		}
	}
	return dangerous, nil
}

func (m *Module) CreateAutomation(w http.ResponseWriter, r *http.Request) {
	var body api.AutomationInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	dangerous, err := m.validate(r.Context(), body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if dangerous {
		if err := auth.RequireElevated(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	token, hash, err := webhookSecret(body.Trigger.Type == "webhook")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	trigger, _ := json.Marshal(body.Trigger)
	conditions, _ := json.Marshal(body.Conditions)
	steps, _ := json.Marshal(body.Actions)
	_, err = m.d.DB.ExecContext(r.Context(), `INSERT INTO automations(id,name,enabled,trigger,conditions,actions,cooldown_seconds,dangerous_authorized,webhook_token_hash,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, strings.TrimSpace(body.Name), body.Enabled, trigger, conditions, steps, body.CooldownSeconds, dangerous, hash, now, now)
	m.d.Audit.Record(r.Context(), "automation.create", id, map[string]any{"dangerous": dangerous}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if token != "" {
		if err := m.d.Settings.SetSecret(r.Context(), "automation.webhook."+id, token); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	v, err := m.rule(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if token != "" {
		path := "/api/v1/hooks/" + token
		v.WebhookUrl = &path
	}
	if err := m.register(v); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, v)
}

func (m *Module) UpdateAutomation(w http.ResponseWriter, r *http.Request, id string) {
	if _, err := m.rule(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.AutomationInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	dangerous, err := m.validate(r.Context(), body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if dangerous {
		if err := auth.RequireElevated(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	trigger, _ := json.Marshal(body.Trigger)
	conditions, _ := json.Marshal(body.Conditions)
	steps, _ := json.Marshal(body.Actions)
	var hash sql.NullString
	if err := m.d.DB.QueryRowContext(r.Context(), `SELECT webhook_token_hash FROM automations WHERE id=?`, id).Scan(&hash); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var token string
	if body.Trigger.Type == "webhook" && !hash.Valid {
		var value any
		token, value, err = webhookSecret(true)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		hash = sql.NullString{String: value.(string), Valid: true}
	}
	if body.Trigger.Type != "webhook" {
		hash = sql.NullString{}
	}
	_, err = m.d.DB.ExecContext(r.Context(), `UPDATE automations SET name=?,enabled=?,trigger=?,conditions=?,actions=?,cooldown_seconds=?,dangerous_authorized=?,webhook_token_hash=?,updated_at=? WHERE id=?`, strings.TrimSpace(body.Name), body.Enabled, trigger, conditions, steps, body.CooldownSeconds, dangerous, hash, time.Now().UTC(), id)
	m.d.Audit.Record(r.Context(), "automation.update", id, map[string]any{"dangerous": dangerous}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if token != "" {
		if err := m.d.Settings.SetSecret(r.Context(), "automation.webhook."+id, token); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	} else if body.Trigger.Type != "webhook" {
		if err := m.d.Settings.Delete(r.Context(), "automation.webhook."+id); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	v, err := m.rule(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := m.register(v); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if token != "" {
		path := "/api/v1/hooks/" + token
		v.WebhookUrl = &path
	}
	httpx.JSON(w, http.StatusOK, v)
}
func (m *Module) DeleteAutomation(w http.ResponseWriter, r *http.Request, id string) {
	result, err := m.d.DB.ExecContext(r.Context(), `DELETE FROM automations WHERE id=?`, id)
	m.d.Audit.Record(r.Context(), "automation.delete", id, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err := m.d.Settings.Delete(r.Context(), "automation.webhook."+id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.mu.Lock()
	if job, ok := m.jobs[id]; ok {
		m.d.Scheduler.Remove(job)
		delete(m.jobs, id)
	}
	m.mu.Unlock()
	httpx.NoContent(w)
}
func (m *Module) GetAutomationCatalog(w http.ResponseWriter, r *http.Request) {
	out := api.Catalog{
		Triggers: []string{"schedule", "event", "metric", "ha_state", "webhook"},
		EventTopics: []string{
			"host.metrics", "host.alert.fired", "host.alert.resolved", "ha.state_changed",
			"monitor.down", "monitor.up", "monitor.expiring", "reminder.due",
			"habit.checked_in", "habit.goal_reached", "focus.done", "brief.sent",
			"subscription.due", "issue.created", "issue.updated", "coding_task.updated",
		},
		Actions: []api.ActionCatalogItem{},
	}
	for _, a := range m.d.Actions.List() {
		var input map[string]any
		_ = json.Unmarshal(a.Input, &input)
		out.Actions = append(out.Actions, api.ActionCatalogItem{Name: a.Name, Title: a.Title, Description: a.Description, Effect: string(a.Effect), Input: input})
	}
	out.Actions = append(out.Actions, api.ActionCatalogItem{Name: "ai.ask", Title: "询问 AI", Description: "Ask Claude and pass the answer to later steps", Effect: "read", Input: map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string"}}, "required": []string{"prompt"}}})
	httpx.JSON(w, http.StatusOK, out)
}
func (m *Module) RunAutomation(w http.ResponseWriter, r *http.Request, id string) {
	v, err := m.rule(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if v.DangerousAuthorized {
		if err := auth.RequireElevated(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	go m.fire(context.WithoutCancel(r.Context()), v, map[string]any{"type": "manual"})
	httpx.JSON(w, http.StatusAccepted, map[string]any{"id": id})
}
func (m *Module) ListAutomationRuns(w http.ResponseWriter, r *http.Request, id string) {
	if _, err := m.rule(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err := m.d.DB.QueryContext(r.Context(), `SELECT id,started_at,finished_at,trigger_data,steps,status FROM automation_runs WHERE automation_id=? ORDER BY started_at DESC LIMIT 100`, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	out := []api.AutomationRun{}
	for rows.Next() {
		var v api.AutomationRun
		var finished sql.NullTime
		var trigger, steps string
		if err := rows.Scan(&v.Id, &v.StartedAt, &finished, &trigger, &steps, &v.Status); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		v.AutomationId = id
		if finished.Valid {
			v.FinishedAt = &finished.Time
		}
		if err := json.Unmarshal([]byte(trigger), &v.TriggerData); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if err := json.Unmarshal([]byte(steps), &v.Steps); err != nil {
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

func webhookSecret(enabled bool) (string, any, error) {
	if !enabled {
		return "", nil, nil
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(hash[:]), nil
}
func (m *Module) webhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	token := chi.URLParam(r, "token")
	if len(token) != 64 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	hash := sha256.Sum256([]byte(token))
	var id string
	err := m.d.DB.QueryRowContext(r.Context(), `SELECT id FROM automations WHERE webhook_token_hash=? AND enabled=1 AND json_extract(trigger,'$.type')='webhook'`, hex.EncodeToString(hash[:])).Scan(&id)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var data map[string]any
	if err := httpx.Decode(r, &data); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := m.rule(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	go m.fire(context.WithoutCancel(r.Context()), v, data)
	httpx.JSON(w, http.StatusAccepted, map[string]any{"accepted": true})
}

func (m *Module) register(v api.Automation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job, ok := m.jobs[v.Id]; ok {
		m.d.Scheduler.Remove(job)
		delete(m.jobs, v.Id)
	}
	if !v.Enabled {
		return nil
	}
	if v.Trigger.Type == "ha_state" && v.Trigger.EntityId != nil {
		if svc, ok := module.Lookup[contracts.HomeAssistant](m.d.Registry, contracts.HomeAssistantKey); ok {
			svc.WatchEntity(*v.Trigger.EntityId)
		}
	}
	if v.Trigger.Type == "schedule" && v.Trigger.Cron != nil {
		job, err := m.d.Scheduler.Cron("automation."+v.Id, *v.Trigger.Cron, func(ctx context.Context) error { m.fire(ctx, v, map[string]any{"type": "schedule"}); return nil })
		if err != nil {
			return err
		}
		m.jobs[v.Id] = job
	}
	return nil
}

func (m *Module) handleEvent(ctx context.Context, ev events.Event) {
	rules, err := m.list(ctx)
	if err != nil {
		m.d.Log.Error("load automation rules", "err", err)
		return
	}
	for _, v := range rules {
		if !v.Enabled {
			continue
		}
		if !matchesTrigger(v.Trigger, ev) {
			continue
		}
		data := map[string]any{"topic": ev.Topic, "data": ev.Data}
		go m.fire(ctx, v, data)
	}
}

func matchesTrigger(t api.Trigger, ev events.Event) bool {
	switch t.Type {
	case "event", "metric":
		topic := "host.metrics"
		if t.Topic != nil {
			topic = *t.Topic
		}
		if !strings.HasPrefix(ev.Topic, topic) {
			return false
		}
		if t.Field != nil && t.Op != nil {
			return compare(pathValue(map[string]any{"data": ev.Data}, *t.Field), *t.Op, t.Value)
		}
		return true
	case "ha_state":
		if ev.Topic != "ha.state_changed" || t.EntityId == nil || t.State == nil {
			return false
		}
		return pathValue(ev.Data, "entityId") == *t.EntityId && pathValue(ev.Data, "state") == *t.State
	default:
		return false
	}
}

func (m *Module) fire(ctx context.Context, v api.Automation, data map[string]any) {
	if !v.Enabled && data["type"] != "manual" {
		return
	}
	if !conditionsPass(v.Conditions, data) {
		return
	}
	// Claim the cooldown atomically. Concurrent events cannot start duplicate runs.
	if data["type"] != "manual" && v.CooldownSeconds > 0 {
		threshold := time.Now().UTC().Add(-time.Duration(v.CooldownSeconds) * time.Second)
		res, err := m.d.DB.ExecContext(ctx, `UPDATE automations SET last_run_at=? WHERE id=? AND (last_run_at IS NULL OR last_run_at<=?)`, time.Now().UTC(), v.Id, threshold)
		if err != nil {
			m.d.Log.Error("automation cooldown", "err", err)
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return
		}
	}
	runID := uuid.NewString()
	start := time.Now().UTC()
	trigger, _ := json.Marshal(data)
	_, err := m.d.DB.ExecContext(ctx, `INSERT INTO automation_runs(id,automation_id,started_at,trigger_data,steps,status) VALUES(?,?,?,?,?,'running')`, runID, v.Id, start, string(trigger), "[]")
	if err != nil {
		m.d.Log.Error("start automation run", "err", err)
		return
	}
	stepResults := []map[string]any{}
	status := "done"
	actionCtx := audit.WithActor(ctx, "automation:"+v.Id)
	var previous any
	for _, step := range v.Actions {
		input := expandInput(step.Input, map[string]any{"trigger": data, "previous": previous})
		raw, _ := json.Marshal(input)
		var result any
		var runErr error
		if step.Action == "ai.ask" {
			asker, ok := module.Lookup[Asker](m.d.Registry, AskerKey)
			if !ok {
				runErr = fmt.Errorf("AI 助手不可用")
			} else if prompt, ok := input["prompt"].(string); !ok || strings.TrimSpace(prompt) == "" {
				runErr = fmt.Errorf("缺少 AI 提示词")
			} else {
				triggerJSON, _ := json.Marshal(data)
				result, runErr = asker.Ask(actionCtx, prompt+"\n\n触发数据："+string(triggerJSON))
			}
		} else {
			a, ok := m.d.Actions.Get(step.Action)
			if !ok {
				runErr = fmt.Errorf("动作不存在: %s", step.Action)
			} else if a.Effect == actions.Dangerous && !v.DangerousAuthorized {
				runErr = fmt.Errorf("高危动作未授权")
			} else {
				result, runErr = m.d.Actions.Run(actionCtx, step.Action, raw)
			}
		}
		m.d.Audit.Record(actionCtx, "automation.step", step.Action, map[string]any{"runId": runID}, runErr)
		stepResults = append(stepResults, map[string]any{"action": step.Action, "input": input, "result": result, "error": errorText(runErr)})
		previous = result
		if runErr != nil {
			status = "failed"
			break
		}
	}
	finished := time.Now().UTC()
	steps, _ := json.Marshal(stepResults)
	if _, err := m.d.DB.ExecContext(ctx, `UPDATE automation_runs SET finished_at=?,steps=?,status=? WHERE id=?`, finished, string(steps), status, runID); err != nil {
		m.d.Log.Error("finish automation run", "err", err)
	}
	m.d.Bus.Publish("automation.run_finished", map[string]any{"automationId": v.Id, "runId": runID, "status": status})
	if status == "failed" {
		_, _ = m.d.Notify.Send(ctx, notify.Notification{Kind: "automation.failed", Title: "自动化运行失败", Body: v.Name, Link: "/automations", Source: "automations"})
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
func conditionsPass(conditions []api.Condition, data map[string]any) bool {
	for _, c := range conditions {
		if !compare(pathValue(data, c.Field), string(c.Op), c.Value) {
			return false
		}
	}
	return true
}
func validOp(op string) bool {
	switch op {
	case "eq", "ne", "gt", "gte", "lt", "lte", "contains":
		return true
	}
	return false
}
func compare(left any, op string, right any) bool {
	if op == "eq" {
		return fmt.Sprint(left) == fmt.Sprint(right)
	}
	if op == "ne" {
		return fmt.Sprint(left) != fmt.Sprint(right)
	}
	if op == "contains" {
		return strings.Contains(fmt.Sprint(left), fmt.Sprint(right))
	}
	l, lok := number(left)
	r, rok := number(right)
	if !lok || !rok {
		return false
	}
	switch op {
	case "gt":
		return l > r
	case "gte":
		return l >= r
	case "lt":
		return l < r
	case "lte":
		return l <= r
	}
	return false
}
func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}
func pathValue(data any, path string) any {
	var v any = data
	for _, part := range strings.Split(path, ".") {
		if m, ok := v.(map[string]any); ok {
			v = m[part]
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var object map[string]any
		if json.Unmarshal(raw, &object) != nil {
			return nil
		}
		v = object[part]
	}
	return v
}
func expandInput(input map[string]any, context map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range input {
		out[k] = expandValue(v, context)
	}
	return out
}
func expandValue(v any, context map[string]any) any {
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(x, "{{") && strings.HasSuffix(x, "}}") {
			inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(x, "{{"), "}}"))
			if !strings.Contains(inner, "{{") {
				return pathValue(context, inner)
			}
		}
		var result strings.Builder
		for {
			before, tail, found := strings.Cut(x, "{{")
			result.WriteString(before)
			if !found {
				return result.String()
			}
			key, rest, closed := strings.Cut(tail, "}}")
			if !closed {
				result.WriteString("{{" + tail)
				return result.String()
			}
			result.WriteString(fmt.Sprint(pathValue(context, strings.TrimSpace(key))))
			x = rest
		}
	case map[string]any:
		return expandInput(x, context)
	case []any:
		for i := range x {
			x[i] = expandValue(x[i], context)
		}
		return x
	default:
		return v
	}
}

var _ api.ServerInterface = (*Module)(nil)
var _ module.Starter = (*Module)(nil)
var _ module.PublicPather = (*Module)(nil)
