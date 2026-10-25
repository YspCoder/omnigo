package llm

import (
	"io"
	"math"
	"time"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
)

// Re-export or use dto types
type StreamToken = dto.StreamToken
type TokenStream = dto.TokenStream

// StreamOption is a function type for configuring streaming behavior.
type StreamOption func(*StreamConfig)

// StreamConfig holds configuration options for streaming.
type StreamConfig struct {
	// BufferSize enables bounded token prefetching. Zero keeps pull-based reads.
	BufferSize int

	// RetryStrategy retries failed stream creation. Once a stream is returned,
	// read failures are propagated without replaying a generation.
	// A strategy instance must not be shared by concurrent Stream calls.
	RetryStrategy RetryStrategy
}

// RetryStrategy controls retries while opening a stream.
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
	limit := s.MaxWait
	if limit <= 0 {
		limit = time.Duration(math.MaxInt64)
	}
	delay := max(0, s.InitialWait)
	for i := 1; i < s.attempts && delay > 0 && delay < limit; i++ {
		if delay > limit/2 {
			return limit
		}
		delay *= 2
	}
	return min(delay, limit)
}

func (s *DefaultRetryStrategy) Reset() {
	s.attempts = 0
}

// SSEDecoder and Event retain the public decoder API shared by the adaptors.
type SSEDecoder = utils.SSEDecoder
type Event = utils.SSEEvent

func NewSSEDecoder(reader io.Reader) *SSEDecoder { return utils.NewSSEDecoder(reader) }
