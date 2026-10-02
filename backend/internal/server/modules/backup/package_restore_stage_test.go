package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type stageEntry struct {
	name string
	body []byte
	kind byte
}

func stageTestArchive(t *testing.T, db []byte, entries ...stageEntry) []byte {
	t.Helper()
	sum := sha256.Sum256(db)
	man := manifest{Format: 1, DBSha256: hex.EncodeToString(sum[:]), Migration: "1", Files: int64(len(entries))}
	raw, err := json.Marshal(man)
	if err != nil {
		t.Fatal(err)
	}
	entries = append([]stageEntry{{name: manifestName, body: raw, kind: tar.TypeReg}, {name: dbName, body: db, kind: tar.TypeReg}}, entries...)
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		kind := entry.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		if err := tw.WriteHeader(&tar.Header{Name: entry.name, Typeflag: kind, Size: int64(len(entry.body)), Mode: 0o600}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(entry.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestPackageStageVerifiesFilesAndKeepsFailedDraft(t *testing.T) {
	ctx := context.Background()
	cfg, conn, _ := startupTestConfig(t)
	dbPath := filepath.Join(t.TempDir(), dbName)
	if _, _, err := snapshotDB(ctx, conn, dbPath); err != nil {
		t.Fatal(err)
	}
	db, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	good := stageTestArchive(t, db, stageEntry{name: "files/notes/a", body: []byte("target")})
	if _, err := stagePackage(ctx, bytes.NewReader(good), cfg.RestoreDir(), "99999999999999", restoreNote{}); err != nil {
		t.Fatal(err)
	}
	if !pending(cfg.RestoreDir()) {
		t.Fatal("verified package was not ready")
	}
	if _, err := stagePackage(ctx, bytes.NewReader(good), cfg.RestoreDir(), "99999999999999", restoreNote{}); err != errBusy {
		t.Fatal("pending stage overwritten", err)
	}
	if err := validateStagedSnapshot(ctx, cfg.RestoreDir()); err != nil {
		t.Fatal(err)
	}
	for _, entries := range [][]stageEntry{
		{{name: "files/notes/a", body: []byte("a")}, {name: "files/notes/A", body: []byte("b")}},
		{{name: "files/notes/a", body: []byte("a")}, {name: "files/notes/a/b", body: []byte("b")}},
		{{name: "files/notes/link", kind: tar.TypeSymlink}},
		{{name: "files/backups/old", body: []byte("a")}},
		{{name: "files/notes/../a", body: []byte("a")}},
	} {
		dir := filepath.Join(t.TempDir(), "stage")
		bad := stageTestArchive(t, db, entries...)
		if _, err := stagePackage(ctx, bytes.NewReader(bad), dir, "99999999999999", restoreNote{}); err == nil {
			t.Fatal("unsafe package accepted", entries)
		}
		if pending(dir) {
			t.Fatal("failed stage was ready")
		}
		if _, err := os.Stat(filepath.Join(dir, dbName)); err != nil {
			t.Fatal("failed draft was discarded", err)
		}
	}
}

func TestPackageStageRejectsInvalidDatabaseAndGzipChecksum(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "stage")
	if _, err := stagePackage(ctx, bytes.NewReader(stageTestArchive(t, []byte("not sqlite"))), dir, "99999999999999", restoreNote{}); err == nil {
		t.Fatal("invalid database accepted")
	}
	cfg, conn, _ := startupTestConfig(t)
	dbPath := filepath.Join(t.TempDir(), dbName)
	if _, _, err := snapshotDB(ctx, conn, dbPath); err != nil {
		t.Fatal(err)
	}
	db, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	raw := stageTestArchive(t, db)
	raw[len(raw)-8] ^= 1
	if _, err := stagePackage(ctx, bytes.NewReader(raw), cfg.RestoreDir(), "99999999999999", restoreNote{}); err == nil {
		t.Fatal("invalid gzip checksum accepted")
	}
	if pending(cfg.RestoreDir()) {
		t.Fatal("damaged archive ready")
	}
}
