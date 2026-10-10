package aiconfig

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

type rig struct {
	t                   *testing.T
	claudeDir, codexDir string
	claudeJSON          string
	state               string
}

func newRig(t *testing.T, claude, codex bool) *rig {
	t.Helper()
	root := t.TempDir()
	r := &rig{t: t, claudeDir: filepath.Join(root, "claude"), codexDir: filepath.Join(root, "codex"), state: filepath.Join(root, "agent")}
	r.claudeJSON = filepath.Join(r.claudeDir, ".claude.json")
	if claude {
		must(t, os.MkdirAll(r.claudeDir, 0o700))
	}
	if codex {
		must(t, os.MkdirAll(r.codexDir, 0o700))
	}
	oldLocate, oldState := locate, stateDir
	locate = func() (places, error) {
		return places{claudeDir: r.claudeDir, claudeJSON: r.claudeJSON, codexDir: r.codexDir, claudeInstalled: claude, codexInstalled: codex}, nil
	}
	stateDir = r.state
	t.Cleanup(func() { locate, stateDir = oldLocate, oldState })
	return r
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (r *rig) write(path, text string) {
	r.t.Helper()
	must(r.t, os.MkdirAll(filepath.Dir(path), 0o700))
	must(r.t, os.WriteFile(path, []byte(text), 0o644))
}

func (r *rig) read(path string) string {
	r.t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

func (r *rig) sync(doc protocol.AIConfigDoc, apply bool) map[string]protocol.AIConfigItem {
	r.t.Helper()
	res, err := Sync(context.Background(), protocol.AIConfigSyncParams{Config: doc, Apply: apply})
	if err != nil {
		r.t.Fatal(err)
	}
	out := map[string]protocol.AIConfigItem{}
	for _, it := range res.Items {
		out[it.Tool+"/"+it.Item] = it
	}
	return out
}

func stdio(name, cmd string, args ...string) protocol.AIConfigMCP {
	return protocol.AIConfigMCP{Name: name, Transport: "stdio", Command: cmd, Args: args}
}

func wantState(t *testing.T, items map[string]protocol.AIConfigItem, key, state, reason string) {
	t.Helper()
	it, ok := items[key]
	if !ok {
		t.Fatalf("no item %s in %v", key, items)
	}
	if it.State != state || it.Reason != reason {
		t.Fatalf("%s = %s/%s (%s), want %s/%s", key, it.State, it.Reason, it.Error, state, reason)
	}
}

func jsonOf(t *testing.T, text string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("not json: %v\n%s", err, text)
	}
	return m
}

func TestRulesKeepOtherContentAndAreIdempotent(t *testing.T) {
	r := newRig(t, true, false)
	path := filepath.Join(r.claudeDir, "CLAUDE.md")
	r.write(path, "# 我的规则\n\n不要用 emoji。\n")
	doc := protocol.AIConfigDoc{Claude: protocol.AIConfigTool{Rules: "回答用中文。\n提交前跑测试。"}}

	items := r.sync(doc, false)
	wantState(t, items, "claude/rules", "drift", "missing")
	if got := r.read(path); got != "# 我的规则\n\n不要用 emoji。\n" {
		t.Fatalf("a check changed the file: %q", got)
	}

	items = r.sync(doc, true)
	wantState(t, items, "claude/rules", "ok", "")
	if !items["claude/rules"].Changed {
		t.Fatal("not reported as changed")
	}
	got := r.read(path)
	if !strings.HasPrefix(got, "# 我的规则\n\n不要用 emoji。\n\n"+mdMarks.begin+"\n回答用中文。\n提交前跑测试。\n"+mdMarks.end+"\n") {
		t.Fatalf("unexpected file: %q", got)
	}
	if r.read(path+".x-console.bak") != "# 我的规则\n\n不要用 emoji。\n" {
		t.Fatal("no backup of the original")
	}

	items = r.sync(doc, true)
	wantState(t, items, "claude/rules", "ok", "")
	if items["claude/rules"].Changed || r.read(path) != got {
		t.Fatal("a second apply changed something")
	}

	// the user edits the block by hand: drift, and an apply puts it back
	r.write(path, strings.Replace(got, "提交前", "提交后", 1))
	wantState(t, r.sync(doc, false), "claude/rules", "drift", "different")
	wantState(t, r.sync(doc, true), "claude/rules", "ok", "")
	if r.read(path) != got {
		t.Fatalf("not restored: %q", r.read(path))
	}
	// the backup still holds the first original
	if r.read(path+".x-console.bak") != "# 我的规则\n\n不要用 emoji。\n" {
		t.Fatal("the backup was replaced")
	}

	// the panel empties the rules: the block goes, the rest stays
	doc.Claude.Rules = ""
	wantState(t, r.sync(doc, false), "claude/rules", "drift", "extra")
	wantState(t, r.sync(doc, true), "claude/rules", "ok", "")
	if r.read(path) != "# 我的规则\n\n不要用 emoji。\n" {
		t.Fatalf("not removed cleanly: %q", r.read(path))
	}
}

func TestRulesFileThatDoesNotExistYetAndCRLF(t *testing.T) {
	r := newRig(t, true, true)
	doc := protocol.AIConfigDoc{Codex: protocol.AIConfigTool{Rules: "Reply in Chinese."}}
	wantState(t, r.sync(doc, true), "codex/rules", "ok", "")
	path := filepath.Join(r.codexDir, "AGENTS.md")
	if r.read(path) != mdMarks.begin+"\nReply in Chinese.\n"+mdMarks.end+"\n" {
		t.Fatalf("new file: %q", r.read(path))
	}
	if _, err := os.Stat(path + ".x-console.bak"); err == nil {
		t.Fatal("a backup of a file that did not exist")
	}

	r.write(path, "line one\r\nline two\r\n")
	wantState(t, r.sync(doc, true), "codex/rules", "ok", "")
	if got := r.read(path); !strings.HasPrefix(got, "line one\r\nline two\r\n\r\n") || strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Fatalf("line breaks were not kept: %q", got)
	}
}

func TestBrokenMarkersAreAConflictAndNotTouched(t *testing.T) {
	r := newRig(t, true, false)
	path := filepath.Join(r.claudeDir, "CLAUDE.md")
	r.write(path, mdMarks.begin+"\nhalf a block\n")
	doc := protocol.AIConfigDoc{Claude: protocol.AIConfigTool{Rules: "x"}}
	wantState(t, r.sync(doc, true), "claude/rules", "conflict", "markers")
	if r.read(path) != mdMarks.begin+"\nhalf a block\n" {
		t.Fatal("a conflicting file was changed")
	}
}

func TestPermissionsMergeAndRemoveOnlyWhatThePanelAdded(t *testing.T) {
	r := newRig(t, true, false)
	path := filepath.Join(r.claudeDir, "settings.json")
	r.write(path, `{"model":"opus","permissions":{"allow":["Bash(ls)","Read(*)"],"defaultMode":"acceptEdits"},"theme":"dark"}`)
	doc := protocol.AIConfigDoc{Claude: protocol.AIConfigTool{
		Allow: []string{"Bash(git status)", "Bash(ls)"},
		Deny:  []string{"Read(.env)"},
	}}
	wantState(t, r.sync(doc, false), "claude/permissions", "drift", "missing")
	wantState(t, r.sync(doc, true), "claude/permissions", "ok", "")

	m := jsonOf(t, r.read(path))
	perms := m["permissions"].(map[string]any)
	if got := perms["allow"].([]any); len(got) != 3 || got[0] != "Bash(ls)" || got[1] != "Read(*)" || got[2] != "Bash(git status)" {
		t.Fatalf("allow = %v", got)
	}
	if got := perms["deny"].([]any); len(got) != 1 || got[0] != "Read(.env)" {
		t.Fatalf("deny = %v", got)
	}
	if perms["defaultMode"] != "acceptEdits" || m["model"] != "opus" || m["theme"] != "dark" {
		t.Fatalf("other settings were lost: %v", m)
	}

	// the panel drops the rules it added and one the user already had
	doc.Claude.Allow = nil
	doc.Claude.Deny = nil
	wantState(t, r.sync(doc, false), "claude/permissions", "drift", "extra")
	wantState(t, r.sync(doc, true), "claude/permissions", "ok", "")
	perms = jsonOf(t, r.read(path))["permissions"].(map[string]any)
	got := perms["allow"].([]any)
	if len(got) != 2 || got[0] != "Bash(ls)" || got[1] != "Read(*)" {
		t.Fatalf("the user's own rules were not kept: %v", got)
	}
	if d, ok := perms["deny"].([]any); !ok || len(d) != 0 {
		t.Fatalf("deny = %v", perms["deny"])
	}
	if perms["defaultMode"] != "acceptEdits" {
		t.Fatal("defaultMode lost")
	}
}

func TestPermissionsRestoreARuleTheUserRemoved(t *testing.T) {
	r := newRig(t, true, false)
	path := filepath.Join(r.claudeDir, "settings.json")
	doc := protocol.AIConfigDoc{Claude: protocol.AIConfigTool{Ask: []string{"Bash(rm *)"}}}
	wantState(t, r.sync(doc, true), "claude/permissions", "ok", "")
	if _, err := os.Stat(r.state); err != nil {
		t.Fatalf("no state file: %v", err)
	}
	r.write(path, `{"permissions":{"ask":[]}}`)
	wantState(t, r.sync(doc, false), "claude/permissions", "drift", "missing")
	wantState(t, r.sync(doc, true), "claude/permissions", "ok", "")
	perms := jsonOf(t, r.read(path))["permissions"].(map[string]any)
	if ask := perms["ask"].([]any); len(ask) != 1 || ask[0] != "Bash(rm *)" {
		t.Fatalf("ask = %v", ask)
	}
}

func TestInvalidJSONIsAConflictAndNotTouched(t *testing.T) {
	r := newRig(t, true, false)
	path := filepath.Join(r.claudeDir, "settings.json")
	r.write(path, `{"permissions": {`)
	doc := protocol.AIConfigDoc{Claude: protocol.AIConfigTool{Allow: []string{"Bash(ls)"}}}
	wantState(t, r.sync(doc, true), "claude/permissions", "conflict", "invalid")
	if r.read(path) != `{"permissions": {` {
		t.Fatal("an invalid file was changed")
	}
	r.write(path, `[1,2]`)
	wantState(t, r.sync(doc, true), "claude/permissions", "conflict", "invalid")
	r.write(path, `{"permissions": {"allow": "Bash(ls)"}}`)
	wantState(t, r.sync(doc, true), "claude/permissions", "conflict", "invalid")
}

func TestClaudeMCP(t *testing.T) {
	r := newRig(t, true, false)
	r.write(r.claudeJSON, `{"numStartups":42,"projects":{"/x":{"allowedTools":[]}},"mcpServers":{"mine":{"type":"stdio","command":"foo","args":[],"env":{}}}}`)
	doc := protocol.AIConfigDoc{Claude: protocol.AIConfigTool{MCP: []protocol.AIConfigMCP{
		stdio("fs", "npx", "-y", "server-fs"),
		{Name: "docs", Transport: "http", URL: "https://mcp.example.com/mcp"},
	}}}
	wantState(t, r.sync(doc, false), "claude/mcp", "drift", "missing")
	wantState(t, r.sync(doc, true), "claude/mcp", "ok", "")
	m := jsonOf(t, r.read(r.claudeJSON))
	if m["numStartups"] != float64(42) || m["projects"] == nil {
		t.Fatalf("other keys lost: %v", m)
	}
	servers := m["mcpServers"].(map[string]any)
	if len(servers) != 3 || servers["mine"] == nil {
		t.Fatalf("servers = %v", servers)
	}
	fs := servers["fs"].(map[string]any)
	if fs["type"] != "stdio" || fs["command"] != "npx" || len(fs["args"].([]any)) != 2 {
		t.Fatalf("fs = %v", fs)
	}
	if servers["docs"].(map[string]any)["url"] != "https://mcp.example.com/mcp" {
		t.Fatalf("docs = %v", servers["docs"])
	}

	// somebody changes a server the panel wrote: drift, apply restores it
	servers["fs"] = map[string]any{"type": "stdio", "command": "other"}
	raw, _ := json.Marshal(m)
	r.write(r.claudeJSON, string(raw))
	wantState(t, r.sync(doc, false), "claude/mcp", "drift", "different")
	wantState(t, r.sync(doc, true), "claude/mcp", "ok", "")

	// the panel drops docs: only docs is removed, the user's own server stays
	doc.Claude.MCP = doc.Claude.MCP[:1]
	items := r.sync(doc, false)
	wantState(t, items, "claude/mcp", "drift", "extra")
	if len(items["claude/mcp"].Names) != 1 || items["claude/mcp"].Names[0] != "docs" {
		t.Fatalf("names = %v", items["claude/mcp"].Names)
	}
	wantState(t, r.sync(doc, true), "claude/mcp", "ok", "")
	servers = jsonOf(t, r.read(r.claudeJSON))["mcpServers"].(map[string]any)
	if len(servers) != 2 || servers["mine"] == nil || servers["fs"] == nil {
		t.Fatalf("servers = %v", servers)
	}
}

func TestClaudeMCPSameNameThatThePanelDidNotWriteIsAConflict(t *testing.T) {
	r := newRig(t, true, false)
	original := `{"mcpServers":{"fs":{"type":"stdio","command":"my-own-fs","env":{"TOKEN":"x"}}}}`
	r.write(r.claudeJSON, original)
	doc := protocol.AIConfigDoc{Claude: protocol.AIConfigTool{MCP: []protocol.AIConfigMCP{stdio("fs", "npx", "server-fs"), stdio("other", "x")}}}
	items := r.sync(doc, true)
	wantState(t, items, "claude/mcp", "conflict", "exists")
	if n := items["claude/mcp"].Names; len(n) != 1 || n[0] != "fs" {
		t.Fatalf("names = %v", n)
	}
	if r.read(r.claudeJSON) != original {
		t.Fatal("a conflicting file was changed")
	}
	// the same content is not a conflict
	r.write(r.claudeJSON, `{"mcpServers":{"fs":{"command":"npx","args":["server-fs"],"env":{}}}}`)
	doc.Claude.MCP = doc.Claude.MCP[:1]
	wantState(t, r.sync(doc, true), "claude/mcp", "ok", "")
}

func TestCodexMCPBlock(t *testing.T) {
	r := newRig(t, false, true)
	path := filepath.Join(r.codexDir, "config.toml")
	r.write(path, "model = \"gpt-5\"\n\n[mcp_servers.mine]\ncommand = \"mine\"\n")
	doc := protocol.AIConfigDoc{Codex: protocol.AIConfigTool{MCP: []protocol.AIConfigMCP{
		stdio("fs", "npx", "-y", `a "quoted" arg`),
		{Name: "docs", Transport: "http", URL: "https://mcp.example.com/mcp"},
	}}}
	wantState(t, r.sync(doc, false), "codex/mcp", "drift", "missing")
	wantState(t, r.sync(doc, true), "codex/mcp", "ok", "")
	got := r.read(path)
	if !strings.HasPrefix(got, "model = \"gpt-5\"\n\n[mcp_servers.mine]\ncommand = \"mine\"\n\n"+tomlMarks.begin+"\n[mcp_servers.fs]\n") {
		t.Fatalf("file = %q", got)
	}
	for _, want := range []string{`args = ["-y", "a \"quoted\" arg"]`, "[mcp_servers.docs]\nurl = \"https://mcp.example.com/mcp\"\n" + tomlMarks.end} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}

	// the panel removes one server, then all
	doc.Codex.MCP = doc.Codex.MCP[:1]
	wantState(t, r.sync(doc, true), "codex/mcp", "ok", "")
	if strings.Contains(r.read(path), "docs") {
		t.Fatal("docs was not removed")
	}
	doc.Codex.MCP = nil
	wantState(t, r.sync(doc, true), "codex/mcp", "ok", "")
	if r.read(path) != "model = \"gpt-5\"\n\n[mcp_servers.mine]\ncommand = \"mine\"\n" {
		t.Fatalf("after removing: %q", r.read(path))
	}
}

func TestCodexMCPSameTableOutsideTheBlockIsAConflict(t *testing.T) {
	r := newRig(t, false, true)
	path := filepath.Join(r.codexDir, "config.toml")
	original := "[mcp_servers.fs]\ncommand = \"my-fs\"\n"
	r.write(path, original)
	doc := protocol.AIConfigDoc{Codex: protocol.AIConfigTool{MCP: []protocol.AIConfigMCP{stdio("fs", "npx")}}}
	items := r.sync(doc, true)
	wantState(t, items, "codex/mcp", "conflict", "exists")
	if r.read(path) != original {
		t.Fatal("a conflicting file was changed")
	}
	// a name that only looks similar is fine
	doc.Codex.MCP = []protocol.AIConfigMCP{stdio("fs2", "npx")}
	wantState(t, r.sync(doc, true), "codex/mcp", "ok", "")
}

func TestToolThatIsNotInstalledIsAbsentAndNothingIsCreated(t *testing.T) {
	r := newRig(t, false, true)
	doc := protocol.AIConfigDoc{
		Claude: protocol.AIConfigTool{Rules: "x", Allow: []string{"Bash(ls)"}, MCP: []protocol.AIConfigMCP{stdio("a", "b")}},
		Codex:  protocol.AIConfigTool{Rules: "y"},
	}
	items := r.sync(doc, true)
	for _, key := range []string{"claude/rules", "claude/permissions", "claude/mcp"} {
		wantState(t, items, key, "absent", "")
	}
	wantState(t, items, "codex/rules", "ok", "")
	if _, err := os.Stat(r.claudeDir); err == nil {
		t.Fatal("the directory of a tool that is not installed was created")
	}
}

func TestEmptyConfigOnAFreshMachineWritesNothing(t *testing.T) {
	r := newRig(t, true, true)
	items := r.sync(protocol.AIConfigDoc{}, true)
	for key, it := range items {
		if it.State != "ok" || it.Changed {
			t.Fatalf("%s = %+v", key, it)
		}
	}
	entries, _ := os.ReadDir(r.claudeDir)
	if len(entries) != 0 {
		t.Fatalf("files were created: %v", entries)
	}
	if _, err := os.Stat(r.state); err == nil {
		t.Fatal("a state file was written for nothing")
	}
}

func TestSymlinkedFileStaysALink(t *testing.T) {
	r := newRig(t, true, false)
	target := filepath.Join(t.TempDir(), "dotfiles", "CLAUDE.md")
	r.write(target, "mine\n")
	link := filepath.Join(r.claudeDir, "CLAUDE.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks are not available:", err)
	}
	doc := protocol.AIConfigDoc{Claude: protocol.AIConfigTool{Rules: "x"}}
	wantState(t, r.sync(doc, true), "claude/rules", "ok", "")
	if st, err := os.Lstat(link); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the link was replaced by a file")
	}
	if !strings.Contains(r.read(target), "x\n") {
		t.Fatalf("target = %q", r.read(target))
	}
}

func TestInvalidParamsAreRefused(t *testing.T) {
	newRig(t, true, true)
	bad := []protocol.AIConfigDoc{
		{Claude: protocol.AIConfigTool{MCP: []protocol.AIConfigMCP{{Name: "bad name", Transport: "stdio", Command: "x"}}}},
		{Claude: protocol.AIConfigTool{MCP: []protocol.AIConfigMCP{{Name: "a", Transport: "http", URL: "ftp://x"}}}},
		{Claude: protocol.AIConfigTool{Allow: []string{"a\nb"}}},
		{Codex: protocol.AIConfigTool{Allow: []string{"Bash(ls)"}}},
		{Claude: protocol.AIConfigTool{Rules: "see " + protocol.AIConfigMarker + "end"}},
	}
	for i, doc := range bad {
		if _, err := Sync(context.Background(), protocol.AIConfigSyncParams{Config: doc, Apply: true}); err == nil {
			t.Fatalf("doc %d was accepted", i)
		}
	}
}
