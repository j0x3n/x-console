package screentime

import "strings"

// Categories, in the order the page shows them.
const (
	catCoding        = "coding"
	catAI            = "ai"
	catChat          = "chat"
	catWeb           = "web"
	catEntertainment = "entertainment"
	catOffice        = "office"
	catOther         = "other"
)

var categories = []string{catCoding, catAI, catChat, catWeb, catEntertainment, catOffice, catOther}

func validCategory(c string) bool {
	for _, k := range categories {
		if k == c {
			return true
		}
	}
	return false
}

// rule is one of the user's own rules.
type rule struct {
	field    string // "app" or "title"
	pattern  string
	category string
}

// normApp lower-cases a program name and drops ".exe", so Code.exe, code.exe
// and code are the same program.
func normApp(app string) string {
	a := strings.ToLower(strings.TrimSpace(app))
	return strings.TrimSuffix(a, ".exe")
}

var builtinApps = map[string]string{}

func init() {
	for category, apps := range map[string][]string{
		catCoding:        {"code", "cursor", "windsurf", "devenv", "idea64", "idea", "pycharm64", "goland64", "webstorm64", "clion64", "rider64", "sublime_text", "notepad++", "windowsterminal", "wt", "powershell", "pwsh", "cmd", "mintty", "git-bash", "wsl"},
		catAI:            {"claude", "chatgpt", "codex", "copilot"},
		catChat:          {"wechat", "weixin", "qq", "telegram", "slack", "discord", "teams", "ms-teams", "zoom", "feishu", "lark", "dingtalk", "outlook", "olk", "thunderbird"},
		catOffice:        {"winword", "excel", "powerpnt", "onenote", "wps", "notion", "obsidian", "acrobat", "acrord32", "sumatrapdf"},
		catEntertainment: {"steam", "spotify", "vlc", "potplayer", "potplayermini64", "cloudmusic", "qqmusic", "epicgameslauncher", "battle.net", "netflix"},
	} {
		for _, a := range apps {
			builtinApps[a] = category
		}
	}
}

var browsers = map[string]bool{
	"chrome": true, "msedge": true, "firefox": true, "brave": true, "opera": true, "vivaldi": true, "arc": true, "iexplore": true,
}

// browserTitles are checked in this order. The first group with a hit wins.
var browserTitles = []struct {
	category string
	words    []string
}{
	{catAI, []string{"claude", "chatgpt", "gemini", "copilot", "deepseek", "grok", "kimi", "豆包", "通义", "perplexity"}},
	{catEntertainment, []string{"youtube", "bilibili", "哔哩哔哩", "netflix", "twitch", "抖音", "爱奇艺", "优酷", "腾讯视频", "disney+"}},
	{catCoding, []string{"github", "stack overflow", "mdn", "gitlab", "pkg.go.dev", "npm"}},
	{catChat, []string{"gmail", "outlook", "飞书", "slack"}},
}

// builtinApp is the category the built-in rules give a program, or false for
// browsers and programs nobody has a rule for.
func builtinApp(app string) (string, bool) {
	c, ok := builtinApps[normApp(app)]
	return c, ok
}

// customApp is the category the user's own rules give a program alone.
func customApp(rules []rule, app string) (string, bool) {
	n := normApp(app)
	for _, r := range rules {
		if r.field == "app" && normApp(r.pattern) == n {
			return r.category, true
		}
	}
	return "", false
}

// classify decides the category of one minute. Order: the user's rules in the
// order they were added (first match wins), the built-in rules, then other.
func classify(rules []rule, app, title string) string {
	n := normApp(app)
	lt := strings.ToLower(title)
	for _, r := range rules {
		switch r.field {
		case "app":
			if normApp(r.pattern) == n {
				return r.category
			}
		case "title":
			if lt != "" && strings.Contains(lt, strings.ToLower(r.pattern)) {
				return r.category
			}
		}
	}
	if c, ok := builtinApps[n]; ok {
		return c
	}
	if browsers[n] {
		for _, g := range browserTitles {
			for _, w := range g.words {
				if strings.Contains(lt, w) {
					return g.category
				}
			}
		}
		return catWeb
	}
	return catOther
}
