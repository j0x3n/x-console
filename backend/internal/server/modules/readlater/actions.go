package readlater

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
)

const maxSearchResults = 20

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "readlater.search",
		Title: "查找稍后阅读",
		Description: "Search the links the user saved to read later. Matches the title, address, summary, note and the saved article text. " +
			"Returns up to 20 items with id, title, url, site, summary, tags, read and createdAt. The article text is not returned. " +
			"Optional `q` (search words), `tag` and `view` (unread, read or all; default all).",
		Input:  actions.Schema(`{"type":"object","properties":{"q":{"type":"string"},"tag":{"type":"string"},"view":{"type":"string","enum":["unread","read","all"]}},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Q    string `json:"q"`
				Tag  string `json:"tag"`
				View string `json:"view"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, err
				}
			}
			if in.View == "" {
				in.View = "all"
			}
			list, err := m.list(ctx, in.View, strings.TrimSpace(in.Tag), in.Q)
			if err != nil {
				return nil, err
			}
			if len(list.Items) > maxSearchResults {
				list.Items = list.Items[:maxSearchResults]
			}
			return map[string]any{"items": list.Items, "counts": list.Counts}, nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:        "readlater.add",
		Title:       "存到稍后阅读",
		Description: "Save a link to read later. The page is fetched in the background and summarised. `url` must start with http or https. Optional `note`.",
		Input:       actions.Schema(`{"type":"object","properties":{"url":{"type":"string"},"note":{"type":"string"}},"required":["url"],"additionalProperties":false}`),
		Effect:      actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				URL  string `json:"url"`
				Note string `json:"note"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
			row, dup, err := m.add(ctx, in.URL, in.Note, "ai")
			m.d.Audit.Record(ctx, "readlater.create", "", map[string]any{"url": in.URL, "duplicate": dup, "channel": "ai"}, err)
			if err != nil {
				return nil, err
			}
			if !dup {
				m.publish("readlater.created", toView(row, false))
			}
			return map[string]any{"id": row.ID, "url": row.Url, "duplicate": dup}, nil
		},
	})
}
