package store

import (
	"context"
	"path/filepath"
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
