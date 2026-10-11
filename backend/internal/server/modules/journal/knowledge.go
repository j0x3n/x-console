package journal

import (
	"context"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// knowledgeSource gives the knowledge module (B139) the diary: one document
// per day the user wrote something.
type knowledgeSource struct{ m *Module }

func (knowledgeSource) Module() string { return "journal" }

func (s knowledgeSource) Each(ctx context.Context, fn func(contracts.KnowledgeDoc) error) error {
	after := ""
	for {
		rows, err := s.m.d.DB.QueryContext(ctx,
			`SELECT day, body FROM journal_diary WHERE body <> '' AND day > ? ORDER BY day LIMIT 50`, after)
		if err != nil {
			return err
		}
		var docs []contracts.KnowledgeDoc
		for rows.Next() {
			var day, body string
			if err := rows.Scan(&day, &body); err != nil {
				rows.Close()
				return err
			}
			after = day
			docs = append(docs, contracts.KnowledgeDoc{Ref: day, Title: "日记 " + day, Text: body, Link: "/journal?date=" + day})
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
		if len(docs) < 50 {
			return nil
		}
	}
}
