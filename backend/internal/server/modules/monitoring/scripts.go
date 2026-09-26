package monitoring

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/db"
)

const (
	shellBash       = "bash"
	shellSh         = "sh"
	shellPowerShell = "powershell"

	defaultScriptTimeout = 300
	maxScriptTimeout     = 1800
	// maxScriptBody keeps the command under Linux's 128 KB limit for one
	// argument (the agent passes it to sh -c).
	maxScriptBody = 64 << 10
	// maxRunOutput is how much of stdout and stderr each run keeps.
	maxRunOutput = 256 << 10
	maxRunHosts  = 50
)

// Who started a run.
const (
	byUser       = "user"
	byAutomation = "automation"
	byAI         = "ai"
)

func toAPIScript(s db.Script) api.Script {
	var hosts []string
	_ = json.Unmarshal([]byte(s.DefaultHostIds), &hosts)
	if hosts == nil {
		hosts = []string{}
	}
	return api.Script{Id: s.ID, Name: s.Name, Description: s.Description, Shell: api.ScriptShell(s.Shell), Body: s.Body,
		DefaultHostIds: hosts, TimeoutSeconds: int(s.TimeoutSeconds), CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
}

func runStatus(r db.ScriptRun) api.ScriptRunStatus {
	switch {
	case r.FinishedAt == nil:
		return api.Running
	case r.ExitCode != nil && *r.ExitCode == 0 && r.Error == "":
		return api.Ok
	}
	return api.Failed
}

func toAPIRun(r db.ScriptRun) api.ScriptRun {
	out := api.ScriptRun{Id: r.ID, ScriptId: r.ScriptID, HostId: r.HostID, HostName: r.HostName, StartedAt: r.StartedAt,
		FinishedAt: r.FinishedAt, Status: runStatus(r), Stdout: r.Stdout, Stderr: r.Stderr, Error: r.Error,
		TriggeredBy: api.ScriptRunTriggeredBy(r.TriggeredBy)}
	if r.ExitCode != nil {
		out.ExitCode = ptr(int(*r.ExitCode))
	}
	return out
}

// scriptFields is a validated script.
type scriptFields struct {
	name, description, shell, body string
	hosts                          []string
	timeout                        int
}

func (f *scriptFields) validate() error {
	f.name = strings.TrimSpace(f.name)
	switch {
	case f.name == "":
		return httpx.Invalid("请填写脚本名称")
	case strings.TrimSpace(f.body) == "":
		return httpx.Invalid("脚本内容不能为空")
	case len(f.body) > maxScriptBody:
		return httpx.Invalid("脚本太长，最多 64 KB")
	case f.shell != shellBash && f.shell != shellSh && f.shell != shellPowerShell:
		return httpx.Invalid("shell 只能是 bash、sh 或 powershell")
	case f.timeout < 1 || f.timeout > maxScriptTimeout:
		return httpx.Invalid("超时时间要在 1 到 1800 秒之间")
	}
	f.hosts = cleanHosts(f.hosts)
	return nil
}

// cleanHosts trims, drops empty ids and duplicates, keeping the order.
func cleanHosts(ids []string) []string {
	out := []string{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func (m *Module) getScript(ctx context.Context, id int64) (db.Script, error) {
	s, err := m.q.GetScript(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Script{}, httpx.ErrNotFound
	}
	return s, err
}

// ListScripts is GET /scripts.
func (m *Module) ListScripts(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListScripts(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.Script, 0, len(rows))
	for _, s := range rows {
		out = append(out, toAPIScript(s))
	}
	httpx.JSON(w, http.StatusOK, out)
}

// CreateScript is POST /scripts.
func (m *Module) CreateScript(w http.ResponseWriter, r *http.Request) {
	var body api.ScriptInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f := scriptFields{name: body.Name, shell: string(body.Shell), body: body.Body, timeout: defaultScriptTimeout}
	if body.Description != nil {
		f.description = *body.Description
	}
	if body.DefaultHostIds != nil {
		f.hosts = *body.DefaultHostIds
	}
	if body.TimeoutSeconds != nil {
		f.timeout = *body.TimeoutSeconds
	}
	if err := f.validate(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	now := m.now()
	s, err := m.q.CreateScript(r.Context(), db.CreateScriptParams{Name: f.name, Description: f.description, Shell: f.shell,
		Body: f.body, DefaultHostIds: mustJSON(f.hosts), TimeoutSeconds: int64(f.timeout), CreatedAt: now, UpdatedAt: now})
	m.d.Audit.Record(r.Context(), "script.create", "", map[string]any{"name": f.name}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := toAPIScript(s)
	m.d.Bus.Publish("script.created", out)
	httpx.JSON(w, http.StatusCreated, out)
}

// GetScript is GET /scripts/{scriptId}.
func (m *Module) GetScript(w http.ResponseWriter, r *http.Request, id int64) {
	s, err := m.getScript(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPIScript(s))
}

// UpdateScript is PATCH /scripts/{scriptId}. Changing what a script runs is
// as sensitive as running it, so it needs elevation too.
func (m *Module) UpdateScript(w http.ResponseWriter, r *http.Request, id int64) {
	var body api.ScriptPatch
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Body != nil || body.Shell != nil {
		if err := auth.RequireElevated(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	cur, err := m.getScript(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	old := toAPIScript(cur)
	f := scriptFields{name: cur.Name, description: cur.Description, shell: cur.Shell, body: cur.Body,
		hosts: old.DefaultHostIds, timeout: int(cur.TimeoutSeconds)}
	if body.Name != nil {
		f.name = *body.Name
	}
	if body.Description != nil {
		f.description = *body.Description
	}
	if body.Shell != nil {
		f.shell = string(*body.Shell)
	}
	if body.Body != nil {
		f.body = *body.Body
	}
	if body.DefaultHostIds != nil {
		f.hosts = *body.DefaultHostIds
	}
	if body.TimeoutSeconds != nil {
		f.timeout = *body.TimeoutSeconds
	}
	if err := f.validate(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s, err := m.q.UpdateScript(r.Context(), db.UpdateScriptParams{ID: id, Name: f.name, Description: f.description, Shell: f.shell,
		Body: f.body, DefaultHostIds: mustJSON(f.hosts), TimeoutSeconds: int64(f.timeout), UpdatedAt: m.now()})
	m.d.Audit.Record(r.Context(), "script.update", itoa(id), map[string]any{"name": f.name, "bodyChanged": body.Body != nil}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := toAPIScript(s)
	m.d.Bus.Publish("script.updated", out)
	httpx.JSON(w, http.StatusOK, out)
}

// DeleteScript is DELETE /scripts/{scriptId}.
func (m *Module) DeleteScript(w http.ResponseWriter, r *http.Request, id int64) {
	err := notFound(m.q.DeleteScript(r.Context(), id))
	m.d.Audit.Record(r.Context(), "script.delete", itoa(id), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("script.deleted", map[string]any{"id": id})
	httpx.NoContent(w)
}

// RunScript is POST /scripts/{scriptId}/run.
func (m *Module) RunScript(w http.ResponseWriter, r *http.Request, id int64) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.RunScriptRequest
	if err := decodeOptional(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s, err := m.getScript(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var hosts []string
	if body.HostIds != nil {
		hosts = *body.HostIds
	}
	runs, _, err := m.startRuns(r.Context(), s, hosts, byUser)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.ScriptRun, 0, len(runs))
	for _, run := range runs {
		out = append(out, toAPIRun(run))
	}
	httpx.JSON(w, http.StatusAccepted, out)
}

// hostNames maps host ids to names. Missing hosts are rejected.
func hostNames(ctx context.Context, hosts contracts.Hosts, ids []string) (map[string]string, error) {
	all, err := hosts.Summaries(ctx)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, h := range all {
		names[h.ID] = h.Name
	}
	for _, id := range ids {
		if _, ok := names[id]; !ok {
			return nil, httpx.Invalid("机器不存在: " + id)
		}
	}
	return names, nil
}

// startRuns records one run per host and executes them in parallel in the
// background. hostIDs empty means the script's default hosts. The returned
// channel is closed when every run finished. Callers check elevation.
func (m *Module) startRuns(ctx context.Context, s db.Script, hostIDs []string, by string) ([]db.ScriptRun, <-chan struct{}, error) {
	hosts, ok := module.Lookup[contracts.Hosts](m.d.Registry, contracts.HostsKey)
	if !ok {
		return nil, nil, httpx.NewError(http.StatusNotImplemented, "feature_unavailable", "服务器模块未启用")
	}
	ids := cleanHosts(hostIDs)
	if len(ids) == 0 {
		ids = toAPIScript(s).DefaultHostIds
	}
	if len(ids) == 0 {
		return nil, nil, httpx.Invalid("请选择要执行的机器")
	}
	if len(ids) > maxRunHosts {
		return nil, nil, httpx.Invalid("一次最多在 50 台机器上执行")
	}
	names, err := hostNames(ctx, hosts, ids)
	if err != nil {
		return nil, nil, err
	}
	now := m.now()
	runs := make([]db.ScriptRun, 0, len(ids))
	for _, id := range ids {
		run, err := m.q.CreateScriptRun(ctx, db.CreateScriptRunParams{ScriptID: s.ID, HostID: id, HostName: names[id],
			StartedAt: now, TriggeredBy: by})
		if err != nil {
			return nil, nil, err
		}
		runs = append(runs, run)
	}
	m.d.Audit.Record(ctx, "script.run", itoa(s.ID), map[string]any{"name": s.Name, "hosts": ids, "triggeredBy": by}, nil)
	for _, run := range runs {
		m.d.Bus.Publish("script_run.started", toAPIRun(run))
	}

	// The runs outlive the request. They keep the acting user for the
	// audit log and stop when the server stops.
	bg := audit.WithActor(m.background(), audit.Actor(ctx))
	command := scriptCommand(s.Shell, s.Body)
	timeout := time.Duration(s.TimeoutSeconds) * time.Second
	done := make(chan struct{})
	var wg sync.WaitGroup
	for _, run := range runs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.execRun(bg, hosts, run, command, timeout)
		}()
	}
	go func() {
		wg.Wait()
		close(done)
	}()
	return runs, done, nil
}

// execRun runs one host's part and stores the result.
func (m *Module) execRun(ctx context.Context, hosts contracts.Hosts, run db.ScriptRun, command string, timeout time.Duration) {
	res, err := hosts.Exec(ctx, run.HostID, command, timeout)
	p := db.FinishScriptRunParams{ID: run.ID, FinishedAt: ptr(m.now())}
	if err != nil {
		p.Error = errText(err)
	} else {
		p.ExitCode = ptr(int64(res.ExitCode))
		p.Stdout = clip(res.Stdout, maxRunOutput)
		p.Stderr = clip(res.Stderr, maxRunOutput)
	}
	// Store the result even when the server is stopping.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	row, err := m.q.FinishScriptRun(saveCtx, p)
	if err != nil {
		m.d.Log.Warn("script run: save result", "run", run.ID, "err", err)
		return
	}
	m.d.Bus.Publish("script_run.finished", toAPIRun(row))
}

// ListScriptRuns is GET /scripts/{scriptId}/runs.
func (m *Module) ListScriptRuns(w http.ResponseWriter, r *http.Request, id int64, params api.ListScriptRunsParams) {
	if _, err := m.getScript(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	before, err := httpx.DecodeIDCursor(params.Cursor)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	limit := httpx.Limit(params.Limit)
	rows, err := m.q.ListScriptRuns(r.Context(), db.ListScriptRunsParams{ScriptID: id, Before: before, Lim: limit + 1})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	page := api.ScriptRunPage{Items: []api.ScriptRun{}}
	if int64(len(rows)) > limit {
		rows = rows[:limit]
		page.NextCursor = ptr(httpx.EncodeIDCursor(rows[len(rows)-1].ID))
	}
	for _, run := range rows {
		page.Items = append(page.Items, toAPIRun(run))
	}
	httpx.JSON(w, http.StatusOK, page)
}

// GetScriptRun is GET /script-runs/{runId}.
func (m *Module) GetScriptRun(w http.ResponseWriter, r *http.Request, id int64) {
	run, err := m.q.GetScriptRun(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPIRun(run))
}
