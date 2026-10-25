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
	done atomic.Bool
}

func (s *mediaStreamState) Close() error {
	s.done.Store(true)
	return nil
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
