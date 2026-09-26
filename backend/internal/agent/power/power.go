// Package power runs the desktop quick actions: power.action (lock, sleep,
// shutdown, restart) and app.open (start a program, file or URL). Only the
// Windows desktop agent supports them.
package power

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Available reports whether power actions and app.open work here.
func Available() bool { return available() }

// Register adds power.action and app.open.
func Register(c *conn.Client) {
	c.Handle(protocol.MethodPowerAction, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.PowerParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		if err := ValidateAction(p.Action); err != nil {
			return nil, err
		}
		return nil, doPower(p.Action)
	})
	c.Handle(protocol.MethodAppOpen, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.AppOpenParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		if err := ValidateOpen(p); err != nil {
			return nil, err
		}
		return nil, open(strings.TrimSpace(p.Target), p.Args)
	})
}

// ValidateAction accepts the four power actions.
func ValidateAction(a string) error {
	switch a {
	case protocol.PowerLock, protocol.PowerSleep, protocol.PowerShutdown, protocol.PowerRestart:
		return nil
	}
	return rpcutil.BadParams("action must be lock, sleep, shutdown or restart")
}

// ValidateOpen checks an app.open request.
func ValidateOpen(p protocol.AppOpenParams) error {
	t := strings.TrimSpace(p.Target)
	if t == "" {
		return rpcutil.BadParams("target is required")
	}
	if len(t) > 2048 || len(p.Args) > 8192 || strings.ContainsAny(t, "\x00\r\n") {
		return rpcutil.BadParams("invalid target")
	}
	return nil
}

// ShutdownArgs are the shutdown.exe arguments for shutdown and restart. The
// short delay lets the agent answer the request before the network goes.
func ShutdownArgs(action string) []string {
	flag := "/s"
	if action == protocol.PowerRestart {
		flag = "/r"
	}
	return []string{flag, "/t", "3", "/c", "X Console"}
}
