package quota

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const claudeSignedOut = "Claude 没有登录，请在这台机器上用该账号的目录运行 claude auth login"

// claudeUsage runs Claude Code's /usage for the account in home and returns
// the text it prints. A var so tests can stand in for the CLI.
var claudeUsage = realClaudeUsage

// claudeMu lets one /usage run at a time: each starts a Claude Code process.
var claudeMu sync.Mutex

func readClaude(ctx context.Context, isDefault bool, home string) (protocol.QuotaReading, error) {
	if !isDefault {
		if st, err := os.Stat(home); err != nil || !st.IsDir() {
			return protocol.QuotaReading{}, signedOut("账号目录 %s 不存在，请先用这个目录登录 Claude Code", home)
		}
	}
	claudeMu.Lock()
	defer claudeMu.Unlock()
	text, err := claudeUsage(ctx, isDefault, home)
	if err != nil {
		return protocol.QuotaReading{}, err
	}
	ws, err := parseClaudeUsage(text, now())
	if err != nil {
		return protocol.QuotaReading{Windows: []protocol.QuotaWindow{}}, err
	}
	return protocol.QuotaReading{Windows: ws}, nil
}

var (
	ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	// "Current session: 13% used · resets Oct 1 at 3:30pm (Asia/Shanghai)"
	// "Current week (all models): 4% used · resets Oct 3 at 2pm (Asia/Shanghai)"
	// "Current week (Fable): 0% used"
	usageLineRE = regexp.MustCompile(`^Current (session|week(?: \(([^)]+)\))?):\s*([0-9.]+)% used(?:\s*·\s*resets (.+))?$`)
	// deniedRE finds an account problem in /usage's text. It wins over any
	// window, so a refusal is never shown as a reading.
	deniedRE = regexp.MustCompile(`(?i)\b(401|403)\b|not (logged|signed) in|signed out|sign-in (has )?expired|unauthorized|forbidden|authentication (failed|required)|invalid (access )?token`)
)

// parseClaudeUsage reads the windows /usage prints. now dates a reset that
// names no year.
func parseClaudeUsage(text string, now time.Time) ([]protocol.QuotaWindow, error) {
	text = strings.TrimSpace(ansiRE.ReplaceAllString(text, ""))
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if deniedRE.MatchString(line) {
			return nil, signedOut("Claude 的 /usage 没有返回额度：%s", clip(strings.TrimSpace(line)))
		}
	}
	out := []protocol.QuotaWindow{}
	for _, line := range lines {
		m := usageLineRE.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		used, err := strconv.ParseFloat(m[3], 64)
		if err != nil {
			continue
		}
		w := protocol.QuotaWindow{Name: "5 小时", UsedPercent: used, SpanSecs: 5 * 3600}
		if m[1] != "session" {
			w.Name, w.SpanSecs = "7 天", 7*86400
			if scope := strings.TrimSpace(m[2]); scope != "" && !strings.EqualFold(scope, "all models") {
				w.Name, w.Model = "7 天 · "+scope, strings.ToLower(strings.NewReplacer(" ", "-", ".", "-").Replace(scope))
			}
		}
		dup := false
		for _, x := range out {
			dup = dup || x.Name == w.Name
		}
		if dup {
			continue
		}
		if t, ok := claudeResetTime(m[4], now); ok {
			w.ResetsAt = &t
		}
		out = append(out, w)
	}
	if len(out) == 0 {
		return nil, unavailable("Claude 的 /usage 没有额度信息：%s", clip(text))
	}
	return out, nil
}

var (
	tzRE      = regexp.MustCompile(`^(.*?)\s*\(([^)]+)\)\s*$`)
	ampmGapRE = regexp.MustCompile(`\s+(am|pm)\b`)
)

// claudeResetTime reads "Oct 3 at 2pm (Asia/Shanghai)", "3:30pm (UTC)" and the
// like. ok is false when it cannot be read, and then no countdown is shown.
func claudeResetTime(s string, now time.Time) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	loc := now.Location()
	if m := tzRE.FindStringSubmatch(s); m != nil {
		l, err := time.LoadLocation(strings.TrimSpace(m[2]))
		if err != nil {
			return time.Time{}, false
		}
		s, loc = strings.TrimSpace(m[1]), l
	}
	s = strings.NewReplacer(" ", " ", " ", " ").Replace(strings.ToLower(s))
	s = ampmGapRE.ReplaceAllString(s, "$1")
	local := now.In(loc)
	for _, layout := range []string{"Jan 2, 2006 at 3:04pm", "Jan 2, 2006 at 3pm"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.UTC(), true
		}
	}
	for _, layout := range []string{"Jan 2 at 3:04pm", "Jan 2 at 3pm"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			t = time.Date(local.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, loc)
			if t.Before(local.Add(-24 * time.Hour)) {
				t = t.AddDate(1, 0, 0)
			}
			return t.UTC(), true
		}
	}
	for _, layout := range []string{"3:04pm", "3pm"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			t = time.Date(local.Year(), local.Month(), local.Day(), t.Hour(), t.Minute(), 0, 0, loc)
			if t.Before(local) {
				t = t.AddDate(0, 0, 1)
			}
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 200 {
		return string(r[:200]) + "…"
	}
	return s
}

// claudeBinary finds the claude executable: the PATH first, then where its
// installer puts it.
func claudeBinary() (string, error) {
	if p, err := lookPath("claude"); err == nil {
		return p, nil
	}
	if h, err := os.UserHomeDir(); err == nil {
		for _, rel := range []string{".local/bin/claude", ".claude/local/claude", ".claude/local/node_modules/.bin/claude", ".npm-global/bin/claude"} {
			p := filepath.Join(h, filepath.FromSlash(rel))
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, nil
			}
		}
	}
	return "", unavailable("这台机器上找不到 claude 命令")
}

func realClaudeUsage(ctx context.Context, isDefault bool, home string) (string, error) {
	binary, err := claudeBinary()
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "xc-quota-")
	if err != nil {
		return "", unavailable("无法创建临时目录：%v", err)
	}
	defer os.RemoveAll(tmp)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := commandContext(ctx, binary, "-p", "/usage", "--output-format", "json",
		"--tools", "", "--strict-mcp-config", "--setting-sources", "", "--no-session-persistence")
	cmd.Dir = tmp
	cmd.Stdin = strings.NewReader("")
	// CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC makes /usage print only an old
	// reading instead of asking, so it is dropped for this run.
	env := withEnv(os.Environ(), []string{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"}, "DISABLE_TELEMETRY=1", "DISABLE_ERROR_REPORTING=1", "DISABLE_AUTOUPDATER=1")
	if !isDefault {
		env = withEnv(env, []string{"CLAUDE_CONFIG_DIR"}, "CLAUDE_CONFIG_DIR="+home)
	}
	cmd.Env = env
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	runErr := cmd.Run()
	if res, ok := claudeResult(out.Bytes()); ok {
		if res.IsError {
			return "", classifyClaude(res.Result)
		}
		return res.Result, nil
	}
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		msg = strings.TrimSpace(out.String())
	}
	if msg == "" && runErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", unavailable("运行 claude /usage 超时")
		}
		return "", unavailable("运行 claude /usage 失败：%v", runErr)
	}
	return "", classifyClaude(msg)
}

// classifyClaude turns what Claude Code printed in place of an allowance into
// an account error when it names one, else a plain failure.
func classifyClaude(msg string) error {
	if deniedRE.MatchString(msg) {
		return signedOut("%s（Claude 说：%s）", claudeSignedOut, clip(msg))
	}
	return unavailable("Claude 的 /usage 失败：%s", clip(msg))
}

type claudeRun struct {
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
}

// claudeResult reads the result from what claude -p --output-format json
// printed: one object, or an array of messages with the result last.
func claudeResult(b []byte) (claudeRun, bool) {
	var res claudeRun
	if json.Unmarshal(b, &res) == nil {
		return res, true
	}
	var all []claudeRun
	if json.Unmarshal(b, &all) == nil {
		for i := len(all) - 1; i >= 0; i-- {
			if all[i].Result != "" || all[i].IsError {
				return all[i], true
			}
		}
	}
	return claudeRun{}, false
}

// commandContext is exec.CommandContext with the console window hidden on
// Windows.
func commandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	hide(cmd)
	return cmd
}

// withEnv returns env without the variables named in drop, then add.
func withEnv(env, drop []string, add ...string) []string {
	out := make([]string, 0, len(env)+len(add))
next:
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		for _, d := range drop {
			if strings.EqualFold(k, d) {
				continue next
			}
		}
		out = append(out, e)
	}
	return append(out, add...)
}
