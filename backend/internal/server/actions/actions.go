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
	"sync"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// Effect says how careful callers must be.
type Effect string

const (
	// Read has no side effects. The AI may run it without asking.
	Read Effect = "read"
	// Write changes data. The AI must show it and wait for the user to confirm.
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

// Get returns an action by name.
func (r *Registry) Get(name string) (Action, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.actions[name]
	return a, ok
}

// List returns all actions sorted by name.
func (r *Registry) List() []Action {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Action, 0, len(r.actions))
	for _, a := range r.actions {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Run looks up and executes an action. It does not check confirmation or
// elevation; callers (the AI assistant, the automation engine) do that.
func (r *Registry) Run(ctx context.Context, name string, input json.RawMessage) (any, error) {
	a, ok := r.Get(name)
	if !ok {
		return nil, httpx.NewError(404, "unknown_action", "没有这个动作: "+name)
	}
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	return a.Run(ctx, input)
}
