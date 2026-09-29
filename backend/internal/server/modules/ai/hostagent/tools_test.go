package hostagent_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/hostagent"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestToolsUseCurrentAgentAndCapabilities(t *testing.T) {
	env := testutil.New(t)
	hostID := env.Agent("server", []string{protocol.CapExec, protocol.CapProcesses}, func(c *conn.Client) {
		c.Handle(protocol.MethodExecRun, func(_ context.Context, raw json.RawMessage) (any, error) {
			var p protocol.ExecParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			return protocol.ExecResult{ExitCode: 0, Stdout: p.Command + strings.Repeat("x", 70<<10)}, nil
		})
		c.Handle(protocol.MethodProcList, func(_ context.Context, raw json.RawMessage) (any, error) {
			var p protocol.ProcListParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			if p.Sort != "cpu" || p.Limit != 50 {
				t.Errorf("process params: %+v", p)
			}
			return protocol.ProcessList{Items: []protocol.ProcessInfo{{PID: 1, Name: "init"}}, Total: 1}, nil
		})
	})
	runner := hostagent.Runner{Deps: env.App.Deps, HostID: hostID}
	if len(hostagent.Tools()) != 10 {
		t.Fatalf("tool count: %d", len(hostagent.Tools()))
	}
	value, err := runner.Run(context.Background(), "host__run_command", json.RawMessage(`{"command":"df -h","timeout":900}`))
	if err != nil {
		t.Fatal(err)
	}
	result := value.(protocol.ExecResult)
	if len(result.Stdout) <= 64<<10 || !strings.Contains(result.Stdout, "已截断") {
		t.Fatalf("output clipping: %d bytes", len(result.Stdout))
	}
	if _, err = runner.Run(context.Background(), "host__list_processes", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = runner.Run(context.Background(), "host__list_containers", json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "没有 Docker") {
		t.Fatalf("capability error: %v", err)
	}
}
