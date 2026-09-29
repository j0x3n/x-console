package files

import (
	"context"
	"io"
	"iter"
	"os"
	"sort"
	"sync"
	"time"
)

// Cached puts a directory in front of a slow Store such as S3. Reads are
// answered from the directory when the file is there, and otherwise
// downloaded into it first. Writes go to the remote Store and to the
// directory. When the directory grows over the limit, the files read longest
// ago are removed.
//
// Everything is written through Cached, so the directory never holds an
// older copy of a file than the remote Store.
type Cached struct {
	Remote Store

	local Local

	mu    sync.Mutex
	limit int64
	used  int64 // bytes in the directory, kept up to date on every change
}

var _ Store = (*Cached)(nil)

// NewCached uses dir as the cache directory with limit bytes of room.
// limit < 0 means the cache may grow without bound, 0 keeps nothing.
func NewCached(remote Store, dir string, limit int64) (*Cached, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	c := &Cached{Remote: remote, local: Local{Root: dir}, limit: limit}
	for info, err := range c.local.List(context.Background(), "") {
		if err != nil {
			return nil, err
		}
		c.used += info.Size
	}
	return c, nil
}

// SetLimit changes the size limit and trims the directory to it now.
func (c *Cached) SetLimit(ctx context.Context, limit int64) error {
	c.mu.Lock()
	c.limit = limit
	c.mu.Unlock()
	return c.Trim(ctx)
}

// Usage reports how many files and bytes the directory holds.
func (c *Cached) Usage(ctx context.Context) (count int, bytes int64, err error) {
	for info, err := range c.local.List(ctx, "") {
		if err != nil {
			return 0, 0, err
		}
		count++
		bytes += info.Size
	}
	return count, bytes, nil
}

// Trim removes the least recently read files until the directory fits.
func (c *Cached) Trim(ctx context.Context) error {
	c.mu.Lock()
	limit, used := c.limit, c.used
	c.mu.Unlock()
	if limit < 0 || used <= limit {
		return nil
	}
	var all []Info
	var total int64
	for info, err := range c.local.List(ctx, "") {
		if err != nil {
			return err
		}
		all = append(all, info)
		total += info.Size
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ModTime.Before(all[j].ModTime) })
	for _, info := range all {
		if total <= limit {
			break
		}
		if err := c.local.Delete(ctx, info.Key); err != nil {
			return err
		}
		total -= info.Size
	}
	c.mu.Lock()
	c.used = total
	c.mu.Unlock()
	return nil
}

func (c *Cached) add(n int64) {
	c.mu.Lock()
	c.used += n
	over := c.limit >= 0 && c.used > c.limit
	c.mu.Unlock()
	if over {
		_ = c.Trim(context.Background())
	}
}

// touch marks a cached file as just read.
func (c *Cached) touch(key string) {
	if p, err := c.local.path(key); err == nil {
		now := time.Now()
		_ = os.Chtimes(p, now, now)
	}
}

func (c *Cached) drop(ctx context.Context, key string) {
	if info, err := c.local.Stat(ctx, key); err == nil {
		if c.local.Delete(ctx, key) == nil {
			c.add(-info.Size)
		}
	}
}

// Put writes to the remote Store, keeping a copy in the directory.
func (c *Cached) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if err := CheckKey(key); err != nil {
		return err
	}
	// Feed the same bytes to the directory while they go to the remote Store.
	// A pipe would tie the two speeds together, so the copy goes to a
	// temporary file first and is moved into place after the upload worked.
	tmp, err := os.CreateTemp(c.local.Root, tempPrefix+"*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := c.Remote.Put(ctx, key, io.TeeReader(r, tmp), size); err != nil {
		tmp.Close()
		return err
	}
	written, err := tmp.Seek(0, io.SeekEnd)
	if err == nil {
		_, err = tmp.Seek(0, io.SeekStart)
	}
	if err != nil {
		tmp.Close()
		c.drop(ctx, key)
		return nil // the remote copy is safe; only the cache missed it
	}
	old, _ := c.local.Stat(ctx, key)
	if err := c.local.Put(ctx, key, tmp, written); err != nil {
		c.drop(ctx, key)
	} else {
		c.add(written - old.Size)
	}
	tmp.Close()
	return nil
}

// fill downloads key into the directory. It returns the cached copy's Info.
func (c *Cached) fill(ctx context.Context, key string) error {
	rc, info, err := c.Remote.Get(ctx, key)
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := c.local.Put(ctx, key, rc, info.Size); err != nil {
		return err
	}
	c.add(info.Size)
	return nil
}

func (c *Cached) Get(ctx context.Context, key string) (io.ReadCloser, Info, error) {
	if rc, info, err := c.local.Get(ctx, key); err == nil {
		c.touch(key)
		return rc, info, nil
	}
	if err := c.fill(ctx, key); err != nil {
		return nil, Info{}, err
	}
	rc, info, err := c.local.Get(ctx, key)
	if err != nil { // trimmed right away because it is bigger than the limit
		return c.Remote.Get(ctx, key)
	}
	return rc, info, nil
}

// GetRange answers from the directory when the file is there. Otherwise it
// reads just the range from the remote Store and does not cache anything,
// because a range says nothing about the rest of the file.
func (c *Cached) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, Info, error) {
	if rc, info, err := c.local.GetRange(ctx, key, offset, length); err == nil {
		c.touch(key)
		return rc, info, nil
	}
	return c.Remote.GetRange(ctx, key, offset, length)
}

func (c *Cached) Stat(ctx context.Context, key string) (Info, error) {
	if info, err := c.local.Stat(ctx, key); err == nil {
		return info, nil
	}
	return c.Remote.Stat(ctx, key)
}

func (c *Cached) Delete(ctx context.Context, key string) error {
	if err := c.Remote.Delete(ctx, key); err != nil {
		return err
	}
	c.drop(ctx, key)
	return nil
}

func (c *Cached) List(ctx context.Context, prefix string) iter.Seq2[Info, error] {
	return c.Remote.List(ctx, prefix)
}

func (c *Cached) Copy(ctx context.Context, from, to string) error {
	if err := c.Remote.Copy(ctx, from, to); err != nil {
		return err
	}
	c.drop(ctx, to)
	return nil
}
