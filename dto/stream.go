package dto

import (
	"context"
	"io"
)

// StreamToken represents a single token from the streaming response.
type StreamToken struct {
	// Text is the actual token text or status message
	Text string

	// Type indicates the event type: text, function_call, usage, finish, error, progress, or url.
	Type string

	// Index is the position of this token in the sequence
	Index int

	// Progress is the percentage of completion (0-100)
	Progress *int

	// URL is the generated media URL (if available in stream)
	URL string

	// Metadata contains provider-specific metadata
	Metadata map[string]interface{}

	// ToolCalls contains incremental function calls, grouped by tool index within a choice.
	ToolCalls    []ToolCallDelta
	Usage        *Usage
	FinishReason string
}

// ToolCallDelta preserves fragments that may not yet form valid JSON arguments.
type ToolCallDelta struct {
	Index    int
	ID       string
	Type     string
	Function ToolCallFunctionDelta
}

type ToolCallFunctionDelta struct {
	Name      string
	Arguments string
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
