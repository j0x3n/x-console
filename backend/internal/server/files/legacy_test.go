package files_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateLegacyLayout(t *testing.T) {
	data := t.TempDir()
	root := filepath.Join(data, "files")
	write(t, filepath.Join(data, "drive/blobs/ab/abcdef"), "blob")
	write(t, filepath.Join(data, "drive/thumbnails/abcdef.jpg"), "thumb")
	write(t, filepath.Join(data, "drive/save-123"), "leftover")
	write(t, filepath.Join(data, "notes/attachments/7"), "attachment")
	write(t, filepath.Join(data, "notes/attachments/.upload-999"), "leftover")
	write(t, filepath.Join(data, "x-console.db"), "db")

	if err := files.MigrateLegacyLayout(slog.Default(), data, root); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"drive/blobs/ab/abcdef":       "blob",
		"drive/thumbnails/abcdef.jpg": "thumb",
		"notes/attachments/7":         "attachment",
	} {
		got, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q %v", path, got, err)
		}
	}
	for _, gone := range []string{"drive", "notes"} {
		if _, err := os.Stat(filepath.Join(data, gone)); !os.IsNotExist(err) {
			t.Errorf("old directory %s still there: %v", gone, err)
		}
	}
	if _, err := os.Stat(filepath.Join(data, "x-console.db")); err != nil {
		t.Fatal("database must stay")
	}

	// Running again changes nothing.
	if err := files.MigrateLegacyLayout(slog.Default(), data, root); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateLegacyLayoutMergesIntoExistingFiles(t *testing.T) {
	data := t.TempDir()
	root := filepath.Join(data, "files")
	// An interrupted earlier run: one blob already moved, one still old.
	write(t, filepath.Join(root, "drive/blobs/ab/abcdef"), "new copy")
	write(t, filepath.Join(data, "drive/blobs/ab/abcdef"), "old copy")
	write(t, filepath.Join(data, "drive/blobs/cd/cdef01"), "second")
	write(t, filepath.Join(data, "notes/keep.txt"), "not ours")

	if err := files.MigrateLegacyLayout(slog.Default(), data, root); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "drive/blobs/ab/abcdef")); string(got) != "new copy" {
		t.Fatalf("existing file was overwritten: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "drive/blobs/cd/cdef01")); string(got) != "second" {
		t.Fatalf("second blob: %q", got)
	}
	if _, err := os.Stat(filepath.Join(data, "notes/keep.txt")); err != nil {
		t.Fatal("unknown files in old directories must stay")
	}
	if _, err := os.Stat(filepath.Join(data, "drive")); !os.IsNotExist(err) {
		t.Fatalf("old drive directory left: %v", err)
	}
}
