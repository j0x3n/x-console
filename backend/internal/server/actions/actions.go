// Package actions is the catalog of operations other parts of the system can
// invoke by name: the AI assistant exposes them as tools and the automation
// engine uses them as rule actions (M12).
//
// Modules register their actions in New:
//
//	d.Actions.Register(actions.Action{
//	    Name:        "projects.create_issue",
//	    Title:       "新建 Issue",
//	    Description: "Create an issue in a project. Returns the issue with its key, e.g. XC-12.",
//	    Input:       actions.Schema(`{"type":"object","properties":{...},"required":["projectId","title"]}`),
//	    Effect:      actions.Write,
//	    Run:         m.runCreateIssue,
//	})
package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// Effect says how careful callers must be.
type Effect string

const (
	// Read has no side effects. The AI may run it without asking.
	Read Effect = "read"
	// Write changes data. The AI executes it directly unless it is destructive
	// or the owner has enabled confirmation for all writes.
	Write Effect = "write"
	// Dangerous runs commands or controls machines. Needs confirmation and a
	// fresh TOTP elevation of the user who confirms it.
	Dangerous Effect = "dangerous"
)

// Action is one invokable operation.
type Action struct {
	// Name is "<module>.<verb_object>", for example "reminders.create".
	Name string `json:"name"`
	// Title is a short Chinese label for the UI.
	Title string `json:"title"`
	// Description tells the model what the action does and returns (English is fine).
	Description string `json:"description"`
	// Input is the JSON Schema of the input object.
	Input json.RawMessage `json:"input"`
	// Effect decides confirmation and elevation rules.
	Effect Effect `json:"effect"`
	// Destructive asks the assistant to confirm a write before executing it.
	Destructive bool `json:"destructive,omitempty"`
	// AliasOf names the action to use instead. An alias still runs (saved
	// automation rules may use it), but List leaves it out, so the assistant
	// and the automation editor see each operation once.
	AliasOf string `json:"aliasOf,omitempty"`
	// PanelOnly keeps the action to the panel assistant (B61: writing the AI
	// memory). Remote AI (MCP), agents and automations never see it.
	PanelOnly bool `json:"panelOnly,omitempty"`
	// MCPOnly keeps the action to remote AI (MCP, B151): it hands out one-time
	// links that only make sense for a client with its own shell. The panel
	// assistant, agents and automations do not see it in List.
	MCPOnly bool `json:"mcpOnly,omitempty"`
	// Run executes the action. ctx carries the acting user or automation.
	Run func(ctx context.Context, input json.RawMessage) (any, error) `json:"-"`
}

// Schema is a helper that panics on invalid JSON so mistakes surface in tests.
func Schema(s string) json.RawMessage {
	if !json.Valid([]byte(s)) {
		panic("actions: invalid JSON schema: " + s)
	}
	return json.RawMessage(s)
}

// Registry holds all actions.
type Registry struct {
	mu      sync.RWMutex
	actions map[string]Action
	hidden  contracts.HiddenModules
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry { return &Registry{actions: map[string]Action{}} }

// Register adds an action. Registering the same name twice panics.
func (r *Registry) Register(a Action) {
	if a.Name == "" || a.Run == nil || a.Effect == "" || len(a.Input) == 0 {
		panic(fmt.Sprintf("actions: incomplete action %q", a.Name))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.actions[a.Name]; dup {
		panic("actions: duplicate action " + a.Name)
	}
	r.actions[a.Name] = a
}

func (r *Registry) SetHidden(h contracts.HiddenModules) {
	r.mu.Lock()
	r.hidden = h
	r.mu.Unlock()
}

// Get returns an action by name when this request may see it.
func (r *Registry) Get(ctx context.Context, name string) (Action, bool) {
	r.mu.RLock()
	a, ok := r.actions[name]
	h := r.hidden
	r.mu.RUnlock()
	if !ok || contracts.ActionHidden(ctx, h, name) {
		return Action{}, false
	}
	return a, true
}

// List returns the actions this request may see, sorted by name. Actions
// marked MCPOnly are left out; ListForMCP has them.
func (r *Registry) List(ctx context.Context) []Action { return r.list(ctx, false) }

// ListForMCP is List for the MCP endpoint, which also offers the MCPOnly actions.
func (r *Registry) ListForMCP(ctx context.Context) []Action { return r.list(ctx, true) }

func (r *Registry) list(ctx context.Context, mcp bool) []Action {
	r.mu.RLock()
	out := make([]Action, 0, len(r.actions))
	for _, a := range r.actions {
		if a.AliasOf == "" && (mcp || !a.MCPOnly) {
			out = append(out, a)
		}
	}
	h := r.hidden
	r.mu.RUnlock()
	kept := out[:0]
	for _, a := range out {
		if !contracts.ActionHidden(ctx, h, a.Name) {
			kept = append(kept, a)
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Name < kept[j].Name })
	return kept
}

// Run looks up and executes an action. It does not check confirmation or
// elevation; callers (the AI assistant, the automation engine) do that.
func (r *Registry) Run(ctx context.Context, name string, input json.RawMessage) (any, error) {
	a, ok := r.Get(ctx, name)
	if !ok {
		return nil, httpx.NewError(404, "unknown_action", "没有这个动作: "+name)
	}
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	return a.Run(ctx, input)
}

// Deletes reports whether an action removes data: marked Destructive, or
// named "*.delete" (the assistant confirms these too).
func Deletes(a Action) bool {
	return a.Destructive || strings.HasSuffix(a.Name, ".delete")
}

// Module is the part of an action name before the first dot, for example
// "notes" for notes.create.
func Module(name string) string {
	if i := strings.IndexByte(name, '.'); i > 0 {
		return name[:i]
	}
	return name
}

// Access levels of callers outside the web app: API tokens (B43) and
// built-in agents (B47).
const (
	AccessRead        = "read"
	AccessWrite       = "write"
	AccessWriteDelete = "write_delete"
)

// AllowedFor reports whether an outside caller with this access level and
// module list (empty means all) may see and run the action. Dangerous
// actions are never allowed.
func AllowedFor(a Action, access string, modules []string) bool {
	if a.Effect == Dangerous || a.AliasOf != "" || a.PanelOnly {
		return false
	}
	switch access {
	case AccessRead:
		if a.Effect != Read {
			return false
		}
	case AccessWrite:
		if a.Effect != Read && a.Effect != Write || Deletes(a) {
			return false
		}
	case AccessWriteDelete:
		if a.Effect != Read && a.Effect != Write {
			return false
		}
	default:
		return false
	}
	if len(modules) == 0 {
		return true
	}
	m := Module(a.Name)
	for _, allowed := range modules {
		if allowed == m {
			return true
		}
	}
	return false
}
