package adapter

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"github.com/YspCoder/omnigo/dto"
)

// mediaStreamState keeps terminal results single-use, including after Close.
type mediaStreamState struct {
	done       atomic.Bool
	pendingErr error
}

func (s *mediaStreamState) streamError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.pendingErr != nil {
		err := s.pendingErr
		s.pendingErr = nil
		return err
	}
	if s.done.Load() {
		return io.EOF
	}
	return nil
}

func (s *mediaStreamState) failWithUsage(token *dto.StreamToken, err error) (*dto.StreamToken, error) {
	s.done.Store(true)
	if token != nil && token.Usage != nil {
		token.Type = "usage"
		s.pendingErr = err
		return token, nil
	}
	return token, err
}

func (s *mediaStreamState) Close() error {
	s.done.Store(true)
	return nil
}

// bodyWithContext also interrupts transports that do not observe request
// cancellation. Only the body is closed concurrently, never decoder state.
func bodyWithContext(ctx context.Context, body io.ReadCloser) io.ReadCloser {
	wrapped := &contextBody{ReadCloser: body, ctx: ctx}
	wrapped.stop = context.AfterFunc(ctx, func() { wrapped.closeSource() })
	return wrapped
}

type contextBody struct {
	io.ReadCloser
	ctx  context.Context
	stop func() bool
	once sync.Once
	err  error
}

func (b *contextBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && b.ctx.Err() != nil {
		err = b.ctx.Err()
	}
	return n, err
}

func (b *contextBody) closeSource() error {
	b.once.Do(func() { b.err = b.ReadCloser.Close() })
	return b.err
}

func (b *contextBody) Close() error {
	b.stop()
	return b.closeSource()
}

// cancellableStream interrupts reads through the SDK request context. Closing
// the decoder is serialized with Next because SDK iterators are not concurrent.
type cancellableStream struct {
	stream dto.TokenStream
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
}

func newCancellableStream(stream dto.TokenStream, cancel context.CancelFunc) dto.TokenStream {
	return &cancellableStream{stream: stream, cancel: cancel}
}

func (s *cancellableStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, io.EOF
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, s.cancel)
	token, err := s.stream.Next(ctx)
	stop()
	if ctxErr := ctx.Err(); ctxErr != nil {
		token, err = nil, ctxErr
	}
	if err != nil {
		s.closeLocked()
	}
	return token, err
}

func (s *cancellableStream) Close() error {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeLocked()
}

func (s *cancellableStream) closeLocked() error {
	if s.closed {
		return nil
	}
	s.closed = true
	s.cancel()
	return s.stream.Close()
}
