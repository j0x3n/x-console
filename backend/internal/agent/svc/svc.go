// Package svc manages system services: systemd units on Linux (through
// systemctl and journalctl) and the Service Control Manager on Windows.
package svc

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Available reports whether this system has a service manager the agent can
// talk to. cmd/agent announces protocol.CapServices only then.
func Available() bool { return available() }

// Register adds svc.list, svc.action and svc.logs.
func Register(c *conn.Client) {
	c.Handle(protocol.MethodSvcList, func(ctx context.Context, _ json.RawMessage) (any, error) {
		items, err := list(ctx)
		if err != nil {
			return nil, err
		}
		sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
		return protocol.ServiceList{Items: items}, nil
	})
	c.Handle(protocol.MethodSvcAction, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.SvcActionParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		if err := ValidateAction(p); err != nil {
			return nil, err
		}
		return nil, action(ctx, p.Name, p.Action)
	})
	c.Handle(protocol.MethodSvcLogs, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.SvcLogsParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		if !ValidName(p.Name) {
			return nil, rpcutil.BadParams("invalid service name")
		}
		if p.Lines <= 0 {
			p.Lines = 200
		}
		p.Lines = min(p.Lines, 5000)
		lines, err := logs(ctx, p.Name, p.Lines)
		if err != nil {
			return nil, err
		}
		return protocol.ServiceLogs{Lines: lines}, nil
	})
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9@._:\\+-][A-Za-z0-9@._:\\+ -]{0,254}$`)

// ValidName rejects names that could be read as command line options.
func ValidName(name string) bool {
	return nameRe.MatchString(name) && !strings.HasPrefix(name, "-")
}

// ValidateAction checks a svc.action request.
func ValidateAction(p protocol.SvcActionParams) error {
	if !ValidName(p.Name) {
		return rpcutil.BadParams("invalid service name")
	}
	switch p.Action {
	case protocol.SvcStart, protocol.SvcStop, protocol.SvcRestart, protocol.SvcEnable, protocol.SvcDisable:
		return nil
	}
	return rpcutil.BadParams("action must be start, stop, restart, enable or disable")
}

// SystemdState maps the ACTIVE column of systemctl to our state names.
func SystemdState(active string) string {
	switch active {
	case "active", "reloading":
		return "running"
	case "inactive":
		return "stopped"
	case "failed":
		return "failed"
	case "activating":
		return "starting"
	case "deactivating":
		return "stopping"
	}
	return "other"
}

// ParseUnits parses `systemctl list-units --type=service --all --no-legend --plain`.
func ParseUnits(out string) []protocol.ServiceInfo {
	var items []protocol.ServiceInfo
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "●"))
		f := strings.Fields(line)
		if len(f) < 4 || !strings.HasSuffix(f[0], ".service") {
			continue
		}
		desc := ""
		if len(f) > 4 {
			desc = strings.Join(f[4:], " ")
		}
		items = append(items, protocol.ServiceInfo{Name: f[0], Description: desc, State: SystemdState(f[2]), SubState: f[3]})
	}
	return items
}

// ParseUnitFiles parses `systemctl list-unit-files --type=service --no-legend`
// into name -> state (enabled, disabled, static, masked, ...).
func ParseUnitFiles(out string) map[string]string {
	states := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || !strings.HasSuffix(f[0], ".service") {
			continue
		}
		states[f[0]] = f[1]
	}
	return states
}

// MergeUnitFiles adds unit files that are not loaded (so disabled services
// can be started) and fills Enabled and StartType. Template units are skipped.
func MergeUnitFiles(units []protocol.ServiceInfo, files map[string]string) []protocol.ServiceInfo {
	seen := map[string]bool{}
	for i := range units {
		seen[units[i].Name] = true
		if st, ok := files[units[i].Name]; ok {
			units[i].StartType = st
			units[i].Enabled = strings.HasPrefix(st, "enabled")
		}
	}
	for name, st := range files {
		if seen[name] || strings.HasSuffix(name, "@.service") || st == "alias" {
			continue
		}
		units = append(units, protocol.ServiceInfo{Name: name, State: "stopped", SubState: "dead", StartType: st,
			Enabled: strings.HasPrefix(st, "enabled")})
	}
	return units
}

// Windows service states and start types (winsvc.h), kept here so the
// mapping is tested on every OS.
const (
	winStopped         = 1
	winStartPending    = 2
	winStopPending     = 3
	winRunning         = 4
	winContinuePending = 5
	winPausePending    = 6
	winPaused          = 7

	winBootStart   = 0
	winSystemStart = 1
	winAutoStart   = 2
	winDemandStart = 3
	winDisabled    = 4
)

// WindowsState maps SERVICE_STATUS.dwCurrentState.
func WindowsState(state uint32) (string, string) {
	switch state {
	case winStopped:
		return "stopped", "stopped"
	case winStartPending:
		return "starting", "start_pending"
	case winStopPending:
		return "stopping", "stop_pending"
	case winRunning:
		return "running", "running"
	case winContinuePending:
		return "starting", "continue_pending"
	case winPausePending:
		return "other", "pause_pending"
	case winPaused:
		return "other", "paused"
	}
	return "other", "unknown"
}

// WindowsStartType maps QUERY_SERVICE_CONFIG.dwStartType.
func WindowsStartType(t uint32, delayed bool) (name string, enabled bool) {
	switch t {
	case winBootStart:
		return "boot", true
	case winSystemStart:
		return "system", true
	case winAutoStart:
		if delayed {
			return "auto_delayed", true
		}
		return "auto", true
	case winDemandStart:
		return "manual", false
	case winDisabled:
		return "disabled", false
	}
	return "unknown", false
}
