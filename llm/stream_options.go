package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

func openConfiguredStream(ctx context.Context, cfg StreamConfig, open func(context.Context) (TokenStream, error)) (TokenStream, error) {
	if cfg.RetryStrategy != nil {
		cfg.RetryStrategy.Reset()
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		stream, err := open(ctx)
		if err == nil {
			if stream == nil {
				return nil, fmt.Errorf("provider returned a nil stream")
			}
			if cfg.BufferSize > 0 {
				return newBufferedStream(ctx, stream, cfg.BufferSize), nil
			}
			return stream, nil
		}
		if stream != nil {
			stream.Close()
			return nil, err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || cfg.RetryStrategy == nil || !cfg.RetryStrategy.ShouldRetry(err) {
			return nil, err
		}
		timer := time.NewTimer(cfg.RetryStrategy.NextDelay())
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

type streamResult struct {
	token *StreamToken
	err   error
}

type bufferedStream struct {
	stream    TokenStream
	parent    context.Context
	cancel    context.CancelFunc
	results   chan streamResult
	finished  chan struct{}
	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

func newBufferedStream(ctx context.Context, stream TokenStream, capacity int) TokenStream {
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	s := &bufferedStream{stream: stream, parent: parent, cancel: cancel, results: make(chan streamResult, capacity), finished: make(chan struct{})}
	go func() {
		defer close(s.finished)
		defer close(s.results)
		defer s.closeSource()
		defer cancel()
		for {
			token, err := stream.Next(ctx)
			select {
			case <-ctx.Done():
				return
			case s.results <- streamResult{token: token, err: err}:
			}
			if err != nil {
				return
			}
		}
	}()
	return s
}

func (s *bufferedStream) Next(ctx context.Context) (*StreamToken, error) {
	if err := ctx.Err(); err != nil {
		s.Close()
		return nil, err
	}
	if s.closed.Load() {
		return nil, io.EOF
	}
	select {
	case <-s.parent.Done():
		s.Close()
		return nil, s.parent.Err()
	case <-ctx.Done():
		s.Close()
		return nil, ctx.Err()
	case result, ok := <-s.results:
		if err := ctx.Err(); err != nil {
			s.Close()
			return nil, err
		}
		if err := s.parent.Err(); err != nil {
			s.Close()
			return nil, err
		}
		if !ok || s.closed.Load() {
			return nil, io.EOF
		}
		return result.token, result.err
	}
}

func (s *bufferedStream) closeSource() {
	s.closeOnce.Do(func() { s.closeErr = s.stream.Close() })
}

func (s *bufferedStream) Close() error {
	s.closed.Store(true)
	s.cancel()
	s.closeSource()
	<-s.finished
	return s.closeErr
}
