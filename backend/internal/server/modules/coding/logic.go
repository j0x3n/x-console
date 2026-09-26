package coding

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// slugify makes the short name of a branch: lower-case ASCII words joined
// by "-", at most 32 characters. It returns "" when nothing is left (for
// example a Chinese prompt).
func slugify(s string) string {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	out := ""
	for _, w := range words {
		next := w
		if out != "" {
			next = out + "-" + w
		}
		if len(next) > 32 {
			if out == "" {
				out = w[:32]
			}
			break
		}
		out = next
	}
	return out
}

// branchName is xc/<id>-<slug>.
func branchName(id int64, slugSources ...string) string {
	slug := ""
	for _, s := range slugSources {
		if slug = slugify(s); slug != "" {
			break
		}
	}
	if slug == "" {
		slug = "task"
	}
	return "xc/" + itoa(id) + "-" + slug
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// titleOf is the first non-empty line of the prompt, at most 120 runes.
func titleOf(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return shorten(line, 120)
		}
	}
	return ""
}

func shorten(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

var (
	scpRemote = regexp.MustCompile(`^[\w.-]+@github\.com:([\w.-]+)/([\w.-]+?)(?:\.git)?/?$`)
	pathPart  = regexp.MustCompile(`^/([\w.-]+)/([\w.-]+?)(?:\.git)?/?$`)
)

// githubRepo returns "owner/name" for a GitHub remote URL, or "".
func githubRepo(remote string) string {
	remote = strings.TrimSpace(remote)
	if m := scpRemote.FindStringSubmatch(remote); m != nil {
		return m[1] + "/" + m[2]
	}
	u, err := url.Parse(remote)
	if err != nil || !strings.EqualFold(u.Hostname(), "github.com") {
		return ""
	}
	if m := pathPart.FindStringSubmatch(u.Path); m != nil {
		return m[1] + "/" + m[2]
	}
	return ""
}

// issuePrompt puts the issue in front of the user's prompt.
func issuePrompt(key, title, description, prompt string) string {
	var b strings.Builder
	b.WriteString(key + ": " + strings.TrimSpace(title) + "\n")
	if d := strings.TrimSpace(description); d != "" {
		b.WriteString("\n" + d + "\n")
	}
	if p := strings.TrimSpace(prompt); p != "" {
		b.WriteString("\n" + p + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// commitMessage is the default commit message of a task.
func commitMessage(id int64, prompt, issueKey string) string {
	subject := shorten(titleOf(prompt), 72)
	if subject == "" {
		subject = "Coding task #" + itoa(id)
	}
	msg := subject + "\n\nX Console coding task #" + itoa(id)
	if issueKey != "" {
		msg += "\nIssue: " + issueKey
	}
	return msg
}

// Task statuses.
const (
	statusQueued    = "queued"
	statusRunning   = "running"
	statusReview    = "review"
	statusFailed    = "failed"
	statusCanceled  = "canceled"
	statusCommitted = "committed"
	statusPushed    = "pushed"
	statusPROpened  = "pr_opened"
	statusDiscarded = "discarded"
)

var allStatuses = []string{statusQueued, statusRunning, statusReview, statusFailed, statusCanceled,
	statusCommitted, statusPushed, statusPROpened, statusDiscarded}

func oneOf(s string, list ...string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
