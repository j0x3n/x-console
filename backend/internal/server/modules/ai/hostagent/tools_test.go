package hostagent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	agentexec "github.com/j0x3n/x-console/backend/internal/agent/exec"
	agentfiles "github.com/j0x3n/x-console/backend/internal/agent/files"
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

func TestReadWriteFileBacksUpOriginal(t *testing.T) {
	env := testutil.New(t)
	filePath := filepath.Join(t.TempDir(), "host-agent.txt")
	if err := os.WriteFile(filePath, []byte("原内容"), 0600); err != nil {
		t.Fatal(err)
	}
	kind := "server"
	if runtime.GOOS == "windows" {
		kind = "desktop"
	}
	hostID := env.Agent(kind, []string{protocol.CapFiles, protocol.CapFilesPrivate, protocol.CapExec}, func(c *conn.Client) {
		agentfiles.Register(c)
		agentexec.Register(c)
	})
	runner := hostagent.Runner{Deps: env.App.Deps, HostID: hostID}
	readArgs, _ := json.Marshal(map[string]any{"path": filePath})
	read, err := runner.Run(context.Background(), "host__read_file", readArgs)
	if err != nil {
		t.Fatal(err)
	}
	if read.(map[string]any)["content"] != "原内容" {
		t.Fatalf("read: %+v", read)
	}
	writeArgs, _ := json.Marshal(map[string]any{"path": filePath, "content": "新内容"})
	written, err := runner.Run(context.Background(), "host__write_file", writeArgs)
	if err != nil {
		t.Fatal(err)
	}
	backupPath := written.(map[string]any)["backupPath"].(string)
	if backupPath == "" {
		t.Fatal("backup path empty")
	}
	t.Cleanup(func() { _ = os.Remove(backupPath) })
	backup, err := os.ReadFile(backupPath)
	if err != nil || string(backup) != "原内容" {
		t.Fatalf("backup=%q err=%v", backup, err)
	}
	if info, err := os.Stat(backupPath); runtime.GOOS != "windows" && (err != nil || info.Mode().Perm() != 0o600) {
		t.Fatalf("backup mode: %v %v", info.Mode(), err)
	}
	current, err := os.ReadFile(filePath)
	if err != nil || string(current) != "新内容" {
		t.Fatalf("current=%q err=%v", current, err)
	}
}

func TestOldAgentBacksUpOnlyPublicFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps backups in the user's own TEMP")
	}
	env := testutil.New(t)
	dir := t.TempDir()
	hostID := env.Agent("server", []string{protocol.CapFiles, protocol.CapExec}, func(c *conn.Client) {
		agentfiles.Register(c)
		agentexec.Register(c)
	})
	runner := hostagent.Runner{Deps: env.App.Deps, HostID: hostID}
	for _, tc := range []struct {
		mode os.FileMode
		ok   bool
	}{{0o600, false}, {0o644, true}} {
		filePath := filepath.Join(dir, tc.mode.String()+".txt")
		if err := os.WriteFile(filePath, []byte("原内容"), tc.mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filePath, tc.mode); err != nil {
			t.Fatal(err)
		}
		args, _ := json.Marshal(map[string]any{"path": filePath, "content": "新内容"})
		written, err := runner.Run(context.Background(), "host__write_file", args)
		if tc.ok {
			if err != nil {
				t.Fatalf("%v: %v", tc.mode, err)
			}
			t.Cleanup(func() { _ = os.Remove(written.(map[string]any)["backupPath"].(string)) })
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "代理版本太旧") {
			t.Fatalf("%v: %v", tc.mode, err)
		}
		if current, _ := os.ReadFile(filePath); string(current) != "原内容" {
			t.Fatalf("written without backup: %q", current)
		}
	}
}

func TestBackupRefusesSymlinkedDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("backups go to the user's own TEMP")
	}
	base, target := t.TempDir(), t.TempDir()
	if err := os.Symlink(target, filepath.Join(base, "xc-agent-backup")); err != nil {
		t.Fatal(err)
	}
	env := testutil.New(t)
	hostID := env.Agent("server", []string{protocol.CapFiles, protocol.CapFilesPrivate, protocol.CapExec}, func(c *conn.Client) {
		agentfiles.Register(c)
		agentexec.Register(c)
	})
	filePath := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(filePath, []byte("原内容"), 0o600); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"path": filePath, "content": "新内容"})
	_, err := hostagent.Runner{Deps: env.App.Deps, HostID: hostID, BackupBase: base}.Run(context.Background(), "host__write_file", args)
	if err == nil || !strings.Contains(err.Error(), "不是普通目录") {
		t.Fatalf("symlinked backup directory: %v", err)
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Fatalf("backup written through the symlink: %v", entries)
	}
	if current, _ := os.ReadFile(filePath); string(current) != "原内容" {
		t.Fatalf("written without backup: %q", current)
	}
}

func TestToolSchemasHaveNoNulls(t *testing.T) {
	for _, tool := range hostagent.Tools() {
		var schema map[string]any
		if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		// OpenAI compatible APIs reject null where they expect an array.
		if required, ok := schema["required"]; ok {
			if _, isArray := required.([]any); !isArray {
				t.Fatalf("%s: required is %v", tool.Name, required)
			}
		}
		if strings.Contains(string(tool.Parameters), "null") {
			t.Fatalf("%s: null in schema: %s", tool.Name, tool.Parameters)
		}
	}
}
