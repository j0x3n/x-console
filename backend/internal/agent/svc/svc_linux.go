//go:build linux

package svc

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func available() bool {
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return false
	}
	_, err := exec.LookPath("systemctl")
	return err == nil
}

func run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	cmd.Env = append(os.Environ(), "SYSTEMD_COLORS=0", "LC_ALL=C")
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		if strings.Contains(msg, "not found") || strings.Contains(msg, "not loaded") {
			return "", &protocol.Error{Code: protocol.CodeNotFound, Message: msg}
		}
		if strings.Contains(msg, "Access denied") || strings.Contains(msg, "authentication required") {
			return "", &protocol.Error{Code: protocol.CodePermission, Message: msg}
		}
		return "", rpcutil.Failed("%s", msg)
	}
	return out.String(), nil
}

func list(ctx context.Context) ([]protocol.ServiceInfo, error) {
	if !available() {
		return nil, rpcutil.Unsupported("systemd")
	}
	units, err := run(ctx, 20*time.Second, "systemctl", "list-units", "--type=service", "--all", "--no-legend", "--no-pager", "--plain")
	if err != nil {
		return nil, err
	}
	files, err := run(ctx, 20*time.Second, "systemctl", "list-unit-files", "--type=service", "--no-legend", "--no-pager")
	if err != nil {
		return nil, err
	}
	return MergeUnitFiles(ParseUnits(units), ParseUnitFiles(files)), nil
}

func action(ctx context.Context, name, act string) error {
	if !available() {
		return rpcutil.Unsupported("systemd")
	}
	_, err := run(ctx, 90*time.Second, "systemctl", act, "--no-ask-password", "--", name)
	return err
}

func logs(ctx context.Context, name string, lines int) ([]string, error) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return nil, rpcutil.Unsupported("journalctl")
	}
	out, err := run(ctx, 20*time.Second, "journalctl", "--no-pager", "-o", "short-iso", "-n", strconv.Itoa(lines), "-u", name)
	if err != nil {
		return nil, err
	}
	res := []string{}
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if l != "" && l != "-- No entries --" {
			res = append(res, l)
		}
	}
	return res, nil
}
