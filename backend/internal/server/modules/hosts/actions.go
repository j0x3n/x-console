package hosts

import (
	"context"
	"encoding/json"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// registerActions adds the hosts actions for the assistant and automations
// (M12). Dangerous actions do not check elevation themselves; the caller
// confirms and elevates. Every run is audited by the shared methods.
func (m *Module) registerActions() {
	reg := m.d.Actions
	reg.Register(actions.Action{
		Name:        "hosts.list",
		Title:       "查看机器状态",
		Description: "List all machines (servers, the Windows PC, SSH-only hosts) with online state and latest CPU, memory and disk percent. `kind` filters by server or desktop.",
		Input:       actions.Schema(`{"type":"object","properties":{"kind":{"type":"string","enum":["server","desktop"]}},"additionalProperties":false}`),
		Effect:      actions.Read,
		Run:         m.actionList,
	})
	reg.Register(actions.Action{
		Name:        "hosts.get_metrics",
		Title:       "查看机器指标",
		Description: "Return the metric history of one machine. `host` is a host id or name, `range` is 1h (10s steps), 24h (5m steps) or 7d (1h steps). Points have cpu, memory and disk percent, network bytes/s and load1.",
		Input:       actions.Schema(`{"type":"object","properties":{"host":{"type":"string"},"range":{"type":"string","enum":["1h","24h","7d"]}},"required":["host"],"additionalProperties":false}`),
		Effect:      actions.Read,
		Run:         m.actionMetrics,
	})
	reg.Register(actions.Action{
		Name:        "hosts.service_action",
		Title:       "操作服务",
		Description: "Start, stop, restart, enable or disable a systemd unit or Windows service on a machine. `host` is a host id or name, `name` the service name such as nginx.service.",
		Input:       actions.Schema(`{"type":"object","properties":{"host":{"type":"string"},"name":{"type":"string"},"action":{"type":"string","enum":["start","stop","restart","enable","disable"]}},"required":["host","name","action"],"additionalProperties":false}`),
		Effect:      actions.Dangerous,
		Run:         m.actionService,
	})
	reg.Register(actions.Action{
		Name:        "hosts.exec",
		Title:       "在机器上执行命令",
		Description: "Run a shell command (sh on Linux, PowerShell on Windows) on a machine and wait for it. Returns exitCode, stdout and stderr (1 MB each at most). `timeoutSeconds` defaults to 60, at most 1800.",
		Input:       actions.Schema(`{"type":"object","properties":{"host":{"type":"string"},"command":{"type":"string"},"timeoutSeconds":{"type":"integer","minimum":1,"maximum":1800}},"required":["host","command"],"additionalProperties":false}`),
		Effect:      actions.Dangerous,
		Run:         m.actionExec,
	})
	reg.Register(actions.Action{
		Name:        "hosts.clipboard_set",
		Title:       "发送到电脑剪贴板",
		Description: "Put text on the clipboard of the Windows PC. `host` is optional; the first online PC with a clipboard is used.",
		Input:       actions.Schema(`{"type":"object","properties":{"host":{"type":"string"},"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
		Effect:      actions.Write,
		Run:         m.actionClipboard,
	})
	reg.Register(actions.Action{
		Name:        "hosts.power",
		Title:       "电脑锁屏、睡眠或关机",
		Description: "Lock, sleep, shut down or restart the Windows PC. `host` is optional; the first online PC is used.",
		Input:       actions.Schema(`{"type":"object","properties":{"host":{"type":"string"},"action":{"type":"string","enum":["lock","sleep","shutdown","restart"]}},"required":["action"],"additionalProperties":false}`),
		Effect:      actions.Dangerous,
		Run:         m.actionPower,
	})
	reg.Register(actions.Action{
		Name:        "hosts.open",
		Title:       "在电脑上打开",
		Description: "Open a program, file or URL on the Windows PC, like double-clicking it. `host` is optional; the first online PC is used.",
		Input:       actions.Schema(`{"type":"object","properties":{"host":{"type":"string"},"target":{"type":"string"},"args":{"type":"string"}},"required":["target"],"additionalProperties":false}`),
		Effect:      actions.Write,
		Run:         m.actionOpen,
	})
}

func decodeInput(raw json.RawMessage, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return httpx.Invalid("参数格式不对: " + err.Error())
	}
	return nil
}

// desktopFor resolves an optional host reference; empty means the first
// online agent with capability c.
func (m *Module) desktopFor(ctx context.Context, ref, c string) (string, error) {
	if ref != "" {
		return m.resolveRef(ctx, ref)
	}
	hosts, err := m.listHosts(ctx, "")
	if err != nil {
		return "", err
	}
	for _, h := range hosts {
		if !h.Online {
			continue
		}
		for _, have := range h.Capabilities {
			if have == c {
				return h.Id, nil
			}
		}
	}
	return "", httpx.ErrAgentOffline
}

func (m *Module) actionList(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Kind string `json:"kind"`
	}
	if err := decodeInput(raw, &in); err != nil {
		return nil, err
	}
	return m.listHosts(ctx, in.Kind)
}

func (m *Module) actionMetrics(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Host  string `json:"host"`
		Range string `json:"range"`
	}
	if err := decodeInput(raw, &in); err != nil {
		return nil, err
	}
	id, err := m.resolveRef(ctx, in.Host)
	if err != nil {
		return nil, err
	}
	return m.series(ctx, id, in.Range)
}

func (m *Module) actionService(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Host   string `json:"host"`
		Name   string `json:"name"`
		Action string `json:"action"`
	}
	if err := decodeInput(raw, &in); err != nil {
		return nil, err
	}
	id, err := m.resolveRef(ctx, in.Host)
	if err != nil {
		return nil, err
	}
	if err := m.serviceAction(ctx, id, in.Name, in.Action); err != nil {
		return nil, err
	}
	return map[string]string{"hostId": id, "name": in.Name, "action": in.Action}, nil
}

func (m *Module) actionExec(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Host           string `json:"host"`
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeoutSeconds"`
	}
	if err := decodeInput(raw, &in); err != nil {
		return nil, err
	}
	id, err := m.resolveRef(ctx, in.Host)
	if err != nil {
		return nil, err
	}
	return m.exec(ctx, id, in.Command, "", time.Duration(in.TimeoutSeconds)*time.Second)
}

func (m *Module) actionClipboard(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Host string `json:"host"`
		Text string `json:"text"`
	}
	if err := decodeInput(raw, &in); err != nil {
		return nil, err
	}
	id, err := m.desktopFor(ctx, in.Host, protocol.CapClipboard)
	if err != nil {
		return nil, err
	}
	if err := m.setClipboard(ctx, id, in.Text); err != nil {
		return nil, err
	}
	return map[string]string{"hostId": id}, nil
}

func (m *Module) actionPower(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Host   string `json:"host"`
		Action string `json:"action"`
	}
	if err := decodeInput(raw, &in); err != nil {
		return nil, err
	}
	id, err := m.desktopFor(ctx, in.Host, protocol.CapPower)
	if err != nil {
		return nil, err
	}
	if err := m.power(ctx, id, in.Action); err != nil {
		return nil, err
	}
	return map[string]string{"hostId": id, "action": in.Action}, nil
}

func (m *Module) actionOpen(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Host   string `json:"host"`
		Target string `json:"target"`
		Args   string `json:"args"`
	}
	if err := decodeInput(raw, &in); err != nil {
		return nil, err
	}
	id, err := m.desktopFor(ctx, in.Host, protocol.CapOpen)
	if err != nil {
		return nil, err
	}
	if err := m.open(ctx, id, in.Target, in.Args); err != nil {
		return nil, err
	}
	return map[string]string{"hostId": id}, nil
}
