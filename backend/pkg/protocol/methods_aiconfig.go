package protocol

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Methods of B121 AI coding tool configuration. The agent announces
// CapAIConfig when this machine has Claude Code or Codex installed, and
// answers aiconfig.sync for them. It only ever touches the part of the
// configuration the panel wrote; see docs/specs/B121.md.
const (
	CapAIConfig        = "aiconfig"
	MethodAIConfigSync = "aiconfig.sync" // AIConfigSyncParams -> AIConfigSyncResult
)

// Tools and items aiconfig.sync reports on.
const (
	AIConfigToolClaude = "claude"
	AIConfigToolCodex  = "codex"

	AIConfigItemRules       = "rules"
	AIConfigItemPermissions = "permissions"
	AIConfigItemMCP         = "mcp"
)

// States of one item. A tool that is not installed is reported as one
// "absent" item per item kind and is never written.
const (
	AIConfigOK       = "ok"
	AIConfigDrift    = "drift"
	AIConfigConflict = "conflict"
	AIConfigAbsent   = "absent"
)

// Reasons for drift and conflict.
const (
	AIConfigMissing   = "missing"   // something the panel wrote is not there
	AIConfigExtra     = "extra"     // something the panel no longer wants is still there
	AIConfigDifferent = "different" // it is there but its content differs
	AIConfigExists    = "exists"    // a server of the same name that the panel did not write
	AIConfigInvalid   = "invalid"   // the file can not be parsed
	AIConfigMarkers   = "markers"   // the begin and end markers are not a single pair
)

// AIConfigMCP is one MCP server. Transport is "stdio" (Command and Args) or
// "http" (URL).
type AIConfigMCP struct {
	Name      string   `json:"name"`
	Transport string   `json:"transport"`
	Command   string   `json:"command,omitempty"`
	Args      []string `json:"args,omitempty"`
	URL       string   `json:"url,omitempty"`
}

// AIConfigTool is the panel's configuration for one tool. Allow, Ask and Deny
// are Claude Code permission rules and stay empty for Codex.
type AIConfigTool struct {
	Rules string        `json:"rules"`
	Allow []string      `json:"allow,omitempty"`
	Ask   []string      `json:"ask,omitempty"`
	Deny  []string      `json:"deny,omitempty"`
	MCP   []AIConfigMCP `json:"mcp,omitempty"`
}

// AIConfigDoc is everything the panel wants on a machine.
type AIConfigDoc struct {
	Claude AIConfigTool `json:"claude"`
	Codex  AIConfigTool `json:"codex"`
}

// AIConfigSyncParams: Apply false only checks, nothing is written.
type AIConfigSyncParams struct {
	Config AIConfigDoc `json:"config"`
	Apply  bool        `json:"apply"`
}

// AIConfigItem is the state of one item of one tool. After an apply it is the
// state read again after writing. Names holds at most 20 affected server names
// or permission rules. Error is set when writing failed.
type AIConfigItem struct {
	Tool    string   `json:"tool"`
	Item    string   `json:"item"`
	State   string   `json:"state"`
	Reason  string   `json:"reason,omitempty"`
	Names   []string `json:"names,omitempty"`
	Changed bool     `json:"changed,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// AIConfigSyncResult answers aiconfig.sync.
type AIConfigSyncResult struct {
	Items []AIConfigItem `json:"items"`
}

// Limits of the configuration, checked by Validate on both sides.
const (
	AIConfigMaxRules   = 64 * 1024
	AIConfigMaxPerms   = 100
	AIConfigMaxPerm    = 200
	AIConfigMaxServers = 20
	AIConfigMaxArgs    = 20
	AIConfigMaxText    = 500
)

// AIConfigMarker is the text both begin and end markers of the managed block
// contain. Rules text must not contain it, or the block could not be found.
const AIConfigMarker = "x-console:"

var aiConfigName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

func noControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// Normalize trims permission rules, drops empty and repeated ones and trims
// the rules text, so what is saved and compared has one shape.
func (d AIConfigDoc) Normalize() AIConfigDoc {
	d.Claude = d.Claude.normalize()
	d.Codex = d.Codex.normalize()
	return d
}

func (t AIConfigTool) normalize() AIConfigTool {
	t.Rules = strings.TrimSpace(strings.ReplaceAll(t.Rules, "\r\n", "\n"))
	t.Allow, t.Ask, t.Deny = tidy(t.Allow), tidy(t.Ask), tidy(t.Deny)
	servers := make([]AIConfigMCP, 0, len(t.MCP))
	for _, s := range t.MCP {
		s.Name = strings.TrimSpace(s.Name)
		s.Command = strings.TrimSpace(s.Command)
		s.URL = strings.TrimSpace(s.URL)
		s.Args = tidyArgs(s.Args)
		servers = append(servers, s)
	}
	t.MCP = servers
	return t
}

func tidy(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// tidyArgs only drops arguments that are empty; unlike permission rules a
// repeated argument is meaningful.
func tidyArgs(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// Validate checks a normalized configuration. The messages are shown to the
// user as they are.
func (d AIConfigDoc) Validate() error {
	if err := d.Claude.validate("Claude Code"); err != nil {
		return err
	}
	if err := d.Codex.validate("Codex"); err != nil {
		return err
	}
	if len(d.Codex.Allow)+len(d.Codex.Ask)+len(d.Codex.Deny) > 0 {
		return errors.New("Codex 这一版不支持权限设置")
	}
	return nil
}

func (t AIConfigTool) validate(who string) error {
	if len(t.Rules) > AIConfigMaxRules {
		return fmt.Errorf("%s 的规则太长，最多 64 KB", who)
	}
	if strings.Contains(t.Rules, AIConfigMarker+"begin") || strings.Contains(t.Rules, AIConfigMarker+"end") {
		return fmt.Errorf("%s 的规则里不能出现 %sbegin 或 %send", who, AIConfigMarker, AIConfigMarker)
	}
	for _, g := range []struct {
		label string
		list  []string
	}{{"允许", t.Allow}, {"询问", t.Ask}, {"禁止", t.Deny}} {
		if len(g.list) > AIConfigMaxPerms {
			return fmt.Errorf("%s 的“%s”最多 %d 条", who, g.label, AIConfigMaxPerms)
		}
		for _, v := range g.list {
			if len(v) > AIConfigMaxPerm || !noControl(v) {
				return fmt.Errorf("%s 的“%s”里有不合格的条目：每条最多 %d 个字符，不能含换行", who, g.label, AIConfigMaxPerm)
			}
		}
	}
	if len(t.MCP) > AIConfigMaxServers {
		return fmt.Errorf("%s 的 MCP 服务器最多 %d 个", who, AIConfigMaxServers)
	}
	seen := map[string]bool{}
	for _, s := range t.MCP {
		if !aiConfigName.MatchString(s.Name) {
			return fmt.Errorf("%s 的 MCP 服务器名称只能用字母、数字、_ 和 -，最多 40 个字符", who)
		}
		if seen[s.Name] {
			return fmt.Errorf("%s 里有两个叫 %s 的 MCP 服务器", who, s.Name)
		}
		seen[s.Name] = true
		switch s.Transport {
		case "stdio":
			if s.Command == "" || len(s.Command) > AIConfigMaxText || !noControl(s.Command) {
				return fmt.Errorf("MCP 服务器 %s 要填命令，不能含换行，最多 %d 个字符", s.Name, AIConfigMaxText)
			}
			if s.URL != "" {
				return fmt.Errorf("MCP 服务器 %s 是本机进程，不要填地址", s.Name)
			}
			if len(s.Args) > AIConfigMaxArgs {
				return fmt.Errorf("MCP 服务器 %s 的参数最多 %d 个", s.Name, AIConfigMaxArgs)
			}
			for _, a := range s.Args {
				if len(a) > AIConfigMaxPerm || !noControl(a) {
					return fmt.Errorf("MCP 服务器 %s 的参数每个最多 %d 个字符，不能含换行", s.Name, AIConfigMaxPerm)
				}
			}
		case "http":
			if s.Command != "" || len(s.Args) > 0 {
				return fmt.Errorf("MCP 服务器 %s 是远程服务器，不要填命令和参数", s.Name)
			}
			u, err := url.Parse(s.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
				len(s.URL) > AIConfigMaxText || !noControl(s.URL) || strings.ContainsAny(s.URL, " \t") {
				return fmt.Errorf("MCP 服务器 %s 的地址要以 http:// 或 https:// 开头", s.Name)
			}
		default:
			return fmt.Errorf("MCP 服务器 %s 的传输方式只能是 stdio 或 http", s.Name)
		}
	}
	return nil
}
