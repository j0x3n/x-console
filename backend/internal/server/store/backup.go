package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Backup(ctx context.Context, source, destination string) error {
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("backup already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return err
	}
	conn, err := sql.Open("sqlite", "file:"+source+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "VACUUM INTO ?", destination); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	return nil
}
