package llm

import (
	"bufio"
	"bytes"
	"io"
	"time"

	"github.com/YspCoder/omnigo/dto"
)

// Re-export or use dto types
type StreamToken = dto.StreamToken
type TokenStream = dto.TokenStream

// StreamOption is a function type for configuring streaming behavior.
type StreamOption func(*StreamConfig)

// StreamConfig holds configuration options for streaming.
type StreamConfig struct {
	// BufferSize is the size of the token buffer
	BufferSize int

	// RetryStrategy defines how to handle stream interruptions
	RetryStrategy RetryStrategy
}

// RetryStrategy defines how to handle stream interruptions.
type RetryStrategy interface {
	// ShouldRetry determines if a retry should be attempted.
	ShouldRetry(error) bool

	// NextDelay returns the delay before the next retry.
	NextDelay() time.Duration

	// Reset resets the retry state.
	Reset()
}

// DefaultRetryStrategy implements a simple exponential backoff strategy.
type DefaultRetryStrategy struct {
	MaxRetries  int
	InitialWait time.Duration
	MaxWait     time.Duration
	attempts    int
}

func (s *DefaultRetryStrategy) ShouldRetry(err error) bool {
	return s.attempts < s.MaxRetries
}

func (s *DefaultRetryStrategy) NextDelay() time.Duration {
	s.attempts++
	delay := s.InitialWait * time.Duration(1<<uint(s.attempts-1))
	if delay > s.MaxWait {
		delay = s.MaxWait
	}
	return delay
}

func (s *DefaultRetryStrategy) Reset() {
	s.attempts = 0
}

// SSEDecoder handles Server-Sent Events (SSE) streaming
type SSEDecoder struct {
	reader  *bufio.Scanner
	current Event
	err     error
}

type Event struct {
	Type string
	Data []byte
}

func NewSSEDecoder(reader io.Reader) *SSEDecoder {
	return &SSEDecoder{
		reader: bufio.NewScanner(reader),
	}
}

func (d *SSEDecoder) Next() bool {
	if d.err != nil {
		return false
	}

	event := ""
	data := bytes.NewBuffer(nil)

	for d.reader.Scan() {
		line := d.reader.Bytes()

		// Dispatch event on empty line
		if len(line) == 0 {
			d.current = Event{
				Type: event,
				Data: data.Bytes(),
			}
			return true
		}

		// Split "event: value" into parts
		name, value, _ := bytes.Cut(line, []byte(":"))

		// Remove optional space after colon
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}

		switch string(name) {
		case "":
			continue // Skip comments
		case "event":
			event = string(value)
		case "data":
			data.Write(value)
			data.WriteRune('\n')
		}
	}

	if err := d.reader.Err(); err != nil {
		d.err = err
	}
	return false
}

func (d *SSEDecoder) Event() Event {
	return d.current
}

func (d *SSEDecoder) Err() error {
	return d.err
}
