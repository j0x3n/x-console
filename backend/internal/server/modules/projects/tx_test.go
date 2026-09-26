package projects

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/db"
	"github.com/j0x3n/x-console/backend/internal/server/store"
)

// On a file database (several connections, WAL) concurrent transactions that
// read before they write must wait for each other instead of failing with
// SQLITE_BUSY. The in-memory test server uses one connection and cannot show this.
func TestConcurrentWritesOnFileDB(t *testing.T) {
	ctx := context.Background()
	conn, err := store.Open(ctx, filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	now := time.Now().UTC()
	pid, err := db.New(conn).CreateProject(ctx, db.CreateProjectParams{Key: "XC", Name: "X", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{d: &module.Deps{DB: conn}, q: db.New(conn), now: func() time.Time { return time.Now().UTC() }}
	var wg sync.WaitGroup
	errs := make(chan error, 30)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- m.tx(ctx, func(q *db.Queries) error {
				if _, err := q.GetProject(ctx, pid); err != nil { // read first
					return err
				}
				_, err := q.TakeIssueNumber(ctx, pid) // then write
				return err
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
	var next int64
	if err := conn.QueryRowContext(ctx, "SELECT next_number FROM projects WHERE id = ?", pid).Scan(&next); err != nil || next != 31 {
		t.Fatalf("next_number = %d, %v", next, err)
	}
}
