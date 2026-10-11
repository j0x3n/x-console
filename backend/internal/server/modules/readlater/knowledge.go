package readlater

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// knowledgeSource gives the knowledge module (B139) the saved articles that
// have a body: the user's own note and the AI summary come first, then the
// text. The link is the address of the article.
type knowledgeSource struct{ m *Module }

func (knowledgeSource) Module() string { return "readlater" }

func (s knowledgeSource) Each(ctx context.Context, fn func(contracts.KnowledgeDoc) error) error {
	var after int64
	for {
		rows, err := s.m.d.DB.QueryContext(ctx,
			`SELECT id, url, title, note, summary, substr(content, 1, 20000), created_at FROM read_items
			 WHERE status = 'ready' AND content <> '' AND id > ? ORDER BY id LIMIT 20`, after)
		if err != nil {
			return err
		}
		var docs []contracts.KnowledgeDoc
		for rows.Next() {
			var id int64
			var url, title, note, summary, content string
			var at time.Time
			if err := rows.Scan(&id, &url, &title, &note, &summary, &content, &at); err != nil {
				rows.Close()
				return err
			}
			after = id
			if strings.TrimSpace(title) == "" {
				title = url
			}
			var text strings.Builder
			for _, part := range []string{note, summary, content} {
				if p := strings.TrimSpace(part); p != "" {
					text.WriteString(p)
					text.WriteString("\n\n")
				}
			}
			docs = append(docs, contracts.KnowledgeDoc{Ref: strconv.FormatInt(id, 10), Title: title, Text: text.String(), Link: url, At: at})
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
		if len(docs) < 20 {
			return nil
		}
	}
}
