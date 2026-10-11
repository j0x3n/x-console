package cards

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// knowledgeSource gives the knowledge module (B139) the review cards that are
// not suspended. A card can come from a hidden note, so the cards count as
// notes: while notes are hidden and locked, so are the cards.
type knowledgeSource struct{ m *Module }

func (knowledgeSource) Module() string { return "notes" }

func (s knowledgeSource) Each(ctx context.Context, fn func(contracts.KnowledgeDoc) error) error {
	var after int64
	for {
		rows, err := s.m.d.DB.QueryContext(ctx,
			`SELECT id, question, answer, source_title FROM cards WHERE suspended = 0 AND id > ? ORDER BY id LIMIT 100`, after)
		if err != nil {
			return err
		}
		var docs []contracts.KnowledgeDoc
		for rows.Next() {
			var id int64
			var question, answer, source string
			if err := rows.Scan(&id, &question, &answer, &source); err != nil {
				rows.Close()
				return err
			}
			after = id
			title := question
			if utf8.RuneCountInString(title) > 60 {
				title = string([]rune(title)[:60]) + "…"
			}
			text := "问：" + question + "\n答：" + answer
			if s := strings.TrimSpace(source); s != "" {
				text += "\n来源：" + s
			}
			docs = append(docs, contracts.KnowledgeDoc{Ref: strconv.FormatInt(id, 10), Title: "卡片：" + title, Text: text, Link: "/cards"})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, d := range docs {
			if err := fn(d); err != nil {
				return err
			}
		}
		if len(docs) < 100 {
			return nil
		}
	}
}
