package backup

import (
	"context"
	"errors"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

type failRestoreStore struct {
	files.Store
	failPut    string
	failDelete string
}

func (s failRestoreStore) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if key == s.failPut {
		return errors.New("upload failed")
	}
	return s.Store.Put(ctx, key, r, size)
}
func (s failRestoreStore) Delete(ctx context.Context, key string) error {
	if key == s.failDelete {
		return errors.New("delete failed")
	}
	return s.Store.Delete(ctx, key)
}

type failListingRestoreStore struct{ files.Store }

func (s failListingRestoreStore) List(ctx context.Context, prefix string) iter.Seq2[files.Info, error] {
	return func(yield func(files.Info, error) bool) {
		for info, err := range s.Store.List(ctx, prefix) {
			if !yield(info, err) {
				return
			}
		}
		yield(files.Info{}, errors.New("listing failed after first page"))
	}
}

func TestRestoreDoesNotWriteOrDeleteAfterIncompleteDestinationList(t *testing.T) {
	ctx := context.Background()
	src := files.Local{Root: t.TempDir()}
	dst := files.Local{Root: t.TempDir()}
	putStartupFile(t, src.Root, "notes/target", "target")
	putStartupFile(t, dst.Root, "notes/extra", "scene")
	if err := applyStagedFiles(ctx, failListingRestoreStore{dst}, src.Root, nil); err == nil {
		t.Fatal("incomplete destination list accepted")
	}
	if _, err := dst.Stat(ctx, "notes/extra"); err != nil {
		t.Fatal("scene deleted", err)
	}
	if _, err := dst.Stat(ctx, "notes/target"); !errors.Is(err, files.ErrNotFound) {
		t.Fatal("target written after incomplete listing", err)
	}
}

func TestRestoreWritesBeforeDeletingExtraFiles(t *testing.T) {
	ctx := context.Background()
	src := files.Local{Root: t.TempDir()}
	dst := files.Local{Root: t.TempDir()}
	for _, item := range []struct {
		s         files.Store
		key, body string
	}{{src, "notes/new", "target"}, {dst, "notes/extra", "scene"}, {dst, "backups/archive", "safe"}} {
		if err := item.s.Put(ctx, item.key, strings.NewReader(item.body), int64(len(item.body))); err != nil {
			t.Fatal(err)
		}
	}
	if err := applyStagedFiles(ctx, failRestoreStore{Store: dst, failPut: "notes/new"}, src.Root, func(int64, int64) {}); err == nil {
		t.Fatal("put succeeded")
	}
	if _, err := dst.Stat(ctx, "notes/extra"); err != nil {
		t.Fatal("deleted scene before successful writes", err)
	}
	if err := applyStagedFiles(ctx, failRestoreStore{Store: dst, failDelete: "notes/extra"}, src.Root, func(int64, int64) {}); err == nil {
		t.Fatal("delete succeeded")
	}
	if _, err := dst.Stat(ctx, "notes/new"); err != nil {
		t.Fatal("did not write target first", err)
	}
	if err := applyStagedFiles(ctx, dst, src.Root, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Stat(ctx, "notes/extra"); !errors.Is(err, files.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := dst.Stat(ctx, "backups/archive"); err != nil {
		t.Fatal("backup removed", err)
	}
}

func TestFailedFileRestoreKeepsStageAndCanRetry(t *testing.T) {
	ctx := context.Background()
	cfg, conn, _ := startupTestConfig(t)
	stageStartupDB(t, cfg, conn)
	putStartupFile(t, filepath.Join(cfg.RestoreDir(), "files"), "notes/target", "target")
	putStartupFile(t, cfg.FilesDir(), "notes/extra", "scene")
	m, err := startupRestoreModule(ctx, cfg, conn)
	if err != nil {
		t.Fatal(err)
	}
	current := files.Local{Root: cfg.FilesDir()}
	m.d.Files.Swap(failRestoreStore{Store: current, failPut: "notes/target"})
	if err := m.finishStagedRestore(ctx, &job{v: api.BackupJob{}}); err == nil {
		t.Fatal("restore succeeded")
	}
	if !pending(cfg.RestoreDir()) {
		t.Fatal("ready stage discarded after put failure")
	}
	if _, err := current.Stat(ctx, "notes/extra"); err != nil {
		t.Fatal("scene deleted before all target files written", err)
	}
	m.d.Files.Swap(failRestoreStore{Store: current, failDelete: "notes/extra"})
	if err := m.finishStagedRestore(ctx, &job{}); err == nil {
		t.Fatal("delete succeeded")
	}
	if !pending(cfg.RestoreDir()) {
		t.Fatal("ready stage discarded after delete failure")
	}
	m.d.Files.Swap(current)
	if err := m.finishStagedRestore(ctx, &job{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg.RestoreDir()); !os.IsNotExist(err) {
		t.Fatal("stage remains after successful retry", err)
	}
}
