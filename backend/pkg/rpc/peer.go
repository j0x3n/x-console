// Package rpc runs the protocol envelopes over one connection. The server hub
// and the agent both use Peer, so request/response, streams and events work
// the same way in both directions.
package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Conn transports envelopes. Write must be safe to call from one goroutine at
// a time; Peer serializes writes.
type Conn interface {
	Read(ctx context.Context) (protocol.Envelope, error)
	Write(ctx context.Context, env protocol.Envelope) error
	Close() error
}

// Handler answers a request. The returned value is JSON encoded as the result.
type Handler func(ctx context.Context, params json.RawMessage) (any, error)

// StreamHandler serves a stream opened by the remote side. The stream is
// closed with the returned error when the handler returns.
type StreamHandler func(ctx context.Context, params json.RawMessage, s *Stream) error

// EventHandler receives events emitted by the remote side.
type EventHandler func(method string, params json.RawMessage)

// ErrClosed is returned when the connection is gone.
var ErrClosed = errors.New("rpc: connection closed")

// Peer is one end of a connection.
type Peer struct {
	conn   Conn
	prefix string
	seq    atomic.Uint64

	wmu sync.Mutex

	mu             sync.Mutex
	pending        map[string]chan protocol.Envelope
	streams        map[string]*Stream
	inflight       map[string]context.CancelFunc
	handlers       map[string]Handler
	streamHandlers map[string]StreamHandler
	onEvent        EventHandler

	done    chan struct{}
	doneErr error
}

// NewPeer wraps conn. prefix keeps ids from both sides apart ("s" or "a").
func NewPeer(conn Conn, prefix string) *Peer {
	return &Peer{
		conn: conn, prefix: prefix,
		pending: map[string]chan protocol.Envelope{}, streams: map[string]*Stream{},
		inflight: map[string]context.CancelFunc{}, handlers: map[string]Handler{},
		streamHandlers: map[string]StreamHandler{}, done: make(chan struct{}),
	}
}

// Handle registers a request handler. Register before Run.
func (p *Peer) Handle(method string, h Handler) {
	p.mu.Lock()
	p.handlers[method] = h
	p.mu.Unlock()
}

// HandleStream registers a stream handler. Register before Run.
func (p *Peer) HandleStream(method string, h StreamHandler) {
	p.mu.Lock()
	p.streamHandlers[method] = h
	p.mu.Unlock()
}

// OnEvent sets the event callback. Register before Run.
func (p *Peer) OnEvent(h EventHandler) {
	p.mu.Lock()
	p.onEvent = h
	p.mu.Unlock()
}

// Done is closed when the connection ends.
func (p *Peer) Done() <-chan struct{} { return p.done }

// Err is the reason the connection ended, valid after Done is closed.
func (p *Peer) Err() error { return p.doneErr }

// Run reads frames until the connection fails or ctx ends.
func (p *Peer) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var err error
	for {
		var env protocol.Envelope
		env, err = p.conn.Read(ctx)
		if err != nil {
			break
		}
		p.dispatch(ctx, env)
	}
	p.shutdown(err)
	return err
}

// Call sends a request and decodes the result into out (which may be nil).
func (p *Peer) Call(ctx context.Context, method string, params any, out any) error {
	id := p.nextID()
	ch := make(chan protocol.Envelope, 1)
	p.mu.Lock()
	if p.isDone() {
		p.mu.Unlock()
		return ErrClosed
	}
	p.pending[id] = ch
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.pending, id)
		p.mu.Unlock()
	}()
	if err := p.write(ctx, protocol.Envelope{Kind: protocol.KindReq, ID: id, Method: method, Params: protocol.Marshal(params)}); err != nil {
		return err
	}
	select {
	case res, ok := <-ch:
		if !ok {
			return ErrClosed
		}
		if res.Error != nil {
			return res.Error
		}
		if out != nil && len(res.Result) > 0 {
			return json.Unmarshal(res.Result, out)
		}
		return nil
	case <-ctx.Done():
		_ = p.write(context.WithoutCancel(ctx), protocol.Envelope{Kind: protocol.KindCancel, ID: id})
		return ctx.Err()
	case <-p.done:
		return ErrClosed
	}
}

// Open starts a stream served by the remote side's StreamHandler.
func (p *Peer) Open(ctx context.Context, method string, params any) (*Stream, error) {
	s := p.newStream(p.nextID())
	p.mu.Lock()
	if p.isDone() {
		p.mu.Unlock()
		return nil, ErrClosed
	}
	p.streams[s.id] = s
	p.mu.Unlock()
	if err := p.write(ctx, protocol.Envelope{Kind: protocol.KindOpen, ID: s.id, Method: method, Params: protocol.Marshal(params)}); err != nil {
		p.dropStream(s.id)
		return nil, err
	}
	return s, nil
}

// Emit sends a fire-and-forget event.
func (p *Peer) Emit(ctx context.Context, method string, params any) error {
	return p.write(ctx, protocol.Envelope{Kind: protocol.KindEvent, Method: method, Params: protocol.Marshal(params)})
}

func (p *Peer) nextID() string {
	return p.prefix + strconv.FormatUint(p.seq.Add(1), 36)
}

func (p *Peer) write(ctx context.Context, env protocol.Envelope) error {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	if p.isDone() {
		return ErrClosed
	}
	return p.conn.Write(ctx, env)
}

func (p *Peer) isDone() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *Peer) dispatch(ctx context.Context, env protocol.Envelope) {
	switch env.Kind {
	case protocol.KindRes:
		p.mu.Lock()
		ch := p.pending[env.ID]
		p.mu.Unlock()
		if ch != nil {
			ch <- env
		}
	case protocol.KindReq:
		p.serveRequest(ctx, env)
	case protocol.KindCancel:
		p.mu.Lock()
		cancel := p.inflight[env.ID]
		p.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	case protocol.KindOpen:
		p.serveStream(ctx, env)
	case protocol.KindData:
		p.mu.Lock()
		s := p.streams[env.ID]
		p.mu.Unlock()
		if s != nil {
			s.deliver(env.Data)
		}
	case protocol.KindEnd:
		p.mu.Lock()
		s := p.streams[env.ID]
		delete(p.streams, env.ID)
		p.mu.Unlock()
		if s != nil {
			var err error = io.EOF
			if env.Error != nil {
				err = env.Error
			}
			s.remoteEnd(err)
		}
	case protocol.KindEvent:
		p.mu.Lock()
		h := p.onEvent
		p.mu.Unlock()
		if h != nil {
			h(env.Method, env.Params)
		}
	}
}

func (p *Peer) serveRequest(ctx context.Context, env protocol.Envelope) {
	p.mu.Lock()
	h := p.handlers[env.Method]
	reqCtx, cancel := context.WithCancel(ctx)
	p.inflight[env.ID] = cancel
	p.mu.Unlock()
	go func() {
		defer func() {
			cancel()
			p.mu.Lock()
			delete(p.inflight, env.ID)
			p.mu.Unlock()
		}()
		res := protocol.Envelope{Kind: protocol.KindRes, ID: env.ID}
		if h == nil {
			res.Error = &protocol.Error{Code: protocol.CodeUnknownMethod, Message: env.Method}
		} else if out, err := safeCall(func() (any, error) { return h(reqCtx, env.Params) }); err != nil {
			res.Error = toProtoError(err)
		} else {
			res.Result = protocol.Marshal(out)
		}
		_ = p.write(context.WithoutCancel(ctx), res)
	}()
}

func (p *Peer) serveStream(ctx context.Context, env protocol.Envelope) {
	p.mu.Lock()
	h := p.streamHandlers[env.Method]
	s := p.newStream(env.ID)
	p.streams[env.ID] = s
	p.mu.Unlock()
	if h == nil {
		s.Close(&protocol.Error{Code: protocol.CodeUnknownMethod, Message: env.Method})
		return
	}
	go func() {
		_, err := safeCall(func() (any, error) { return nil, h(s.ctx, env.Params, s) })
		s.Close(err)
	}()
}

func (p *Peer) dropStream(id string) {
	p.mu.Lock()
	delete(p.streams, id)
	p.mu.Unlock()
}

func (p *Peer) shutdown(err error) {
	p.mu.Lock()
	if p.isDone() {
		p.mu.Unlock()
		return
	}
	if err == nil {
		err = ErrClosed
	}
	p.doneErr = err
	close(p.done)
	pending := p.pending
	streams := p.streams
	inflight := p.inflight
	p.pending = map[string]chan protocol.Envelope{}
	p.streams = map[string]*Stream{}
	p.mu.Unlock()
	for _, ch := range pending {
		close(ch)
	}
	for _, s := range streams {
		s.remoteEnd(ErrClosed)
	}
	for _, cancel := range inflight {
		cancel()
	}
	_ = p.conn.Close()
}

func safeCall(fn func() (any, error)) (out any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return fn()
}

func toProtoError(err error) *protocol.Error {
	if err == nil {
		return nil
	}
	var pe *protocol.Error
	if errors.As(err, &pe) {
		return pe
	}
	if errors.Is(err, context.Canceled) {
		return &protocol.Error{Code: protocol.CodeCanceled, Message: err.Error()}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &protocol.Error{Code: protocol.CodeTimeout, Message: err.Error()}
	}
	return &protocol.Error{Code: protocol.CodeFailed, Message: err.Error()}
}
