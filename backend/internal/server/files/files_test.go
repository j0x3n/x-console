package files_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

func read(t *testing.T, rc io.ReadCloser, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func put(t *testing.T, s files.Store, key, data string) {
	t.Helper()
	if err := s.Put(context.Background(), key, strings.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("put %s: %v", key, err)
	}
}

func keys(t *testing.T, s files.Store, prefix string) []string {
	t.Helper()
	var out []string
	for info, err := range s.List(context.Background(), prefix) {
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, info.Key)
	}
	return out
}

// runStoreTests checks the behaviour every Store must have. The S3 and cache
// implementations run the same tests.
func runStoreTests(t *testing.T, s files.Store) {
	ctx := context.Background()

	put(t, s, "drive/blobs/ab/abcdef", "hello world")
	put(t, s, "drive/thumbnails/x.jpg", "jpg")
	put(t, s, "notes/attachments/12", "note")

	rc, info, err := s.Get(ctx, "drive/blobs/ab/abcdef")
	if got := read(t, rc, err); got != "hello world" || info.Size != 11 || info.Key != "drive/blobs/ab/abcdef" || info.ModTime.IsZero() {
		t.Fatalf("get: %q %+v", got, info)
	}
	rc, _, err = s.GetRange(ctx, "drive/blobs/ab/abcdef", 6, 5)
	if got := read(t, rc, err); got != "world" {
		t.Fatalf("range: %q", got)
	}
	rc, _, err = s.GetRange(ctx, "drive/blobs/ab/abcdef", 6, -1)
	if got := read(t, rc, err); got != "world" {
		t.Fatalf("range to end: %q", got)
	}
	if info, err := s.Stat(ctx, "notes/attachments/12"); err != nil || info.Size != 4 {
		t.Fatalf("stat: %+v %v", info, err)
	}

	// Replacing keeps one file.
	put(t, s, "notes/attachments/12", "replaced!")
	rc, _, err = s.Get(ctx, "notes/attachments/12")
	if got := read(t, rc, err); got != "replaced!" {
		t.Fatalf("replace: %q", got)
	}

	if _, _, err := s.Get(ctx, "nope/missing"); !errors.Is(err, files.ErrNotFound) {
		t.Fatalf("missing get: %v", err)
	}
	if _, err := s.Stat(ctx, "nope/missing"); !errors.Is(err, files.ErrNotFound) {
		t.Fatalf("missing stat: %v", err)
	}

	if err := s.Copy(ctx, "drive/blobs/ab/abcdef", "drive/copy/one"); err != nil {
		t.Fatal(err)
	}
	rc, _, err = s.Get(ctx, "drive/copy/one")
	if got := read(t, rc, err); got != "hello world" {
		t.Fatalf("copy: %q", got)
	}
	if err := s.Copy(ctx, "nope/missing", "drive/copy/two"); !errors.Is(err, files.ErrNotFound) {
		t.Fatalf("copy missing: %v", err)
	}

	if got := keys(t, s, "drive"); !slices.Equal(got, []string{"drive/blobs/ab/abcdef", "drive/copy/one", "drive/thumbnails/x.jpg"}) {
		t.Fatalf("list drive: %v", got)
	}
	if got := keys(t, s, "drive/blobs/"); !slices.Equal(got, []string{"drive/blobs/ab/abcdef"}) {
		t.Fatalf("list with trailing slash: %v", got)
	}
	if got := keys(t, s, ""); len(got) != 4 {
		t.Fatalf("list all: %v", got)
	}
	if got := keys(t, s, "empty"); len(got) != 0 {
		t.Fatalf("list empty prefix: %v", got)
	}
	// A prefix is a directory, not a string prefix.
	if got := keys(t, s, "dri"); len(got) != 0 {
		t.Fatalf("list partial segment: %v", got)
	}

	if err := s.Delete(ctx, "drive/copy/one"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "drive/copy/one"); err != nil {
		t.Fatalf("delete twice: %v", err)
	}
	if _, err := s.Stat(ctx, "drive/copy/one"); !errors.Is(err, files.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}

	for _, key := range []string{"", "/abs", "a//b", "a/../b", "../x", "a/./b", "a/", `a\b`, "a/\x00b"} {
		if err := s.Put(ctx, key, strings.NewReader("x"), 1); !errors.Is(err, files.ErrBadKey) {
			t.Errorf("put %q: %v", key, err)
		}
		if _, _, err := s.Get(ctx, key); !errors.Is(err, files.ErrBadKey) {
			t.Errorf("get %q: %v", key, err)
		}
	}

	// A write that fails part way leaves nothing behind.
	err = s.Put(ctx, "drive/broken", io.MultiReader(strings.NewReader("abc"), errReader{}), -1)
	if err == nil {
		t.Fatal("put with failing reader should fail")
	}
	if _, err := s.Stat(ctx, "drive/broken"); !errors.Is(err, files.ErrNotFound) {
		t.Fatalf("broken put left a file: %v", err)
	}
	// A wrong size is an error too.
	if err := s.Put(ctx, "drive/short", strings.NewReader("abc"), 10); err == nil {
		t.Fatal("size mismatch should fail")
	}
	if _, err := s.Stat(ctx, "drive/short"); !errors.Is(err, files.ErrNotFound) {
		t.Fatalf("size mismatch left a file: %v", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestLocal(t *testing.T) {
	root := t.TempDir()
	runStoreTests(t, files.Local{Root: root})

	// No temporary files are left and the mode is private.
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(d.Name(), ".xc-tmp-") {
			t.Errorf("temp file left: %s", p)
		}
		if !d.IsDir() {
			if st, _ := d.Info(); st.Mode().Perm() != 0600 {
				t.Errorf("%s mode %v", p, st.Mode())
			}
		}
		return nil
	})
	// Deleting the last file of a directory removes the directory.
	l := files.Local{Root: t.TempDir()}
	put(t, l, "a/b/c/file", "x")
	if err := l.Delete(context.Background(), "a/b/c/file"); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(l.Root); len(entries) != 0 {
		t.Fatalf("empty directories left: %v", entries)
	}
}

func TestScopedStaysInItsModule(t *testing.T) {
	root := t.TempDir()
	base := files.Local{Root: root}
	notes := files.Scoped(base, "notes")
	drive := files.Scoped(base, "drive")
	put(t, notes, "a/1", "note")
	put(t, drive, "a/1", "drive")
	rc, _, err := notes.Get(context.Background(), "a/1")
	if got := read(t, rc, err); got != "note" {
		t.Fatalf("notes read: %q", got)
	}
	if got := keys(t, notes, ""); !slices.Equal(got, []string{"a/1"}) {
		t.Fatalf("notes list: %v", got)
	}
	if got := keys(t, base, ""); !slices.Equal(got, []string{"drive/a/1", "notes/a/1"}) {
		t.Fatalf("base list: %v", got)
	}
	if err := notes.Put(context.Background(), "../drive/a/1", strings.NewReader("x"), 1); !errors.Is(err, files.ErrBadKey) {
		t.Fatalf("escape: %v", err)
	}
	if _, err := notes.Stat(context.Background(), "drive/a/1"); !errors.Is(err, files.ErrNotFound) {
		t.Fatalf("other module visible: %v", err)
	}
	runStoreTests(t, files.Scoped(files.Local{Root: t.TempDir()}, "test"))
}

func TestManagerSwapAndMirror(t *testing.T) {
	ctx := context.Background()
	a, b := files.Local{Root: t.TempDir()}, files.Local{Root: t.TempDir()}
	m := files.NewManager(a)
	notes := m.For("notes") // kept across the swap
	put(t, notes, "1", "one")

	m.SetMirror(b)
	put(t, notes, "2", "two")
	if err := notes.Copy(ctx, "1", "3"); err != nil {
		t.Fatal(err)
	}
	if err := notes.Delete(ctx, "1"); err != nil {
		t.Fatal(err)
	}
	// Writes and deletes reached both stores; the old file 1 was never in b.
	if got := keys(t, b, ""); !slices.Equal(got, []string{"notes/2", "notes/3"}) {
		t.Fatalf("mirror: %v", got)
	}
	if got := keys(t, a, ""); !slices.Equal(got, []string{"notes/2", "notes/3"}) {
		t.Fatalf("current: %v", got)
	}

	old := m.Swap(b)
	if old != files.Store(a) {
		t.Fatal("swap should return the previous store")
	}
	put(t, notes, "4", "four")
	if got := keys(t, a, ""); slices.Contains(got, "notes/4") {
		t.Fatalf("write after swap went to the old store: %v", got)
	}
	rc, _, err := notes.Get(ctx, "4")
	if got := read(t, rc, err); got != "four" {
		t.Fatalf("read after swap: %q", got)
	}
	// Swapping clears the mirror.
	put(t, notes, "5", "five")
	if got := keys(t, a, ""); slices.Contains(got, "notes/5") {
		t.Fatalf("mirror not cleared: %v", got)
	}
}

func TestOpenSeekerServesRanges(t *testing.T) {
	ctx := context.Background()
	l := files.Local{Root: t.TempDir()}
	put(t, l, "a/f", "0123456789")
	// Local files are real files.
	sr, info, err := files.OpenSeeker(ctx, l, "a/f")
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Close()
	if _, ok := sr.(*os.File); !ok || info.Size != 10 {
		t.Fatalf("local seeker: %T %+v", sr, info)
	}

	// A store without seekable readers is read by ranges.
	ranged := &rangeOnly{Store: l}
	sr, _, err = files.OpenSeeker(ctx, ranged, "a/f")
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Close()
	if _, err := sr.Seek(4, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 3)
	if _, err := io.ReadFull(sr, buf); err != nil || string(buf) != "456" {
		t.Fatalf("read after seek: %q %v", buf, err)
	}
	if end, _ := sr.Seek(-2, io.SeekEnd); end != 8 {
		t.Fatalf("seek end: %d", end)
	}
	rest, _ := io.ReadAll(sr)
	if !bytes.Equal(rest, []byte("89")) {
		t.Fatalf("read to end: %q", rest)
	}
	if ranged.rangeCalls < 2 {
		t.Fatalf("expected ranged reads, got %d", ranged.rangeCalls)
	}
}

// rangeOnly hides the *os.File behind a plain reader, like a network store.
type rangeOnly struct {
	files.Store
	rangeCalls int
}

type plain struct{ io.ReadCloser }

func (r *rangeOnly) Get(ctx context.Context, key string) (io.ReadCloser, files.Info, error) {
	rc, info, err := r.Store.Get(ctx, key)
	if err != nil {
		return nil, info, err
	}
	return plain{rc}, info, nil
}

func (r *rangeOnly) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, files.Info, error) {
	r.rangeCalls++
	rc, info, err := r.Store.GetRange(ctx, key, offset, length)
	if err != nil {
		return nil, info, err
	}
	return plain{rc}, info, nil
}
