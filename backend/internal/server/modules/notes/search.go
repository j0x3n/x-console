package notes

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/db"
)

// Search. The FTS5 table uses the trigram tokenizer, which matches any
// substring of three or more characters, including Chinese words. Queries with
// a term shorter than that fall back to LIKE. sqlc cannot parse MATCH, so these
// queries use database/sql directly.

// Snippet markers around matched text (Unicode private use characters).
const (
	markOpen  = ""
	markClose = ""
)

// listFilter is the input of listNotes.
type listFilter struct {
	Q        string
	Tag      string
	Pinned   *bool
	Archived bool
	Hidden   bool
	Limit    int
	Offset   int
}

// listNotes returns one page and the offset of the next page (0 when done).
func (m *Module) listNotes(ctx context.Context, f listFilter) ([]api.NoteSummary, int, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Hidden && !auth.VaultUnlocked(ctx) {
		return []api.NoteSummary{}, 0, nil
	}
	var (
		notes    []db.Note
		snippets []string
		err      error
	)
	terms := strings.Fields(f.Q)
	switch {
	case len(terms) == 0:
		notes, err = m.plainList(ctx, f)
	case f.Hidden || !useFTS(terms):
		notes, snippets, err = m.likeSearch(ctx, f, terms)
	default:
		notes, snippets, err = m.ftsSearch(ctx, f, terms)
	}
	if err != nil {
		return nil, 0, err
	}
	next := 0
	if len(notes) > f.Limit {
		notes = notes[:f.Limit]
		next = f.Offset + f.Limit
	}
	ids := make([]int64, len(notes))
	for i, n := range notes {
		ids[i] = n.ID
	}
	tags := map[int64][]string{}
	if len(ids) > 0 {
		rows, err := m.q.ListTagsForNotes(ctx, ids)
		if err != nil {
			return nil, 0, err
		}
		for _, r := range rows {
			tags[r.NoteID] = append(tags[r.NoteID], r.Tag)
		}
	}
	thumbs := m.thumbnails(ctx, notes)
	out := make([]api.NoteSummary, len(notes))
	for i, n := range notes {
		out[i] = toSummary(n, tags[n.ID])
		out[i].Thumbnail = thumbs[n.ID]
		if i < len(snippets) && snippets[i] != "" {
			s := snippets[i]
			out[i].Snippet = &s
		}
	}
	return out, next, nil
}

// useFTS reports whether every term is long enough for the trigram index.
func useFTS(terms []string) bool {
	for _, t := range terms {
		if utf8.RuneCountInString(t) < 3 {
			return false
		}
	}
	return true
}

func (m *Module) plainList(ctx context.Context, f listFilter) ([]db.Note, error) {
	p := db.ListNotesParams{Archived: f.Archived, Hidden: boolInt(f.Hidden), Lim: int64(f.Limit + 1), Off: int64(f.Offset)}
	if f.Pinned != nil {
		p.Pinned = boolInt(*f.Pinned)
	}
	if f.Tag != "" {
		p.Tag = f.Tag
	}
	return m.q.ListNotes(ctx, p)
}

// filters builds the WHERE conditions shared by both searches.
func filters(f listFilter) ([]string, []any) {
	where := []string{"(n.archived_at IS NOT NULL) = ?", "n.hidden = ?"}
	args := []any{f.Archived, boolInt(f.Hidden)}
	if f.Pinned != nil {
		where = append(where, "n.pinned = ?")
		args = append(args, boolInt(*f.Pinned))
	}
	if f.Tag != "" {
		where = append(where, "n.id IN (SELECT note_id FROM note_tags WHERE tag = ?)")
		args = append(args, f.Tag)
	}
	return where, args
}

const noteColumns = "n.id, n.title, n.body, n.pinned, n.archived_at, n.created_at, n.updated_at, n.hidden"

func (m *Module) ftsSearch(ctx context.Context, f listFilter, terms []string) ([]db.Note, []string, error) {
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	where, args := filters(f)
	where = append([]string{"notes_fts MATCH ?"}, where...)
	args = append([]any{strings.Join(quoted, " ")}, args...)
	// Title matches weigh five times more than body matches.
	query := "SELECT " + noteColumns + ", snippet(notes_fts, -1, char(57344), char(57345), '…', 24)" +
		" FROM notes_fts JOIN notes n ON n.id = notes_fts.rowid WHERE " + strings.Join(where, " AND ") +
		" ORDER BY bm25(notes_fts, 5.0, 1.0), n.updated_at DESC LIMIT ? OFFSET ?"
	args = append(args, f.Limit+1, f.Offset)
	return m.query(ctx, query, args, true, nil)
}

func (m *Module) likeSearch(ctx context.Context, f listFilter, terms []string) ([]db.Note, []string, error) {
	where, args := filters(f)
	for _, t := range terms {
		pattern := "%" + escapeLike(t) + "%"
		where = append(where, `(n.title LIKE ? ESCAPE '\' OR n.body LIKE ? ESCAPE '\')`)
		args = append(args, pattern, pattern)
	}
	query := "SELECT " + noteColumns + " FROM notes n WHERE " + strings.Join(where, " AND ") +
		` ORDER BY (n.title LIKE ? ESCAPE '\') DESC, n.pinned DESC, n.updated_at DESC, n.id DESC LIMIT ? OFFSET ?`
	args = append(args, "%"+escapeLike(terms[0])+"%", f.Limit+1, f.Offset)
	return m.query(ctx, query, args, false, terms)
}

// query runs a search. With withSnippet the last column is the FTS snippet;
// otherwise a snippet is built from terms.
func (m *Module) query(ctx context.Context, query string, args []any, withSnippet bool, terms []string) ([]db.Note, []string, error) {
	rows, err := m.d.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var notes []db.Note
	var snippets []string
	for rows.Next() {
		var n db.Note
		dest := []any{&n.ID, &n.Title, &n.Body, &n.Pinned, &n.ArchivedAt, &n.CreatedAt, &n.UpdatedAt, &n.Hidden}
		var snip string
		if withSnippet {
			dest = append(dest, &snip)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, nil, err
		}
		if !withSnippet {
			snip = likeSnippet(n.Title, n.Body, terms)
		}
		notes = append(notes, n)
		snippets = append(snippets, snip)
	}
	return notes, snippets, rows.Err()
}

// likeSnippet cuts a window around the first match in the body (or title)
// and marks every term inside it.
func likeSnippet(title, body string, terms []string) string {
	text := body
	pos := firstMatch(body, terms)
	if pos < 0 {
		text, pos = title, firstMatch(title, terms)
	}
	if pos < 0 {
		return ""
	}
	runes := []rune(text)
	start, end := pos-20, pos+40
	prefix, suffix := "…", "…"
	if start <= 0 {
		start, prefix = 0, ""
	}
	if end >= len(runes) {
		end, suffix = len(runes), ""
	}
	window := strings.ReplaceAll(string(runes[start:end]), "\n", " ")
	return prefix + markTerms(window, terms) + suffix
}

// firstMatch returns the rune index of the earliest term in s, or -1.
func firstMatch(s string, terms []string) int {
	lower := strings.ToLower(s)
	best := -1
	for _, t := range terms {
		if i := strings.Index(lower, strings.ToLower(t)); i >= 0 {
			r := utf8.RuneCountInString(lower[:i])
			if best < 0 || r < best {
				best = r
			}
		}
	}
	return best
}

// markTerms wraps case-insensitive occurrences of terms with the markers.
func markTerms(s string, terms []string) string {
	lower := strings.ToLower(s)
	if len(lower) != len(s) {
		// Lowercasing changed byte lengths; fall back to exact matching.
		lower = s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		matched := 0
		for _, t := range terms {
			lt := strings.ToLower(t)
			if lt != "" && strings.HasPrefix(lower[i:], lt) && len(lt) > matched {
				matched = len(lt)
			}
		}
		if matched > 0 {
			b.WriteString(markOpen + s[i:i+matched] + markClose)
			i += matched
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		b.WriteString(s[i : i+size])
		i += size
	}
	return b.String()
}

// escapeLike escapes LIKE wildcards; use with ESCAPE '\'.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
