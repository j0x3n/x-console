package contracts

import (
	"context"
	"strings"
)

type ignoreHiddenKey struct{}

// IgnoreHidden marks work the user set up in advance, such as automation
// steps. It runs in the background without a session, so it would otherwise
// always count as locked and lose every hidden module.
func IgnoreHidden(ctx context.Context) context.Context {
	return context.WithValue(ctx, ignoreHiddenKey{}, true)
}

// HidingIgnored reports whether ctx was marked by IgnoreHidden.
func HidingIgnored(ctx context.Context) bool {
	return ctx.Value(ignoreHiddenKey{}) != nil
}

func BackendBlocked(ctx context.Context, h HiddenModules, backend, path string) bool {
	if h == nil {
		return false
	}
	switch backend {
	case "reminders":
		if !strings.Contains(path, "/reminders") {
			return false
		}
		return h.Hidden(ctx, "reminders")
	case "hosts":
		return h.Hidden(ctx, "servers") && h.Hidden(ctx, "pc")
	default:
		side, ok := backendSidebar[backend]
		if !ok {
			return false
		}
		return h.Hidden(ctx, side)
	}
}

var backendSidebar = map[string]string{
	"projects":      "projects",
	"coding":        "coding",
	"aiagents":      "coding",
	"notes":         "notes",
	"mail":          "mail",
	"habits":        "habits",
	"drive":         "drive",
	"calendar":      "calendar",
	"monitoring":    "monitoring",
	"homeassistant": "home",
	"automations":   "automations",
	"github":        "github",
	"linear":        "github",
	"router":        "router", // B65
}

func ActionHidden(ctx context.Context, h HiddenModules, name string) bool {
	if h == nil {
		return false
	}
	prefix := name
	if i := strings.IndexByte(name, '.'); i > 0 {
		prefix = name[:i]
	}
	switch prefix {
	case "projects", "issues", "milestones":
		return h.Hidden(ctx, "projects")
	case "coding", "aiagents":
		return h.Hidden(ctx, "coding")
	case "notes":
		return h.Hidden(ctx, "notes")
	case "reminders":
		return h.Hidden(ctx, "reminders")
	case "habits", "workouts":
		return h.Hidden(ctx, "habits")
	case "drive":
		return h.Hidden(ctx, "drive")
	case "calendar":
		return h.Hidden(ctx, "calendar")
	case "hosts":
		return h.Hidden(ctx, "servers") && h.Hidden(ctx, "pc")
	case "scripts", "monitors", "subscriptions", "monitoring":
		return h.Hidden(ctx, "monitoring")
	case "ha", "homeassistant":
		return h.Hidden(ctx, "home")
	case "automations":
		return h.Hidden(ctx, "automations")
	case "github", "linear":
		return h.Hidden(ctx, "github")
	case "mail":
		return h.Hidden(ctx, "mail")
	default:
		return false
	}
}

func EventRelevant(topic string) bool {
	_, ok := eventSidebar(topic)
	return ok
}

func EventHidden(ctx context.Context, h HiddenModules, topic string) bool {
	side, ok := eventSidebar(topic)
	if !ok || h == nil || side == "hosts" {
		return false
	}
	return h.Hidden(ctx, side)
}

func eventSidebar(topic string) (string, bool) {
	switch {
	case strings.HasPrefix(topic, "host."):
		return "hosts", true
	case strings.HasPrefix(topic, "note."):
		return "notes", true
	case strings.HasPrefix(topic, "issue"), strings.HasPrefix(topic, "project"),
		strings.HasPrefix(topic, "board."), strings.HasPrefix(topic, "label."), strings.HasPrefix(topic, "milestone."):
		return "projects", true
	case strings.HasPrefix(topic, "coding_"), strings.HasPrefix(topic, "ai_agent."), strings.HasPrefix(topic, "git_connection."):
		return "coding", true
	case strings.HasPrefix(topic, "reminder."):
		return "reminders", true
	case strings.HasPrefix(topic, "habit."), strings.HasPrefix(topic, "workout."):
		return "habits", true
	case strings.HasPrefix(topic, "drive"):
		return "drive", true
	case strings.HasPrefix(topic, "calendar."):
		return "calendar", true
	case strings.HasPrefix(topic, "monitor."), strings.HasPrefix(topic, "script"),
		strings.HasPrefix(topic, "subscription."), strings.HasPrefix(topic, "docker."):
		return "monitoring", true
	case strings.HasPrefix(topic, "github."), strings.HasPrefix(topic, "linear."):
		return "github", true
	case strings.HasPrefix(topic, "ha."):
		return "home", true
	case strings.HasPrefix(topic, "automation."):
		return "automations", true
	case strings.HasPrefix(topic, "mail."):
		return "mail", true
	case strings.HasPrefix(topic, "router."): // B65
		return "router", true
	default:
		return "", false
	}
}

func NoticeHidden(ctx context.Context, h HiddenModules, source, link string) bool {
	if h == nil {
		return false
	}
	if side, decided := linkSidebar(link); decided {
		if side == "" {
			return false
		}
		if side == "hosts" {
			return h.Hidden(ctx, "servers") && h.Hidden(ctx, "pc")
		}
		return h.Hidden(ctx, side)
	}
	return sourceHidden(ctx, h, source)
}

func linkSidebar(link string) (string, bool) {
	if link == "" {
		return "", false
	}
	path := link
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	if !strings.HasPrefix(path, "/") {
		return "", false
	}
	seg, _, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	switch seg {
	case "", "settings":
		return "", true
	case "pc", "servers", "notes", "mail", "projects", "coding", "reminders", "habits",
		"drive", "calendar", "monitoring", "home", "automations", "github",
		"router": // B65
		return seg, true
	default:
		return "", false
	}
}

func sourceHidden(ctx context.Context, h HiddenModules, source string) bool {
	src, _, _ := strings.Cut(source, ":")
	switch src {
	case "projects", "notes", "mail", "reminders", "habits", "drive", "calendar", "monitoring", "github", "coding":
		return h.Hidden(ctx, src)
	case "router": // B65
		return h.Hidden(ctx, "router")
	case "home", "homeassistant", "ha":
		return h.Hidden(ctx, "home")
	case "automations", "automation":
		return h.Hidden(ctx, "automations")
	case "linear":
		return h.Hidden(ctx, "github")
	case "hosts":
		return h.Hidden(ctx, "servers") && h.Hidden(ctx, "pc")
	default:
		return false
	}
}

func BriefHidden(ctx context.Context, h HiddenModules, section string) bool {
	if h == nil {
		return false
	}
	switch section {
	case "calendar":
		return h.Hidden(ctx, "calendar")
	case "issues":
		return h.Hidden(ctx, "projects")
	case "reminders":
		return h.Hidden(ctx, "reminders")
	case "habits":
		return h.Hidden(ctx, "habits")
	case "renewals":
		return h.Hidden(ctx, "monitoring")
	case "alerts":
		return h.Hidden(ctx, "servers") && h.Hidden(ctx, "pc")
	default:
		return false
	}
}
