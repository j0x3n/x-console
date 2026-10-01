package store

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Read-then-write transactions on a file database must wait for each other
// instead of failing with SQLITE_BUSY. busy_timeout does not help a deferred
// transaction that upgrades from a read lock, so Open must make every
// transaction take the write lock when it begins.
func TestConcurrentReadThenWriteTransactions(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE counter (n INTEGER NOT NULL); INSERT INTO counter VALUES (0)`); err != nil {
		t.Fatal(err)
	}
	const workers = 30
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				errs <- err
				return
			}
			defer tx.Rollback()
			var n int
			if err := tx.QueryRow(`SELECT n FROM counter`).Scan(&n); err != nil {
				errs <- err
				return
			}
			if _, err := tx.Exec(`UPDATE counter SET n = ?`, n+1); err != nil {
				errs <- err
				return
			}
			errs <- tx.Commit()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("transaction failed: %v", err)
		}
	}
	var n int
	_ = db.QueryRow(`SELECT n FROM counter`).Scan(&n)
	if n != workers {
		t.Fatalf("lost updates: counter = %d, want %d", n, workers)
	}
}

func TestSnapshotCopiesAMigratedDatabase(t *testing.T) {
	ctx := context.Background()
	src, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.Exec(`INSERT INTO settings (key, value, updated_at) VALUES ('snap', '"1"', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	image, err := Snapshot(ctx, src)
	if err != nil || len(image) == 0 {
		t.Fatalf("snapshot: %d bytes, %v", len(image), err)
	}
	a, err := OpenSnapshot(ctx, image)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenSnapshot(ctx, image)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	// Each copy is its own database, can grow, and keeps foreign keys on.
	if _, err := a.Exec(`INSERT INTO settings (key, value, updated_at) VALUES ('only-a', ?, CURRENT_TIMESTAMP)`, strings.Repeat("x", 1<<20)); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := b.QueryRow(`SELECT count(*) FROM settings WHERE key IN ('snap', 'only-a')`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("copy b sees %d rows: %v", n, err)
	}
	var fk int
	if err := a.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign keys: %d %v", fk, err)
	}
	// Nothing left to migrate.
	if err := Migrate(ctx, a); err != nil {
		t.Fatal(err)
	}
}
