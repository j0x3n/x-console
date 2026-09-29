package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"
)

// tempPrefix marks files that are still being written. List skips them.
const tempPrefix = ".xc-tmp-"

// Local keeps files in a directory, one file per key. Files are written to a
// temporary file next to their final place and renamed, so a crash never
// leaves a half written file under a real key.
type Local struct{ Root string }

var _ Store = Local{}

func (l Local) path(key string) (string, error) {
	if err := CheckKey(key); err != nil {
		return "", err
	}
	return filepath.Join(l.Root, filepath.FromSlash(key)), nil
}

func (l Local) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	dest, err := l.path(key)
	if err != nil {
		return err
	}
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, tempPrefix+"*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	n, err := io.Copy(tmp, &ctxReader{ctx: ctx, r: r})
	if err == nil && size >= 0 && n != size {
		err = fmt.Errorf("files: %s: expected %d bytes, got %d", key, size, n)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, dest)
	}
	if err != nil {
		_ = os.Remove(name)
	}
	return err
}

func (l Local) Get(ctx context.Context, key string) (io.ReadCloser, Info, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, Info{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, Info{}, notFound(err)
	}
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		f.Close()
		return nil, Info{}, ErrNotFound
	}
	return f, Info{Key: key, Size: st.Size(), ModTime: st.ModTime()}, nil
}

func (l Local) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, Info, error) {
	rc, info, err := l.Get(ctx, key)
	if err != nil {
		return nil, Info{}, err
	}
	f := rc.(*os.File)
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		f.Close()
		return nil, Info{}, err
	}
	if length < 0 {
		return f, info, nil
	}
	return &limited{Reader: io.LimitReader(f, length), Closer: f}, info, nil
}

type limited struct {
	io.Reader
	io.Closer
}

func (l Local) Stat(ctx context.Context, key string) (Info, error) {
	p, err := l.path(key)
	if err != nil {
		return Info{}, err
	}
	st, err := os.Stat(p)
	if err != nil {
		return Info{}, notFound(err)
	}
	if st.IsDir() {
		return Info{}, ErrNotFound
	}
	return Info{Key: key, Size: st.Size(), ModTime: st.ModTime()}, nil
}

// Delete removes the file and any parent directories left empty.
func (l Local) Delete(ctx context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	root := filepath.Clean(l.Root)
	for dir := filepath.Dir(p); dir != root && strings.HasPrefix(dir, root); dir = filepath.Dir(dir) {
		if os.Remove(dir) != nil { // not empty, or already gone
			break
		}
	}
	return nil
}

func (l Local) List(ctx context.Context, prefix string) iter.Seq2[Info, error] {
	prefix, err := cleanPrefix(prefix)
	if err != nil {
		return failed(err)
	}
	start := filepath.Clean(l.Root)
	if prefix != "" {
		start = filepath.Join(start, filepath.FromSlash(prefix))
	}
	return func(yield func(Info, error) bool) {
		err := filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) && p == start {
					return filepath.SkipAll // nothing stored there yet
				}
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.IsDir() || strings.HasPrefix(d.Name(), tempPrefix) {
				return nil
			}
			st, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(l.Root, p)
			if err != nil {
				return err
			}
			if !yield(Info{Key: filepath.ToSlash(rel), Size: st.Size(), ModTime: st.ModTime()}, nil) {
				return filepath.SkipAll
			}
			return nil
		})
		if err != nil {
			yield(Info{}, err)
		}
	}
}

func (l Local) Copy(ctx context.Context, from, to string) error {
	rc, info, err := l.Get(ctx, from)
	if err != nil {
		return err
	}
	defer rc.Close()
	return l.Put(ctx, to, rc, info.Size)
}

func notFound(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	}
	return err
}

// ctxReader stops a long copy when the context ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
