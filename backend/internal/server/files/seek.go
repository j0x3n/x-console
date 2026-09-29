package files

import (
	"context"
	"errors"
	"io"
	"os"
)

// SeekReader is a file opened for http.ServeContent and similar callers.
type SeekReader interface {
	io.ReadSeekCloser
}

// OpenSeeker opens key as a SeekReader. Local files are returned as they are.
// For other Stores it reads by ranges, so serving a Range request from S3
// downloads only the bytes asked for.
func OpenSeeker(ctx context.Context, s Store, key string) (SeekReader, Info, error) {
	rc, info, err := s.Get(ctx, key)
	if err != nil {
		return nil, Info{}, err
	}
	if f, ok := rc.(*os.File); ok {
		return f, info, nil
	}
	// Not seekable: drop this stream and read by ranges instead.
	rc.Close()
	return &rangeReader{ctx: ctx, s: s, key: key, size: info.Size}, info, nil
}

type rangeReader struct {
	ctx  context.Context
	s    Store
	key  string
	size int64
	pos  int64
	cur  io.ReadCloser // stream starting at pos, or nil
}

func (r *rangeReader) Read(p []byte) (int, error) {
	if r.pos >= r.size {
		return 0, io.EOF
	}
	if r.cur == nil {
		rc, _, err := r.s.GetRange(r.ctx, r.key, r.pos, -1)
		if err != nil {
			return 0, err
		}
		r.cur = rc
	}
	n, err := r.cur.Read(p)
	r.pos += int64(n)
	return n, err
}

func (r *rangeReader) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.pos + offset
	case io.SeekEnd:
		abs = r.size + offset
	default:
		return 0, errors.New("files: bad whence")
	}
	if abs < 0 {
		return 0, errors.New("files: negative position")
	}
	if abs != r.pos && r.cur != nil {
		r.cur.Close()
		r.cur = nil
	}
	r.pos = abs
	return abs, nil
}

func (r *rangeReader) Close() error {
	if r.cur != nil {
		return r.cur.Close()
	}
	return nil
}
