package hosts

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

const (
	defaultExecTimeout = 60 * time.Second
	maxExecTimeout     = 30 * time.Minute
)

// agentErr turns agent failures into API errors with useful statuses. The
// hub already maps the standard codes; the file codes are refined here.
func agentErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, rpc.ErrClosed) {
		return httpx.ErrAgentOffline
	}
	code, msg := "", ""
	var he *httpx.Error
	var pe *protocol.Error
	switch {
	case errors.As(err, &he):
		if !strings.HasPrefix(he.Code, "agent_") {
			return he
		}
		code, msg = strings.TrimPrefix(he.Code, "agent_"), he.Message
	case errors.As(err, &pe):
		code, msg = pe.Code, pe.Message
	case errors.Is(err, context.DeadlineExceeded):
		return httpx.NewError(http.StatusGatewayTimeout, "agent_timeout", "代理响应超时")
	default:
		return err
	}
	switch code {
	case protocol.CodeNotFound:
		return httpx.NewError(http.StatusNotFound, "not_found", msg)
	case protocol.CodePermission:
		return httpx.NewError(http.StatusForbidden, "permission_denied", "代理没有权限: "+msg)
	case protocol.CodeExists:
		return httpx.NewError(http.StatusConflict, "conflict", msg)
	case protocol.CodeSyslogPermission:
		return httpx.NewError(http.StatusForbidden, "syslog_permission", msg)
	case protocol.CodeUnsupported, protocol.CodeUnknownMethod:
		return httpx.NewError(http.StatusNotImplemented, "agent_"+code, msg)
	case protocol.CodeBadParams:
		return httpx.NewError(http.StatusBadRequest, "agent_bad_params", msg)
	case protocol.CodeTimeout:
		return httpx.NewError(http.StatusGatewayTimeout, "agent_timeout", msg)
	}
	return httpx.NewError(http.StatusBadGateway, "agent_"+code, msg)
}

// call runs method on an agent that has capability c.
func (m *Module) call(ctx context.Context, hostID, c, method string, params, out any) error {
	a, err := m.agentFor(ctx, hostID, c)
	if err != nil {
		return err
	}
	return agentErr(m.d.Agents.Call(ctx, a.ID, method, params, out))
}

// decodeOptional decodes a JSON body that may be empty.
func decodeOptional(r *http.Request, v any) error {
	if r.ContentLength == 0 {
		return nil
	}
	err := httpx.Decode(r, v)
	var he *httpx.Error
	if errors.As(err, &he) && strings.Contains(he.Message, io.EOF.Error()) {
		return nil
	}
	return err
}

// ---- processes ----

// ListProcesses is GET /hosts/{hostId}/processes.
func (m *Module) ListProcesses(w http.ResponseWriter, r *http.Request, hostID string, params api.ListProcessesParams) {
	p := protocol.ProcListParams{Limit: 200}
	if params.Sort != nil {
		p.Sort = string(*params.Sort)
	}
	if params.Limit != nil {
		p.Limit = *params.Limit
	}
	var out protocol.ProcessList
	if err := m.call(r.Context(), hostID, protocol.CapProcesses, protocol.MethodProcList, p, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := make([]api.Process, 0, len(out.Items))
	for _, x := range out.Items {
		items = append(items, api.Process{Pid: x.PID, Ppid: x.PPID, Name: x.Name, User: x.User, Cpu: x.CPU, MemRss: int64(x.MemRSS),
			MemPercent: x.MemPercent, Cmdline: x.Cmdline, StartedAt: x.StartedAt, Status: x.Status})
	}
	httpx.JSON(w, http.StatusOK, api.ProcessList{Items: items, Total: out.Total})
}

// KillProcess is POST /hosts/{hostId}/processes/{pid}/kill.
func (m *Module) KillProcess(w http.ResponseWriter, r *http.Request, hostID string, pid int32) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.KillProcessJSONRequestBody
	if err := decodeOptional(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p := protocol.ProcKillParams{PID: pid}
	if body.Signal != nil {
		p.Signal = *body.Signal
	}
	err := m.call(r.Context(), hostID, protocol.CapProcesses, protocol.MethodProcKill, p, nil)
	m.d.Audit.Record(r.Context(), "host.process.kill", hostID, map[string]any{"pid": pid, "signal": p.Signal}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("host.process.killed", map[string]any{"hostId": hostID, "pid": pid})
	httpx.NoContent(w)
}

// ---- services ----

// ListServices is GET /hosts/{hostId}/services.
func (m *Module) ListServices(w http.ResponseWriter, r *http.Request, hostID string) {
	var out protocol.ServiceList
	if err := m.call(r.Context(), hostID, protocol.CapServices, protocol.MethodSvcList, nil, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := make([]api.Service, 0, len(out.Items))
	for _, s := range out.Items {
		items = append(items, api.Service{Name: s.Name, Description: s.Description, State: s.State, SubState: s.SubState,
			Enabled: s.Enabled, StartType: s.StartType})
	}
	httpx.JSON(w, http.StatusOK, api.ServiceList{Items: items})
}

// serviceNeedsElevation lists the service actions that can take a machine's
// function away.
func serviceNeedsElevation(action string) bool {
	return action == protocol.SvcStop || action == protocol.SvcDisable
}

// serviceAction runs a service action; callers check elevation.
func (m *Module) serviceAction(ctx context.Context, hostID, name, action string) error {
	switch action {
	case protocol.SvcStart, protocol.SvcStop, protocol.SvcRestart, protocol.SvcEnable, protocol.SvcDisable:
	default:
		return httpx.Invalid("action 只能是 start、stop、restart、enable 或 disable")
	}
	if strings.TrimSpace(name) == "" {
		return httpx.Invalid("服务名不能为空")
	}
	cctx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	err := m.call(cctx, hostID, protocol.CapServices, protocol.MethodSvcAction, protocol.SvcActionParams{Name: name, Action: action}, nil)
	m.d.Audit.Record(ctx, "host.service."+action, hostID, map[string]any{"service": name}, err)
	if err == nil {
		m.d.Bus.Publish("host.service.changed", map[string]any{"hostId": hostID, "name": name, "action": action})
	}
	return err
}

// ServiceAction is POST /hosts/{hostId}/services/{name}/{action}.
func (m *Module) ServiceAction(w http.ResponseWriter, r *http.Request, hostID string, name string, action api.ServiceAction) {
	if serviceNeedsElevation(string(action)) {
		if err := auth.RequireElevated(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if err := m.serviceAction(r.Context(), hostID, name, string(action)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// GetServiceLogs is GET /hosts/{hostId}/services/{name}/logs.
func (m *Module) GetServiceLogs(w http.ResponseWriter, r *http.Request, hostID string, name string, params api.GetServiceLogsParams) {
	p := protocol.SvcLogsParams{Name: name, Lines: 200}
	if params.Lines != nil {
		p.Lines = *params.Lines
	}
	var out protocol.ServiceLogs
	if err := m.call(r.Context(), hostID, protocol.CapServices, protocol.MethodSvcLogs, p, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if out.Lines == nil {
		out.Lines = []string{}
	}
	httpx.JSON(w, http.StatusOK, api.ServiceLogs{Lines: out.Lines})
}

// ---- files ----

func toAPIEntry(e protocol.FileEntry) api.FileEntry {
	return api.FileEntry{Name: e.Name, Path: e.Path, Type: e.Type, Size: e.Size, ModTime: e.ModTime, Mode: e.Mode}
}

// ListFiles is GET /hosts/{hostId}/files.
func (m *Module) ListFiles(w http.ResponseWriter, r *http.Request, hostID string, params api.ListFilesParams) {
	p := protocol.FilesListParams{}
	if params.Path != nil {
		p.Path = *params.Path
	}
	var out protocol.FileList
	if err := m.call(r.Context(), hostID, protocol.CapFiles, protocol.MethodFilesList, p, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	entries := make([]api.FileEntry, 0, len(out.Entries))
	for _, e := range out.Entries {
		entries = append(entries, toAPIEntry(e))
	}
	httpx.JSON(w, http.StatusOK, api.FileList{Path: out.Path, Parent: out.Parent, Sep: out.Sep, Entries: entries})
}

// DeleteFile is DELETE /hosts/{hostId}/files.
func (m *Module) DeleteFile(w http.ResponseWriter, r *http.Request, hostID string, params api.DeleteFileParams) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p := protocol.FilesRemoveParams{Path: params.Path, Recursive: params.Recursive != nil && *params.Recursive}
	err := m.call(r.Context(), hostID, protocol.CapFiles, protocol.MethodFilesRemove, p, nil)
	m.d.Audit.Record(r.Context(), "host.file.delete", hostID, map[string]any{"path": p.Path, "recursive": p.Recursive}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// MakeDirectory is POST /hosts/{hostId}/files/mkdir.
func (m *Module) MakeDirectory(w http.ResponseWriter, r *http.Request, hostID string) {
	var body api.MakeDirectoryJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var out protocol.FileEntry
	err := m.call(r.Context(), hostID, protocol.CapFiles, protocol.MethodFilesMkdir, protocol.FilesPathParams{Path: body.Path}, &out)
	m.d.Audit.Record(r.Context(), "host.file.mkdir", hostID, map[string]any{"path": body.Path}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toAPIEntry(out))
}

// RenameFile is POST /hosts/{hostId}/files/rename.
func (m *Module) RenameFile(w http.ResponseWriter, r *http.Request, hostID string) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.RenameFileJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var out protocol.FileEntry
	err := m.call(r.Context(), hostID, protocol.CapFiles, protocol.MethodFilesRename, protocol.FilesRenameParams{From: body.From, To: body.To}, &out)
	m.d.Audit.Record(r.Context(), "host.file.rename", hostID, map[string]any{"from": body.From, "to": body.To}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPIEntry(out))
}

// ---- exec ----

func execTimeout(seconds *int) time.Duration {
	if seconds == nil || *seconds <= 0 {
		return defaultExecTimeout
	}
	return min(time.Duration(*seconds)*time.Second, maxExecTimeout)
}

// Exec implements contracts.Hosts. It does not check elevation; HTTP and
// action callers do. Every run is audited.
func (m *Module) Exec(ctx context.Context, hostID, command string, timeout time.Duration) (contracts.ExecResult, error) {
	res, err := m.exec(ctx, hostID, command, "", timeout)
	return contracts.ExecResult{ExitCode: res.ExitCode, Stdout: res.Stdout, Stderr: res.Stderr}, err
}

func (m *Module) exec(ctx context.Context, hostID, command, cwd string, timeout time.Duration) (api.ExecResult, error) {
	if strings.TrimSpace(command) == "" {
		return api.ExecResult{}, httpx.Invalid("命令不能为空")
	}
	if timeout <= 0 {
		timeout = defaultExecTimeout
	}
	timeout = min(timeout, maxExecTimeout)
	h, err := m.host(ctx, hostID)
	if err != nil {
		return api.ExecResult{}, err
	}
	var res api.ExecResult
	if h.ssh != nil {
		res, err = m.sshExec(ctx, *h.ssh, command, timeout)
	} else {
		var out protocol.ExecResult
		cctx, cancel := context.WithTimeout(ctx, timeout+15*time.Second)
		err = m.call(cctx, hostID, protocol.CapExec, protocol.MethodExecRun,
			protocol.ExecParams{Command: command, Cwd: cwd, TimeoutSeconds: int(timeout / time.Second)}, &out)
		cancel()
		res = api.ExecResult{ExitCode: out.ExitCode, Stdout: out.Stdout, Stderr: out.Stderr, TimedOut: out.TimedOut,
			Truncated: out.Truncated, DurationMs: out.DurationMs}
	}
	detail := map[string]any{"command": clip(command, 500)}
	if err == nil {
		detail["exitCode"] = res.ExitCode
	}
	m.d.Audit.Record(ctx, "host.exec", hostID, detail, err)
	return res, err
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

// ExecCommand is POST /hosts/{hostId}/exec.
func (m *Module) ExecCommand(w http.ResponseWriter, r *http.Request, hostID string) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.ExecCommandJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cwd := ""
	if body.Cwd != nil {
		cwd = *body.Cwd
	}
	res, err := m.exec(r.Context(), hostID, body.Command, cwd, execTimeout(body.TimeoutSeconds))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

// ---- desktop quick actions ----

// GetClipboard is GET /hosts/{hostId}/clipboard.
func (m *Module) GetClipboard(w http.ResponseWriter, r *http.Request, hostID string) {
	var out protocol.Clipboard
	if err := m.call(r.Context(), hostID, protocol.CapClipboard, protocol.MethodClipboardGet, nil, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, api.Clipboard{Text: out.Text})
}

func (m *Module) setClipboard(ctx context.Context, hostID, text string) error {
	if len(text) > 1<<20 {
		return httpx.Invalid("文字不能超过 1 MB")
	}
	err := m.call(ctx, hostID, protocol.CapClipboard, protocol.MethodClipboardSet, protocol.Clipboard{Text: text}, nil)
	m.d.Audit.Record(ctx, "host.clipboard.set", hostID, map[string]any{"length": utf8.RuneCountInString(text)}, err)
	return err
}

// SetClipboard is PUT /hosts/{hostId}/clipboard.
func (m *Module) SetClipboard(w http.ResponseWriter, r *http.Request, hostID string) {
	var body api.SetClipboardJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := m.setClipboard(r.Context(), hostID, body.Text); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func powerNeedsElevation(action string) bool {
	return action == protocol.PowerShutdown || action == protocol.PowerRestart
}

func (m *Module) power(ctx context.Context, hostID, action string) error {
	switch action {
	case protocol.PowerLock, protocol.PowerSleep, protocol.PowerShutdown, protocol.PowerRestart:
	default:
		return httpx.Invalid("action 只能是 lock、sleep、shutdown 或 restart")
	}
	err := m.call(ctx, hostID, protocol.CapPower, protocol.MethodPowerAction, protocol.PowerParams{Action: action}, nil)
	m.d.Audit.Record(ctx, "host.power."+action, hostID, nil, err)
	return err
}

// PowerAction is POST /hosts/{hostId}/power.
func (m *Module) PowerAction(w http.ResponseWriter, r *http.Request, hostID string) {
	var body api.PowerActionJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if powerNeedsElevation(string(body.Action)) {
		if err := auth.RequireElevated(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if err := m.power(r.Context(), hostID, string(body.Action)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) open(ctx context.Context, hostID, target, args string) error {
	if strings.TrimSpace(target) == "" {
		return httpx.Invalid("要打开的内容不能为空")
	}
	err := m.call(ctx, hostID, protocol.CapOpen, protocol.MethodAppOpen, protocol.AppOpenParams{Target: target, Args: args}, nil)
	m.d.Audit.Record(ctx, "host.open", hostID, map[string]any{"target": clip(target, 500)}, err)
	return err
}

// OpenOnHost is POST /hosts/{hostId}/open.
func (m *Module) OpenOnHost(w http.ResponseWriter, r *http.Request, hostID string) {
	var body api.OpenOnHostJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	args := ""
	if body.Args != nil {
		args = *body.Args
	}
	if err := m.open(r.Context(), hostID, body.Target, args); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}
