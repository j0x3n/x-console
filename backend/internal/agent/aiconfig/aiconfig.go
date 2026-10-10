// Package aiconfig keeps the part of the Claude Code and Codex configuration
// that the panel owns, for the agent. See docs/specs/B121.md.
//
// It never reads anything else back to the server, and it only changes what the
// panel wrote: a marked block in the rules files, the permission rules and MCP
// servers it added earlier. What the user wrote stays as it is. The first time
// a file is changed a copy is kept next to it.
package aiconfig

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/j0x3n/x-console/backend/internal/agent/config"
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// Registrar is the part of conn.Client the handlers need.
type Registrar interface {
	Handle(method string, h rpc.Handler)
}

// Register adds aiconfig.sync.
func Register(c Registrar) {
	c.Handle(protocol.MethodAIConfigSync, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.AIConfigSyncParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return Sync(ctx, p)
	})
}

// stateDir holds aiconfig-state.json. cmd/agent sets it to the directory of
// the agent's own config file.
var stateDir = filepath.Dir(config.DefaultPath())

// SetStateDir moves the state file next to the agent's config file.
func SetStateDir(dir string) { stateDir = dir }

// places says where each tool keeps its files on this machine.
type places struct {
	claudeDir, claudeJSON, codexDir string
	claudeInstalled, codexInstalled bool
}

var (
	lookPath = exec.LookPath
	locate   = defaultPlaces
	mu       sync.Mutex // one sync at a time: they read and write the same files
)

func defaultPlaces() (places, error) {
	home, herr := os.UserHomeDir()
	var p places
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); v != "" {
		p.claudeDir = v
		p.claudeJSON = filepath.Join(v, ".claude.json")
	} else if herr == nil {
		p.claudeDir = filepath.Join(home, ".claude")
		p.claudeJSON = filepath.Join(home, ".claude.json")
	}
	if v := strings.TrimSpace(os.Getenv("CODEX_HOME")); v != "" {
		p.codexDir = v
	} else if herr == nil {
		p.codexDir = filepath.Join(home, ".codex")
	}
	if p.claudeDir == "" && p.codexDir == "" {
		return p, herr
	}
	p.claudeInstalled = installed(p.claudeDir, "claude", p.claudeJSON)
	p.codexInstalled = installed(p.codexDir, "codex")
	return p, nil
}

func installed(dir, command string, files ...string) bool {
	if dir == "" {
		return false
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return true
	}
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}
	_, err := lookPath(command)
	return err == nil
}

// Available reports whether Claude Code or Codex is on this machine.
// cmd/agent announces protocol.CapAIConfig only then.
func Available() bool {
	p, err := locate()
	return err == nil && (p.claudeInstalled || p.codexInstalled)
}

// Sync answers aiconfig.sync: it checks every item, and writes the ones that
// differ when p.Apply is set.
func Sync(_ context.Context, p protocol.AIConfigSyncParams) (protocol.AIConfigSyncResult, error) {
	doc := p.Config.Normalize()
	if err := doc.Validate(); err != nil {
		return protocol.AIConfigSyncResult{}, rpcutil.BadParams("%v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	where, err := locate()
	if err != nil {
		return protocol.AIConfigSyncResult{}, rpcutil.Failed("cannot find the home directory: %v", err)
	}
	st := loadState()
	r := runner{apply: p.Apply, st: &st}
	if where.claudeInstalled {
		r.run(protocol.AIConfigToolClaude, protocol.AIConfigItemRules, func() plan {
			return planRules(filepath.Join(where.claudeDir, "CLAUDE.md"), doc.Claude.Rules, mdMarks)
		})
		r.run(protocol.AIConfigToolClaude, protocol.AIConfigItemPermissions, func() plan {
			return planPermissions(filepath.Join(where.claudeDir, "settings.json"), doc.Claude, &st)
		})
		r.run(protocol.AIConfigToolClaude, protocol.AIConfigItemMCP, func() plan {
			return planClaudeMCP(where.claudeJSON, doc.Claude.MCP, &st)
		})
	} else {
		r.absent(protocol.AIConfigToolClaude, protocol.AIConfigItemRules, protocol.AIConfigItemPermissions, protocol.AIConfigItemMCP)
	}
	if where.codexInstalled {
		r.run(protocol.AIConfigToolCodex, protocol.AIConfigItemRules, func() plan {
			return planRules(filepath.Join(where.codexDir, "AGENTS.md"), doc.Codex.Rules, mdMarks)
		})
		r.run(protocol.AIConfigToolCodex, protocol.AIConfigItemMCP, func() plan {
			return planCodexMCP(filepath.Join(where.codexDir, "config.toml"), doc.Codex.MCP)
		})
	} else {
		r.absent(protocol.AIConfigToolCodex, protocol.AIConfigItemRules, protocol.AIConfigItemMCP)
	}
	if st.dirty {
		if err := saveState(st); err != nil {
			r.fail("saving the state file: " + err.Error())
		}
	}
	return protocol.AIConfigSyncResult{Items: r.items}, nil
}

// plan is what one item needs. write is set when state is drift and there is
// something to write.
type plan struct {
	state, reason string
	names         []string
	write         func() error
}

type runner struct {
	apply bool
	st    *state
	items []protocol.AIConfigItem
}

func (r *runner) absent(tool string, items ...string) {
	for _, it := range items {
		r.items = append(r.items, protocol.AIConfigItem{Tool: tool, Item: it, State: protocol.AIConfigAbsent})
	}
}

// fail marks the last item that was written as failed, for a state file that
// could not be saved.
func (r *runner) fail(msg string) {
	for i := len(r.items) - 1; i >= 0; i-- {
		if r.items[i].Changed {
			r.items[i].Error = msg
			return
		}
	}
}

func (r *runner) run(tool, item string, build func() plan) {
	p := build()
	out := protocol.AIConfigItem{Tool: tool, Item: item}
	if r.apply && p.state == protocol.AIConfigDrift && p.write != nil {
		if err := p.write(); err != nil {
			out.Error = err.Error()
		} else {
			out.Changed = true
			p = build()
		}
	}
	out.State, out.Reason, out.Names = p.state, p.reason, capNames(p.names)
	r.items = append(r.items, out)
}

func capNames(in []string) []string {
	if len(in) > 20 {
		in = in[:20]
	}
	return in
}

// ---- state file ----

// state records which entries the panel added, so a later sync can remove
// exactly those and leave everything else alone.
type state struct {
	Claude struct {
		Permissions map[string][]string `json:"permissions,omitempty"`
		MCP         []string            `json:"mcp,omitempty"`
	} `json:"claude"`
	dirty bool
}

func statePath() string { return filepath.Join(stateDir, "aiconfig-state.json") }

func loadState() state {
	var s state
	raw, err := os.ReadFile(statePath())
	if err == nil {
		_ = json.Unmarshal(raw, &s)
	}
	if s.Claude.Permissions == nil {
		s.Claude.Permissions = map[string][]string{}
	}
	return s
}

func saveState(s state) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	return writeFileAtomic(statePath(), append(raw, '\n'), 0o600, false)
}

var errNotExist = os.ErrNotExist

func isNotExist(err error) bool { return errors.Is(err, errNotExist) }
