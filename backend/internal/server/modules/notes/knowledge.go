package notes

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// knowledgeSource gives the knowledge module (B139) the notes and memos.
// Hidden notes and archived notes never leave this module.
type knowledgeSource struct{ m *Module }

func (knowledgeSource) Module() string { return "notes" }

// Each reads the notes a page at a time, so no database cursor is open while
// the caller works on a note.
func (s knowledgeSource) Each(ctx context.Context, fn func(contracts.KnowledgeDoc) error) error {
	var after int64
	for {
		rows, err := s.m.d.DB.QueryContext(ctx,
			`SELECT id, title, body, updated_at FROM notes WHERE hidden = 0 AND archived_at IS NULL AND id > ? ORDER BY id LIMIT 50`, after)
		if err != nil {
			return err
		}
		var docs []contracts.KnowledgeDoc
		for rows.Next() {
			var id int64
			var title, body string
			var at time.Time
			if err := rows.Scan(&id, &title, &body, &at); err != nil {
				rows.Close()
				return err
			}
			after = id
			if strings.TrimSpace(title) == "" {
				title = firstLine(body)
			}
			docs = append(docs, contracts.KnowledgeDoc{Ref: strconv.FormatInt(id, 10), Title: title, Text: body, Link: notePath(id), At: at})
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

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimLeft(s, "# ")
	if utf8.RuneCountInString(s) > 40 {
		s = string([]rune(s)[:40]) + "…"
	}
	return s
}
