package hosts_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	agentexec "github.com/j0x3n/x-console/backend/internal/agent/exec"
	"github.com/j0x3n/x-console/backend/internal/agent/files"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/db"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

var serverCaps = []string{protocol.CapSystemInfo, protocol.CapMetrics, protocol.CapProcesses, protocol.CapServices,
	protocol.CapFiles, protocol.CapExec, protocol.CapPTY}

// fakeAgent records what the server asked for.
type fakeAgent struct {
	mu       sync.Mutex
	killed   []protocol.ProcKillParams
	svc      []protocol.SvcActionParams
	clip     string
	power    []string
	opened   []string
	ptyEnded chan struct{}
}

func (f *fakeAgent) register(c *conn.Client) {
	c.Handle(protocol.MethodSystemInfo, func(ctx context.Context, _ json.RawMessage) (any, error) {
		return protocol.SystemInfo{Hostname: "tokyo-1", OS: "linux", Platform: "ubuntu", PlatformVer: "24.04", CPUModel: "Fake CPU", CPUCores: 4, MemoryTotal: 8 << 30}, nil
	})
	c.Handle(protocol.MethodProcList, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.ProcListParams
		_ = json.Unmarshal(raw, &p)
		return protocol.ProcessList{Items: []protocol.ProcessInfo{{PID: 42, Name: "nginx", User: "www", CPU: 12.5, MemRSS: 1 << 20, Cmdline: "nginx -g daemon off; sort=" + p.Sort}}, Total: 1}, nil
	})
	c.Handle(protocol.MethodProcKill, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.ProcKillParams
		_ = json.Unmarshal(raw, &p)
		if p.PID == 404 {
			return nil, &protocol.Error{Code: protocol.CodeNotFound, Message: "no such process"}
		}
		f.mu.Lock()
		f.killed = append(f.killed, p)
		f.mu.Unlock()
		return nil, nil
	})
	c.Handle(protocol.MethodSvcList, func(ctx context.Context, _ json.RawMessage) (any, error) {
		return protocol.ServiceList{Items: []protocol.ServiceInfo{{Name: "nginx.service", Description: "web", State: "running", Enabled: true}}}, nil
	})
	c.Handle(protocol.MethodSvcAction, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.SvcActionParams
		_ = json.Unmarshal(raw, &p)
		f.mu.Lock()
		f.svc = append(f.svc, p)
		f.mu.Unlock()
		return nil, nil
	})
	c.Handle(protocol.MethodSvcLogs, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.SvcLogsParams
		_ = json.Unmarshal(raw, &p)
		return protocol.ServiceLogs{Lines: []string{fmt.Sprintf("%s: %d lines", p.Name, p.Lines)}}, nil
	})
	c.Handle(protocol.MethodClipboardGet, func(ctx context.Context, _ json.RawMessage) (any, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return protocol.Clipboard{Text: f.clip}, nil
	})
	c.Handle(protocol.MethodClipboardSet, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.Clipboard
		_ = json.Unmarshal(raw, &p)
		f.mu.Lock()
		f.clip = p.Text
		f.mu.Unlock()
		return nil, nil
	})
	c.Handle(protocol.MethodPowerAction, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.PowerParams
		_ = json.Unmarshal(raw, &p)
		f.mu.Lock()
		f.power = append(f.power, p.Action)
		f.mu.Unlock()
		return nil, nil
	})
	c.Handle(protocol.MethodAppOpen, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.AppOpenParams
		_ = json.Unmarshal(raw, &p)
		f.mu.Lock()
		f.opened = append(f.opened, p.Target)
		f.mu.Unlock()
		return nil, nil
	})
	// A fake terminal: echoes input and reports resizes as text.
	f.ptyEnded = make(chan struct{})
	c.HandleStream(protocol.MethodPTYOpen, func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
		defer close(f.ptyEnded)
		var p protocol.PTYOpenParams
		_ = json.Unmarshal(raw, &p)
		_ = s.Send(ctx, append([]byte{protocol.PTYFrameData}, fmt.Sprintf("welcome %dx%d\r\n", p.Cols, p.Rows)...))
		for {
			b, err := s.Recv(ctx)
			if err != nil {
				return nil
			}
			switch b[0] {
			case protocol.PTYFrameData:
				_ = s.Send(ctx, append([]byte{protocol.PTYFrameData}, b[1:]...))
			case protocol.PTYFrameResize:
				var r protocol.PTYResize
				_ = json.Unmarshal(b[1:], &r)
				_ = s.Send(ctx, append([]byte{protocol.PTYFrameData}, fmt.Sprintf("resized %dx%d", r.Cols, r.Rows)...))
			}
		}
	})
	files.Register(c)
	agentexec.Register(c)
}

func TestHostListDetailAndMetrics(t *testing.T) {
	env, m := setup(t)
	fa := &fakeAgent{}
	var client *conn.Client
	id, _, _ := startAgent(t, env, "tokyo-1", "server", serverCaps, func(c *conn.Client) {
		fa.register(c)
		client = c
	})
	events, cancel := env.App.Deps.Bus.Subscribe("host.metrics", 16)
	defer cancel()
	if err := client.Emit(context.Background(), protocol.EventMetrics, sample(42, 50, 100, 92)); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-events:
		p := ev.Data.(hosts.HostMetricsEvent)
		if p.HostID != id || p.Sample.Cpu != 42 {
			t.Fatalf("event: %+v", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no host.metrics event")
	}
	// A second sample right away is stored but not published (throttle).
	_ = client.Emit(context.Background(), protocol.EventMetrics, sample(43, 50, 100, 92))
	waitFor(t, "second sample", func() bool { x, _ := m.LatestSample(id); return x.CPU == 43 })
	select {
	case ev := <-events:
		t.Fatalf("unthrottled event %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}

	var hosts []api.Host
	env.MustDo(http.MethodGet, "/hosts", nil, &hosts)
	if len(hosts) != 1 || hosts[0].Id != id || !hosts[0].Online || hosts[0].Source != api.Agent || hosts[0].Metrics == nil {
		t.Fatalf("hosts: %+v", hosts)
	}
	if *hosts[0].Cpu != 43 || *hosts[0].Memory != 50 || *hosts[0].Disk != 92 {
		t.Fatalf("summary: cpu %v mem %v disk %v", *hosts[0].Cpu, *hosts[0].Memory, *hosts[0].Disk)
	}
	env.MustDo(http.MethodGet, "/hosts?kind=desktop", nil, &hosts)
	if len(hosts) != 0 {
		t.Fatalf("desktop filter: %+v", hosts)
	}

	var d api.HostDetail
	env.MustDo(http.MethodGet, "/hosts/"+id, nil, &d)
	if d.SystemInfo == nil || d.SystemInfo.CpuModel != "Fake CPU" || d.Name != "tokyo-1" {
		t.Fatalf("detail: %+v", d)
	}

	var s api.MetricsSeries
	env.MustDo(http.MethodGet, "/hosts/"+id+"/metrics?range=1h", nil, &s)
	if len(s.Points) != 2 || s.StepSeconds != 10 || s.Points[1].Cpu != 43 || s.Points[0].Disk != 92 {
		t.Fatalf("1h series: %+v", s)
	}
	expectStatus(t, env, http.MethodGet, "/hosts/"+id+"/metrics?range=2d", nil, http.StatusBadRequest, "")
	expectStatus(t, env, http.MethodGet, "/hosts/nope", nil, http.StatusNotFound, "not_found")
	expectStatus(t, env, http.MethodGet, "/hosts/ssh:99", nil, http.StatusNotFound, "not_found")

	// contracts.Hosts
	sums, err := m.Summaries(context.Background())
	if err != nil || len(sums) != 1 || sums[0].CPU != 43 || !sums[0].Online {
		t.Fatalf("summaries: %+v %v", sums, err)
	}
}

func TestRollupAndRanges(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	now := time.Now().UTC()
	minute := now.Truncate(time.Minute)
	prev := minute.Add(-time.Minute)
	for i, cpu := range []float64{10, 30} {
		x := sample(cpu, 40, 100, 50)
		x.At = prev.Add(time.Duration(10+i*10) * time.Second)
		m.AddSample("agent-x", x)
	}
	if err := m.Rollup(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := m.Queries().ListMetrics1m(ctx, db.ListMetrics1mParams{HostID: "agent-x", Since: prev.Add(-time.Second), Until: minute})
	if err != nil || len(rows) != 1 {
		t.Fatalf("1m rows: %+v %v", rows, err)
	}
	if rows[0].Cpu != 20 || rows[0].MemUsed != 40 || !strings.Contains(rows[0].DiskJson, `"/data"`) {
		t.Fatalf("1m row: %+v", rows[0])
	}
	// Idempotent.
	if err := m.Rollup(ctx); err != nil {
		t.Fatal(err)
	}
	h, err := m.Queries().ListMetrics1h(ctx, db.ListMetrics1hParams{HostID: "agent-x", Since: now.Add(-3 * time.Hour), Until: now.Add(time.Hour)})
	if err != nil || len(h) != 1 || h[0].Cpu != 20 {
		t.Fatalf("1h rows: %+v %v", h, err)
	}
	s, err := m.Series(ctx, "agent-x", "24h")
	if err != nil || len(s.Points) != 1 || s.Points[0].Cpu != 20 || s.Points[0].Disk != 50 || s.Points[0].Memory != 40 {
		t.Fatalf("24h: %+v %v", s, err)
	}
	s, err = m.Series(ctx, "agent-x", "7d")
	if err != nil || len(s.Points) != 1 {
		t.Fatalf("7d: %+v %v", s, err)
	}
	// Retention: very old rows go away, recent ones stay.
	_ = m.Queries().UpsertMetric1m(ctx, db.UpsertMetric1mParams{HostID: "agent-x", At: now.Add(-8 * 24 * time.Hour), DiskJson: "[]"})
	_ = m.Queries().UpsertMetric1h(ctx, db.UpsertMetric1hParams{HostID: "agent-x", At: now.Add(-91 * 24 * time.Hour), DiskJson: "[]"})
	if err := m.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	all, _ := m.Queries().ListMetrics1mWindow(ctx, db.ListMetrics1mWindowParams{Since: now.Add(-100 * 24 * time.Hour), Until: now})
	if len(all) != 1 {
		t.Fatalf("after cleanup: %d rows", len(all))
	}
	_ = env
}

func TestProcessesAndServices(t *testing.T) {
	env, _ := setup(t)
	fa := &fakeAgent{}
	id, _, _ := startAgent(t, env, "tokyo-1", "server", serverCaps, fa.register)
	relogin(t, env)

	var procs api.ProcessList
	env.MustDo(http.MethodGet, "/hosts/"+id+"/processes?sort=mem&limit=10", nil, &procs)
	if len(procs.Items) != 1 || procs.Items[0].Pid != 42 || !strings.HasSuffix(procs.Items[0].Cmdline, "sort=mem") {
		t.Fatalf("procs: %+v", procs)
	}
	path := "/hosts/" + id + "/processes/42/kill"
	expectStatus(t, env, http.MethodPost, path, map[string]string{"signal": "KILL"}, http.StatusForbidden, "elevation_required")
	env.Elevate()
	env.MustDo(http.MethodPost, path, map[string]string{"signal": "KILL"}, nil)
	env.MustDo(http.MethodPost, "/hosts/"+id+"/processes/43/kill", nil, nil) // body is optional
	expectStatus(t, env, http.MethodPost, "/hosts/"+id+"/processes/404/kill", nil, http.StatusNotFound, "not_found")
	fa.mu.Lock()
	if len(fa.killed) != 2 || fa.killed[0].PID != 42 || fa.killed[0].Signal != "KILL" {
		t.Fatalf("killed: %+v", fa.killed)
	}
	fa.mu.Unlock()

	var svcs api.ServiceList
	env.MustDo(http.MethodGet, "/hosts/"+id+"/services", nil, &svcs)
	if len(svcs.Items) != 1 || svcs.Items[0].Name != "nginx.service" || !svcs.Items[0].Enabled {
		t.Fatalf("services: %+v", svcs)
	}
	var logs api.ServiceLogs
	env.MustDo(http.MethodGet, "/hosts/"+id+"/services/nginx.service/logs?lines=50", nil, &logs)
	if len(logs.Lines) != 1 || logs.Lines[0] != "nginx.service: 50 lines" {
		t.Fatalf("logs: %+v", logs)
	}
	relogin(t, env)
	env.MustDo(http.MethodPost, "/hosts/"+id+"/services/nginx.service/restart", nil, nil)
	expectStatus(t, env, http.MethodPost, "/hosts/"+id+"/services/nginx.service/stop", nil, http.StatusForbidden, "elevation_required")
	expectStatus(t, env, http.MethodPost, "/hosts/"+id+"/services/nginx.service/mask", nil, http.StatusBadRequest, "")
	env.Elevate()
	env.MustDo(http.MethodPost, "/hosts/"+id+"/services/nginx.service/stop", nil, nil)
	fa.mu.Lock()
	if len(fa.svc) != 2 || fa.svc[1].Action != "stop" {
		t.Fatalf("service actions: %+v", fa.svc)
	}
	fa.mu.Unlock()

	var audit struct {
		Items []struct{ Action, Target string }
	}
	env.MustDo(http.MethodGet, "/audit?limit=200", nil, &audit)
	want := map[string]bool{"host.process.kill": false, "host.service.stop": false, "host.service.restart": false}
	for _, e := range audit.Items {
		if _, ok := want[e.Action]; ok && e.Target == id {
			want[e.Action] = true
		}
	}
	for a, ok := range want {
		if !ok {
			t.Fatalf("audit entry %s missing", a)
		}
	}
}

func TestFilesThroughRealAgentPackage(t *testing.T) {
	env, _ := setup(t)
	fa := &fakeAgent{}
	id, _, _ := startAgent(t, env, "tokyo-1", "server", serverCaps, fa.register)
	dir := t.TempDir()
	q := func(p string) string { return url.QueryEscape(filepath.Join(dir, p)) }

	var fe api.FileEntry
	env.MustDo(http.MethodPost, "/hosts/"+id+"/files/mkdir", map[string]string{"path": filepath.Join(dir, "sub")}, &fe)
	if fe.Type != "dir" {
		t.Fatalf("mkdir: %+v", fe)
	}
	data := make([]byte, 200_000)
	_, _ = rand.Read(data)
	resp, raw := rawDo(t, env, http.MethodPut, "/hosts/"+id+"/files/content?path="+q("sub/a.bin"), bytes.NewReader(data), int64(len(data)))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload: %d %s", resp.StatusCode, raw)
	}
	var list api.FileList
	env.MustDo(http.MethodGet, "/hosts/"+id+"/files?path="+q("sub"), nil, &list)
	if len(list.Entries) != 1 || list.Entries[0].Size != int64(len(data)) || list.Entries[0].Name != "a.bin" {
		t.Fatalf("list: %+v", list)
	}
	resp, raw = rawDo(t, env, http.MethodGet, "/hosts/"+id+"/files/content?path="+q("sub/a.bin"), nil, 0)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(raw, data) || !strings.Contains(resp.Header.Get("Content-Disposition"), "a.bin") {
		t.Fatalf("download: %d, %d bytes, %q", resp.StatusCode, len(raw), resp.Header.Get("Content-Disposition"))
	}
	resp, raw = rawDo(t, env, http.MethodGet, "/hosts/"+id+"/files/content?path="+q("missing"), nil, 0)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing download: %d %s", resp.StatusCode, raw)
	}
	env.MustDo(http.MethodPost, "/hosts/"+id+"/files/rename", map[string]string{"from": filepath.Join(dir, "sub", "a.bin"), "to": filepath.Join(dir, "b.bin")}, &fe)
	if _, err := os.Stat(filepath.Join(dir, "b.bin")); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, env, http.MethodPost, "/hosts/"+id+"/files/rename", map[string]string{"from": filepath.Join(dir, "b.bin"), "to": filepath.Join(dir, "sub")}, http.StatusConflict, "")
	env.MustDo(http.MethodDelete, "/hosts/"+id+"/files?path="+q("b.bin"), nil, nil)
	if _, err := os.Stat(filepath.Join(dir, "b.bin")); !os.IsNotExist(err) {
		t.Fatalf("still there: %v", err)
	}
	expectStatus(t, env, http.MethodGet, "/hosts/"+id+"/files?path=relative", nil, http.StatusBadRequest, "")

	// Without elevation: upload and delete are refused, listing works.
	relogin(t, env)
	resp, raw = rawDo(t, env, http.MethodPut, "/hosts/"+id+"/files/content?path="+q("c.bin"), bytes.NewReader([]byte("x")), 1)
	if resp.StatusCode != http.StatusForbidden || errCode(raw) != "elevation_required" {
		t.Fatalf("unelevated upload: %d %s", resp.StatusCode, raw)
	}
	expectStatus(t, env, http.MethodDelete, "/hosts/"+id+"/files?path="+q("sub"), nil, http.StatusForbidden, "elevation_required")
	env.MustDo(http.MethodGet, "/hosts/"+id+"/files?path="+q(""), nil, &list)
}

func TestExec(t *testing.T) {
	env, m := setup(t)
	fa := &fakeAgent{}
	id, _, _ := startAgent(t, env, "tokyo-1", "server", serverCaps, fa.register)
	var res api.ExecResult
	env.MustDo(http.MethodPost, "/hosts/"+id+"/exec", map[string]any{"command": "echo hi; exit 3", "timeoutSeconds": 5}, &res)
	if res.Stdout != "hi\n" || res.ExitCode != 3 {
		t.Fatalf("exec: %+v", res)
	}
	expectStatus(t, env, http.MethodPost, "/hosts/"+id+"/exec", map[string]any{"command": " "}, http.StatusBadRequest, "")
	relogin(t, env)
	expectStatus(t, env, http.MethodPost, "/hosts/"+id+"/exec", map[string]any{"command": "id"}, http.StatusForbidden, "elevation_required")
	// contracts.Hosts.Exec does not check elevation (callers do).
	out, err := m.Exec(context.Background(), id, "printf ok", 5*time.Second)
	if err != nil || out.Stdout != "ok" {
		t.Fatalf("contract exec: %+v %v", out, err)
	}
}

// wsCookie returns the session cookie header for WebSocket dials.
func wsHeader(env *testutil.Env) http.Header {
	u, _ := url.Parse(env.Server.URL)
	var parts []string
	for _, c := range env.Client.Jar.Cookies(u) {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return http.Header{"Cookie": []string{strings.Join(parts, "; ")}}
}

func readText(t *testing.T, ctx context.Context, ws *websocket.Conn, want string) {
	t.Helper()
	var got string
	for !strings.Contains(got, want) {
		typ, b, err := ws.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for %q (have %q): %v", want, got, err)
		}
		if typ != websocket.MessageBinary {
			t.Fatalf("unexpected text frame %q", b)
		}
		got += string(b)
	}
}

func TestTerminalWebSocket(t *testing.T) {
	env, _ := setup(t)
	fa := &fakeAgent{}
	id, _, _ := startAgent(t, env, "tokyo-1", "server", serverCaps, fa.register)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path := "/hosts/" + id + "/terminal?cols=120&rows=30"

	relogin(t, env)
	_, resp, err := websocket.Dial(ctx, env.WSURL(path), &websocket.DialOptions{HTTPHeader: wsHeader(env)})
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unelevated terminal: %v %+v", err, resp)
	}

	env.Elevate()
	ws, _, err := websocket.Dial(ctx, env.WSURL(path), &websocket.DialOptions{HTTPHeader: wsHeader(env)})
	if err != nil {
		t.Fatal(err)
	}
	readText(t, ctx, ws, "welcome 120x30")
	if err := ws.Write(ctx, websocket.MessageBinary, []byte("ls -la\r")); err != nil {
		t.Fatal(err)
	}
	readText(t, ctx, ws, "ls -la")
	if err := ws.Write(ctx, websocket.MessageText, []byte(`{"type":"resize","cols":100,"rows":40}`)); err != nil {
		t.Fatal(err)
	}
	readText(t, ctx, ws, "resized 100x40")
	// Closing the page ends the agent side.
	_ = ws.Close(websocket.StatusNormalClosure, "bye")
	select {
	case <-fa.ptyEnded:
	case <-time.After(5 * time.Second):
		t.Fatal("agent terminal still running after the browser left")
	}
	// Hosts without the pty capability are refused before the upgrade.
	other, _, _ := startAgent(t, env, "no-pty", "server", []string{protocol.CapMetrics}, nil)
	expectStatus(t, env, http.MethodGet, "/hosts/"+other+"/terminal", nil, http.StatusNotImplemented, "feature_unavailable")
}

func TestDesktopQuickActions(t *testing.T) {
	env, _ := setup(t)
	fa := &fakeAgent{}
	caps := append([]string{protocol.CapClipboard, protocol.CapPower, protocol.CapOpen}, serverCaps...)
	id, _, _ := startAgent(t, env, "my-pc", "desktop", caps, fa.register)
	relogin(t, env)
	env.MustDo(http.MethodPut, "/hosts/"+id+"/clipboard", map[string]string{"text": "hello 你好"}, nil)
	var clip api.Clipboard
	env.MustDo(http.MethodGet, "/hosts/"+id+"/clipboard", nil, &clip)
	if clip.Text != "hello 你好" {
		t.Fatalf("clipboard: %+v", clip)
	}
	env.MustDo(http.MethodPost, "/hosts/"+id+"/power", map[string]string{"action": "lock"}, nil)
	expectStatus(t, env, http.MethodPost, "/hosts/"+id+"/power", map[string]string{"action": "shutdown"}, http.StatusForbidden, "elevation_required")
	expectStatus(t, env, http.MethodPost, "/hosts/"+id+"/power", map[string]string{"action": "explode"}, http.StatusBadRequest, "")
	env.Elevate()
	env.MustDo(http.MethodPost, "/hosts/"+id+"/power", map[string]string{"action": "shutdown"}, nil)
	env.MustDo(http.MethodPost, "/hosts/"+id+"/open", map[string]string{"target": "https://example.com"}, nil)
	fa.mu.Lock()
	if strings.Join(fa.power, ",") != "lock,shutdown" || len(fa.opened) != 1 {
		t.Fatalf("power %v opened %v", fa.power, fa.opened)
	}
	fa.mu.Unlock()
	var hosts []api.Host
	env.MustDo(http.MethodGet, "/hosts?kind=desktop", nil, &hosts)
	if len(hosts) != 1 || hosts[0].Kind != api.Desktop {
		t.Fatalf("desktops: %+v", hosts)
	}
	// A server agent without the clipboard capability answers 501.
	srv, _, _ := startAgent(t, env, "srv", "server", serverCaps, fa.register)
	expectStatus(t, env, http.MethodGet, "/hosts/"+srv+"/clipboard", nil, http.StatusNotImplemented, "feature_unavailable")
}

func TestAlertRulesFireAndResolve(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	fa := &fakeAgent{}
	id, _, stop := startAgent(t, env, "tokyo-1", "server", serverCaps, fa.register)

	expectStatus(t, env, http.MethodPost, "/alert-rules", map[string]any{"metric": "cpu", "threshold": 150}, http.StatusBadRequest, "validation_failed")
	expectStatus(t, env, http.MethodPost, "/alert-rules", map[string]any{"metric": "temperature"}, http.StatusBadRequest, "")
	expectStatus(t, env, http.MethodPost, "/alert-rules", map[string]any{"metric": "cpu", "threshold": 80, "hostId": "nope"}, http.StatusBadRequest, "")
	var cpuRule, slowRule, offRule api.AlertRule
	env.MustDo(http.MethodPost, "/alert-rules", map[string]any{"metric": "cpu", "op": "gt", "threshold": 80, "durationSeconds": 0, "hostId": id}, &cpuRule)
	env.MustDo(http.MethodPost, "/alert-rules", map[string]any{"metric": "disk", "threshold": 90, "durationSeconds": 600}, &slowRule)
	env.MustDo(http.MethodPost, "/alert-rules", map[string]any{"metric": "offline", "durationSeconds": 0, "severity": "critical"}, &offRule)
	if !cpuRule.Enabled || cpuRule.Severity != api.Warning || offRule.Severity != api.Critical {
		t.Fatalf("rules: %+v %+v", cpuRule, offRule)
	}

	fired, cancel := env.App.Deps.Bus.Subscribe("host.alert.", 16)
	defer cancel()
	m.RecordSample(id, sample(95, 50, 100, 95))
	if err := m.EvaluateAlerts(ctx); err != nil {
		t.Fatal(err)
	}
	ev := <-fired
	if ev.Topic != "host.alert.fired" || ev.Data.(api.AlertEvent).Metric != api.AlertMetricCpu {
		t.Fatalf("event: %+v", ev)
	}
	// Evaluating again does not fire twice; the disk rule waits for its duration.
	_ = m.EvaluateAlerts(ctx)
	var alerts struct{ Items []api.AlertEvent }
	env.MustDo(http.MethodGet, "/alerts?active=true", nil, &alerts)
	if len(alerts.Items) != 1 || alerts.Items[0].HostName != "tokyo-1" || !strings.Contains(alerts.Items[0].Message, "95%") {
		t.Fatalf("active alerts: %+v", alerts.Items)
	}
	var notes struct {
		Items []struct{ Kind, Title, Priority, Link string }
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) != 1 || notes.Items[0].Kind != "host.alert" || notes.Items[0].Priority != "high" || notes.Items[0].Link != "/servers/"+id {
		t.Fatalf("notifications: %+v", notes.Items)
	}
	var hosts []api.Host
	env.MustDo(http.MethodGet, "/hosts", nil, &hosts)
	if hosts[0].ActiveAlerts != 1 {
		t.Fatalf("active alerts on host: %d", hosts[0].ActiveAlerts)
	}

	// Back to normal: resolved.
	m.RecordSample(id, sample(10, 50, 100, 50))
	_ = m.EvaluateAlerts(ctx)
	if ev := <-fired; ev.Topic != "host.alert.resolved" {
		t.Fatalf("event: %+v", ev)
	}
	env.MustDo(http.MethodGet, "/alerts?active=true", nil, &alerts)
	if len(alerts.Items) != 0 {
		t.Fatalf("still active: %+v", alerts.Items)
	}

	// Offline: the agent disconnects and the offline rule fires urgently.
	stop()
	if err := m.EvaluateAlerts(ctx); err != nil {
		t.Fatal(err)
	}
	ev = <-fired
	a := ev.Data.(api.AlertEvent)
	if ev.Topic != "host.alert.fired" || a.Metric != api.AlertMetricOffline || a.Severity != api.Critical {
		t.Fatalf("offline event: %+v", ev)
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) != 2 || notes.Items[0].Kind != "host.alert" || notes.Items[0].Priority != "urgent" || !strings.Contains(notes.Items[0].Title, "离线") {
		t.Fatalf("offline notification: %+v", notes.Items)
	}
	hs, _ := m.Alerts(ctx, time.Now().Add(-time.Hour))
	if len(hs) != 2 {
		t.Fatalf("contract alerts: %+v", hs)
	}

	// Deleting the rule resolves its open alert.
	env.MustDo(http.MethodDelete, fmt.Sprintf("/alert-rules/%d", offRule.Id), nil, nil)
	if ev := <-fired; ev.Topic != "host.alert.resolved" {
		t.Fatalf("event: %+v", ev)
	}
	expectStatus(t, env, http.MethodDelete, fmt.Sprintf("/alert-rules/%d", offRule.Id), nil, http.StatusNotFound, "not_found")

	// Update and list.
	var upd api.AlertRule
	env.MustDo(http.MethodPut, fmt.Sprintf("/alert-rules/%d", slowRule.Id), map[string]any{"metric": "disk", "threshold": 70, "enabled": false}, &upd)
	if upd.Enabled || upd.Threshold != 70 {
		t.Fatalf("update: %+v", upd)
	}
	var rules []api.AlertRule
	env.MustDo(http.MethodGet, "/alert-rules", nil, &rules)
	if len(rules) != 2 {
		t.Fatalf("rules: %+v", rules)
	}
}

func TestSSHHosts(t *testing.T) {
	env, m := setup(t)
	ctx := context.Background()
	fake := startFakeSSH(t, "s3cret")
	host, port, _ := strings.Cut(fake.addr, ":")
	var portN int
	fmt.Sscan(port, &portN)
	input := map[string]any{"name": "old-box", "address": host, "port": portN, "username": "root", "auth": "password", "secret": "s3cret"}

	relogin(t, env)
	expectStatus(t, env, http.MethodPost, "/ssh-hosts", input, http.StatusForbidden, "elevation_required")
	env.Elevate()
	expectStatus(t, env, http.MethodPost, "/ssh-hosts", map[string]any{"name": "x", "address": host, "username": "root", "auth": "password"}, http.StatusBadRequest, "validation_failed")

	var test api.SshTestResult
	env.MustDo(http.MethodPost, "/ssh-hosts/test", input, &test)
	if !test.Ok || test.Fingerprint == nil {
		t.Fatalf("test: %+v", test)
	}
	bad := map[string]any{"name": "old-box", "address": host, "port": portN, "username": "root", "auth": "password", "secret": "wrong"}
	env.MustDo(http.MethodPost, "/ssh-hosts/test", bad, &test)
	if test.Ok {
		t.Fatalf("wrong password accepted: %+v", test)
	}

	var sh api.SshHost
	env.MustDo(http.MethodPost, "/ssh-hosts", input, &sh)
	if sh.HostId != hosts.SSHHostID(sh.Id) || sh.HostKeyFingerprint != "" {
		t.Fatalf("created: %+v", sh)
	}
	// The secret is stored encrypted.
	row, _ := m.Queries().GetSSHHost(ctx, sh.Id)
	if strings.Contains(row.Secret, "s3cret") {
		t.Fatal("secret stored in plain text")
	}

	var res api.ExecResult
	env.MustDo(http.MethodPost, "/hosts/"+sh.HostId+"/exec", map[string]any{"command": "uptime"}, &res)
	if res.ExitCode != 0 || res.Stdout != "ran: uptime\n" {
		t.Fatalf("ssh exec: %+v", res)
	}
	env.MustDo(http.MethodPost, "/hosts/"+sh.HostId+"/exec", map[string]any{"command": "fail"}, &res)
	if res.ExitCode != 7 || res.Stderr != "boom\n" {
		t.Fatalf("ssh exec failure: %+v", res)
	}
	// Trust on first use: the key is remembered.
	env.MustDo(http.MethodGet, fmt.Sprintf("/ssh-hosts/%d", sh.Id), nil, &sh)
	if !strings.HasPrefix(sh.HostKeyFingerprint, "SHA256:") {
		t.Fatalf("fingerprint not stored: %+v", sh)
	}

	// Metrics over SSH.
	if err := m.PollSSH(ctx); err != nil {
		t.Fatal(err)
	}
	var d api.HostDetail
	env.MustDo(http.MethodGet, "/hosts/"+sh.HostId, nil, &d)
	if !d.Online || d.Source != api.Ssh || d.Metrics == nil || d.Metrics.MemTotal != 2000000*1024 || *d.Disk != 92 {
		t.Fatalf("ssh detail: %+v", d)
	}
	expectStatus(t, env, http.MethodGet, "/hosts/"+sh.HostId+"/processes", nil, http.StatusNotImplemented, "feature_unavailable")

	// Terminal over SSH (the fake shell answers in upper case).
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(wctx, env.WSURL("/hosts/"+sh.HostId+"/terminal"), &websocket.DialOptions{HTTPHeader: wsHeader(env)})
	if err != nil {
		t.Fatal(err)
	}
	_ = ws.Write(wctx, websocket.MessageText, []byte(`{"type":"resize","cols":90,"rows":20}`))
	_ = ws.Write(wctx, websocket.MessageBinary, []byte("hello"))
	readText(t, wctx, ws, "HELLO")
	_ = ws.Close(websocket.StatusNormalClosure, "")

	// A changed host key is refused.
	_ = m.Queries().SetSSHHostKey(ctx, db.SetSSHHostKeyParams{HostKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl", ID: sh.Id})
	m.DropSSH(sh.Id)
	expectStatus(t, env, http.MethodPost, "/hosts/"+sh.HostId+"/exec", map[string]any{"command": "uptime"}, http.StatusBadGateway, "ssh_host_key_changed")

	// Update without a secret keeps it; changing the address forgets the key.
	upd := map[string]any{"name": "renamed", "address": "localhost", "port": portN, "username": "root", "auth": "password"}
	env.MustDo(http.MethodPut, fmt.Sprintf("/ssh-hosts/%d", sh.Id), upd, &sh)
	if sh.Name != "renamed" || sh.HostKeyFingerprint != "" {
		t.Fatalf("updated: %+v", sh)
	}
	env.MustDo(http.MethodPost, fmt.Sprintf("/ssh-hosts/%d/test", sh.Id), nil, &test)
	if !test.Ok {
		t.Fatalf("saved test: %+v", test)
	}
	var list []api.SshHost
	env.MustDo(http.MethodGet, "/ssh-hosts", nil, &list)
	if len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/ssh-hosts/%d", sh.Id), nil, nil)
	expectStatus(t, env, http.MethodGet, "/hosts/"+sh.HostId, nil, http.StatusNotFound, "not_found")
}

func TestActions(t *testing.T) {
	env, _ := setup(t)
	fa := &fakeAgent{}
	caps := append([]string{protocol.CapClipboard, protocol.CapPower, protocol.CapOpen}, serverCaps...)
	pc, _, _ := startAgent(t, env, "my-pc", "desktop", caps, fa.register)
	srv, _, _ := startAgent(t, env, "tokyo-1", "server", serverCaps, fa.register)
	reg := env.App.Deps.Actions
	ctx := context.Background()
	run := func(name, input string) any {
		t.Helper()
		out, err := reg.Run(ctx, name, json.RawMessage(input))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	if hosts := run("hosts.list", `{"kind":"server"}`).([]api.Host); len(hosts) != 1 || hosts[0].Id != srv {
		t.Fatalf("hosts.list: %+v", hosts)
	}
	run("hosts.get_metrics", `{"host":"tokyo-1","range":"1h"}`)
	run("hosts.service_action", `{"host":"tokyo-1","name":"nginx.service","action":"restart"}`)
	if res := run("hosts.exec", `{"host":"`+srv+`","command":"echo from-action"}`).(api.ExecResult); res.Stdout != "from-action\n" {
		t.Fatalf("hosts.exec: %+v", res)
	}
	run("hosts.clipboard_set", `{"text":"copied"}`)
	run("hosts.power", `{"action":"lock"}`)
	run("hosts.open", `{"host":"my-pc","target":"notepad.exe"}`)
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if fa.clip != "copied" || len(fa.power) != 1 || fa.opened[0] != "notepad.exe" || fa.svc[0].Name != "nginx.service" {
		t.Fatalf("fake agent: %+v", fa)
	}
	if _, err := reg.Run(ctx, "hosts.exec", json.RawMessage(`{"host":"nowhere","command":"x"}`)); err == nil {
		t.Fatal("unknown host accepted")
	}
	_ = pc
}
