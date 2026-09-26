package monitoring_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// execLog records exec.run calls of the fake agents.
type execLog struct {
	mu    sync.Mutex
	calls []execCall
}

type execCall struct {
	host       string
	command    string
	timeout    int
	start, end time.Time
}

// fakeExec answers exec.run after a short pause so parallel runs overlap.
func (l *execLog) fakeExec(host string, exit int) func(c *conn.Client) {
	return func(c *conn.Client) {
		c.Handle(protocol.MethodExecRun, func(ctx context.Context, raw json.RawMessage) (any, error) {
			var p protocol.ExecParams
			_ = json.Unmarshal(raw, &p)
			start := time.Now()
			time.Sleep(300 * time.Millisecond)
			l.mu.Lock()
			l.calls = append(l.calls, execCall{host: host, command: p.Command, timeout: p.TimeoutSeconds, start: start, end: time.Now()})
			l.mu.Unlock()
			return protocol.ExecResult{ExitCode: exit, Stdout: "hello from " + host, Stderr: fmt.Sprintf("exit %d", exit)}, nil
		})
	}
}

func (l *execLog) snapshot() []execCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]execCall(nil), l.calls...)
}

func overlap(a, b execCall) bool { return a.start.Before(b.end) && b.start.Before(a.end) }

func waitRuns(t *testing.T, env *testutil.Env, runs []api.ScriptRun) map[string]api.ScriptRun {
	t.Helper()
	out := map[string]api.ScriptRun{}
	for _, r := range runs {
		var cur api.ScriptRun
		waitFor(t, "script run", func() bool {
			env.MustDo(http.MethodGet, fmt.Sprintf("/script-runs/%d", r.Id), nil, &cur)
			return cur.Status != api.Running
		})
		out[cur.HostName] = cur
	}
	return out
}

func TestScriptsRunInParallel(t *testing.T) {
	env, _ := setup(t)
	expectStatus(t, env, http.MethodPost, "/scripts", api.ScriptInput{Name: "x", Shell: "zsh", Body: "ls"}, 400, "validation_failed")
	expectStatus(t, env, http.MethodPost, "/scripts", api.ScriptInput{Name: " ", Shell: "sh", Body: "ls"}, 400, "validation_failed")
	expectStatus(t, env, http.MethodGet, "/scripts/42", nil, 404, "not_found")
	expectStatus(t, env, http.MethodGet, "/script-runs/42", nil, 404, "not_found")

	var s api.Script
	env.MustDo(http.MethodPost, "/scripts", api.ScriptInput{Name: "Disk usage", Shell: "bash", Body: "df -h | grep '/$'",
		Description: ptr("root disk"), TimeoutSeconds: ptr(90)}, &s)
	if s.TimeoutSeconds != 90 || len(s.DefaultHostIds) != 0 || s.Shell != api.Bash {
		t.Fatalf("created: %+v", s)
	}
	path := fmt.Sprintf("/scripts/%d", s.Id)

	// Running needs elevation.
	expectStatus(t, env, http.MethodPost, path+"/run", api.RunScriptRequest{HostIds: &[]string{"x"}}, 403, "elevation_required")

	log := &execLog{}
	web := startAgent(t, env, "web", []string{protocol.CapExec}, log.fakeExec("web", 0))
	db := startAgent(t, env, "db", []string{protocol.CapExec}, log.fakeExec("db", 3))

	// startAgent elevated the session.
	expectStatus(t, env, http.MethodPost, path+"/run", nil, 400, "validation_failed") // no hosts, no defaults
	expectStatus(t, env, http.MethodPost, path+"/run", api.RunScriptRequest{HostIds: &[]string{"nope"}}, 400, "validation_failed")
	expectStatus(t, env, http.MethodPost, "/scripts/42/run", api.RunScriptRequest{HostIds: &[]string{web}}, 404, "not_found")

	var runs []api.ScriptRun
	status, raw := env.Do(http.MethodPost, path+"/run", api.RunScriptRequest{HostIds: &[]string{web, db, web}}, &runs)
	if status != http.StatusAccepted || len(runs) != 2 || runs[0].Status != api.Running || runs[0].TriggeredBy != api.User {
		t.Fatalf("run: %d %s", status, raw)
	}
	done := waitRuns(t, env, runs)
	if r := done["web"]; r.Status != api.Ok || r.ExitCode == nil || *r.ExitCode != 0 || r.Stdout != "hello from web" || r.FinishedAt == nil {
		t.Fatalf("web run: %+v", r)
	}
	if r := done["db"]; r.Status != api.Failed || *r.ExitCode != 3 || r.Stderr != "exit 3" {
		t.Fatalf("db run: %+v", r)
	}
	calls := log.snapshot()
	if len(calls) != 2 || !overlap(calls[0], calls[1]) {
		t.Fatalf("runs were not parallel: %+v", calls)
	}
	if calls[0].command != `bash -c 'df -h | grep '\''/$'\'''` || calls[0].timeout != 90 {
		t.Fatalf("command: %+v", calls[0])
	}
	if auditCount(t, env, "script.run") != 1 || auditCount(t, env, "host.exec") != 2 {
		t.Fatalf("audit: script.run %d host.exec %d", auditCount(t, env, "script.run"), auditCount(t, env, "host.exec"))
	}

	// Default hosts, and the action with host names.
	env.MustDo(http.MethodPatch, path, api.ScriptPatch{DefaultHostIds: &[]string{db}}, &s)
	env.MustDo(http.MethodPost, path+"/run", nil, &runs)
	if len(runs) != 1 || runs[0].HostId != db {
		t.Fatalf("default hosts: %+v", runs)
	}
	waitRuns(t, env, runs)

	var list []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	runAction(t, env, "scripts.list", map[string]any{}, &list)
	if len(list) != 1 || list[0].Name != "Disk usage" {
		t.Fatalf("scripts.list: %+v", list)
	}
	var actionRuns []api.ScriptRun
	runAction(t, env, "scripts.run", map[string]any{"script": "disk usage", "hosts": []string{"web", "DB"}}, &actionRuns)
	if len(actionRuns) != 2 || actionRuns[0].Status == api.Running || actionRuns[1].Status == api.Running ||
		actionRuns[0].TriggeredBy != api.Ai {
		t.Fatalf("scripts.run: %+v", actionRuns)
	}

	// History, newest first, with a cursor.
	var page api.ScriptRunPage
	env.MustDo(http.MethodGet, path+"/runs?limit=2", nil, &page)
	if len(page.Items) != 2 || page.NextCursor == nil || page.Items[0].Id < page.Items[1].Id {
		t.Fatalf("page 1: %+v", page)
	}
	cursor := *page.NextCursor
	page = api.ScriptRunPage{}
	env.MustDo(http.MethodGet, path+"/runs?limit=10&cursor="+cursor, nil, &page)
	if len(page.Items) != 3 || page.NextCursor != nil {
		t.Fatalf("page 2: %+v", page)
	}

	// Changing the body needs elevation; renaming does not.
	relogin(t, env)
	expectStatus(t, env, http.MethodPatch, path, api.ScriptPatch{Body: ptr("rm -rf /tmp/x")}, 403, "elevation_required")
	env.MustDo(http.MethodPatch, path, api.ScriptPatch{Name: ptr("Disk")}, &s)
	if s.Name != "Disk" || s.Body != "df -h | grep '/$'" {
		t.Fatalf("patched: %+v", s)
	}
	var scripts []api.Script
	env.MustDo(http.MethodGet, "/scripts", nil, &scripts)
	if len(scripts) != 1 {
		t.Fatalf("list: %+v", scripts)
	}

	env.MustDo(http.MethodDelete, path, nil, nil)
	expectStatus(t, env, http.MethodGet, fmt.Sprintf("/script-runs/%d", runs[0].Id), nil, 404, "not_found")
}

func TestScriptRunOnOfflineHost(t *testing.T) {
	env, _ := setup(t)
	log := &execLog{}
	id, stop := startStoppableAgent(t, env, "web", []string{protocol.CapExec}, log.fakeExec("web", 0))
	var s api.Script
	env.MustDo(http.MethodPost, "/scripts", api.ScriptInput{Name: "ps", Shell: "powershell", Body: "Get-Date", DefaultHostIds: &[]string{id}}, &s)
	stop()
	// The host is known but offline: the run is recorded with the reason.
	var runs []api.ScriptRun
	env.MustDo(http.MethodPost, fmt.Sprintf("/scripts/%d/run", s.Id), nil, &runs)
	r := waitRuns(t, env, runs)["web"]
	if r.Status != api.Failed || r.ExitCode != nil || r.Error != "代理不在线" {
		t.Fatalf("offline run: %+v", r)
	}
	// A revoked host is no longer a host.
	env.MustDo(http.MethodDelete, "/agents/"+id, nil, nil)
	expectStatus(t, env, http.MethodPost, fmt.Sprintf("/scripts/%d/run", s.Id), nil, 400, "validation_failed")
}
