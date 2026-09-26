package rpc

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Stream is a bidirectional byte stream multiplexed on a Peer.
// Recv returns io.EOF when the remote side ended normally.
type Stream struct {
	id     string
	p      *Peer
	in     chan []byte
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	endErr   error
	inClosed bool
	closed   bool
}

func (p *Peer) newStream(id string) *Stream {
	ctx, cancel := context.WithCancel(context.Background())
	return &Stream{id: id, p: p, in: make(chan []byte, 1024), ctx: ctx, cancel: cancel}
}

// ID is the stream id.
func (s *Stream) ID() string { return s.id }

// Context is canceled when either side ends the stream.
func (s *Stream) Context() context.Context { return s.ctx }

// Send writes one chunk to the remote side.
func (s *Stream) Send(ctx context.Context, b []byte) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return io.ErrClosedPipe
	}
	return s.p.write(ctx, protocol.Envelope{Kind: protocol.KindData, ID: s.id, Data: b})
}

// Recv waits for the next chunk. Buffered chunks are returned before the
// end error.
func (s *Stream) Recv(ctx context.Context) ([]byte, error) {
	select {
	case b, ok := <-s.in:
		if ok {
			return b, nil
		}
		return nil, s.end()
	case <-s.ctx.Done():
	case <-ctx.Done():
		if s.ctx.Err() == nil {
			return nil, ctx.Err()
		}
	}
	// The stream ended: hand out anything still buffered first.
	select {
	case b, ok := <-s.in:
		if ok {
			return b, nil
		}
	default:
	}
	return nil, s.end()
}

func (s *Stream) end() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.endErr == nil {
		return io.EOF
	}
	return s.endErr
}

// Close ends the stream. A non-nil err is reported to the remote side.
// Calling Close more than once is safe.
func (s *Stream) Close(err error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	s.p.dropStream(s.id)
	env := protocol.Envelope{Kind: protocol.KindEnd, ID: s.id}
	if err != nil && !errors.Is(err, io.EOF) {
		env.Error = toProtoError(err)
	}
	_ = s.p.write(context.Background(), env)
	s.mu.Lock()
	if s.endErr == nil {
		s.endErr = io.EOF
	}
	s.mu.Unlock()
	s.cancel()
}

// Write lets a Stream be used as an io.Writer (for example as process stdout).
func (s *Stream) Write(b []byte) (int, error) {
	buf := make([]byte, len(b))
	copy(buf, b)
	if err := s.Send(s.ctx, buf); err != nil {
		return 0, err
	}
	return len(b), nil
}

// deliver and remoteEnd are only called from the peer's read goroutine (or
// after it stopped), so closing s.in never races with a send.
func (s *Stream) deliver(b []byte) {
	select {
	case s.in <- b:
	case <-s.ctx.Done():
	}
}

func (s *Stream) remoteEnd(err error) {
	s.mu.Lock()
	if s.inClosed {
		s.mu.Unlock()
		return
	}
	s.inClosed = true
	if s.endErr == nil {
		s.endErr = err
	}
	close(s.in)
	s.mu.Unlock()
	s.cancel()
}
