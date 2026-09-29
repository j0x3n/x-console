package files

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// legacyMoves are the directories older versions used, and where their files
// live now. Paths are relative to the data directory and to the files root.
var legacyMoves = []struct{ from, to string }{
	{"drive/blobs", "drive/blobs"},
	{"drive/thumbnails", "drive/thumbnails"},
	{"notes/attachments", "notes/attachments"},
}

// MigrateLegacyLayout moves files from the old directories under dataDir
// (data/drive, data/notes) to the new root (data/files). It is safe to run on
// every start: nothing happens when the old directories are gone.
func MigrateLegacyLayout(log *slog.Logger, dataDir, root string) error {
	for _, m := range legacyMoves {
		src := filepath.Join(dataDir, filepath.FromSlash(m.from))
		dst := filepath.Join(root, filepath.FromSlash(m.to))
		if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		n, err := moveTree(src, dst)
		if err != nil {
			return fmt.Errorf("%s: %w", m.from, err)
		}
		log.Info("moved files to the new layout", "from", src, "to", dst, "files", n)
	}
	// Old temporary files and empty directories are of no use any more.
	for _, dir := range []string{"drive", "notes"} {
		removeIfEmptyTree(filepath.Join(dataDir, dir))
	}
	return nil
}

// moveTree moves every file under src to the same relative place under dst
// and removes src. Files already at the destination are kept and the source
// copy is dropped, so an interrupted run can be repeated.
func moveTree(src, dst string) (int, error) {
	if _, err := os.Stat(dst); errors.Is(err, fs.ErrNotExist) {
		// The common case: one rename, instant on the same volume.
		if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
			return 0, err
		}
		if err := os.Rename(src, dst); err == nil {
			return countFiles(dst), nil
		}
	}
	moved := 0
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if _, err := os.Stat(target); err == nil {
			return os.Remove(p) // already there
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := os.Rename(p, target); err != nil {
			if err := copyFile(p, target); err != nil {
				return err
			}
			if err := os.Remove(p); err != nil {
				return err
			}
		}
		moved++
		return nil
	})
	if err != nil {
		return moved, err
	}
	return moved, os.RemoveAll(src)
}

func countFiles(dir string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// copyFile copies across volumes, where rename does not work.
func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), tempPrefix+"*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	if _, err = io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// legacyTempPrefixes are the temporary file names old versions left behind.
var legacyTempPrefixes = []string{"save-", "upload-", "thumbnail-", ".upload-"}

func legacyTemp(name string) bool {
	for _, p := range legacyTempPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// removeIfEmptyTree removes dir when it holds only empty directories and
// leftover temporary files. Anything else stays where it is.
func removeIfEmptyTree(dir string) {
	var junk []string
	keep := false
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if legacyTemp(d.Name()) {
			junk = append(junk, p)
		} else {
			keep = true
		}
		return nil
	})
	if keep {
		return
	}
	for _, p := range junk {
		os.Remove(p)
	}
	_ = os.RemoveAll(dir)
}
