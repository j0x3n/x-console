package monitoring

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/db"
)

// Monitor kinds.
const (
	kindHTTP   = "http"
	kindTLS    = "tls"
	kindDomain = "domain"
)

// defaultInterval and minInterval are per kind, in seconds.
var (
	defaultInterval = map[string]int{kindHTTP: 60, kindTLS: 6 * 3600, kindDomain: 24 * 3600}
	minInterval     = map[string]int{kindHTTP: 30, kindTLS: 300, kindDomain: 3600}
)

const (
	defaultTimeoutMs = 10000
	maxInterval      = 7 * 24 * 3600
)

func itoa(id int64) string { return strconv.FormatInt(id, 10) }

// resultDetail is the JSON stored in monitor_results.detail.
type resultDetail struct {
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	DaysLeft  *float64   `json:"daysLeft,omitempty"`
	Subject   string     `json:"subject,omitempty"`
	Issuer    string     `json:"issuer,omitempty"`
	Registrar string     `json:"registrar,omitempty"`
}

func toAPIDetail(raw string) api.MonitorResultDetail {
	var d resultDetail
	_ = json.Unmarshal([]byte(raw), &d)
	out := api.MonitorResultDetail{ExpiresAt: d.ExpiresAt, DaysLeft: d.DaysLeft}
	if d.Subject != "" {
		out.Subject = ptr(d.Subject)
	}
	if d.Issuer != "" {
		out.Issuer = ptr(d.Issuer)
	}
	if d.Registrar != "" {
		out.Registrar = ptr(d.Registrar)
	}
	return out
}

func toAPIMonitor(x db.Monitor, now time.Time, iconAt *time.Time) api.Monitor {
	out := api.Monitor{Id: x.ID, Kind: api.MonitorKind(x.Kind), Name: x.Name, Target: x.Target,
		IntervalSeconds: int(x.IntervalSeconds), ExpectedStatus: int(x.ExpectedStatus), Keyword: x.Keyword,
		TimeoutMs: int(x.TimeoutMs), Enabled: x.Enabled == 1, LastStatus: api.MonitorStatus(x.LastStatus),
		LastCheckedAt: x.LastCheckedAt, LastError: x.LastError, ConsecutiveFailures: int(x.ConsecutiveFailures),
		ExpiresAt: x.ExpiresAt, CreatedAt: x.CreatedAt, IconAt: iconAt}
	if x.ExpiresAt != nil {
		out.DaysLeft = ptr(daysLeft(*x.ExpiresAt, now))
	}
	return out
}

func (m *Module) iconAtOf(ctx context.Context, id int64) *time.Time {
	at, err := m.q.GetMonitorIconTime(ctx, id)
	if err != nil {
		return nil
	}
	return &at
}

func (m *Module) iconTimes(ctx context.Context) map[int64]time.Time {
	rows, err := m.q.ListMonitorIconTimes(ctx)
	if err != nil {
		return map[int64]time.Time{}
	}
	out := make(map[int64]time.Time, len(rows))
	for _, row := range rows {
		out[row.MonitorID] = row.FetchedAt
	}
	return out
}

func lookupIconAt(times map[int64]time.Time, id int64) *time.Time {
	at, ok := times[id]
	if !ok {
		return nil
	}
	return &at
}

func toAPIResult(r db.MonitorResult) api.MonitorResult {
	out := api.MonitorResult{Id: ptr(r.ID), At: r.At, Ok: r.Ok == 1, LatencyMs: int(r.LatencyMs), Error: r.Error,
		Detail: toAPIDetail(r.Detail)}
	if r.StatusCode != nil {
		out.StatusCode = ptr(int(*r.StatusCode))
	}
	return out
}

// monitorFields is a validated monitor configuration.
type monitorFields struct {
	kind, name, target, keyword string
	interval, expected, timeout int
	enabled                     bool
}

// normalizeTarget checks the target of a kind and returns its canonical form.
func normalizeTarget(kind, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", httpx.Invalid("请填写监控目标")
	}
	switch kind {
	case kindHTTP:
		if !strings.Contains(target, "://") {
			target = "https://" + target
		}
		u, err := url.Parse(target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "", httpx.Invalid("网址要以 http:// 或 https:// 开头")
		}
		return target, nil
	case kindTLS:
		host, port, err := tlsAddress(target)
		if err != nil {
			return "", err
		}
		if port == "443" {
			return host, nil
		}
		return net.JoinHostPort(host, port), nil
	case kindDomain:
		d := domainName(target)
		if d == "" || !strings.Contains(d, ".") || strings.ContainsAny(d, " /:@") {
			return "", httpx.Invalid("请填写域名，例如 example.com")
		}
		return d, nil
	}
	return "", httpx.Invalid("kind 只能是 http、tls 或 domain")
}

// tlsAddress accepts host, host:port or a URL and returns host and port.
func tlsAddress(target string) (string, string, error) {
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil || u.Hostname() == "" {
			return "", "", httpx.Invalid("证书目标要填域名，可以带端口，例如 example.com:8443")
		}
		port := u.Port()
		if port == "" {
			port = "443"
		}
		return u.Hostname(), port, nil
	}
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		host, port = strings.Trim(target, "[]"), "443"
	}
	if host == "" || strings.ContainsAny(host, " /") {
		return "", "", httpx.Invalid("证书目标要填域名，可以带端口，例如 example.com:8443")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "", "", httpx.Invalid("端口不对")
	}
	return host, port, nil
}

// domainName strips a scheme, path and trailing dot from a domain target.
func domainName(target string) string {
	d := strings.ToLower(strings.TrimSpace(target))
	if strings.Contains(d, "://") {
		if u, err := url.Parse(d); err == nil {
			d = u.Hostname()
		}
	}
	d = strings.TrimSuffix(strings.TrimPrefix(d, "www."), ".")
	return d
}

func (f *monitorFields) validate() error {
	f.name = strings.TrimSpace(f.name)
	if f.name == "" {
		return httpx.Invalid("请填写名称")
	}
	target, err := normalizeTarget(f.kind, f.target)
	if err != nil {
		return err
	}
	f.target = target
	if f.interval < minInterval[f.kind] || f.interval > maxInterval {
		return httpx.Invalid("检查间隔最少 " + strconv.Itoa(minInterval[f.kind]) + " 秒，最多 7 天")
	}
	if f.expected < 0 || f.expected > 599 {
		return httpx.Invalid("期望状态码不对")
	}
	if f.timeout < 100 || f.timeout > 60000 {
		return httpx.Invalid("超时要在 100 到 60000 毫秒之间")
	}
	return nil
}

func (m *Module) getMonitor(ctx context.Context, id int64) (db.Monitor, error) {
	x, err := m.q.GetMonitor(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Monitor{}, httpx.ErrNotFound
	}
	return x, err
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// ListMonitors is GET /monitors.
func (m *Module) ListMonitors(w http.ResponseWriter, r *http.Request, params api.ListMonitorsParams) {
	rows, err := m.q.ListMonitors(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	now := m.now()
	times := m.iconTimes(r.Context())
	out := make([]api.Monitor, 0, len(rows))
	for _, x := range rows {
		if params.Kind != nil && x.Kind != string(*params.Kind) {
			continue
		}
		out = append(out, toAPIMonitor(x, now, lookupIconAt(times, x.ID)))
	}
	httpx.JSON(w, http.StatusOK, out)
}

// CreateMonitor is POST /monitors.
func (m *Module) CreateMonitor(w http.ResponseWriter, r *http.Request) {
	var body api.MonitorInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f := monitorFields{kind: string(body.Kind), name: body.Name, target: body.Target, interval: defaultInterval[string(body.Kind)],
		timeout: defaultTimeoutMs, enabled: true}
	if body.IntervalSeconds != nil {
		f.interval = *body.IntervalSeconds
	}
	if body.ExpectedStatus != nil {
		f.expected = *body.ExpectedStatus
	}
	if body.Keyword != nil {
		f.keyword = *body.Keyword
	}
	if body.TimeoutMs != nil {
		f.timeout = *body.TimeoutMs
	}
	if body.Enabled != nil {
		f.enabled = *body.Enabled
	}
	if err := f.validate(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	x, err := m.q.CreateMonitor(r.Context(), db.CreateMonitorParams{Kind: f.kind, Name: f.name, Target: f.target,
		IntervalSeconds: int64(f.interval), ExpectedStatus: int64(f.expected), Keyword: f.keyword, TimeoutMs: int64(f.timeout),
		Enabled: boolInt(f.enabled), CreatedAt: m.now()})
	m.d.Audit.Record(r.Context(), "monitor.create", "", map[string]any{"kind": f.kind, "target": f.target}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if x.Kind == kindHTTP {
		m.scheduleIcon(x.ID, x.Target)
	}
	out := toAPIMonitor(x, m.now(), nil)
	m.d.Bus.Publish("monitor.created", out)
	httpx.JSON(w, http.StatusCreated, out)
}

// GetMonitor is GET /monitors/{monitorId}.
func (m *Module) GetMonitor(w http.ResponseWriter, r *http.Request, id int64) {
	x, err := m.getMonitor(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPIMonitor(x, m.now(), m.iconAtOf(r.Context(), x.ID)))
}

// UpdateMonitor is PATCH /monitors/{monitorId}.
func (m *Module) UpdateMonitor(w http.ResponseWriter, r *http.Request, id int64) {
	var body api.MonitorPatch
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	lock := m.monitorLock(id)
	lock.Lock()
	defer lock.Unlock()
	cur, err := m.getMonitor(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f := monitorFields{kind: cur.Kind, name: cur.Name, target: cur.Target, keyword: cur.Keyword, interval: int(cur.IntervalSeconds),
		expected: int(cur.ExpectedStatus), timeout: int(cur.TimeoutMs), enabled: cur.Enabled == 1}
	if body.Name != nil {
		f.name = *body.Name
	}
	if body.Target != nil {
		f.target = *body.Target
	}
	if body.IntervalSeconds != nil {
		f.interval = *body.IntervalSeconds
	}
	if body.ExpectedStatus != nil {
		f.expected = *body.ExpectedStatus
	}
	if body.Keyword != nil {
		f.keyword = *body.Keyword
	}
	if body.TimeoutMs != nil {
		f.timeout = *body.TimeoutMs
	}
	if body.Enabled != nil {
		f.enabled = *body.Enabled
	}
	if err := f.validate(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	x, err := m.q.UpdateMonitor(ctx, db.UpdateMonitorParams{ID: id, Name: f.name, Target: f.target, IntervalSeconds: int64(f.interval),
		ExpectedStatus: int64(f.expected), Keyword: f.keyword, TimeoutMs: int64(f.timeout), Enabled: boolInt(f.enabled)})
	if err == nil && f.target != cur.Target {
		if err = m.q.ResetMonitorState(ctx, id); err == nil {
			x, err = m.q.GetMonitor(ctx, id)
		}
	}
	m.d.Audit.Record(ctx, "monitor.update", itoa(id), map[string]any{"target": f.target, "enabled": f.enabled}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := toAPIMonitor(x, m.now(), m.iconAtOf(ctx, x.ID))
	m.d.Bus.Publish("monitor.updated", out)
	httpx.JSON(w, http.StatusOK, out)
}

// DeleteMonitor is DELETE /monitors/{monitorId}.
func (m *Module) DeleteMonitor(w http.ResponseWriter, r *http.Request, id int64) {
	err := notFound(m.q.DeleteMonitor(r.Context(), id))
	m.d.Audit.Record(r.Context(), "monitor.delete", itoa(id), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("monitor.deleted", map[string]any{"id": id})
	httpx.NoContent(w)
}

// CheckMonitor is POST /monitors/{monitorId}/check.
func (m *Module) CheckMonitor(w http.ResponseWriter, r *http.Request, id int64) {
	res, err := m.checkNow(r.Context(), id, m.now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPIResult(res))
}

var resultRanges = map[api.GetMonitorResultsParamsRange]time.Duration{
	api.N24h: 24 * time.Hour, api.N7d: 7 * 24 * time.Hour, api.N30d: 30 * 24 * time.Hour,
}

// GetMonitorResults is GET /monitors/{monitorId}/results.
func (m *Module) GetMonitorResults(w http.ResponseWriter, r *http.Request, id int64, params api.GetMonitorResultsParams) {
	rng := api.N24h
	if params.Range != nil {
		rng = *params.Range
	}
	span, ok := resultRanges[rng]
	if !ok {
		httpx.Fail(w, r, httpx.Invalid("range 只能是 24h、7d 或 30d"))
		return
	}
	if _, err := m.getMonitor(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	since := m.now().Add(-span)
	rows, err := m.q.ListMonitorResults(r.Context(), db.ListMonitorResultsParams{MonitorID: id, At: since})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	points := make([]resultPoint, 0, len(rows))
	for _, x := range rows {
		points = append(points, resultPoint{At: x.At, OK: x.Ok == 1, StatusCode: x.StatusCode, LatencyMs: x.LatencyMs,
			Error: x.Error, Detail: x.Detail})
	}
	up, avg := uptime(points)
	merged, step := downsample(points, since, span, maxPoints)
	out := api.MonitorResults{Items: make([]api.MonitorResult, 0, len(merged)), Uptime: up, AvgLatencyMs: avg, Total: len(points)}
	if step > 0 {
		out.StepSeconds = ptr(step)
	}
	if step == 0 {
		for _, x := range rows {
			out.Items = append(out.Items, toAPIResult(x))
		}
	} else {
		for _, p := range merged {
			res := api.MonitorResult{At: p.At, Ok: p.OK, LatencyMs: int(p.LatencyMs), Error: p.Error, Detail: toAPIDetail(p.Detail)}
			if p.StatusCode != nil {
				res.StatusCode = ptr(int(*p.StatusCode))
			}
			out.Items = append(out.Items, res)
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}
