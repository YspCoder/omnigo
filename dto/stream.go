package dto

import (
	"context"
	"io"
)

// StreamToken represents a single token from the streaming response.
type StreamToken struct {
	// Text is the actual token text or status message
	Text string

	// Type indicates the type of token (e.g., "text", "function_call", "error", "progress", "url")
	Type string

	// Index is the position of this token in the sequence
	Index int

	// Progress is the percentage of completion (0-100)
	Progress *int

	// URL is the generated media URL (if available in stream)
	URL string

	// Metadata contains provider-specific metadata
	Metadata map[string]interface{}
}

// TokenStream represents a stream of tokens from the LLM.
// It follows Go's io.ReadCloser pattern but with token-level granularity.
type TokenStream interface {
	// Next returns the next token in the stream.
	// When the stream is finished, it returns io.EOF.
	Next(context.Context) (*StreamToken, error)

	// Close releases any resources associated with the stream.
	io.Closer
}
