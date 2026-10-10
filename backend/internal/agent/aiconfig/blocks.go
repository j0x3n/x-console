package aiconfig

import (
	"regexp"
	"strings"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// marks are the lines that fence the block the panel owns in a text file.
type marks struct{ begin, end string }

var (
	mdMarks = marks{
		begin: "<!-- " + protocol.AIConfigMarker + "begin 以下内容由 X Console 管理，请在面板里修改 -->",
		end:   "<!-- " + protocol.AIConfigMarker + "end -->",
	}
	tomlMarks = marks{
		begin: "# " + protocol.AIConfigMarker + "begin 以下内容由 X Console 管理，请在面板里修改",
		end:   "# " + protocol.AIConfigMarker + "end",
	}
)

// block is where the managed block sits in a file's lines. begin and end are
// the line numbers of the two markers.
type block struct {
	found      bool
	begin, end int
}

// findBlock looks for exactly one begin marker followed by one end marker.
// ok is false for anything else.
func findBlock(lines []string) (b block, ok bool) {
	nb, ne := 0, 0
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(t, "<!--") && !strings.HasPrefix(t, "#") {
			continue
		}
		switch {
		case strings.Contains(t, protocol.AIConfigMarker+"begin"):
			nb++
			b.begin = i
		case strings.Contains(t, protocol.AIConfigMarker+"end"):
			ne++
			b.end = i
		}
	}
	switch {
	case nb == 0 && ne == 0:
		return block{}, true
	case nb == 1 && ne == 1 && b.begin < b.end:
		b.found = true
		return b, true
	}
	return block{}, false
}

func splitLines(text string) []string { return strings.Split(text, "\n") }

func joinLines(lines []string) string { return strings.Join(lines, "\n") }

// content is what is between the markers.
func (b block) content(lines []string) string {
	return strings.TrimSpace(joinLines(lines[b.begin+1 : b.end]))
}

// withBlock returns the lines with the block replaced by body (a nil body
// removes it). When there is no block yet, body is added at the end after a
// blank line. The result ends with a line break unless it is empty.
func withBlock(lines []string, b block, m marks, body []string) []string {
	var fenced []string
	if body != nil {
		fenced = append(append([]string{m.begin}, body...), m.end)
	}
	if b.found {
		head := append([]string{}, lines[:b.begin]...)
		if body == nil && len(head) > 0 && head[len(head)-1] == "" {
			head = head[:len(head)-1]
		}
		return append(append(head, fenced...), lines[b.end+1:]...)
	}
	out := append([]string{}, lines...)
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) > 0 {
		out = append(out, "")
	}
	return append(append(out, fenced...), "")
}

func finish(lines []string, crlf bool) []byte {
	text := joinLines(lines)
	if crlf {
		text = toCRLF(text)
	}
	return []byte(text)
}

// planRules handles a Markdown rules file.
func planRules(path, want string, m marks) plan {
	text, crlf, err := readText(path)
	if err != nil {
		return plan{state: protocol.AIConfigConflict, reason: protocol.AIConfigInvalid}
	}
	lines := splitLines(text)
	b, ok := findBlock(lines)
	if !ok {
		return plan{state: protocol.AIConfigConflict, reason: protocol.AIConfigMarkers}
	}
	switch {
	case !b.found && want == "":
		return plan{state: protocol.AIConfigOK}
	case !b.found:
		return plan{state: protocol.AIConfigDrift, reason: protocol.AIConfigMissing, write: ruleWriter(path, lines, b, m, want, crlf)}
	case want == "":
		return plan{state: protocol.AIConfigDrift, reason: protocol.AIConfigExtra, write: ruleWriter(path, lines, b, m, want, crlf)}
	case b.content(lines) == want:
		return plan{state: protocol.AIConfigOK}
	}
	return plan{state: protocol.AIConfigDrift, reason: protocol.AIConfigDifferent, write: ruleWriter(path, lines, b, m, want, crlf)}
}

func ruleWriter(path string, lines []string, b block, m marks, want string, crlf bool) func() error {
	return func() error {
		var body []string
		if want != "" {
			body = splitLines(want)
		}
		return writeFileAtomic(path, finish(withBlock(lines, b, m, body), crlf), 0o644, true)
	}
}

// ---- Codex config.toml ----

// tomlString writes s as a TOML basic string. Validate has already refused
// control characters.
func tomlString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func tomlBody(servers []protocol.AIConfigMCP) []string {
	var out []string
	for i, s := range servers {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, "[mcp_servers."+s.Name+"]")
		if s.Transport == "http" {
			out = append(out, "url = "+tomlString(s.URL))
			continue
		}
		out = append(out, "command = "+tomlString(s.Command))
		if len(s.Args) > 0 {
			args := make([]string, len(s.Args))
			for j, a := range s.Args {
				args[j] = tomlString(a)
			}
			out = append(out, "args = ["+strings.Join(args, ", ")+"]")
		}
	}
	return out
}

// tableOutside reports which of the wanted servers already have a
// [mcp_servers.<name>] table in lines other than the managed block.
func tableOutside(lines []string, b block, servers []protocol.AIConfigMCP) []string {
	rest := lines
	if b.found {
		rest = append(append([]string{}, lines[:b.begin]...), lines[b.end+1:]...)
	}
	text := joinLines(rest)
	var out []string
	for _, s := range servers {
		re := regexp.MustCompile(`(?m)^[ \t]*\[[ \t]*mcp_servers[ \t]*\.[ \t]*["']?` + regexp.QuoteMeta(s.Name) + `["']?[ \t]*(?:\.[^\]\n]*)?\]`)
		if re.MatchString(text) {
			out = append(out, s.Name)
		}
	}
	return out
}

func planCodexMCP(path string, want []protocol.AIConfigMCP) plan {
	text, crlf, err := readText(path)
	if err != nil {
		return plan{state: protocol.AIConfigConflict, reason: protocol.AIConfigInvalid}
	}
	lines := splitLines(text)
	b, ok := findBlock(lines)
	if !ok {
		return plan{state: protocol.AIConfigConflict, reason: protocol.AIConfigMarkers}
	}
	if clash := tableOutside(lines, b, want); len(clash) > 0 {
		return plan{state: protocol.AIConfigConflict, reason: protocol.AIConfigExists, names: clash}
	}
	body := tomlBody(want)
	write := func() error {
		var fenced []string
		if len(want) > 0 {
			fenced = body
		}
		return writeFileAtomic(path, finish(withBlock(lines, b, tomlMarks, fenced), crlf), 0o644, true)
	}
	names := make([]string, len(want))
	for i, s := range want {
		names[i] = s.Name
	}
	switch {
	case !b.found && len(want) == 0:
		return plan{state: protocol.AIConfigOK}
	case !b.found:
		return plan{state: protocol.AIConfigDrift, reason: protocol.AIConfigMissing, names: names, write: write}
	case len(want) == 0:
		return plan{state: protocol.AIConfigDrift, reason: protocol.AIConfigExtra, write: write}
	case b.content(lines) == strings.TrimSpace(joinLines(body)):
		return plan{state: protocol.AIConfigOK}
	}
	return plan{state: protocol.AIConfigDrift, reason: protocol.AIConfigDifferent, names: names, write: write}
}
