package repo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

func (r *Repository) writeFile(ctx context.Context, w io.Writer, f File) error {
	h := sha256.New()
	for _, b := range f.Blocks {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := readObject(ctx, r.store, blobPath(b.Hash), maxBlob)
		if err != nil {
			return err
		}
		plain, err := r.decodeBlock(raw, b)
		if err != nil {
			return err
		}
		if _, err := io.Copy(io.MultiWriter(w, h), bytes.NewReader(plain)); err != nil {
			return err
		}
	}
	if hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
		return fmt.Errorf("%w：文件内容校验失败", ErrCorrupt)
	}
	return nil
}

func (r *Repository) Restore(ctx context.Context, id string, database io.Writer, dest files.Store, progress func(int64, int64)) (Snapshot, error) {
	var s Snapshot
	if err := r.writable(ctx); err != nil {
		return s, err
	}
	s, err := r.ReadSnapshot(ctx, id)
	if err != nil {
		return s, err
	}
	if err := r.writeFile(ctx, database, s.Database); err != nil {
		return s, err
	}
	for i, f := range s.Files {
		pr, pw := io.Pipe()
		finished := make(chan error, 1)
		go func() { err := r.writeFile(ctx, pw, f); pw.CloseWithError(err); finished <- err }()
		err := dest.Put(ctx, f.Path, pr, f.Size)
		pr.CloseWithError(err)
		readErr := <-finished
		if err != nil {
			return s, err
		}
		if readErr != nil {
			return s, readErr
		}
		if progress != nil {
			progress(int64(i+1), int64(len(s.Files)))
		}
	}
	return s, nil
}
