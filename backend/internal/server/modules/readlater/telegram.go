package readlater

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

const (
	maxLinksPerMessage = 5
	maxTelegramNote    = 500
)

// A link ends at a space or at punctuation, including the full-width marks
// that follow a link in Chinese text.
var linkPattern = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `，。！？；：、（）【】《》「」『』“”‘’]+`)

// trimLink drops the punctuation that follows a link in a sentence.
func trimLink(s string) string {
	return strings.TrimRight(s, ".,;:!?)]}>、。，；：！？）】》」』”’")
}

// extractLinks returns the addresses written in text and the ones behind link
// text, once each, at most five.
func extractLinks(text string, hidden []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(raw string) {
		raw = trimLink(strings.TrimSpace(raw))
		if raw == "" || seen[raw] || len(out) == maxLinksPerMessage {
			return
		}
		seen[raw] = true
		out = append(out, raw)
	}
	for _, l := range linkPattern.FindAllString(text, -1) {
		add(l)
	}
	for _, l := range hidden {
		add(l)
	}
	return out
}

// HandleTelegramMessage saves the links in a message the user sent to the bot
// (B117). Messages without a link are not for this module.
func (m *Module) HandleTelegramMessage(ctx context.Context, msg contracts.TelegramMessage) (string, bool) {
	links := extractLinks(msg.Text, msg.Links)
	if len(links) == 0 {
		return "", false
	}
	note := ""
	if len(links) == 1 {
		// a few words next to a single link are the user's own note
		note = strings.TrimSpace(strings.ReplaceAll(msg.Text, links[0], ""))
		note = clipRunes(strings.TrimSpace(trimLink(note)), maxTelegramNote)
	}
	var saved, existed, bad int
	for _, link := range links {
		row, dup, err := m.add(ctx, link, note, "telegram")
		target := ""
		if err == nil {
			target = fmt.Sprint(row.ID)
		}
		m.d.Audit.Record(ctx, "readlater.create", target, map[string]any{"url": link, "duplicate": dup, "channel": "telegram"}, err)
		switch {
		case err != nil:
			bad++
		case dup:
			existed++
		default:
			saved++
			m.publish("readlater.created", toView(row, false))
		}
	}
	var parts []string
	if saved > 0 {
		parts = append(parts, fmt.Sprintf("已存 %d 条", saved))
	}
	if existed > 0 {
		if saved == 0 {
			parts = append(parts, fmt.Sprintf("这 %d 条之前已经存过", existed))
		} else {
			parts = append(parts, fmt.Sprintf("%d 条之前已经存过", existed))
		}
	}
	if bad > 0 {
		parts = append(parts, fmt.Sprintf("%d 条网址不能存", bad))
	}
	return strings.Join(parts, "，"), true
}
