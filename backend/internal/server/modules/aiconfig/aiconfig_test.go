package aiconfig_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	agentconfig "github.com/j0x3n/x-console/backend/internal/agent/aiconfig"
	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

type hostView struct {
	ID, Name          string
	Online, Supported bool
}

type state struct {
	Claude, Codex protocol.AIConfigTool
	HostIDs       []string
	Hosts         []hostView
	UpdatedAt     *string
}

type hostStatus struct {
	HostID, Name, State, Error string
	Items                      []protocol.AIConfigItem
}

type statusOut struct{ Hosts []hostStatus }

func save(t *testing.T, env *testutil.Env, body map[string]any) state {
	t.Helper()
	var s state
	env.MustDo(http.MethodPut, "/aiconfig", body, &s)
	return s
}

// scripted is a fake agent: it records what it was asked and answers from a
// function.
type scripted struct {
	mu     sync.Mutex
	params []protocol.AIConfigSyncParams
	reply  func(p protocol.AIConfigSyncParams) (protocol.AIConfigSyncResult, error)
}

func (s *scripted) register(c *conn.Client) {
	c.Handle(protocol.MethodAIConfigSync, func(_ context.Context, raw json.RawMessage) (any, error) {
		var p protocol.AIConfigSyncParams
		_ = json.Unmarshal(raw, &p)
		s.mu.Lock()
		s.params = append(s.params, p)
		s.mu.Unlock()
		return s.reply(p)
	})
}

func (s *scripted) calls() []protocol.AIConfigSyncParams {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]protocol.AIConfigSyncParams{}, s.params...)
}

func item(tool, it, state, reason string) protocol.AIConfigItem {
	return protocol.AIConfigItem{Tool: tool, Item: it, State: state, Reason: reason}
}

var caps = []string{protocol.CapSystemInfo, protocol.CapAIConfig}

func TestSaveNormalizesAndValidates(t *testing.T) {
	env := testutil.New(t)
	var s state
	env.MustDo(http.MethodGet, "/aiconfig", nil, &s)
	if s.UpdatedAt != nil || len(s.HostIDs) != 0 || len(s.Hosts) != 0 {
		t.Fatalf("fresh state: %+v", s)
	}

	plain := env.Agent("server", []string{protocol.CapSystemInfo}, nil)
	good := env.Agent("server", caps, (&scripted{reply: func(protocol.AIConfigSyncParams) (protocol.AIConfigSyncResult, error) {
		return protocol.AIConfigSyncResult{}, nil
	}}).register)
	s = save(t, env, map[string]any{
		"claude": map[string]any{
			"rules": "  用中文回答。\r\n  ",
			"allow": []string{" Bash(git status) ", "Bash(git status)", "", "Bash(ls)"},
			"mcp":   []map[string]any{{"name": "fs", "transport": "stdio", "command": " npx ", "args": []string{"-y", " ", "pkg"}}},
		},
		"codex":   map[string]any{"rules": ""},
		"hostIds": []string{good, plain, good},
	})
	if s.Claude.Rules != "用中文回答。" || len(s.Claude.Allow) != 2 || s.Claude.MCP[0].Command != "npx" || len(s.Claude.MCP[0].Args) != 2 {
		t.Fatalf("not normalized: %+v", s.Claude)
	}
	if len(s.HostIDs) != 2 || s.UpdatedAt == nil {
		t.Fatalf("hosts or time: %+v", s)
	}
	supported := map[string]bool{}
	for _, h := range s.Hosts {
		supported[h.ID] = h.Supported
	}
	if supported[good] != true || supported[plain] != false {
		t.Fatalf("supported: %+v", s.Hosts)
	}
	var back state
	env.MustDo(http.MethodGet, "/aiconfig", nil, &back)
	if back.Claude.Rules != s.Claude.Rules || len(back.HostIDs) != 2 {
		t.Fatalf("not stored: %+v", back)
	}

	bad := map[string]map[string]any{
		"bad name":     {"claude": map[string]any{"rules": "", "mcp": []map[string]any{{"name": "a b", "transport": "stdio", "command": "x"}}}, "codex": map[string]any{"rules": ""}, "hostIds": []string{}},
		"duplicate":    {"claude": map[string]any{"rules": "", "mcp": []map[string]any{{"name": "a", "transport": "stdio", "command": "x"}, {"name": "a", "transport": "stdio", "command": "y"}}}, "codex": map[string]any{"rules": ""}, "hostIds": []string{}},
		"no command":   {"claude": map[string]any{"rules": "", "mcp": []map[string]any{{"name": "a", "transport": "stdio"}}}, "codex": map[string]any{"rules": ""}, "hostIds": []string{}},
		"bad url":      {"claude": map[string]any{"rules": "", "mcp": []map[string]any{{"name": "a", "transport": "http", "url": "javascript:x"}}}, "codex": map[string]any{"rules": ""}, "hostIds": []string{}},
		"codex perms":  {"claude": map[string]any{"rules": ""}, "codex": map[string]any{"rules": "", "allow": []string{"Bash(ls)"}}, "hostIds": []string{}},
		"newline rule": {"claude": map[string]any{"rules": "", "deny": []string{"a\nb"}}, "codex": map[string]any{"rules": ""}, "hostIds": []string{}},
		"marker":       {"claude": map[string]any{"rules": "x-console:end"}, "codex": map[string]any{"rules": ""}, "hostIds": []string{}},
		"unknown host": {"claude": map[string]any{"rules": ""}, "codex": map[string]any{"rules": ""}, "hostIds": []string{"nope"}},
		"too long":     {"claude": map[string]any{"rules": strings.Repeat("x", 64*1024+1)}, "codex": map[string]any{"rules": ""}, "hostIds": []string{}},
	}
	for name, body := range bad {
		if status, raw := env.Do(http.MethodPut, "/aiconfig", body, nil); status != http.StatusBadRequest {
			t.Errorf("%s: status %d: %s", name, status, raw)
		}
	}
	// a rejected save changes nothing
	env.MustDo(http.MethodGet, "/aiconfig", nil, &back)
	if back.Claude.Rules != "用中文回答。" {
		t.Fatalf("a rejected save changed the config: %+v", back.Claude)
	}
}

func TestStatusStatesPerMachine(t *testing.T) {
	env := testutil.New(t)
	answer := func(items ...protocol.AIConfigItem) func(protocol.AIConfigSyncParams) (protocol.AIConfigSyncResult, error) {
		return func(protocol.AIConfigSyncParams) (protocol.AIConfigSyncResult, error) {
			return protocol.AIConfigSyncResult{Items: items}, nil
		}
	}
	mk := func(items ...protocol.AIConfigItem) string {
		return env.Agent("server", caps, (&scripted{reply: answer(items...)}).register)
	}
	same := mk(item("claude", "rules", "ok", ""), item("codex", "mcp", "absent", ""))
	drift := mk(item("claude", "rules", "drift", "missing"), item("claude", "mcp", "ok", ""))
	clash := mk(item("claude", "rules", "drift", "different"), item("claude", "mcp", "conflict", "exists"))
	none := mk(item("claude", "rules", "absent", ""), item("codex", "rules", "absent", ""))
	failed := env.Agent("server", caps, (&scripted{reply: func(protocol.AIConfigSyncParams) (protocol.AIConfigSyncResult, error) {
		return protocol.AIConfigSyncResult{}, &protocol.Error{Code: protocol.CodeFailed, Message: "disk full"}
	}}).register)
	old := env.Agent("desktop", []string{protocol.CapSystemInfo}, nil)
	save(t, env, map[string]any{
		"claude": map[string]any{"rules": "x"}, "codex": map[string]any{"rules": ""},
		"hostIds": []string{same, drift, clash, none, failed, old},
	})
	var out statusOut
	env.MustDo(http.MethodGet, "/aiconfig/status", nil, &out)
	got := map[string]string{}
	for _, h := range out.Hosts {
		got[h.HostID] = h.State
	}
	want := map[string]string{same: "ok", drift: "drift", clash: "conflict", none: "absent", failed: "error", old: "unsupported"}
	for id, state := range want {
		if got[id] != state {
			t.Errorf("%s = %q, want %q (all: %v)", id, got[id], state, got)
		}
	}
	for _, h := range out.Hosts {
		if h.HostID == failed && !strings.Contains(h.Error, "disk full") {
			t.Errorf("error text: %q", h.Error)
		}
		if h.HostID == clash && len(h.Items) != 2 {
			t.Errorf("items: %+v", h.Items)
		}
	}
}

func TestStatusNeverWritesAndApplyNeedsElevation(t *testing.T) {
	env := testutil.New(t)
	// no agent yet, so nothing has elevated the session
	if status, _ := env.Do(http.MethodPost, "/aiconfig/apply", nil, nil); status != http.StatusForbidden {
		t.Fatalf("apply without elevation: %d", status)
	}
	a := &scripted{reply: func(p protocol.AIConfigSyncParams) (protocol.AIConfigSyncResult, error) {
		if p.Apply {
			it := item("claude", "rules", "ok", "")
			it.Changed = true
			return protocol.AIConfigSyncResult{Items: []protocol.AIConfigItem{it}}, nil
		}
		return protocol.AIConfigSyncResult{Items: []protocol.AIConfigItem{item("claude", "rules", "drift", "missing")}}, nil
	}}
	b := &scripted{reply: a.reply}
	ha := env.Agent("server", caps, a.register)
	hb := env.Agent("server", caps, b.register)
	other := env.Agent("server", caps, (&scripted{reply: a.reply}).register)
	save(t, env, map[string]any{"claude": map[string]any{"rules": "规则"}, "codex": map[string]any{"rules": ""}, "hostIds": []string{ha, hb}})

	var st statusOut
	env.MustDo(http.MethodGet, "/aiconfig/status", nil, &st)
	if len(st.Hosts) != 2 || st.Hosts[0].State != "drift" {
		t.Fatalf("status: %+v", st)
	}
	for _, c := range append(a.calls(), b.calls()...) {
		if c.Apply {
			t.Fatal("a status check asked to write")
		}
		if c.Config.Claude.Rules != "规则" {
			t.Fatalf("the saved config was not sent: %+v", c.Config)
		}
	}

	// a machine that was not selected can not be applied to
	if status, _ := env.Do(http.MethodPost, "/aiconfig/apply", map[string]any{"hostIds": []string{other}}, nil); status != http.StatusBadRequest {
		t.Fatalf("apply to an unselected machine: %d", status)
	}
	// one machine only
	env.MustDo(http.MethodPost, "/aiconfig/apply", map[string]any{"hostIds": []string{ha}}, &st)
	if len(st.Hosts) != 1 || st.Hosts[0].HostID != ha || st.Hosts[0].State != "ok" || !st.Hosts[0].Items[0].Changed {
		t.Fatalf("apply one: %+v", st)
	}
	if len(b.calls()) != 1 {
		t.Fatalf("the other machine was called: %d", len(b.calls()))
	}
	// everyone selected
	env.MustDo(http.MethodPost, "/aiconfig/apply", nil, &st)
	if len(st.Hosts) != 2 {
		t.Fatalf("apply all: %+v", st)
	}
	applied := 0
	for _, c := range append(a.calls(), b.calls()...) {
		if c.Apply {
			applied++
		}
	}
	if applied != 3 {
		t.Fatalf("applies = %d, want 3", applied)
	}
}

func TestEndToEndWithTheRealAgentCode(t *testing.T) {
	claude, codex := filepath.Join(t.TempDir(), "claude"), filepath.Join(t.TempDir(), "codex")
	for _, d := range []string{claude, codex} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	t.Setenv("CODEX_HOME", codex)
	agentconfig.SetStateDir(t.TempDir())
	env := testutil.New(t)
	own := "# 我自己的规则\n\n别用 emoji。\n"
	if err := os.WriteFile(filepath.Join(claude, "CLAUDE.md"), []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claude, "settings.json"), []byte(`{"permissions":{"allow":["Read(*)"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	host := env.Agent("server", caps, func(c *conn.Client) { agentconfig.Register(c) })
	save(t, env, map[string]any{
		"claude": map[string]any{
			"rules": "提交信息用中文。",
			"allow": []string{"Bash(git status)"},
			"mcp":   []map[string]any{{"name": "docs", "transport": "http", "url": "https://mcp.example.com/mcp"}},
		},
		"codex":   map[string]any{"rules": "Answer in Chinese.", "mcp": []map[string]any{{"name": "fs", "transport": "stdio", "command": "npx", "args": []string{"-y", "fs"}}}},
		"hostIds": []string{host},
	})
	var st statusOut
	env.MustDo(http.MethodGet, "/aiconfig/status", nil, &st)
	if st.Hosts[0].State != "drift" || len(st.Hosts[0].Items) != 5 {
		t.Fatalf("before: %+v", st.Hosts[0])
	}
	if raw, _ := os.ReadFile(filepath.Join(claude, "CLAUDE.md")); string(raw) != own {
		t.Fatal("a check wrote the file")
	}

	env.MustDo(http.MethodPost, "/aiconfig/apply", nil, &st)
	if st.Hosts[0].State != "ok" {
		t.Fatalf("after apply: %+v", st.Hosts[0])
	}
	raw, _ := os.ReadFile(filepath.Join(claude, "CLAUDE.md"))
	if !strings.HasPrefix(string(raw), own) || !strings.Contains(string(raw), "提交信息用中文。") {
		t.Fatalf("CLAUDE.md: %q", raw)
	}
	raw, _ = os.ReadFile(filepath.Join(claude, "settings.json"))
	if !strings.Contains(string(raw), "Read(*)") || !strings.Contains(string(raw), "Bash(git status)") {
		t.Fatalf("settings.json: %s", raw)
	}
	if raw, _ = os.ReadFile(filepath.Join(codex, "config.toml")); !strings.Contains(string(raw), "[mcp_servers.fs]") {
		t.Fatalf("config.toml: %s", raw)
	}
	env.MustDo(http.MethodGet, "/aiconfig/status", nil, &st)
	if st.Hosts[0].State != "ok" {
		t.Fatalf("status after apply: %+v", st.Hosts[0])
	}

	// the panel takes the permission and the Codex server away
	save(t, env, map[string]any{
		"claude":  map[string]any{"rules": "提交信息用中文。", "mcp": []map[string]any{{"name": "docs", "transport": "http", "url": "https://mcp.example.com/mcp"}}},
		"codex":   map[string]any{"rules": "Answer in Chinese."},
		"hostIds": []string{host},
	})
	env.MustDo(http.MethodGet, "/aiconfig/status", nil, &st)
	if st.Hosts[0].State != "drift" {
		t.Fatalf("after the panel changed: %+v", st.Hosts[0])
	}
	env.MustDo(http.MethodPost, "/aiconfig/apply", nil, &st)
	raw, _ = os.ReadFile(filepath.Join(claude, "settings.json"))
	if !strings.Contains(string(raw), "Read(*)") || strings.Contains(string(raw), "Bash(git status)") {
		t.Fatalf("settings.json after removal: %s", raw)
	}
	if raw, _ = os.ReadFile(filepath.Join(codex, "config.toml")); strings.Contains(string(raw), "mcp_servers") {
		t.Fatalf("config.toml after removal: %s", raw)
	}
}

func TestHiddenModuleGateAndAction(t *testing.T) {
	env := testutil.New(t)
	host := env.Agent("server", caps, (&scripted{reply: func(protocol.AIConfigSyncParams) (protocol.AIConfigSyncResult, error) {
		return protocol.AIConfigSyncResult{Items: []protocol.AIConfigItem{item("claude", "rules", "drift", "missing")}}, nil
	}}).register)
	save(t, env, map[string]any{"claude": map[string]any{"rules": "x"}, "codex": map[string]any{"rules": ""}, "hostIds": []string{host}})

	out, err := env.App.Deps.Actions.Run(context.Background(), "aiconfig.status", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), `"state":"drift"`) || !strings.Contains(string(raw), `"selectedHosts":1`) {
		t.Fatalf("action output: %s", raw)
	}

	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, nil)
	env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"coding"}}, nil)
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	if status, _ := env.Do(http.MethodGet, "/aiconfig", nil, nil); status != http.StatusNotFound {
		t.Fatalf("locked: %d", status)
	}
	if _, err := env.App.Deps.Actions.Run(context.Background(), "aiconfig.status", json.RawMessage(`{}`)); err == nil {
		t.Fatal("the action must be hidden while the module is locked")
	}
}
