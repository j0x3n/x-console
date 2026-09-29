package monitoring

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/db"
)

// actionRunWait is how long scripts.run waits for the runs to finish before
// it returns what it has.
const actionRunWait = 10 * time.Minute

// registerActions adds the M10 actions for the assistant and automations.
// scripts.run is dangerous: the caller confirms and elevates.
func (m *Module) registerActions() {
	reg := m.d.Actions
	reg.Register(actions.Action{
		Name:        "scripts.list",
		Title:       "查看脚本库",
		Description: "List saved scripts with id, name, description, shell and default host ids.",
		Input:       actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		Effect:      actions.Read,
		Run:         m.actionListScripts,
	})
	reg.Register(actions.Action{
		Name:  "scripts.run",
		Title: "执行脚本",
		Description: "Run a saved script on one or more machines in parallel and wait for it. `script` is the script id or name. " +
			"`hosts` are host ids or names; empty means the script's default hosts. Returns one run per host with status, exitCode, stdout and stderr.",
		Input:  actions.Schema(`{"type":"object","properties":{"script":{"type":"string"},"hosts":{"type":"array","items":{"type":"string"}}},"required":["script"],"additionalProperties":false}`),
		Effect: actions.Dangerous,
		Run:    m.actionRunScript,
	})
	reg.Register(actions.Action{
		Name:        "monitors.list",
		Title:       "查看网站和证书监控",
		Description: "List website, TLS certificate and domain monitors with status (up, down, unknown), last error and days until expiry.",
		Input:       actions.Schema(`{"type":"object","properties":{"kind":{"type":"string","enum":["http","tls","domain"]}},"additionalProperties":false}`),
		Effect:      actions.Read,
		Run:         m.actionListMonitors,
	})
	reg.Register(actions.Action{
		Name:        "subscriptions.list",
		Title:       "查看订阅和续费",
		Description: "List active subscriptions sorted by next renewal date, with amount, currency, cycle, daysLeft and monthly cost, plus the spend summary per currency.",
		Input:       actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		Effect:      actions.Read,
		Run:         m.actionListSubscriptions,
	})
}

func decodeInput(raw json.RawMessage, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return httpx.Invalid("参数格式不对: " + err.Error())
	}
	return nil
}

func (m *Module) actionListScripts(ctx context.Context, _ json.RawMessage) (any, error) {
	rows, err := m.q.ListScripts(ctx)
	if err != nil {
		return nil, err
	}
	type item struct {
		ID             int64    `json:"id"`
		Name           string   `json:"name"`
		Description    string   `json:"description"`
		Shell          string   `json:"shell"`
		DefaultHostIds []string `json:"defaultHostIds"`
	}
	out := make([]item, 0, len(rows))
	for _, s := range rows {
		a := toAPIScript(s)
		out = append(out, item{ID: s.ID, Name: s.Name, Description: s.Description, Shell: s.Shell, DefaultHostIds: a.DefaultHostIds})
	}
	return out, nil
}

// findScript accepts an id or a name.
func (m *Module) findScript(ctx context.Context, ref string) (db.Script, error) {
	ref = strings.TrimSpace(ref)
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		if s, err := m.getScript(ctx, id); err == nil {
			return s, nil
		}
	}
	rows, err := m.q.ListScripts(ctx)
	if err != nil {
		return db.Script{}, err
	}
	for _, s := range rows {
		if strings.EqualFold(s.Name, ref) {
			return s, nil
		}
	}
	return db.Script{}, httpx.NewError(404, "not_found", "没有这个脚本: "+ref)
}

// resolveHosts maps host names to ids; ids pass through.
func (m *Module) resolveHosts(ctx context.Context, refs []string) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	hosts, ok := module.Lookup[contracts.Hosts](m.d.Registry, contracts.HostsKey)
	if !ok {
		return nil, httpx.NewError(501, "feature_unavailable", "服务器模块未启用")
	}
	all, err := hosts.Summaries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		id := ref
		for _, h := range all {
			if h.ID == ref {
				break
			}
			if strings.EqualFold(h.Name, ref) {
				id = h.ID
				break
			}
		}
		out = append(out, id)
	}
	return out, nil
}

// triggeredBy tells automations from the assistant by the audit actor.
func triggeredBy(ctx context.Context) string {
	if strings.Contains(strings.ToLower(audit.Actor(ctx)), "automation") {
		return byAutomation
	}
	return byAI
}

func (m *Module) actionRunScript(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Script string   `json:"script"`
		Hosts  []string `json:"hosts"`
	}
	if err := decodeInput(raw, &in); err != nil {
		return nil, err
	}
	s, err := m.findScript(ctx, in.Script)
	if err != nil {
		return nil, err
	}
	hosts, err := m.resolveHosts(ctx, in.Hosts)
	if err != nil {
		return nil, err
	}
	runs, done, err := m.startRuns(ctx, s, hosts, triggeredBy(ctx))
	if err != nil {
		return nil, err
	}
	select {
	case <-done:
	case <-ctx.Done():
	case <-time.After(actionRunWait):
	}
	out := make([]api.ScriptRun, 0, len(runs))
	for _, r := range runs {
		if row, err := m.q.GetScriptRun(context.WithoutCancel(ctx), r.ID); err == nil {
			r = row
		}
		out = append(out, toAPIRun(r))
	}
	return out, nil
}

func (m *Module) actionListMonitors(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Kind string `json:"kind"`
	}
	if err := decodeInput(raw, &in); err != nil {
		return nil, err
	}
	rows, err := m.q.ListMonitors(ctx)
	if err != nil {
		return nil, err
	}
	now := m.now()
	out := make([]api.Monitor, 0, len(rows))
	for _, x := range rows {
		if in.Kind == "" || x.Kind == in.Kind {
			out = append(out, toAPIMonitor(x, now))
		}
	}
	return out, nil
}

func (m *Module) actionListSubscriptions(ctx context.Context, _ json.RawMessage) (any, error) {
	rows, err := m.q.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	cats, err := m.categoryNames(ctx)
	if err != nil {
		return nil, err
	}
	day := today(m.now(), m.loc())
	items := make([]api.Subscription, 0, len(rows))
	for _, s := range rows {
		if s.ArchivedAt == nil {
			items = append(items, toAPISubscription(s, day, cats))
		}
	}
	return map[string]any{"items": items, "summary": summary(rows, cats)}, nil
}
