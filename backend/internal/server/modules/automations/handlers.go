package automations

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/automations/api"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
)

type rule struct {
	api.Automation
	tokenHash, tokenEnc string
}

func (m *Module) readRule(ctx context.Context, id int64) (rule, error) {
	var r rule
	var trigger, conditions, steps string
	var enabled, authorized int
	err := m.d.DB.QueryRowContext(ctx, `SELECT id,name,enabled,trigger,conditions,actions,cooldown_seconds,authorized,
       COALESCE(webhook_token_hash,''),COALESCE(webhook_token_enc,''),created_at,updated_at
       FROM automations WHERE id=?`, id).Scan(&r.Id, &r.Name, &enabled, &trigger, &conditions, &steps, &r.CooldownSeconds, &authorized, &r.tokenHash, &r.tokenEnc, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return r, notFound(err)
	}
	r.Enabled = enabled != 0
	r.Authorized = authorized != 0
	if err = json.Unmarshal([]byte(trigger), &r.Trigger); err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(conditions), &r.Conditions); err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(steps), &r.Actions); err != nil {
		return r, err
	}
	if r.tokenEnc != "" {
		token, e := m.d.Secrets.Open(r.tokenEnc)
		if e != nil {
			return r, e
		}
		path := "/hooks/" + token
		r.Trigger.WebhookPath = &path
	}
	var run api.Run
	var data, runSteps string
	run.AutomationId = id
	err = m.d.DB.QueryRowContext(ctx, "SELECT id,started_at,finished_at,trigger_data,steps,status FROM automation_runs WHERE automation_id=? ORDER BY id DESC LIMIT 1", id).Scan(&run.Id, &run.StartedAt, &run.FinishedAt, &data, &runSteps, &run.Status)
	if err == nil {
		_ = json.Unmarshal([]byte(data), &run.TriggerData)
		_ = json.Unmarshal([]byte(runSteps), &run.Steps)
		r.LastRun = &run
	} else if err != sql.ErrNoRows {
		return r, err
	}
	return r, nil
}
func (m *Module) listRules(ctx context.Context) ([]api.Automation, error) {
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id FROM automations ORDER BY updated_at DESC,id DESC")
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := make([]api.Automation, 0, len(ids))
	for _, id := range ids {
		r, e := m.readRule(ctx, id)
		if e != nil {
			return nil, e
		}
		out = append(out, r.Automation)
	}
	return out, nil
}
func (m *Module) ListAutomations(w http.ResponseWriter, r *http.Request) {
	out, err := m.listRules(r.Context())
	if m.fail(w, r, err) {
		return
	}
	httpx.JSON(w, 200, out)
}
func (m *Module) GetAutomation(w http.ResponseWriter, r *http.Request, id api.AutomationId) {
	row, err := m.readRule(r.Context(), id)
	if m.fail(w, r, err) {
		return
	}
	httpx.JSON(w, 200, row.Automation)
}

func (m *Module) validate(ctx context.Context, in api.AutomationInput) (bool, error) {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 120 || len(in.Actions) == 0 || len(in.Actions) > 20 || in.CooldownSeconds < 0 {
		return false, httpx.Invalid("规则名称、动作或冷却时间不正确")
	}
	switch in.Trigger.Type {
	case api.Schedule:
		if in.Trigger.Cron == nil {
			return false, httpx.Invalid("缺少 cron")
		}
		if _, err := m.d.Scheduler.Next(*in.Trigger.Cron, time.Now()); err != nil {
			return false, httpx.Invalid("cron 不正确")
		}
	case api.Event:
		if in.Trigger.Topic == nil || *in.Trigger.Topic == "" {
			return false, httpx.Invalid("缺少事件主题")
		}
	case api.Metric:
		if in.Trigger.Metric == nil || in.Trigger.Op == nil || in.Trigger.Value == nil || !in.Trigger.Metric.Valid() || !in.Trigger.Op.Valid() {
			return false, httpx.Invalid("指标条件不正确")
		}
	case api.HaState:
		if in.Trigger.EntityId == nil || *in.Trigger.EntityId == "" {
			return false, httpx.Invalid("缺少实体 ID")
		}
	case api.Webhook:
	default:
		return false, httpx.Invalid("触发类型不正确")
	}
	for _, c := range in.Conditions {
		if c.Field == "" || !c.Op.Valid() {
			return false, httpx.Invalid("条件不正确")
		}
	}
	dangerous := false
	for _, s := range in.Actions {
		if s.Action == "ai.ask" {
			continue
		}
		a, ok := m.d.Actions.Get(s.Action)
		if !ok {
			return false, httpx.Invalid("未知动作: " + s.Action)
		}
		if a.Effect == actions.Dangerous {
			dangerous = true
		}
	}
	if dangerous {
		if err := auth.RequireElevated(ctx); err != nil {
			return false, err
		}
	}
	return dangerous, nil
}
func (m *Module) save(w http.ResponseWriter, r *http.Request, id int64, create bool) {
	ctx := r.Context()
	var in api.AutomationInput
	if m.fail(w, r, httpx.Decode(r, &in)) {
		return
	}
	authorized, err := m.validate(ctx, in)
	if m.fail(w, r, err) {
		return
	}
	var hash, enc *string
	if in.Trigger.Type == api.Webhook {
		token := secrets.RandomToken(32)
		digest := secrets.Hash(token)
		hash = &digest
		ciphertext, sealErr := m.d.Secrets.Seal(token)
		enc, err = &ciphertext, sealErr
		if m.fail(w, r, err) {
			return
		}
	}
	in.Trigger.WebhookPath = nil
	trigger, _ := json.Marshal(in.Trigger)
	conditions, _ := json.Marshal(in.Conditions)
	steps, _ := json.Marshal(in.Actions)
	now := time.Now().UTC()
	enabled, authz := 0, 0
	if in.Enabled {
		enabled = 1
	}
	if authorized {
		authz = 1
	}
	if create {
		err = m.d.DB.QueryRowContext(ctx, `INSERT INTO automations(name,enabled,trigger,conditions,actions,cooldown_seconds,authorized,webhook_token_hash,webhook_token_enc,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?) RETURNING id`, strings.TrimSpace(in.Name), enabled, string(trigger), string(conditions), string(steps), in.CooldownSeconds, authz, hash, enc, now, now).Scan(&id)
	} else {
		var result sql.Result
		result, err = m.d.DB.ExecContext(ctx, `UPDATE automations SET name=?,enabled=?,trigger=?,conditions=?,actions=?,cooldown_seconds=?,authorized=?,webhook_token_hash=?,webhook_token_enc=?,updated_at=? WHERE id=?`, strings.TrimSpace(in.Name), enabled, string(trigger), string(conditions), string(steps), in.CooldownSeconds, authz, hash, enc, now, id)
		if err == nil {
			n, _ := result.RowsAffected()
			if n == 0 {
				err = httpx.ErrNotFound
			}
		}
	}
	if m.fail(w, r, err) {
		return
	}
	if err = m.reload(ctx); m.fail(w, r, err) {
		return
	}
	row, err := m.readRule(ctx, id)
	if m.fail(w, r, err) {
		return
	}
	m.d.Audit.Record(ctx, "automation.save", strconv.FormatInt(id, 10), nil, nil)
	m.d.Bus.Publish("automation.updated", map[string]any{"automationId": id})
	status := 200
	if create {
		status = 201
	}
	httpx.JSON(w, status, row.Automation)
}
func (m *Module) CreateAutomation(w http.ResponseWriter, r *http.Request) { m.save(w, r, 0, true) }
func (m *Module) UpdateAutomation(w http.ResponseWriter, r *http.Request, id api.AutomationId) {
	m.save(w, r, id, false)
}
func (m *Module) ToggleAutomation(w http.ResponseWriter, r *http.Request, id api.AutomationId) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if m.fail(w, r, httpx.Decode(r, &in)) {
		return
	}
	result, err := m.d.DB.ExecContext(r.Context(), "UPDATE automations SET enabled=?,updated_at=? WHERE id=?", in.Enabled, time.Now().UTC(), id)
	if m.fail(w, r, err) {
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err = m.reload(r.Context()); m.fail(w, r, err) {
		return
	}
	row, err := m.readRule(r.Context(), id)
	if m.fail(w, r, err) {
		return
	}
	m.d.Audit.Record(r.Context(), "automation.toggle", strconv.FormatInt(id, 10), map[string]any{"enabled": in.Enabled}, nil)
	m.d.Bus.Publish("automation.updated", map[string]any{"automationId": id})
	httpx.JSON(w, 200, row.Automation)
}
func (m *Module) DeleteAutomation(w http.ResponseWriter, r *http.Request, id api.AutomationId) {
	res, err := m.d.DB.ExecContext(r.Context(), "DELETE FROM automations WHERE id=?", id)
	if m.fail(w, r, err) {
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err = m.reload(r.Context()); m.fail(w, r, err) {
		return
	}
	m.d.Audit.Record(r.Context(), "automation.delete", strconv.FormatInt(id, 10), nil, nil)
	m.d.Bus.Publish("automation.updated", map[string]any{"automationId": id})
	httpx.NoContent(w)
}
func (m *Module) GetAutomationCatalog(w http.ResponseWriter, r *http.Request) {
	out := api.Catalog{Actions: []api.CatalogAction{}, Topics: []struct {
		Title string `json:"title"`
		Topic string `json:"topic"`
	}{}}
	for _, a := range m.d.Actions.List() {
		var schema map[string]any
		_ = json.Unmarshal(a.Input, &schema)
		desc := a.Description
		out.Actions = append(out.Actions, api.CatalogAction{Name: a.Name, Title: a.Title, Description: &desc, Effect: api.CatalogActionEffect(a.Effect), Input: schema})
	}
	out.Actions = append(out.Actions, api.CatalogAction{Name: "ai.ask", Title: "询问 AI", Effect: api.Write, Input: map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string"}}, "required": []string{"prompt"}}})
	for _, item := range [][2]string{{"host.metrics", "主机指标"}, {"ha.state_changed", "智能家居状态"}, {"monitor.down", "监控异常"}, {"issue.updated", "Issue 更新"}, {"reminder.due", "提醒到期"}} {
		out.Topics = append(out.Topics, struct {
			Title string `json:"title"`
			Topic string `json:"topic"`
		}{item[1], item[0]})
	}
	httpx.JSON(w, 200, out)
}
func (m *Module) ListAutomationRuns(w http.ResponseWriter, r *http.Request, id api.AutomationId, params api.ListAutomationRunsParams) {
	if _, err := m.readRule(r.Context(), id); m.fail(w, r, err) {
		return
	}
	limit := 50
	if params.Limit != nil && *params.Limit > 0 && *params.Limit <= 100 {
		limit = *params.Limit
	}
	rows, err := m.d.DB.QueryContext(r.Context(), "SELECT id,started_at,finished_at,trigger_data,steps,status FROM automation_runs WHERE automation_id=? ORDER BY id DESC LIMIT ?", id, limit)
	if m.fail(w, r, err) {
		return
	}
	defer rows.Close()
	out := []api.Run{}
	for rows.Next() {
		var row api.Run
		var data, steps string
		row.AutomationId = id
		if err = rows.Scan(&row.Id, &row.StartedAt, &row.FinishedAt, &data, &steps, &row.Status); err != nil {
			break
		}
		_ = json.Unmarshal([]byte(data), &row.TriggerData)
		_ = json.Unmarshal([]byte(steps), &row.Steps)
		out = append(out, row)
	}
	if m.fail(w, r, err) {
		return
	}
	if m.fail(w, r, rows.Err()) {
		return
	}
	httpx.JSON(w, 200, out)
}
func (m *Module) RunAutomation(w http.ResponseWriter, r *http.Request, id api.AutomationId) {
	row, err := m.readRule(r.Context(), id)
	if m.fail(w, r, err) {
		return
	}
	runID, err := m.startRun(r.Context(), row, map[string]any{}, runManual)
	if m.fail(w, r, err) {
		return
	}
	httpx.JSON(w, 202, map[string]any{"runId": runID})
}
