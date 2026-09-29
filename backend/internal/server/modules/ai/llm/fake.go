package llm

import (
	"context"
	"sync"
)

// Fake returns queued results in order. Tests can use it without an HTTP server.
type Fake struct {
	mu    sync.Mutex
	queue []fakeReply
	Calls []Request
}

type fakeReply struct {
	result Result
	err    error
}

func NewFake() *Fake { return &Fake{} }

func (f *Fake) Queue(result Result, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = append(f.queue, fakeReply{result: result, err: err})
}

func (f *Fake) pop(req Request) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, req)
	if len(f.queue) == 0 {
		return Result{}, ErrNotConfigured
	}
	next := f.queue[0]
	f.queue = f.queue[1:]
	return next.result, next.err
}

func (f *Fake) Complete(_ context.Context, req Request) (Result, error) { return f.pop(req) }

func (f *Fake) Stream(_ context.Context, req Request) (Stream, error) {
	result, err := f.pop(req)
	if err != nil {
		return nil, err
	}
	return &fakeStream{result: result}, nil
}

type fakeStream struct {
	result Result
	sent   bool
}

func (f *fakeStream) Next() bool {
	if f.sent {
		return false
	}
	f.sent = true
	return true
}
func (f *fakeStream) Current() Delta { return Delta{Text: f.result.Text} }
func (f *fakeStream) Result() Result { return f.result }
func (f *fakeStream) Err() error     { return nil }
func (f *fakeStream) Close() error   { return nil }
