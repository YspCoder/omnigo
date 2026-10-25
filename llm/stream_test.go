package llm

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSSEDecoderHandlesCRLFEventDelimiters(t *testing.T) {
	decoder := NewSSEDecoder(strings.NewReader(": heartbeat\r\n\r\nevent: empty\r\n\r\nevent: message\r\ndata: hello\r\n\r\n"))

	if !decoder.Next() {
		t.Fatalf("Next() = false, want event; err = %v", decoder.Err())
	}
	event := decoder.Event()
	if event.Type != "message" {
		t.Fatalf("event type = %q, want message", event.Type)
	}
	if string(event.Data) != "hello\n" {
		t.Fatalf("event data = %q, want %q", event.Data, "hello\n")
	}
	if decoder.Next() {
		t.Fatal("Next() = true after the only event, want false")
	}
	if err := decoder.Err(); err != nil {
		t.Fatalf("decoder error = %v", err)
	}
}

type optionTestStream struct {
	next   func(context.Context) (*StreamToken, error)
	closes atomic.Int32
}

func (s *optionTestStream) Next(ctx context.Context) (*StreamToken, error) { return s.next(ctx) }
func (s *optionTestStream) Close() error                                   { s.closes.Add(1); return nil }

func TestStreamOptionsLifecycle(t *testing.T) {
	t.Run("partial stream creation is not retried", func(t *testing.T) {
		failure := errors.New("partial creation")
		source := &optionTestStream{}
		attempts := 0
		_, err := openConfiguredStream(context.Background(), StreamConfig{RetryStrategy: &DefaultRetryStrategy{MaxRetries: 1}}, func(context.Context) (TokenStream, error) {
			attempts++
			return source, failure
		})
		if !errors.Is(err, failure) || attempts != 1 || source.closes.Load() != 1 {
			t.Fatalf("attempts=%d closes=%d err=%v", attempts, source.closes.Load(), err)
		}
	})
	t.Run("retry creation without replaying read errors", func(t *testing.T) {
		failure := errors.New("connection lost")
		attempts, reads := 0, 0
		source := &optionTestStream{next: func(context.Context) (*StreamToken, error) {
			reads++
			if reads == 1 {
				return &StreamToken{Text: "partial"}, nil
			}
			return nil, failure
		}}
		stream, err := openConfiguredStream(context.Background(), StreamConfig{BufferSize: 1, RetryStrategy: &DefaultRetryStrategy{MaxRetries: 1}}, func(context.Context) (TokenStream, error) {
			attempts++
			if attempts == 1 {
				return nil, failure
			}
			return source, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		if token, err := stream.Next(context.Background()); err != nil || token.Text != "partial" {
			t.Fatalf("token=%+v err=%v", token, err)
		}
		if _, err := stream.Next(context.Background()); !errors.Is(err, failure) {
			t.Fatalf("read error=%v", err)
		}
		if _, err := stream.Next(context.Background()); err != io.EOF {
			t.Fatalf("terminal error=%v", err)
		}
		if attempts != 2 {
			t.Fatalf("attempts=%d", attempts)
		}
		stream.Close()
		if source.closes.Load() != 1 {
			t.Fatalf("close calls=%d", source.closes.Load())
		}
	})
	t.Run("close unblocks prefetch", func(t *testing.T) {
		started := make(chan struct{})
		source := &optionTestStream{next: func(ctx context.Context) (*StreamToken, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		stream := newBufferedStream(context.Background(), source, 1)
		<-started
		closed := make(chan struct{})
		go func() { stream.Close(); close(closed) }()
		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Fatal("Close blocked")
		}
		if _, err := stream.Next(context.Background()); err != io.EOF {
			t.Fatalf("closed error=%v", err)
		}
	})
	t.Run("parent cancellation remains visible", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		source := &optionTestStream{next: func(ctx context.Context) (*StreamToken, error) { <-ctx.Done(); return nil, ctx.Err() }}
		stream := newBufferedStream(ctx, source, 1)
		defer stream.Close()
		cancel()
		if _, err := stream.Next(context.Background()); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error=%v", err)
		}
	})
}

func TestSSEDecoderFlushesEventAtEOF(t *testing.T) {
	decoder := NewSSEDecoder(strings.NewReader("event: done\ndata: final\n"))

	if !decoder.Next() {
		t.Fatalf("Next() = false, want final event; err = %v", decoder.Err())
	}
	event := decoder.Event()
	if event.Type != "done" || string(event.Data) != "final\n" {
		t.Fatalf("event = %#v, want type done and data %q", event, "final\n")
	}
	if decoder.Next() {
		t.Fatal("Next() = true after EOF event was flushed, want false")
	}
}

func TestSSEDecoderAcceptsLongDataLines(t *testing.T) {
	data := strings.Repeat("x", 128*1024)
	decoder := NewSSEDecoder(strings.NewReader("data: " + data + "\n\n"))

	if !decoder.Next() {
		t.Fatalf("Next() = false for long event; err = %v", decoder.Err())
	}
	if got := string(decoder.Event().Data); got != data+"\n" {
		t.Fatalf("data length = %d, want %d", len(got), len(data)+1)
	}
	if err := decoder.Err(); err != nil {
		t.Fatalf("decoder error = %v", err)
	}
}
