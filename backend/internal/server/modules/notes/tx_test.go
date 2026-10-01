package notes

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/db"
	"github.com/j0x3n/x-console/backend/internal/server/store"
)

// Autosave from several windows sends overlapping PATCHes. On a file database
// they must queue up, not fail with SQLITE_BUSY, and the FTS index must stay
// in sync with the last write.
func TestConcurrentUpdatesOnFileDB(t *testing.T) {
	ctx := context.Background()
	conn, err := store.Open(ctx, filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	now := time.Now().UTC()
	n, err := db.New(conn).CreateNote(ctx, db.CreateNoteParams{Body: "start", Kind: "note", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{d: &module.Deps{DB: conn}, q: db.New(conn)}
	var wg sync.WaitGroup
	errs := make(chan error, 30)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- m.tx(ctx, func(q *db.Queries) error {
				note, err := q.GetNote(ctx, n.ID) // read first
				if err != nil {
					return err
				}
				return q.UpdateNote(ctx, db.UpdateNoteParams{Title: note.Title, Body: note.Body + " 追加", Pinned: note.Pinned,
					UpdatedAt: time.Now().UTC(), Kind: note.Kind, Color: note.Color, ID: n.ID}) // then write
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent tx: %v", err)
		}
	}
	var hits int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM notes_fts WHERE notes_fts MATCH '"追加 追加"'`).Scan(&hits); err != nil || hits != 1 {
		t.Fatalf("fts hits = %d, %v", hits, err)
	}
}
