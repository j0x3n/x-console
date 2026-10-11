package knowledge

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "knowledge.search",
		Title: "搜索个人资料",
		Description: "Semantic search over the user's own notes, saved articles, diary and review cards. query is a question or a few words, up to 500 characters. " +
			"Returns up to 8 passages (source, title, link, snippet, score) that match the meaning, not only the words. Hidden notes are never included. " +
			"Fails when the user has not turned the knowledge index on. Answer from the snippets and cite their links.",
		Input:  actions.Schema(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, httpx.Invalid("参数不正确")
			}
			hits, err := m.search(ctx, in.Query)
			if err != nil {
				return nil, err
			}
			return map[string]any{"items": hits}, nil
		},
	})
}
