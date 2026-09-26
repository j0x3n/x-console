// Package files browses and transfers files: files.list, files.read and
// files.write (chunked streams of protocol.FileChunkSize), files.remove,
// files.mkdir and files.rename.
package files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// Register adds the file methods.
func Register(c *conn.Client) {
	c.Handle(protocol.MethodFilesList, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.FilesListParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return List(p.Path)
	})
	c.Handle(protocol.MethodFilesRemove, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.FilesRemoveParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return nil, Remove(p.Path, p.Recursive)
	})
	c.Handle(protocol.MethodFilesMkdir, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.FilesPathParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return Mkdir(p.Path)
	})
	c.Handle(protocol.MethodFilesRename, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.FilesRenameParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return Rename(p.From, p.To)
	})
	c.HandleStream(protocol.MethodFilesRead, ServeRead)
	c.HandleStream(protocol.MethodFilesWrite, ServeWrite)
}

// Resolve turns a request path into a clean absolute path. "~" means home.
func Resolve(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", rpcutil.Failed("no home directory: %v", err)
		}
		path = filepath.Join(home, path[1:])
	}
	if path == "" || !filepath.IsAbs(path) {
		return "", rpcutil.BadParams("path must be absolute")
	}
	return filepath.Clean(path), nil
}

// List reads a directory. Directories come first, then files, by name.
func List(path string) (protocol.FileList, error) {
	if path == "" {
		path = "~"
	}
	if isDrivesRoot(path) {
		return drives()
	}
	dir, err := Resolve(path)
	if err != nil {
		return protocol.FileList{}, err
	}
	dirents, err := os.ReadDir(dir)
	if err != nil {
		return protocol.FileList{}, rpcutil.FromOS(err)
	}
	out := protocol.FileList{Path: dir, Parent: parentOf(dir), Sep: string(filepath.Separator), Entries: make([]protocol.FileEntry, 0, len(dirents))}
	for _, d := range dirents {
		info, err := d.Info()
		if err != nil {
			continue
		}
		out.Entries = append(out.Entries, entry(filepath.Join(dir, d.Name()), info))
	}
	SortEntries(out.Entries)
	return out, nil
}

// SortEntries puts directories first, then sorts by name ignoring case.
func SortEntries(e []protocol.FileEntry) {
	sort.SliceStable(e, func(i, j int) bool {
		di, dj := e[i].Type == "dir", e[j].Type == "dir"
		if di != dj {
			return di
		}
		return strings.ToLower(e[i].Name) < strings.ToLower(e[j].Name)
	})
}

func parentOf(dir string) string {
	p := filepath.Dir(dir)
	if p == dir {
		return rootParent()
	}
	return p
}

func entry(path string, info fs.FileInfo) protocol.FileEntry {
	e := protocol.FileEntry{Name: info.Name(), Path: path, Size: info.Size(), ModTime: info.ModTime().UTC(), Mode: info.Mode().String()}
	switch m := info.Mode(); {
	case m.IsDir():
		e.Type, e.Size = "dir", 0
	case m&fs.ModeSymlink != 0:
		e.Type = "symlink"
		if st, err := os.Stat(path); err == nil && st.IsDir() {
			e.Type = "symlink_dir"
		}
	case m.IsRegular():
		e.Type = "file"
	default:
		e.Type = "other"
	}
	return e
}

// Stat returns the entry of one path.
func Stat(path string) (protocol.FileEntry, error) {
	p, err := Resolve(path)
	if err != nil {
		return protocol.FileEntry{}, err
	}
	info, err := os.Lstat(p)
	if err != nil {
		return protocol.FileEntry{}, rpcutil.FromOS(err)
	}
	return entry(p, info), nil
}

// isRoot is true for "/" or a drive root; those are never deleted or moved.
func isRoot(p string) bool { return filepath.Dir(p) == p }

// Remove deletes a file or directory.
func Remove(path string, recursive bool) error {
	p, err := Resolve(path)
	if err != nil {
		return err
	}
	if isRoot(p) {
		return rpcutil.BadParams("refusing to delete a root directory")
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == p {
		return rpcutil.BadParams("refusing to delete the home directory")
	}
	info, err := os.Lstat(p)
	if err != nil {
		return rpcutil.FromOS(err)
	}
	if info.IsDir() && recursive {
		return rpcutil.FromOS(os.RemoveAll(p))
	}
	if err := os.Remove(p); err != nil {
		if info.IsDir() {
			return rpcutil.Failed("directory is not empty; delete it recursively")
		}
		return rpcutil.FromOS(err)
	}
	return nil
}

// Mkdir creates one directory; the parent must exist.
func Mkdir(path string) (protocol.FileEntry, error) {
	p, err := Resolve(path)
	if err != nil {
		return protocol.FileEntry{}, err
	}
	if err := os.Mkdir(p, 0o755); err != nil {
		return protocol.FileEntry{}, rpcutil.FromOS(err)
	}
	return Stat(p)
}

// Rename moves from to to. to must not exist yet.
func Rename(from, to string) (protocol.FileEntry, error) {
	src, err := Resolve(from)
	if err != nil {
		return protocol.FileEntry{}, err
	}
	dst, err := Resolve(to)
	if err != nil {
		return protocol.FileEntry{}, err
	}
	if isRoot(src) {
		return protocol.FileEntry{}, rpcutil.BadParams("refusing to move a root directory")
	}
	if _, err := os.Lstat(dst); err == nil {
		return protocol.FileEntry{}, &protocol.Error{Code: protocol.CodeExists, Message: dst + " already exists"}
	}
	if err := os.Rename(src, dst); err != nil {
		return protocol.FileEntry{}, rpcutil.FromOS(err)
	}
	return Stat(dst)
}

// ServeRead sends a FileHeader chunk, then the file in 64 KB chunks.
func ServeRead(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
	var p protocol.FilesReadParams
	if err := rpcutil.Decode(raw, &p); err != nil {
		return err
	}
	path, err := Resolve(p.Path)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return rpcutil.FromOS(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return rpcutil.FromOS(err)
	}
	if !info.Mode().IsRegular() {
		return rpcutil.BadParams("%s is not a regular file", path)
	}
	header, _ := json.Marshal(protocol.FileHeader{Name: info.Name(), Size: info.Size(), ModTime: info.ModTime().UTC()})
	if err := s.Send(ctx, header); err != nil {
		return err
	}
	buf := make([]byte, protocol.FileChunkSize)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if serr := s.Send(ctx, chunk); serr != nil {
				return serr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return rpcutil.FromOS(err)
		}
	}
}

// ServeWrite receives exactly Size bytes into a temporary file next to the
// target, renames it into place and answers with the new FileEntry.
func ServeWrite(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
	var p protocol.FilesWriteParams
	if err := rpcutil.Decode(raw, &p); err != nil {
		return err
	}
	path, err := Resolve(p.Path)
	if err != nil {
		return err
	}
	if p.Size < 0 {
		return rpcutil.BadParams("size must not be negative")
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		if !info.Mode().IsRegular() {
			return rpcutil.BadParams("%s exists and is not a regular file", path)
		}
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".upload-*")
	if err != nil {
		return rpcutil.FromOS(err)
	}
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	var total int64
	for total < p.Size {
		chunk, err := s.Recv(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return rpcutil.Failed("upload ended after %d of %d bytes", total, p.Size)
			}
			return err
		}
		total += int64(len(chunk))
		if total > p.Size {
			return rpcutil.BadParams("received more than %d bytes", p.Size)
		}
		if _, err := tmp.Write(chunk); err != nil {
			return rpcutil.FromOS(err)
		}
	}
	_ = tmp.Chmod(mode) // Windows ignores most mode bits
	if err := tmp.Sync(); err != nil {
		return rpcutil.FromOS(err)
	}
	if err := tmp.Close(); err != nil {
		return rpcutil.FromOS(err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return rpcutil.FromOS(err)
	}
	ok = true
	e, err := Stat(path)
	if err != nil {
		return err
	}
	ack, _ := json.Marshal(e)
	if err := s.Send(ctx, ack); err != nil {
		return fmt.Errorf("send ack: %w", err)
	}
	return nil
}
