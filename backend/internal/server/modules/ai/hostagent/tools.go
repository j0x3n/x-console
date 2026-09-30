package hostagent

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/agenthub"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

type Runner struct {
	Deps   *module.Deps
	HostID string
	// BackupBase replaces /tmp as the parent of the backup directory on
	// Linux and macOS. Tests set it; empty means /tmp.
	BackupBase string
}

func schema(fields map[string]string, required ...string) json.RawMessage {
	properties := map[string]any{"reason": map[string]any{"type": "string", "description": "为什么要做这一步"}}
	for name, description := range fields {
		properties[name] = map[string]any{"type": "string", "description": description}
	}
	if required == nil {
		required = []string{} // "required": null is rejected by the API
	}
	raw, _ := json.Marshal(map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false})
	return raw
}

func Tools() []llm.Tool {
	return []llm.Tool{
		{Name: "host__run_command", Description: "在当前机器执行一条命令。说明 reason；timeout 是秒，默认 60，最多 600。", Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"},"cwd":{"type":"string"},"timeout":{"type":"integer","minimum":1,"maximum":600},"reason":{"type":"string"}},"required":["command"],"additionalProperties":false}`)},
		{Name: "host__read_file", Description: "读取当前机器上的文本文件，最多 256 KB。说明 reason。", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"startLine":{"type":"integer","minimum":1},"endLine":{"type":"integer","minimum":1},"reason":{"type":"string"}},"required":["path"],"additionalProperties":false}`)},
		{Name: "host__write_file", Description: "写当前机器上的文件。覆盖前会备份原文件。说明 reason。", Parameters: schema(map[string]string{"path": "文件路径", "content": "新文件内容"}, "path", "content")},
		{Name: "host__list_dir", Description: "列当前机器上的目录，最多 500 条。说明 reason。", Parameters: schema(map[string]string{"path": "目录路径"}, "path")},
		{Name: "host__system_info", Description: "读取当前机器的系统和最新资源指标。说明 reason。", Parameters: schema(nil)},
		{Name: "host__list_processes", Description: "按 CPU 排序列出最多 50 个进程。说明 reason。", Parameters: schema(nil)},
		{Name: "host__list_services", Description: "列出服务，可用 filter 筛选名字。说明 reason。", Parameters: schema(map[string]string{"filter": "服务名筛选"})},
		{Name: "host__list_containers", Description: "列出容器。说明 reason。", Parameters: schema(nil)},
		{Name: "host__service_action", Description: "启动、停止或重启服务。说明 reason。", Parameters: schema(map[string]string{"name": "服务名", "action": "start、stop 或 restart"}, "name", "action")},
		{Name: "host__container_action", Description: "启动、停止或重启容器。说明 reason。", Parameters: schema(map[string]string{"id": "容器 ID", "action": "start、stop 或 restart"}, "id", "action")},
	}
}

func (r Runner) agent(ctx context.Context, capability string) (agenthub.Agent, error) {
	a, err := r.Deps.Agents.Get(ctx, r.HostID)
	if err != nil {
		return a, err
	}
	if !a.Online {
		return a, errors.New("机器离线")
	}
	if !a.Has(capability) {
		if capability == protocol.CapDocker {
			return a, errors.New("这台机器没有 Docker")
		}
		return a, fmt.Errorf("这台机器不支持 %s", capability)
	}
	return a, nil
}

func (r Runner) call(ctx context.Context, capability, method string, params, out any) error {
	if _, err := r.agent(ctx, capability); err != nil {
		return err
	}
	return r.Deps.Agents.Call(ctx, r.HostID, method, params, out)
}

func (r Runner) Run(ctx context.Context, name string, input json.RawMessage) (any, error) {
	var args struct {
		Command   string `json:"command"`
		Cwd       string `json:"cwd"`
		Timeout   int    `json:"timeout"`
		Path      string `json:"path"`
		Content   string `json:"content"`
		StartLine int    `json:"startLine"`
		EndLine   int    `json:"endLine"`
		Filter    string `json:"filter"`
		Name      string `json:"name"`
		ID        string `json:"id"`
		Action    string `json:"action"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("工具参数无效: %w", err)
	}
	switch name {
	case "host__run_command":
		if strings.TrimSpace(args.Command) == "" {
			return nil, errors.New("命令不能为空")
		}
		seconds := args.Timeout
		if seconds <= 0 {
			seconds = 60
		}
		if seconds > 600 {
			seconds = 600
		}
		if strings.HasPrefix(r.HostID, "ssh:") {
			hosts, ok := module.Lookup[contracts.Hosts](r.Deps.Registry, contracts.HostsKey)
			if !ok {
				return nil, errors.New("机器服务不可用")
			}
			out, err := hosts.Exec(ctx, r.HostID, args.Command, time.Duration(seconds)*time.Second)
			out.Stdout, out.Stderr = clipOutput(out.Stdout), clipOutput(out.Stderr)
			return out, err
		}
		var out protocol.ExecResult
		err := r.call(ctx, protocol.CapExec, protocol.MethodExecRun, protocol.ExecParams{Command: args.Command, Cwd: args.Cwd, TimeoutSeconds: seconds}, &out)
		out.Stdout, out.Stderr = clipOutput(out.Stdout), clipOutput(out.Stderr)
		return out, err
	case "host__read_file":
		if args.Path == "" {
			return nil, errors.New("文件路径不能为空")
		}
		data, size, err := r.readFile(ctx, args.Path, 256<<10)
		if err != nil {
			return nil, err
		}
		if bytes.IndexByte(data[:min(len(data), 8<<10)], 0) >= 0 {
			return "这是二进制文件", nil
		}
		content := string(data)
		if args.StartLine > 0 || args.EndLine > 0 {
			lines := strings.Split(content, "\n")
			start, end := max(args.StartLine, 1)-1, args.EndLine
			if end <= 0 || end > len(lines) {
				end = len(lines)
			}
			if start >= end {
				return "", nil
			}
			content = strings.Join(lines[start:end], "\n")
		}
		return map[string]any{"content": content, "size": size, "truncated": size > int64(len(data))}, nil
	case "host__write_file":
		if args.Path == "" {
			return nil, errors.New("文件路径不能为空")
		}
		if len(args.Content) > 1<<20 {
			return nil, errors.New("写入内容不能超过 1 MB")
		}
		backup, err := r.backup(ctx, args.Path)
		if err != nil {
			return nil, err
		}
		if err = r.writeFile(ctx, args.Path, []byte(args.Content)); err != nil {
			return nil, err
		}
		return map[string]any{"path": args.Path, "backupPath": backup, "bytes": len(args.Content)}, nil
	case "host__list_dir":
		var out protocol.FileList
		if err := r.call(ctx, protocol.CapFiles, protocol.MethodFilesList, protocol.FilesListParams{Path: args.Path}, &out); err != nil {
			return nil, err
		}
		if len(out.Entries) > 500 {
			out.Entries = out.Entries[:500]
		}
		return out, nil
	case "host__system_info":
		var out protocol.SystemInfo
		if err := r.call(ctx, protocol.CapSystemInfo, protocol.MethodSystemInfo, nil, &out); err != nil {
			return nil, err
		}
		var cpu, netRx, netTx float64
		var memUsed, memTotal uint64
		var disks string
		err := r.Deps.DB.QueryRowContext(ctx, "SELECT cpu,mem_used,mem_total,disk_json,net_rx,net_tx FROM host_metrics_1m WHERE host_id=? ORDER BY at DESC LIMIT 1", r.HostID).Scan(&cpu, &memUsed, &memTotal, &disks, &netRx, &netTx)
		result := map[string]any{"system": out}
		if err == nil {
			var disk any
			_ = json.Unmarshal([]byte(disks), &disk)
			result["metrics"] = map[string]any{"cpu": cpu, "memUsed": memUsed, "memTotal": memTotal, "disks": disk, "netRx": netRx, "netTx": netTx}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return result, nil
	case "host__list_processes":
		var out protocol.ProcessList
		err := r.call(ctx, protocol.CapProcesses, protocol.MethodProcList, protocol.ProcListParams{Sort: "cpu", Limit: 50}, &out)
		if len(out.Items) > 50 {
			out.Items = out.Items[:50]
		}
		return out, err
	case "host__list_services":
		var out protocol.ServiceList
		if err := r.call(ctx, protocol.CapServices, protocol.MethodSvcList, nil, &out); err != nil {
			return nil, err
		}
		if args.Filter != "" {
			filtered := out.Items[:0]
			for _, svc := range out.Items {
				if strings.Contains(strings.ToLower(svc.Name), strings.ToLower(args.Filter)) {
					filtered = append(filtered, svc)
				}
			}
			out.Items = filtered
		}
		return out, nil
	case "host__list_containers":
		var out protocol.DockerContainerList
		err := r.call(ctx, protocol.CapDocker, protocol.MethodDockerPS, protocol.DockerPSParams{All: true}, &out)
		return out, err
	case "host__service_action":
		if args.Name == "" || !validAction(args.Action) {
			return nil, errors.New("服务名或动作无效")
		}
		err := r.call(ctx, protocol.CapServices, protocol.MethodSvcAction, protocol.SvcActionParams{Name: args.Name, Action: args.Action}, nil)
		return map[string]any{"name": args.Name, "action": args.Action}, err
	case "host__container_action":
		if args.ID == "" || !validAction(args.Action) {
			return nil, errors.New("容器 ID 或动作无效")
		}
		err := r.call(ctx, protocol.CapDocker, protocol.MethodDockerAction, protocol.DockerActionParams{ID: args.ID, Action: args.Action}, nil)
		return map[string]any{"id": args.ID, "action": args.Action}, err
	}
	return nil, errors.New("未知的机器工具")
}

func validAction(action string) bool {
	return action == "start" || action == "stop" || action == "restart"
}

func clipOutput(value string) string {
	if len(value) <= 64<<10 {
		return value
	}
	return value[:64<<10] + "\n[输出已截断，最多显示 64 KB]"
}

func (r Runner) readFile(ctx context.Context, filePath string, limit int64) ([]byte, int64, error) {
	if _, err := r.agent(ctx, protocol.CapFiles); err != nil {
		return nil, 0, err
	}
	s, err := r.Deps.Agents.Open(ctx, r.HostID, protocol.MethodFilesRead, protocol.FilesReadParams{Path: filePath})
	if err != nil {
		return nil, 0, err
	}
	defer s.Close(nil)
	first, err := s.Recv(ctx)
	if err != nil {
		return nil, 0, err
	}
	var header protocol.FileHeader
	if err = json.Unmarshal(first, &header); err != nil {
		return nil, 0, err
	}
	if header.Size < 0 {
		return nil, 0, errors.New("代理返回的文件大小无效")
	}
	data := make([]byte, 0, min(header.Size, limit))
	for int64(len(data)) < limit {
		chunk, err := s.Recv(ctx)
		if errors.Is(err, io.EOF) {
			return data, header.Size, nil
		}
		if err != nil {
			return nil, 0, err
		}
		room := limit - int64(len(data))
		data = append(data, chunk[:min(int64(len(chunk)), room)]...)
	}
	return data, header.Size, nil
}

func (r Runner) writeFile(ctx context.Context, filePath string, data []byte) error {
	return r.write(ctx, filePath, data, false)
}

func (r Runner) write(ctx context.Context, filePath string, data []byte, private bool) error {
	if _, err := r.agent(ctx, protocol.CapFiles); err != nil {
		return err
	}
	s, err := r.Deps.Agents.Open(ctx, r.HostID, protocol.MethodFilesWrite, protocol.FilesWriteParams{Path: filePath, Size: int64(len(data)), Private: private})
	if err != nil {
		return err
	}
	defer s.Close(nil)
	for len(data) > 0 {
		n := min(len(data), protocol.FileChunkSize)
		if err := s.Send(ctx, data[:n]); err != nil {
			return err
		}
		data = data[n:]
	}
	ack, err := s.Recv(ctx)
	if err != nil {
		return err
	}
	var entry protocol.FileEntry
	return json.Unmarshal(ack, &entry)
}

func (r Runner) backup(ctx context.Context, filePath string) (string, error) {
	if _, err := r.agent(ctx, protocol.CapFiles); err != nil {
		return "", err
	}
	var entry protocol.FileEntry
	err := r.Deps.Agents.Call(ctx, r.HostID, protocol.MethodFilesStat, protocol.FilesPathParams{Path: filePath}, &entry)
	if err != nil {
		if agentCode(err, protocol.CodeNotFound) {
			return "", nil
		}
		return "", err
	}
	if entry.Size > 10<<20 {
		return "", errors.New("原文件超过 10 MB，无法安全备份")
	}
	a, err := r.Deps.Agents.Get(ctx, r.HostID)
	if err != nil {
		return "", err
	}
	// Old agents write the backup as 0644. That is only safe for a file
	// everyone can read already.
	private := a.Has(protocol.CapFilesPrivate)
	if !private && a.Os != "windows" && !otherReadable(entry.Mode) {
		return "", errors.New("这台机器的代理版本太旧，不能安全备份这个文件。请先更新代理")
	}
	original, size, err := r.readFile(ctx, filePath, 10<<20)
	if err != nil {
		return "", err
	}
	if size > int64(len(original)) {
		return "", errors.New("原文件读取不完整，已取消写入")
	}
	base := "/tmp"
	if r.BackupBase != "" {
		base = r.BackupBase
	}
	if a.Os == "windows" {
		if _, err = r.agent(ctx, protocol.CapExec); err != nil {
			return "", err
		}
		var temp protocol.ExecResult
		if err = r.Deps.Agents.Call(ctx, r.HostID, protocol.MethodExecRun, protocol.ExecParams{Command: "Write-Output $env:TEMP"}, &temp); err != nil {
			return "", err
		}
		if temp.ExitCode != 0 || strings.TrimSpace(temp.Stdout) == "" {
			return "", errors.New("无法获取 Windows 临时目录")
		}
		base = strings.TrimSpace(temp.Stdout)
	}
	sep := "/"
	if a.Os == "windows" {
		sep = "\\"
	}
	dir := strings.TrimRight(base, "/\\") + sep + "xc-agent-backup"
	var made protocol.FileEntry
	if err = r.Deps.Agents.Call(ctx, r.HostID, protocol.MethodFilesMkdir, protocol.FilesPathParams{Path: dir}, &made); err != nil {
		if !agentCode(err, protocol.CodeExists) {
			return "", err
		}
		// Anyone can create this path in /tmp first. A symlink would send
		// the backup (as root) wherever it points. files.stat follows links
		// and takes files only, so look the entry up in its parent.
		var list protocol.FileList
		if err = r.Deps.Agents.Call(ctx, r.HostID, protocol.MethodFilesList, protocol.FilesListParams{Path: base}, &list); err != nil {
			return "", err
		}
		found := false
		for _, e := range list.Entries {
			if e.Name == "xc-agent-backup" {
				found = e.Type == "dir"
				break
			}
		}
		if !found {
			return "", errors.New("备份目录 " + dir + " 不是普通目录，已取消写入")
		}
	}
	backupPath := dir + sep + time.Now().UTC().Format("20060102T150405.000000000") + "-" + path.Base(strings.ReplaceAll(filePath, "\\", "/"))
	if err = r.write(ctx, backupPath, original, private); err != nil {
		return "", err
	}
	return backupPath, nil
}

func agentCode(err error, code string) bool {
	var pe *protocol.Error
	if errors.As(err, &pe) && pe.Code == code {
		return true
	}
	var he *httpx.Error
	return errors.As(err, &he) && he.Code == "agent_"+code
}

// otherReadable reports whether a mode such as "-rw-r--r--" lets everyone read.
func otherReadable(mode string) bool {
	return len(mode) >= 10 && mode[len(mode)-3] == 'r'
}
