package aiconfig

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"slices"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

type object = map[string]json.RawMessage

// readObject reads a JSON file whose top level is an object. A missing or
// empty file is an empty object. ok is false when the file is something else.
func readObject(path string) (obj object, ok bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if isNotExist(err) {
			return object{}, true, nil
		}
		return nil, false, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return object{}, true, nil
	}
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, false, nil
	}
	return obj, true, nil
}

func writeObject(path string, obj object) error {
	raw, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(raw, '\n'), 0o600, true)
}

var invalid = plan{state: protocol.AIConfigConflict, reason: protocol.AIConfigInvalid}

// ---- Claude Code permissions ----

var permKinds = []string{"allow", "ask", "deny"}

func wantedPerms(t protocol.AIConfigTool, kind string) []string {
	switch kind {
	case "allow":
		return t.Allow
	case "ask":
		return t.Ask
	}
	return t.Deny
}

// planPermissions keeps permissions.allow, ask and deny of settings.json.
// The panel's rules are added to each list; rules the panel added earlier and
// no longer wants are taken out. Nothing else in the file is touched.
func planPermissions(path string, want protocol.AIConfigTool, st *state) plan {
	top, ok, err := readObject(path)
	if err != nil || !ok {
		return invalid
	}
	perms := object{}
	if raw, has := top["permissions"]; has {
		if json.Unmarshal(raw, &perms) != nil || perms == nil {
			return invalid
		}
	}
	cur := map[string][]string{}
	for _, k := range permKinds {
		if raw, has := perms[k]; has {
			var list []string
			if json.Unmarshal(raw, &list) != nil {
				return invalid
			}
			cur[k] = list
		}
	}

	var missing, extra []string
	next := map[string][]string{}  // lists after the write
	owned := map[string][]string{} // what the panel added, after the write
	for _, k := range permKinds {
		d, c, s := wantedPerms(want, k), cur[k], st.Claude.Permissions[k]
		out := []string{}
		for _, v := range c {
			if slices.Contains(s, v) && !slices.Contains(d, v) {
				extra = append(extra, k+" "+v)
				continue
			}
			out = append(out, v)
		}
		var mine []string
		for _, v := range d {
			if !slices.Contains(c, v) {
				missing = append(missing, k+" "+v)
				out = append(out, v)
				mine = append(mine, v)
			} else if slices.Contains(s, v) {
				mine = append(mine, v)
			}
		}
		next[k], owned[k] = out, mine
	}

	write := func() error {
		for _, k := range permKinds {
			if _, had := cur[k]; !had && len(next[k]) == 0 {
				continue
			}
			raw, _ := json.Marshal(next[k])
			perms[k] = raw
		}
		raw, _ := json.Marshal(perms)
		top["permissions"] = raw
		if err := writeObject(path, top); err != nil {
			return err
		}
		for _, k := range permKinds {
			if len(owned[k]) == 0 {
				delete(st.Claude.Permissions, k)
			} else {
				st.Claude.Permissions[k] = owned[k]
			}
		}
		st.dirty = true
		return nil
	}
	switch {
	case len(missing) == 0 && len(extra) == 0:
		return plan{state: protocol.AIConfigOK}
	case len(extra) == 0:
		return plan{state: protocol.AIConfigDrift, reason: protocol.AIConfigMissing, names: missing, write: write}
	case len(missing) == 0:
		return plan{state: protocol.AIConfigDrift, reason: protocol.AIConfigExtra, names: extra, write: write}
	}
	return plan{state: protocol.AIConfigDrift, reason: protocol.AIConfigDifferent, names: append(missing, extra...), write: write}
}

// ---- Claude Code MCP servers ----

// serverObject is how a server is written into .claude.json.
func serverObject(s protocol.AIConfigMCP) map[string]any {
	if s.Transport == "http" {
		return map[string]any{"type": "http", "url": s.URL}
	}
	o := map[string]any{"type": "stdio", "command": s.Command}
	if len(s.Args) > 0 {
		o["args"] = s.Args
	}
	return o
}

// canonical brings a server object to one shape for comparing: an empty env
// or headers (which `claude mcp add` writes) and a missing type do not count.
func canonical(raw json.RawMessage) (map[string]any, bool) {
	var o map[string]any
	if json.Unmarshal(raw, &o) != nil || o == nil {
		return nil, false
	}
	for _, k := range []string{"env", "headers"} {
		if m, ok := o[k].(map[string]any); ok && len(m) == 0 {
			delete(o, k)
		}
	}
	if _, ok := o["type"]; !ok {
		if _, has := o["command"]; has {
			o["type"] = "stdio"
		}
	}
	if a, ok := o["args"].([]any); ok && len(a) == 0 {
		delete(o, "args")
	}
	return o, true
}

func sameServer(raw json.RawMessage, want protocol.AIConfigMCP) bool {
	got, ok := canonical(raw)
	if !ok {
		return false
	}
	wantRaw, _ := json.Marshal(serverObject(want))
	exp, _ := canonical(wantRaw)
	return reflect.DeepEqual(got, exp)
}

// planClaudeMCP keeps the panel's servers in mcpServers of .claude.json.
func planClaudeMCP(path string, want []protocol.AIConfigMCP, st *state) plan {
	top, ok, err := readObject(path)
	if err != nil || !ok {
		return invalid
	}
	servers := object{}
	if raw, has := top["mcpServers"]; has {
		if json.Unmarshal(raw, &servers) != nil || servers == nil {
			return invalid
		}
	}
	owned := st.Claude.MCP
	wanted := map[string]bool{}
	var missing, different, clash, extra, mine []string
	for _, s := range want {
		wanted[s.Name] = true
		raw, has := servers[s.Name]
		switch {
		case !has:
			missing = append(missing, s.Name)
			mine = append(mine, s.Name)
		case sameServer(raw, s):
			if slices.Contains(owned, s.Name) {
				mine = append(mine, s.Name)
			}
		case slices.Contains(owned, s.Name):
			different = append(different, s.Name)
			mine = append(mine, s.Name)
		default:
			clash = append(clash, s.Name)
		}
	}
	for _, name := range owned {
		if _, has := servers[name]; has && !wanted[name] {
			extra = append(extra, name)
		}
	}
	if len(clash) > 0 {
		return plan{state: protocol.AIConfigConflict, reason: protocol.AIConfigExists, names: clash}
	}

	write := func() error {
		for _, name := range extra {
			delete(servers, name)
		}
		for _, s := range want {
			raw, _ := json.Marshal(serverObject(s))
			servers[s.Name] = raw
		}
		raw, _ := json.Marshal(servers)
		top["mcpServers"] = raw
		if err := writeObject(path, top); err != nil {
			return err
		}
		st.Claude.MCP = mine
		st.dirty = true
		return nil
	}
	all := append(append(append([]string{}, missing...), different...), extra...)
	switch {
	case len(all) == 0:
		return plan{state: protocol.AIConfigOK}
	case len(different) == 0 && len(extra) == 0:
		return plan{state: protocol.AIConfigDrift, reason: protocol.AIConfigMissing, names: all, write: write}
	case len(missing) == 0 && len(different) == 0:
		return plan{state: protocol.AIConfigDrift, reason: protocol.AIConfigExtra, names: all, write: write}
	}
	return plan{state: protocol.AIConfigDrift, reason: protocol.AIConfigDifferent, names: all, write: write}
}
