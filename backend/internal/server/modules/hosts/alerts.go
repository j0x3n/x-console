package hosts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// staleAfter is how old the latest sample may be for metric rules.
const staleAfter = 3 * time.Minute

type alertKey struct {
	rule int64
	host string
}

// alertState remembers since when each (rule, host) pair is breaching.
type alertState struct {
	mu    sync.Mutex
	since map[alertKey]time.Time
	run   sync.Mutex // one evaluation at a time
}

func newAlertState() *alertState { return &alertState{since: map[alertKey]time.Time{}} }

func toAPIRule(r db.AlertRule) api.AlertRule {
	return api.AlertRule{Id: r.ID, HostId: r.HostID, Metric: api.AlertMetric(r.Metric), Op: api.AlertOp(r.Op),
		Threshold: r.Threshold, DurationSeconds: int(r.DurationSeconds), Severity: api.AlertSeverity(r.Severity),
		Enabled: r.Enabled == 1, CreatedAt: r.CreatedAt}
}

func toAPIAlert(e db.AlertEvent) api.AlertEvent {
	return api.AlertEvent{Id: e.ID, RuleId: e.RuleID, HostId: e.HostID, HostName: e.HostName, Metric: api.AlertMetric(e.Metric),
		Severity: api.AlertSeverity(e.Severity), Value: e.Value, Message: e.Message, FiredAt: e.FiredAt, ResolvedAt: e.ResolvedAt}
}

// ---- evaluation ----

// check tells whether host breaches rule now. ok is false when there is no
// usable data, in which case the alert state is left alone.
func check(r db.AlertRule, h api.Host, now time.Time) (breached bool, value float64, since time.Time, ok bool) {
	if r.Metric == "offline" {
		if h.Online {
			return false, 0, now, true
		}
		since = now
		if h.LastSeenAt != nil {
			since = *h.LastSeenAt
		}
		return true, now.Sub(since).Minutes(), since, true
	}
	if !h.Online || h.Metrics == nil || now.Sub(h.Metrics.At) > staleAfter {
		return false, 0, now, false
	}
	switch r.Metric {
	case "cpu":
		value = h.Metrics.Cpu
	case "memory":
		value = percent(uint64(h.Metrics.MemUsed), uint64(h.Metrics.MemTotal))
	case "disk":
		if h.Disk == nil {
			return false, 0, now, false
		}
		value = *h.Disk
	default:
		return false, 0, now, false
	}
	if r.Op == "lt" {
		return value < r.Threshold, value, now, true
	}
	return value > r.Threshold, value, now, true
}

var metricNames = map[string]string{"cpu": "CPU 使用率", "memory": "内存使用率", "disk": "磁盘使用率"}

func alertMessage(r db.AlertRule, h api.Host, value float64) (title, message string) {
	if r.Metric == "offline" {
		return h.Name + " 离线了", fmt.Sprintf("已经离线 %s", humanMinutes(value))
	}
	name := metricNames[r.Metric]
	word := "超过"
	if r.Op == "lt" {
		word = "低于"
	}
	msg := fmt.Sprintf("%s %.0f%%，%s %.0f%%", name, value, word, r.Threshold)
	if r.Metric == "disk" && h.Metrics != nil {
		ds := make([]string, 0, 1)
		for _, d := range h.Metrics.Disks {
			if percent(uint64(d.Used), uint64(d.Total)) == value {
				ds = append(ds, d.Mount)
				break
			}
		}
		if len(ds) > 0 {
			msg = fmt.Sprintf("磁盘 %s 使用率 %.0f%%，%s %.0f%%", ds[0], value, word, r.Threshold)
		}
	}
	return fmt.Sprintf("%s %s %.0f%%", h.Name, name, value), msg
}

func humanMinutes(v float64) string {
	mins := int(v)
	if mins < 1 {
		return "不到 1 分钟"
	}
	if mins < 60 {
		return fmt.Sprintf("%d 分钟", mins)
	}
	if mins < 48*60 {
		return fmt.Sprintf("%d 小时 %d 分钟", mins/60, mins%60)
	}
	return fmt.Sprintf("%d 天", mins/(24*60))
}

func hostLink(h api.Host) string {
	if h.Kind == api.Desktop {
		return "/pc"
	}
	return "/servers/" + h.Id
}

// evaluateAlerts checks every enabled rule against every host. The
// scheduler runs it every 15 seconds; tests call it directly.
func (m *Module) evaluateAlerts(ctx context.Context) error {
	m.alerts.run.Lock()
	defer m.alerts.run.Unlock()
	now := m.now()
	rules, err := m.q.ListEnabledAlertRules(ctx)
	if err != nil {
		return err
	}
	hosts, err := m.gatherHosts(ctx)
	if err != nil {
		return err
	}
	openRows, err := m.q.ListOpenAlertEvents(ctx)
	if err != nil {
		return err
	}
	open := map[alertKey]db.AlertEvent{}
	var orphans []db.AlertEvent
	for _, ev := range openRows {
		if ev.RuleID == nil {
			orphans = append(orphans, ev)
			continue
		}
		open[alertKey{*ev.RuleID, ev.HostID}] = ev
	}
	visited := map[alertKey]bool{}
	for _, r := range rules {
		for _, h := range hosts {
			if r.HostID != nil && *r.HostID != h.Id {
				continue
			}
			key := alertKey{r.ID, h.Id}
			breached, value, since, ok := check(r, h, now)
			ev, isOpen := open[key]
			if !ok {
				if isOpen {
					visited[key] = true // no data now; keep the alert as it is
				}
				continue
			}
			visited[key] = true
			if !breached {
				m.alerts.mu.Lock()
				delete(m.alerts.since, key)
				m.alerts.mu.Unlock()
				if isOpen {
					m.resolveAlert(ctx, ev, now)
				}
				continue
			}
			m.alerts.mu.Lock()
			start, seen := m.alerts.since[key]
			if !seen || since.Before(start) {
				start = since
				m.alerts.since[key] = start
			}
			m.alerts.mu.Unlock()
			if isOpen || now.Sub(start) < time.Duration(r.DurationSeconds)*time.Second {
				continue
			}
			if err := m.fireAlert(ctx, r, h, value, now); err != nil {
				return err
			}
		}
	}
	// Alerts whose rule was disabled or removed, or whose host is gone.
	for key, ev := range open {
		if !visited[key] {
			m.resolveAlert(ctx, ev, now)
		}
	}
	for _, ev := range orphans {
		m.resolveAlert(ctx, ev, now)
	}
	return nil
}

func (m *Module) fireAlert(ctx context.Context, r db.AlertRule, h api.Host, value float64, now time.Time) error {
	title, msg := alertMessage(r, h, value)
	ruleID := r.ID
	ev, err := m.q.InsertAlertEvent(ctx, db.InsertAlertEventParams{RuleID: &ruleID, HostID: h.Id, HostName: h.Name,
		Metric: r.Metric, Severity: r.Severity, Value: round1(value), Message: msg, FiredAt: now})
	if err != nil {
		return err
	}
	a := toAPIAlert(ev)
	m.d.Bus.Publish("host.alert.fired", a)
	priority := notify.PriorityHigh
	if r.Severity == "critical" {
		priority = notify.PriorityUrgent
	}
	if _, err := m.d.Notify.Send(ctx, notify.Notification{Kind: "host.alert", Title: title, Body: msg, Link: hostLink(h),
		Priority: priority, Source: "hosts", Data: map[string]any{"hostId": h.Id, "alertId": ev.ID, "ruleId": r.ID}}); err != nil {
		m.d.Log.Warn("alert notification failed", "err", err)
	}
	return nil
}

func (m *Module) resolveAlert(ctx context.Context, ev db.AlertEvent, now time.Time) {
	row, err := m.q.ResolveAlertEvent(ctx, db.ResolveAlertEventParams{ResolvedAt: &now, ID: ev.ID})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			m.d.Log.Warn("resolve alert failed", "id", ev.ID, "err", err)
		}
		return
	}
	m.d.Bus.Publish("host.alert.resolved", toAPIAlert(row))
}

// Alerts implements contracts.Hosts: alerts fired since the given time.
func (m *Module) Alerts(ctx context.Context, since time.Time) ([]contracts.HostAlert, error) {
	rows, err := m.q.ListAlertEvents(ctx, db.ListAlertEventsParams{Since: since, HostID: nil, OpenOnly: 0, Lim: 500})
	if err != nil {
		return nil, err
	}
	out := make([]contracts.HostAlert, 0, len(rows))
	for _, e := range rows {
		if !m.alertVisible(ctx, e.HostID) {
			continue
		}
		out = append(out, contracts.HostAlert{HostID: e.HostID, HostName: e.HostName, Rule: e.Metric, Message: e.Message,
			FiredAt: e.FiredAt, Resolved: e.ResolvedAt})
	}
	return out, nil
}

// ---- HTTP ----

// ListAlerts is GET /alerts.
func (m *Module) ListAlerts(w http.ResponseWriter, r *http.Request, params api.ListAlertsParams) {
	since := m.now().Add(-7 * 24 * time.Hour)
	if params.Since != nil {
		since = params.Since.UTC()
	}
	p := db.ListAlertEventsParams{Since: since, OpenOnly: 0, Lim: int64(httpx.Limit(params.Limit))}
	if params.HostId != nil && *params.HostId != "" {
		p.HostID = *params.HostId
	}
	if params.Active != nil && *params.Active {
		p.OpenOnly = 1
	}
	rows, err := m.q.ListAlertEvents(r.Context(), p)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := make([]api.AlertEvent, 0, len(rows))
	for _, e := range rows {
		if !m.alertVisible(r.Context(), e.HostID) {
			continue
		}
		items = append(items, toAPIAlert(e))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// ListAlertRules is GET /alert-rules.
func (m *Module) ListAlertRules(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListAlertRules(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.AlertRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAPIRule(row))
	}
	httpx.JSON(w, http.StatusOK, out)
}

type ruleInput struct {
	hostID    *string
	metric    string
	op        string
	threshold float64
	duration  int64
	severity  string
	enabled   int64
}

func (m *Module) parseRule(ctx context.Context, body api.AlertRuleInput) (ruleInput, error) {
	in := ruleInput{metric: string(body.Metric), op: "gt", severity: "warning", enabled: 1}
	switch in.metric {
	case "cpu", "memory", "disk", "offline":
	default:
		return in, httpx.Invalid("metric 只能是 cpu、memory、disk 或 offline")
	}
	if body.HostId != nil && *body.HostId != "" {
		if _, err := m.host(ctx, *body.HostId); err != nil {
			if errors.Is(err, httpx.ErrNotFound) {
				return in, httpx.Invalid("找不到这台机器")
			}
			return in, err
		}
		id := *body.HostId
		in.hostID = &id
	}
	if body.Op != nil {
		in.op = string(*body.Op)
		if in.op != "gt" && in.op != "lt" {
			return in, httpx.Invalid("op 只能是 gt 或 lt")
		}
	}
	if body.Threshold != nil {
		in.threshold = *body.Threshold
	}
	if in.metric != "offline" && (body.Threshold == nil || in.threshold < 0 || in.threshold > 100) {
		return in, httpx.Invalid("阈值要在 0 到 100 之间")
	}
	if body.DurationSeconds != nil {
		in.duration = int64(*body.DurationSeconds)
	} else if in.metric == "offline" {
		in.duration = 300
	}
	if in.duration < 0 || in.duration > 7*24*3600 {
		return in, httpx.Invalid("持续时间要在 0 到 7 天之间")
	}
	if body.Severity != nil {
		in.severity = string(*body.Severity)
		if in.severity != "warning" && in.severity != "critical" {
			return in, httpx.Invalid("severity 只能是 warning 或 critical")
		}
	}
	if body.Enabled != nil && !*body.Enabled {
		in.enabled = 0
	}
	return in, nil
}

// CreateAlertRule is POST /alert-rules.
func (m *Module) CreateAlertRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body api.CreateAlertRuleJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in, err := m.parseRule(ctx, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.q.CreateAlertRule(ctx, db.CreateAlertRuleParams{HostID: in.hostID, Metric: in.metric, Op: in.op,
		Threshold: in.threshold, DurationSeconds: in.duration, Severity: in.severity, Enabled: in.enabled, CreatedAt: m.now()})
	m.d.Audit.Record(ctx, "host.alert_rule.create", in.metric, map[string]any{"hostId": in.hostID, "threshold": in.threshold}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("host.alert_rule.created", toAPIRule(row))
	httpx.JSON(w, http.StatusCreated, toAPIRule(row))
}

// UpdateAlertRule is PUT /alert-rules/{ruleId}.
func (m *Module) UpdateAlertRule(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	var body api.UpdateAlertRuleJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in, err := m.parseRule(ctx, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.q.UpdateAlertRule(ctx, db.UpdateAlertRuleParams{HostID: in.hostID, Metric: in.metric, Op: in.op,
		Threshold: in.threshold, DurationSeconds: in.duration, Severity: in.severity, Enabled: in.enabled, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(ctx, "host.alert_rule.update", fmt.Sprint(id), map[string]any{"metric": in.metric, "threshold": in.threshold}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.clearRuleState(id)
	m.d.Bus.Publish("host.alert_rule.updated", toAPIRule(row))
	httpx.JSON(w, http.StatusOK, toAPIRule(row))
}

// DeleteAlertRule is DELETE /alert-rules/{ruleId}. Its open alerts resolve.
func (m *Module) DeleteAlertRule(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	open, err := m.q.ListOpenAlertEvents(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	n, err := m.q.DeleteAlertRule(ctx, id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(ctx, "host.alert_rule.delete", fmt.Sprint(id), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	now := m.now()
	for _, ev := range open {
		if ev.RuleID != nil && *ev.RuleID == id {
			m.resolveAlert(ctx, ev, now)
		}
	}
	m.clearRuleState(id)
	m.d.Bus.Publish("host.alert_rule.deleted", map[string]int64{"id": id})
	httpx.NoContent(w)
}

func (m *Module) clearRuleState(id int64) {
	m.alerts.mu.Lock()
	defer m.alerts.mu.Unlock()
	for k := range m.alerts.since {
		if k.rule == id {
			delete(m.alerts.since, k)
		}
	}
}
