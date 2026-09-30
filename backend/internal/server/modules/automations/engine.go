package automations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/automations/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/scheduler"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
)

func (m *Module) reload(ctx context.Context) error {
	rules, err := m.listRules(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, job := range m.jobs {
		m.d.Scheduler.Remove(job)
	}
	m.jobs = map[int64]scheduler.EntryID{}
	m.eventRules = m.eventRules[:0:0]
	for _, r := range rules {
		if _, seen := m.last[r.Id]; !seen {
			var last *time.Time
			if e := m.d.DB.QueryRowContext(ctx, "SELECT max(started_at) FROM automation_runs WHERE automation_id=?", r.Id).Scan(&last); e == nil && last != nil {
				m.last[r.Id] = *last
			}
		}
		if !r.Enabled {
			continue
		}
		switch r.Trigger.Type {
		case api.Event, api.Metric, api.HaState:
			m.eventRules = append(m.eventRules, r)
		}
		if r.Trigger.Type == api.Schedule {
			id := r.Id
			entry, e := m.d.Scheduler.Cron("automation."+strconv.FormatInt(id, 10), *r.Trigger.Cron, func(ctx context.Context) error {
				row, e := m.readRule(ctx, id)
				if e != nil || !row.Enabled {
					return e
				}
				_, e = m.startRun(ctx, row, map[string]any{"type": "schedule"}, runOther)
				return e
			})
			if e != nil {
				return e
			}
			m.jobs[id] = entry
		}
		if r.Trigger.Type == api.HaState {
			if ha, ok := module.Lookup[contracts.HomeAssistant](m.d.Registry, contracts.HomeAssistantKey); ok {
				ha.WatchEntity(*r.Trigger.EntityId)
			}
		}
	}
	return nil
}
func anyMap(value any) map[string]any {
	raw, _ := json.Marshal(value)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
func field(value any, path string) any {
	for _, part := range strings.Split(path, ".") {
		switch v := value.(type) {
		case map[string]any:
			value = v[part]
		case []any:
			n, e := strconv.Atoi(part)
			if e != nil || n < 0 || n >= len(v) {
				return nil
			}
			value = v[n]
		default:
			return nil
		}
	}
	return value
}
func compare(left any, op, want string) bool {
	if left == nil {
		return false
	}
	str := fmt.Sprint(left)
	if op == "contains" {
		return strings.Contains(str, want)
	}
	if a, e := strconv.ParseFloat(str, 64); e == nil {
		if b, e := strconv.ParseFloat(want, 64); e == nil {
			switch op {
			case "==":
				return a == b
			case "!=":
				return a != b
			case ">":
				return a > b
			case ">=":
				return a >= b
			case "<":
				return a < b
			case "<=":
				return a <= b
			}
		}
	}
	switch op {
	case "==":
		return str == want
	case "!=":
		return str != want
	case ">":
		return str > want
	case ">=":
		return str >= want
	case "<":
		return str < want
	case "<=":
		return str <= want
	}
	return false
}
func (m *Module) matches(r api.Automation, ev events.Event) bool {
	data := anyMap(ev.Data)
	t := r.Trigger
	switch t.Type {
	case api.Event:
		if !strings.HasPrefix(ev.Topic, *t.Topic) {
			return false
		}
		if t.Match != nil && *t.Match != "" {
			parts := strings.SplitN(*t.Match, "==", 2)
			if len(parts) != 2 {
				return false
			}
			if !compare(field(map[string]any{"data": data}, strings.TrimSpace(parts[0])), "==", strings.Trim(strings.TrimSpace(parts[1]), `"'`)) {
				return false
			}
		}
	case api.Metric:
		if ev.Topic != "host.metrics" {
			return false
		}
		if t.HostId != nil && *t.HostId != "" && data["hostId"] != *t.HostId {
			return false
		}
		sample, _ := data["sample"].(map[string]any)
		var value float64
		switch *t.Metric {
		case api.Cpu:
			value, _ = sample["cpu"].(float64)
		case api.Mem:
			used, _ := sample["memUsed"].(float64)
			total, _ := sample["memTotal"].(float64)
			if total > 0 {
				value = 100 * used / total
			}
		case api.Disk:
			disks, _ := sample["disks"].([]any)
			for _, disk := range disks {
				d, _ := disk.(map[string]any)
				used, _ := d["used"].(float64)
				total, _ := d["total"].(float64)
				if total > 0 && 100*used/total > value {
					value = 100 * used / total
				}
			}
		}
		if !compare(value, string(*t.Op), fmt.Sprint(*t.Value)) {
			return false
		}
	case api.HaState:
		if ev.Topic != "ha.state_changed" || data["entityId"] != *t.EntityId {
			return false
		}
		if t.To != nil && *t.To != "" && data["state"] != *t.To {
			return false
		}
	default:
		return false
	}
	for _, c := range r.Conditions {
		if !compare(field(map[string]any{"topic": ev.Topic, "data": data}, c.Field), string(c.Op), c.Value) {
			return false
		}
	}
	return true
}
func (m *Module) handleEvent(ctx context.Context, ev events.Event) {
	if ctx.Err() != nil {
		return
	}
	if strings.HasPrefix(ev.Topic, "automation.") || strings.HasPrefix(ev.Topic, "ai.") {
		return
	}
	m.mu.Lock()
	rules := m.eventRules
	m.mu.Unlock()
	for _, candidate := range rules {
		if !m.matches(candidate, ev) {
			if candidate.Trigger.Type == api.Metric && ev.Topic == "host.metrics" {
				m.setMetricHigh(candidate.Id, ev, false)
			}
			continue
		}
		// A metric rule fires when the value crosses the threshold, not on
		// every sample while it stays above it.
		if candidate.Trigger.Type == api.Metric && m.setMetricHigh(candidate.Id, ev, true) {
			continue
		}
		r, e := m.readRule(ctx, candidate.Id)
		if e != nil {
			m.d.Log.Error("automation read failed", "err", e)
			continue
		}
		if !r.Enabled {
			continue
		}
		_, e = m.startRun(ctx, r, map[string]any{"topic": ev.Topic, "data": anyMap(ev.Data)}, runEvent)
		if e != nil {
			m.d.Log.Error("automation start failed", "err", e)
		}
	}
}

// setMetricHigh records whether a metric rule's condition holds for the
// sample's host and returns what it was before.
func (m *Module) setMetricHigh(ruleID int64, ev events.Event, high bool) bool {
	host, _ := anyMap(ev.Data)["hostId"].(string)
	key := strconv.FormatInt(ruleID, 10) + ":" + host
	m.mu.Lock()
	defer m.mu.Unlock()
	was := m.metricHigh[key]
	if high {
		m.metricHigh[key] = true
	} else {
		delete(m.metricHigh, key)
	}
	return was
}

type runKind int

const (
	runOther  runKind = iota // schedule and webhook
	runEvent                 // a bus event
	runManual                // the run button; skips the cooldown
)

// loopLimits stop a rule whose own actions keep triggering it, for example a
// rule on notification.* whose action sends a notification. A fast loop runs
// many times a second; a slow one (with ai.ask) every few seconds. Metrics
// arrive at most once a second, so a real rule stays under both.
var loopLimits = []struct {
	window time.Duration
	runs   int
}{{10 * time.Second, 20}, {time.Hour, 200}}

// looping records an event-started run and reports whether the rule went over
// a loop limit. Caller holds m.mu.
func (m *Module) looping(id int64, now time.Time) bool {
	longest := loopLimits[len(loopLimits)-1].window
	kept := m.eventRuns[id][:0]
	for _, at := range m.eventRuns[id] {
		if now.Sub(at) < longest {
			kept = append(kept, at)
		}
	}
	kept = append(kept, now)
	m.eventRuns[id] = kept
	for _, limit := range loopLimits {
		n := 0
		for _, at := range kept {
			if now.Sub(at) < limit.window {
				n++
			}
		}
		if n > limit.runs {
			delete(m.eventRuns, id)
			return true
		}
	}
	return false
}

// stopLoop disables a looping rule and tells the user.
func (m *Module) stopLoop(ctx context.Context, r rule) {
	id := strconv.FormatInt(r.Id, 10)
	_, err := m.d.DB.ExecContext(ctx, "UPDATE automations SET enabled=0,updated_at=? WHERE id=?", time.Now().UTC(), r.Id)
	if err == nil {
		err = m.reload(ctx)
	}
	m.d.Audit.Record(audit.WithActor(ctx, "automation:"+id), "automation.loop_stopped", id, nil, err)
	if err != nil {
		m.d.Log.Error("automation loop stop failed", "automation", r.Id, "err", err)
		return
	}
	m.d.Bus.Publish("automation.updated", map[string]any{"automationId": r.Id})
	_, _ = m.d.Notify.Send(ctx, notify.Notification{Kind: "automation.loop", Title: "自动化触发太频繁，已停用",
		Body: r.Name + " 被事件触发的次数太多，可能是它的动作又触发了它自己。检查规则后再打开。", Link: "/automations",
		Priority: notify.PriorityHigh, Source: "automation:" + id})
}
func (m *Module) startRun(ctx context.Context, r rule, data map[string]any, kind runKind) (int64, error) {
	now := time.Now()
	m.mu.Lock()
	if kind != runManual && r.CooldownSeconds > 0 && now.Sub(m.last[r.Id]) < time.Duration(r.CooldownSeconds)*time.Second {
		m.mu.Unlock()
		return 0, nil
	}
	if kind == runEvent && m.looping(r.Id, now) {
		m.mu.Unlock()
		m.stopLoop(ctx, r)
		return 0, nil
	}
	m.last[r.Id] = now
	m.mu.Unlock()
	raw, _ := json.Marshal(data)
	started := now.UTC()
	var id int64
	err := m.d.DB.QueryRowContext(ctx, "INSERT INTO automation_runs(automation_id,started_at,trigger_data,steps,status) VALUES(?,?,?,'[]','running') RETURNING id", r.Id, started, string(raw)).Scan(&id)
	if err != nil {
		return 0, err
	}
	work := m.ctx
	if work == nil {
		work = context.Background()
	}
	go m.execute(work, id, r, data)
	return id, nil
}

var templateRE = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

func interpolate(value any, scope map[string]any) any {
	switch v := value.(type) {
	case string:
		if match := templateRE.FindStringSubmatch(v); match != nil && match[0] == v {
			return field(scope, strings.TrimSpace(match[1]))
		}
		return templateRE.ReplaceAllStringFunc(v, func(match string) string {
			key := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}"))
			x := field(scope, key)
			if x == nil {
				return ""
			}
			return fmt.Sprint(x)
		})
	case map[string]any:
		out := map[string]any{}
		for k, x := range v {
			out[k] = interpolate(x, scope)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = interpolate(x, scope)
		}
		return out
	default:
		return value
	}
}
func (m *Module) execute(ctx context.Context, id int64, r rule, data map[string]any) {
	ctx = audit.WithActor(auth.WithoutVault(ctx), "automation:"+strconv.FormatInt(r.Id, 10))
	steps := []api.RunStep{}
	scope := map[string]any{"trigger": data, "steps": []any{}}
	status := api.Ok
	for _, s := range r.Actions {
		input, _ := interpolate(s.Input, scope).(map[string]any)
		if input == nil {
			input = map[string]any{}
		}
		step := api.RunStep{Action: s.Action, Input: input}
		var result any
		var err error
		if s.Action == "ai.ask" {
			result, err = m.ask(ctx, input, data)
		} else {
			a, ok := m.d.Actions.Get(s.Action)
			if !ok {
				err = fmt.Errorf("未知动作: %s", s.Action)
			} else if string(a.Effect) == "dangerous" && !r.Authorized {
				err = fmt.Errorf("高危动作尚未授权")
			} else {
				raw, _ := json.Marshal(input)
				result, err = a.Run(ctx, raw)
			}
		}
		m.d.Audit.Record(ctx, "automation.step", strconv.FormatInt(r.Id, 10), map[string]any{"action": s.Action, "runId": id}, err)
		if err != nil {
			message := err.Error()
			step.Error = &message
			status = api.Failed
		} else {
			step.Result = result
		}
		steps = append(steps, step)
		scope["steps"] = append(scope["steps"].([]any), map[string]any{"result": result, "error": step.Error})
		raw, _ := json.Marshal(steps)
		_, _ = m.d.DB.ExecContext(ctx, "UPDATE automation_runs SET steps=? WHERE id=?", string(raw), id)
		if err != nil {
			break
		}
	}
	finished := time.Now().UTC()
	_, err := m.d.DB.ExecContext(ctx, "UPDATE automation_runs SET status=?,finished_at=? WHERE id=?", status, finished, id)
	if err != nil {
		m.d.Log.Error("automation finish failed", "err", err)
	}
	m.d.Bus.Publish("automation.run", map[string]any{"automationId": r.Id, "runId": id, "status": status})
	if status == api.Failed {
		_, _ = m.d.Notify.Send(ctx, notify.Notification{Kind: "automation.failed", Title: "自动化运行失败", Body: r.Name + ": " + *steps[len(steps)-1].Error, Link: "/automations", Source: "automation:" + strconv.FormatInt(r.Id, 10)})
	}
}
func (m *Module) ask(ctx context.Context, input, data map[string]any) (any, error) {
	prompt, _ := input["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("ai.ask 缺少 prompt")
	}
	client, ok := module.Lookup[contracts.LLM](m.d.Registry, contracts.LLMKey)
	if !ok {
		return nil, fmt.Errorf("AI 模块未启用")
	}
	raw, _ := json.Marshal(data)
	return client.CompleteText(contracts.WithAIUsage(ctx, "automation", ""), "agent", "按自动化规则处理触发数据，简短回答。", prompt+"\n触发数据: "+string(raw))
}
func (m *Module) hook(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if len(token) < 40 {
		http.NotFound(w, r)
		return
	}
	var id int64
	err := m.d.DB.QueryRowContext(r.Context(), "SELECT id FROM automations WHERE webhook_token_hash=? AND enabled=1", secrets.Hash(token)).Scan(&id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rule, err := m.readRule(r.Context(), id)
	if err != nil || rule.Trigger.Type != api.Webhook {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var body any
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "JSON 不正确", 400)
		return
	}
	data := map[string]any{"data": body, "type": "webhook"}
	for _, c := range rule.Conditions {
		if !compare(field(data, c.Field), string(c.Op), c.Value) {
			w.WriteHeader(204)
			return
		}
	}
	id, err = m.startRun(r.Context(), rule, data, runOther)
	if err != nil {
		http.Error(w, "触发失败", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(202)
	_ = json.NewEncoder(w).Encode(map[string]any{"runId": id})
}
