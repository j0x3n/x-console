// Package store opens the SQLite database and applies migrations.
//
// Every module adds its tables as a goose migration in store/migrations.
// File names are <UTC timestamp>_<module>_<summary>.sql, for example
// 20260927120000_m5_projects.sql, so parallel work never collides.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open opens (or creates) the database at path and migrates it to the latest version.
// Use ":memory:" in tests; the pool is then limited to one connection so all
// queries see the same in-memory database.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	db, err := open(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := Migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// open opens the database without migrating it.
func open(ctx context.Context, path string) (*sql.DB, error) {
	dsn := path
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		dsn = "file:" + path
	}
	// _txlock=immediate makes BEGIN take the write lock, so concurrent
	// read-then-write transactions wait on busy_timeout instead of failing
	// with SQLITE_BUSY when they upgrade from a read lock.
	dsn += "?_txlock=immediate&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_time_format=sqlite"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// serializer is the part of the modernc.org/sqlite connection that copies a
// whole database to and from memory.
type serializer interface {
	Serialize() ([]byte, error)
	Deserialize([]byte) error
}

// Snapshot returns the whole content of a database opened with ":memory:".
// Tests migrate one database, take a snapshot and start every test from a
// copy (OpenSnapshot): running the migrations takes seconds under -race.
func Snapshot(ctx context.Context, db *sql.DB) ([]byte, error) {
	c, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	var out []byte
	err = c.Raw(func(dc any) error {
		s, ok := dc.(serializer)
		if !ok {
			return fmt.Errorf("store: driver cannot serialize")
		}
		out, err = s.Serialize()
		return err
	})
	return out, err
}

// OpenSnapshot opens a new in-memory database with the content of a
// Snapshot. It does not run the migrations: the snapshot must come from a
// database Open migrated with the same binary.
func OpenSnapshot(ctx context.Context, image []byte) (*sql.DB, error) {
	db, err := open(ctx, ":memory:")
	if err != nil {
		return nil, err
	}
	c, err := db.Conn(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}
	err = c.Raw(func(dc any) error {
		s, ok := dc.(serializer)
		if !ok {
			return fmt.Errorf("store: driver cannot deserialize")
		}
		return s.Deserialize(image)
	})
	c.Close()
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Migrate applies all embedded migrations.
func Migrate(ctx context.Context, db *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, mustSub(migrations, "migrations"))
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
