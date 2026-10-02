package backup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/repo"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
	"github.com/j0x3n/x-console/backend/internal/server/store"
)

func startupTestConfig(t *testing.T) (config.Config, *sql.DB, *settings.Store) {
	t.Helper()
	cfg := config.Config{DataDir: t.TempDir(), MasterKey: bytes.Repeat([]byte{7}, 32), Location: time.UTC}
	conn, err := store.Open(context.Background(), cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	box, err := secrets.NewBox(cfg.MasterKey)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, conn, settings.New(conn, box)
}

func putStartupFile(t *testing.T, root, key, body string) {
	t.Helper()
	if err := (files.Local{Root: root}).Put(context.Background(), key, strings.NewReader(body), int64(len(body))); err != nil {
		t.Fatal(err)
	}
}

func stageStartupDB(t *testing.T, cfg config.Config, conn *sql.DB) {
	t.Helper()
	dir := cfg.RestoreDir()
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := snapshotDB(context.Background(), conn, filepath.Join(dir, dbName)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, readyName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStartupRestoreBacksUpFinalSceneBeforeReplacingDatabase(t *testing.T) {
	ctx := context.Background()
	cfg, conn, st := startupTestConfig(t)
	if err := st.Set(ctx, "test.scene", "target"); err != nil {
		t.Fatal(err)
	}
	stageStartupDB(t, cfg, conn)
	putStartupFile(t, filepath.Join(cfg.RestoreDir(), "files"), "notes/target", "target file")
	if err := st.Set(ctx, "test.scene", "last scene"); err != nil {
		t.Fatal(err)
	}
	putStartupFile(t, cfg.FilesDir(), "notes/extra", "last scene file")
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := PrepareBeforeRestore(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(cfg.RestoreDir(), "pre-backup"))
	if err != nil {
		t.Fatal(err)
	}
	rc, _, err := (files.Local{Root: cfg.BackupsDir()}).Get(ctx, string(raw))
	if err != nil {
		t.Fatal(err)
	}
	ar, err := openArchive(rc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ar.readManifest(); err != nil {
		t.Fatal(err)
	}
	found := false
	for {
		hdr, err := ar.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(ar.tr)
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Name == "files/notes/extra" {
			found = string(body) == "last scene file"
		}
	}
	ar.Close()
	rc.Close()
	if !found {
		t.Fatal("final scene was not saved")
	}
	if waiting, err := applyPreparedDatabase(cfg.DataDir); err != nil || !waiting {
		t.Fatal(waiting, err)
	}
	restored, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := FinishBeforeStart(ctx, cfg, restored); err != nil {
		t.Fatal(err)
	}
	var marker string
	if err := restored.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'test.scene'").Scan(&marker); err != nil || marker != `"target"` {
		t.Fatal(marker, err)
	}
	if pending(cfg.RestoreDir()) {
		t.Fatal("ready stage was not removed")
	}
	if _, err := (files.Local{Root: cfg.FilesDir()}).Stat(ctx, "notes/extra"); !errors.Is(err, files.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestStartupRestoreRefusesMissingS3CredentialsAndRetries(t *testing.T) {
	ctx := context.Background()
	cfg, conn, st := startupTestConfig(t)
	stageStartupDB(t, cfg, conn)
	putStartupFile(t, filepath.Join(cfg.RestoreDir(), "files"), "notes/target", "target")
	putStartupFile(t, cfg.FilesDir(), "notes/extra", "scene")
	if err := st.Set(ctx, "storage.backend", "s3"); err != nil {
		t.Fatal(err)
	}
	if err := FinishBeforeStart(ctx, cfg, conn); err == nil {
		t.Fatal("unavailable storage was accepted")
	}
	if !pending(cfg.RestoreDir()) {
		t.Fatal("stage was discarded after storage failure")
	}
	if _, err := (files.Local{Root: cfg.FilesDir()}).Stat(ctx, "notes/extra"); err != nil {
		t.Fatal("local scene was deleted", err)
	}
	if err := st.Set(ctx, "storage.backend", "local"); err != nil {
		t.Fatal(err)
	}
	if err := FinishBeforeStart(ctx, cfg, conn); err != nil {
		t.Fatal(err)
	}
}

func TestStartupRestoreRejectsChangedStageBeforeWritingScene(t *testing.T) {
	ctx := context.Background()
	cfg, conn, _ := startupTestConfig(t)
	stageStartupDB(t, cfg, conn)
	stageFiles := filepath.Join(cfg.RestoreDir(), "files")
	putStartupFile(t, stageFiles, "notes/target", "target")
	putStartupFile(t, cfg.FilesDir(), "notes/extra", "scene")
	snapshot := repo.Snapshot{Files: []repo.File{{Path: "notes/target", Size: 6, SHA256: strings.Repeat("0", 64)}}}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.RestoreDir(), "snapshot.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := FinishBeforeStart(ctx, cfg, conn); !errors.Is(err, repo.ErrCorrupt) {
		t.Fatal(err)
	}
	if !pending(cfg.RestoreDir()) {
		t.Fatal("stage was discarded")
	}
	if _, err := (files.Local{Root: cfg.FilesDir()}).Stat(ctx, "notes/extra"); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseReplacementRequiresCompletedSceneBackup(t *testing.T) {
	cfg, conn, st := startupTestConfig(t)
	ctx := context.Background()
	if err := st.Set(ctx, "test.scene", "target"); err != nil {
		t.Fatal(err)
	}
	stageStartupDB(t, cfg, conn)
	if err := st.Set(ctx, "test.scene", "scene"); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if applied, err := applyPreparedDatabase(cfg.DataDir); err == nil || applied {
		t.Fatal("database applied without scene backup", applied, err)
	}
	if _, err := os.Stat(filepath.Join(cfg.RestoreDir(), dbName)); err != nil {
		t.Fatal("staged database removed", err)
	}
	if err := PrepareBeforeRestore(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if applied, err := applyPreparedDatabase(cfg.DataDir); err != nil || !applied {
		t.Fatal(applied, err)
	}
	if applied, err := applyPreparedDatabase(cfg.DataDir); err != nil || !applied {
		t.Fatal("retry failed", applied, err)
	}
}
