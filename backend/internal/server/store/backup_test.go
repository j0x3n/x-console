package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupCopiesCommittedDataWhileDatabaseIsOpen(t *testing.T) {
	ctx := context.Background()
	source := filepath.Join(t.TempDir(), "data", "x-console.db")
	db, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE TABLE backup_probe (value TEXT); INSERT INTO backup_probe VALUES ('before')`); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "backups", "snapshot.db")
	if err := Backup(ctx, source, destination); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO backup_probe VALUES ('after')`); err != nil {
		t.Fatal(err)
	}
	snapshot, err := sql.Open("sqlite", "file:"+destination+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	var value string
	if err := snapshot.QueryRowContext(ctx, `SELECT value FROM backup_probe`).Scan(&value); err != nil || value != "before" {
		t.Fatalf("snapshot value = %q, err = %v", value, err)
	}
	var count int
	if err := snapshot.QueryRowContext(ctx, `SELECT count(*) FROM backup_probe`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("snapshot rows = %d, err = %v", count, err)
	}
	if err := Backup(ctx, source, destination); err == nil {
		t.Fatal("overwrote existing backup")
	}
}

func TestBackupRequiresExistingDatabase(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "snapshot.db")
	if err := Backup(context.Background(), filepath.Join(t.TempDir(), "missing.db"), destination); err == nil {
		t.Fatal("missing source accepted")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup file: %v", err)
	}
}
