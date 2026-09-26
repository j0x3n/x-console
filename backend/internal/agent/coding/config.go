package coding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Config is the "coding" object of the agent config file:
//
//	"coding": {
//	  "roots": ["~/code", "D:\\work"],
//	  "depth": 3,
//	  "executors": {
//	    "claude": {"path": "C:\\Users\\me\\.local\\bin\\claude.exe", "extraArgs": ["--model", "opus"]},
//	    "codex":  {"args": ["exec", "--json", "--sandbox", "workspace-write", "-"]}
//	  }
//	}
//
// Everything is optional.
type Config struct {
	// Roots are scanned for repositories. Default: code, projects and src in
	// the home directory. "~" at the start means the home directory.
	Roots []string `json:"roots,omitempty"`
	// Depth is how many directory levels below a root are scanned (default 3).
	Depth int `json:"depth,omitempty"`
	// Executors by name ("claude", "codex").
	Executors map[string]ExecutorConfig `json:"executors,omitempty"`
}

// ExecutorConfig overrides how one executor is started.
type ExecutorConfig struct {
	// Path of the program. Default: the executor name looked up on PATH.
	Path string `json:"path,omitempty"`
	// Args replace the default arguments. An argument containing "{prompt}"
	// gets the prompt substituted; without one the prompt goes to stdin.
	Args []string `json:"args,omitempty"`
	// ExtraArgs are appended to Args, for example a model choice.
	ExtraArgs []string `json:"extraArgs,omitempty"`
	// Env adds environment variables.
	Env map[string]string `json:"env,omitempty"`
	// Format picks the output parser: "claude" (stream-json) or "codex"
	// (exec --json). Default: the executor name.
	Format string `json:"format,omitempty"`
}

// Defaults. Verified against Claude Code 2.1 (`claude --help`): -p reads the
// prompt from stdin when no prompt argument is given. Codex was not
// installed during development; its arguments follow `codex exec --help` of
// the 0.4x releases, where "-" reads the prompt from stdin.
var defaultArgs = map[string][]string{
	protocol.ExecutorClaude: {"-p", "--output-format", "stream-json", "--verbose", "--permission-mode", "acceptEdits"},
	protocol.ExecutorCodex:  {"exec", "--json", "--full-auto", "-"},
}

// Executors lists the supported executor names in display order.
var Executors = []string{protocol.ExecutorClaude, protocol.ExecutorCodex}

const (
	defaultDepth   = 3
	defaultTimeout = 60 * time.Minute
	maxTimeout     = 24 * time.Hour
)

// ParseConfig decodes the raw config. Empty input gives the defaults.
func ParseConfig(raw json.RawMessage) (Config, error) {
	var c Config
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &c); err != nil {
			return c, err
		}
	}
	return c, nil
}

func (c Config) roots() []string {
	roots := c.Roots
	if len(roots) == 0 {
		roots = []string{"~/code", "~/projects", "~/src"}
	}
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		if r = expandHome(strings.TrimSpace(r)); r != "" {
			out = append(out, r)
		}
	}
	return out
}

func (c Config) depth() int {
	if c.Depth <= 0 {
		return defaultDepth
	}
	return c.Depth
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// command is a resolved executor invocation.
type command struct {
	path   string
	args   []string
	stdin  string // the prompt when no argument takes it
	env    []string
	format string
}

func (c Config) executor(name string) ExecutorConfig {
	return c.Executors[name]
}

// resolve finds the program of an executor. It does not run it.
func (c Config) resolve(name string) (string, error) {
	e := c.executor(name)
	path := e.Path
	if path == "" {
		path = name
	}
	return lookPath(expandHome(path))
}

// command builds the invocation of executor name for prompt.
func (c Config) command(name, prompt string) (command, error) {
	path, err := c.resolve(name)
	if err != nil {
		return command{}, err
	}
	e := c.executor(name)
	args := e.Args
	if len(args) == 0 {
		args = defaultArgs[name]
	}
	cmd := command{path: path, format: e.Format}
	if cmd.format == "" {
		cmd.format = name
	}
	usesArg := false
	for _, a := range args {
		if strings.Contains(a, "{prompt}") {
			usesArg = true
			a = strings.ReplaceAll(a, "{prompt}", prompt)
		}
		cmd.args = append(cmd.args, a)
	}
	cmd.args = append(cmd.args, e.ExtraArgs...)
	if !usesArg {
		cmd.stdin = prompt
	}
	for k, v := range e.Env {
		cmd.env = append(cmd.env, k+"="+v)
	}
	return cmd, nil
}

func validExecutor(name string) bool {
	for _, e := range Executors {
		if e == name {
			return true
		}
	}
	return false
}

// Timeout returns the effective run timeout.
func Timeout(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultTimeout
	}
	return min(time.Duration(seconds)*time.Second, maxTimeout)
}
