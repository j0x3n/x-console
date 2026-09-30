package hostagent

import (
	"encoding/json"
	"regexp"
	"strings"
)

type Effect string

const (
	Read      Effect = "read"
	Write     Effect = "write"
	Dangerous Effect = "dangerous"
)

var dangerRules = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:^|[;&|\s])(?:[^\s]*/)?rm\s+(?:-[^\s]*\s+)*(?:-[^\s]*r[^\s]*\s+|--recursive\s+)(?:-[^\s]*\s+)*(?:--\s+)?(?:/|/\*|~|/etc|/usr|/var|/home)(?:/|\s|$)`),
	regexp.MustCompile(`(?i)(?:^|[;&|\s])(?:[^\s]*/)?mkfs(?:\.[^\s]+)?(?:\s|$)`),
	regexp.MustCompile(`(?i)(?:^|[;&|\s])(?:[^\s]*/)?dd\b[^\n]*\bof=/dev/`),
	regexp.MustCompile(`(?i)(?:^|[;&|\s])(?:[^\s]*/)?(?:shutdown|reboot|poweroff|halt)(?:\s|$)`),
	regexp.MustCompile(`(?i)(?:^|[;&|\s])(?:[^\s]*/)?init\s+[06](?:\s|$)`),
	regexp.MustCompile(`:\s*\(\s*\)\s*\{`),
	regexp.MustCompile(`(?i)(?:^|[;&|\s])(?:[^\s]*/)?iptables\b[^\n]*\s-F(?:\s|$)`),
	regexp.MustCompile(`(?i)(?:^|[;&|\s])(?:[^\s]*/)?nft\s+flush\b`),
	regexp.MustCompile(`(?i)(?:^|[;&|\s])(?:[^\s]*/)?ufw\s+disable\b`),
	regexp.MustCompile(`(?i)(?:^|[;&|\s])(?:[^\s]*/)?chmod\s+-R\s+777\s+/(?:\s|$)`),
	regexp.MustCompile(`(?i)(?:^|[;&|\s])(?:[^\s]*/)?chown\s+-R\b[^\n]*\s+(?:/|/\*|~|/etc|/usr|/var|/home)(?:/|\s|$)`),
	regexp.MustCompile(`(?i)>\s*/dev/sd`),
}

var simpleReads = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "grep": true, "egrep": true,
	"df": true, "du": true, "free": true, "uptime": true, "ps": true, "ss": true,
	"netstat": true, "wc": true, "sort": true, "uniq": true, "stat": true,
	"file": true, "which": true, "whoami": true, "id": true, "hostname": true,
	"uname": true, "date": true, "lsblk": true, "env": true, "printenv": true,
	"journalctl": true, "dir": true, "type": true, "ipconfig": true, "tasklist": true,
}

func Classify(tool string, input json.RawMessage) Effect {
	switch strings.TrimPrefix(tool, "host__") {
	case "read_file", "list_dir", "system_info", "list_processes", "list_services", "list_containers":
		return Read
	case "run_command":
		var args struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(input, &args) != nil {
			return Write
		}
		return ClassifyCommand(args.Command)
	default:
		return Write
	}
}

func ClassifyCommand(command string) Effect {
	command = strings.TrimSpace(command)
	if command == "" {
		return Write
	}
	for _, rule := range dangerRules {
		if rule.MatchString(command) {
			return Dangerous
		}
	}
	// A single & also starts another command (background in sh, a separator in cmd.exe).
	if strings.ContainsAny(command, "><;&`\r\n\\") || strings.Contains(command, "||") || strings.Contains(command, "$(") {
		return Write
	}
	segments := strings.Split(command, "|")
	for _, segment := range segments {
		fields := strings.Fields(segment)
		if len(fields) == 0 {
			return Write
		}
		if fields[0] == "sudo" {
			return Write
		}
		name := fields[0]
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		if !readSegment(name, fields[1:]) {
			return Write
		}
	}
	return Read
}

func readSegment(name string, args []string) bool {
	joined := strings.Join(args, " ")
	switch name {
	case "find":
		// -fprint* and -fls write their output to a file.
		return !strings.Contains(joined, "-delete") && !strings.Contains(joined, "-exec") && !strings.Contains(joined, "-ok") &&
			!strings.Contains(joined, "-fprint") && !strings.Contains(joined, "-fls")
	case "top":
		return hasArg(args, "-b")
	case "ip":
		if len(args) == 0 || !in(args[0], "addr", "route", "link") {
			return false
		}
		for _, arg := range args[1:] {
			if in(arg, "set", "add", "del", "delete", "flush", "replace", "change") {
				return false
			}
		}
		return true
	case "awk":
		return !strings.Contains(strings.ReplaceAll(joined, " ", ""), "system(")
	case "sed":
		return readSed(args)
	case "uniq":
		return readUniq(args)
	case "ss":
		return !hasArg(args, "-K") && !hasArg(args, "--kill")
	case "file":
		return !hasArg(args, "-C") && !hasArg(args, "--compile")
	case "env", "hostname":
		return len(args) == 0
	case "date":
		return !hasArg(args, "-s") && !hasArg(args, "--set")
	case "ipconfig":
		for _, arg := range args {
			if in(strings.ToLower(arg), "/release", "/renew", "/flushdns", "/registerdns") {
				return false
			}
		}
		return true
	case "sort":
		for _, arg := range args {
			if arg == "-o" || strings.HasPrefix(arg, "--output") {
				return false
			}
		}
		return true
	case "journalctl":
		for _, arg := range args {
			if strings.HasPrefix(arg, "--vacuum") || in(arg, "--rotate", "--sync", "--flush", "--relinquish-var") {
				return false
			}
		}
		return true
	case "mount":
		return len(args) == 0
	case "systemctl":
		return len(args) > 0 && in(args[0], "status", "list-units", "is-active", "is-enabled", "show")
	case "docker":
		if len(args) == 0 || !in(args[0], "ps", "logs", "inspect", "images", "stats") {
			return false
		}
		return args[0] != "stats" || hasArg(args[1:], "--no-stream")
	}
	if simpleReads[name] {
		return true
	}
	return strings.HasPrefix(name, "Get-") && len(name) > 4 && !strings.ContainsAny(joined, "$(){}@")
}

// sedPrint is a script that only prints lines picked by number or pattern,
// such as 10,20p or /error/p.
var sedPrint = regexp.MustCompile(`^(?:[0-9]+|\$|/[^/]*/)(?:,(?:[0-9]+|\$|/[^/]*/))?p$`)

// readSed allows only sed -n with a print script. sed can also write files
// (w, s///w) and run commands (e, s///e), so any other script is a write.
func readSed(args []string) bool {
	quiet, script := false, false
	for _, arg := range args {
		switch {
		case arg == "-n" || arg == "--quiet" || arg == "--silent":
			quiet = true
		case arg == "-E" || arg == "-r":
		case strings.HasPrefix(arg, "-"):
			return false
		case !script:
			if !sedPrint.MatchString(strings.Trim(arg, `'"`)) {
				return false
			}
			script = true
		}
	}
	return quiet && script
}

// readUniq rejects a second file name, which uniq writes its output to.
func readUniq(args []string) bool {
	files := 0
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case in(arg, "-f", "-s", "-w", "--skip-fields", "--skip-chars", "--check-chars"):
			i++
		case strings.HasPrefix(arg, "-"):
		default:
			files++
		}
	}
	return files <= 1
}

func hasArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}

func in(value string, allowed ...string) bool {
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	return false
}
