// Package dto defines standardized request and response payloads.
package dto

// Message represents a single message in a chat conversation.
type Message struct {
	Role        string      `json:"role"`
	Content     interface{} `json:"content"`
	ImageURL    string      `json:"image_url,omitempty"`
	ImageDetail string      `json:"image_detail,omitempty"`
	VideoURL    string      `json:"video_url,omitempty"`
	VideoFPS    float64     `json:"video_fps,omitempty"`
	FileURL     string      `json:"file_url,omitempty"`
	FileID      string      `json:"file_id,omitempty"`
	FileName    string      `json:"file_name,omitempty"`
	Name        string      `json:"name,omitempty"`
}

// ChatRequest represents a chat completion request following the OpenAI schema.
type ChatRequest struct {
	Model       string                 `json:"model"`
	Messages    []Message              `json:"messages"`
	Stream      bool                   `json:"stream,omitempty"`
	Temperature float64                `json:"temperature,omitempty"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
	Prompt      string                 `json:"-"`
	Options     map[string]interface{} `json:"-"`
	Schema      interface{}            `json:"-"`
}

// ChatResponse represents a chat completion response.
type ChatResponse struct {
	ID      string       `json:"id,omitempty"`
	Object  string       `json:"object,omitempty"`
	Created int64        `json:"created,omitempty"`
	Model   string       `json:"model,omitempty"`
	Choices []ChatChoice `json:"choices,omitempty"`
	Usage   Usage        `json:"usage,omitempty"`
}

// GenerateResponse preserves both the extracted text and the raw provider payload.
type GenerateResponse struct {
	Text string        `json:"text,omitempty"`
	Raw  *ChatResponse `json:"raw,omitempty"`
}

// ChatChoice represents a single response choice.
type ChatChoice struct {
	Index        int     `json:"index,omitempty"`
	Message      Message `json:"message,omitempty"`
	FinishReason string  `json:"finish_reason,omitempty"`
}

// Usage represents token usage statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}
