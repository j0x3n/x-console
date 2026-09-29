// Package files is the one place where the site keeps uploaded files.
//
// Every file has a key such as "drive/blobs/ab/abcdef" or
// "notes/attachments/12". The first segment is the module. A Store keeps the
// bytes of those keys in a directory (Local) or in an S3 bucket (S3). Modules
// never build paths themselves: they ask the Manager for their own scoped
// Store with Manager.For("notes"), so switching the storage location, or
// backing up everything, works for all of them at once.
package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
	"time"
)

// Info describes one stored file.
type Info struct {
	Key     string
	Size    int64
	ModTime time.Time
}

// ErrNotFound is returned when a key has no file.
var ErrNotFound = errors.New("files: not found")

// ErrBadKey is returned for keys that are not allowed.
var ErrBadKey = errors.New("files: bad key")

// Store keeps files by key. Keys use "/" between segments.
type Store interface {
	// Put stores r under key, replacing any file there. size is the number of
	// bytes r will deliver, or -1 when unknown. Readers never see a half
	// written file.
	Put(ctx context.Context, key string, r io.Reader, size int64) error
	// Get opens the file. Local files come back as an *os.File, so callers
	// that need seeking can ask for io.ReadSeeker (see OpenSeeker).
	Get(ctx context.Context, key string) (io.ReadCloser, Info, error)
	// GetRange reads [offset, offset+length). length < 0 reads to the end.
	GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, Info, error)
	Stat(ctx context.Context, key string) (Info, error)
	// Delete removes the file. A missing file is not an error.
	Delete(ctx context.Context, key string) error
	// List yields every file under the directory prefix (all files when
	// prefix is empty), segment by segment in sorted order.
	List(ctx context.Context, prefix string) iter.Seq2[Info, error]
	Copy(ctx context.Context, from, to string) error
}

// maxKeyLen is the longest key accepted.
const maxKeyLen = 900

// CheckKey validates a key: non-empty segments separated by "/", no "." or
// ".." segments, no leading slash, no backslash or control characters.
func CheckKey(key string) error {
	if key == "" || len(key) > maxKeyLen {
		return fmt.Errorf("%w: %q", ErrBadKey, key)
	}
	for _, seg := range strings.Split(key, "/") {
		if err := checkSegment(seg); err != nil {
			return fmt.Errorf("%w: %q", ErrBadKey, key)
		}
	}
	return nil
}

func checkSegment(seg string) error {
	if seg == "" || seg == "." || seg == ".." {
		return ErrBadKey
	}
	for _, r := range seg {
		if r == '\\' || r < 0x20 || r == 0x7f {
			return ErrBadKey
		}
	}
	return nil
}

// cleanPrefix validates a directory prefix and drops a trailing slash. The
// empty prefix stays empty.
func cleanPrefix(prefix string) (string, error) {
	prefix = strings.TrimSuffix(prefix, "/")
	if prefix == "" {
		return "", nil
	}
	if err := CheckKey(prefix); err != nil {
		return "", err
	}
	return prefix, nil
}

// Module returns the first segment of a key.
func Module(key string) string {
	mod, _, _ := strings.Cut(key, "/")
	return mod
}

// Scoped returns a Store that only reads and writes keys under name/. Keys
// passed to it and Info.Key values coming out of it are relative to that
// directory.
func Scoped(s Store, name string) Store {
	if checkSegment(name) != nil || strings.Contains(name, "/") {
		panic("files: bad scope name " + name)
	}
	return &scoped{s: s, prefix: name + "/"}
}

type scoped struct {
	s      Store
	prefix string
}

func (c *scoped) key(key string) (string, error) {
	if err := CheckKey(key); err != nil {
		return "", err
	}
	return c.prefix + key, nil
}

func (c *scoped) info(i Info) Info {
	i.Key = strings.TrimPrefix(i.Key, c.prefix)
	return i
}

func (c *scoped) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	full, err := c.key(key)
	if err != nil {
		return err
	}
	return c.s.Put(ctx, full, r, size)
}

func (c *scoped) Get(ctx context.Context, key string) (io.ReadCloser, Info, error) {
	full, err := c.key(key)
	if err != nil {
		return nil, Info{}, err
	}
	rc, info, err := c.s.Get(ctx, full)
	return rc, c.info(info), err
}

func (c *scoped) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, Info, error) {
	full, err := c.key(key)
	if err != nil {
		return nil, Info{}, err
	}
	rc, info, err := c.s.GetRange(ctx, full, offset, length)
	return rc, c.info(info), err
}

func (c *scoped) Stat(ctx context.Context, key string) (Info, error) {
	full, err := c.key(key)
	if err != nil {
		return Info{}, err
	}
	info, err := c.s.Stat(ctx, full)
	return c.info(info), err
}

func (c *scoped) Delete(ctx context.Context, key string) error {
	full, err := c.key(key)
	if err != nil {
		return err
	}
	return c.s.Delete(ctx, full)
}

func (c *scoped) List(ctx context.Context, prefix string) iter.Seq2[Info, error] {
	prefix, err := cleanPrefix(prefix)
	if err != nil {
		return failed(err)
	}
	full := strings.TrimSuffix(c.prefix, "/")
	if prefix != "" {
		full += "/" + prefix
	}
	return func(yield func(Info, error) bool) {
		for info, err := range c.s.List(ctx, full) {
			if !yield(c.info(info), err) {
				return
			}
		}
	}
}

func (c *scoped) Copy(ctx context.Context, from, to string) error {
	f, err := c.key(from)
	if err != nil {
		return err
	}
	t, err := c.key(to)
	if err != nil {
		return err
	}
	return c.s.Copy(ctx, f, t)
}

func failed(err error) iter.Seq2[Info, error] {
	return func(yield func(Info, error) bool) { yield(Info{}, err) }
}
