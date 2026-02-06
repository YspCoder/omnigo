// Package adapter provides Anthropic adaptor implementation.
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/YspCoder/omnigo/dto"
)

type anthropicMessageContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicMessage struct {
	Role    string                    `json:"role"`
	Content []anthropicMessageContent `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicResponse struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Role       string `json:"role"`
	Model      string `json:"model"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// AnthropicAdaptor converts requests and responses for Anthropic APIs.
type AnthropicAdaptor struct {
	BaseURL string
}

// GetRequestURL returns the Anthropic endpoint for the given mode.
func (a *AnthropicAdaptor) GetRequestURL(mode string, config *ProviderConfig) (string, error) {
	if mode != ModeChat {
		return "", fmt.Errorf("unsupported mode for anthropic: %s", mode)
	}

	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		base = "https://api.anthropic.com/v1"
	}

	const suffix = "/messages"
	parsed, err := url.Parse(base)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		path := strings.TrimRight(parsed.Path, "/")
		if !strings.HasSuffix(path, suffix) {
			path = strings.TrimSuffix(path, suffix) + suffix
			parsed.Path = path
		}
		return parsed.String(), nil
	}

	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, suffix) {
		return base, nil
	}
	return base + suffix, nil
}

// SetupHeaders sets Anthropic-specific headers.
func (a *AnthropicAdaptor) SetupHeaders(req *http.Request, config *ProviderConfig, mode string) error {
	if config.AuthHeader != "" && config.APIKey != "" {
		req.Header.Set(config.AuthHeader, config.AuthPrefix+config.APIKey)
	} else if config.APIKey != "" {
		req.Header.Set("x-api-key", config.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")
	if _, ok := config.Headers["anthropic-version"]; !ok {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	return nil
}

// ConvertChatRequest marshals the Anthropic chat request.
func (a *AnthropicAdaptor) ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	_ = ctx
	_ = config

	messages := request.Messages
	if len(messages) == 0 && request.Prompt != "" {
		messages = []dto.Message{{Role: "user", Content: request.Prompt}}
	}

	payload := anthropicRequest{
		Model:    request.Model,
		Messages: make([]anthropicMessage, 0, len(messages)),
	}

	systemParts := make([]string, 0, 2)
	if systemPrompt, ok := request.Options["system_prompt"].(string); ok && systemPrompt != "" {
		systemParts = append(systemParts, systemPrompt)
	}

	for _, msg := range messages {
		content := strings.TrimSpace(fmt.Sprint(msg.Content))
		if content == "" {
			continue
		}
		role := strings.ToLower(msg.Role)
		switch role {
		case "assistant", "user":
			payload.Messages = append(payload.Messages, anthropicMessage{
				Role: role,
				Content: []anthropicMessageContent{
					{Type: "text", Text: content},
				},
			})
		case "system":
			systemParts = append(systemParts, content)
		default:
			payload.Messages = append(payload.Messages, anthropicMessage{
				Role: "user",
				Content: []anthropicMessageContent{
					{Type: "text", Text: content},
				},
			})
		}
	}

	if len(payload.Messages) == 0 {
		return nil, fmt.Errorf("anthropic request requires at least one user or assistant message")
	}

	if len(systemParts) > 0 {
		payload.System = strings.Join(systemParts, "\n\n")
	}

	if request.MaxTokens > 0 {
		payload.MaxTokens = request.MaxTokens
	}
	if maxTokens, ok := request.Options["max_tokens"].(int); ok && maxTokens > 0 {
		payload.MaxTokens = maxTokens
	}
	if payload.MaxTokens == 0 {
		payload.MaxTokens = 1024
	}

	if request.Temperature != 0 {
		payload.Temperature = request.Temperature
	}
	if temperature, ok := request.Options["temperature"].(float64); ok {
		payload.Temperature = temperature
	}

	if request.Stream {
		payload.Stream = true
	}
	if stream, ok := request.Options["stream"].(bool); ok && stream {
		payload.Stream = true
	}

	return json.Marshal(payload)
}

// ConvertChatResponse unmarshals the Anthropic chat response.
func (a *AnthropicAdaptor) ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error) {
	_ = ctx

	var response anthropicResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, &dto.LLMError{
			Code:     http.StatusBadRequest,
			Message:  response.Error.Message,
			Provider: config.Name,
		}
	}

	textParts := make([]string, 0, len(response.Content))
	for _, block := range response.Content {
		if block.Type == "text" && block.Text != "" {
			textParts = append(textParts, block.Text)
		}
	}

	content := strings.Join(textParts, "")
	chatResp := &dto.ChatResponse{
		ID:     response.ID,
		Object: response.Type,
		Model:  response.Model,
		Choices: []dto.ChatChoice{{
			Index:        0,
			Message:      dto.Message{Role: "assistant", Content: content},
			FinishReason: response.StopReason,
		}},
		Usage: dto.Usage{
			PromptTokens:     response.Usage.InputTokens,
			CompletionTokens: response.Usage.OutputTokens,
			TotalTokens:      response.Usage.InputTokens + response.Usage.OutputTokens,
		},
	}
	return chatResp, nil
}

// ConvertMediaRequest is not supported for Anthropic.
func (a *AnthropicAdaptor) ConvertMediaRequest(ctx context.Context, config *ProviderConfig, mode string, request *dto.MediaRequest) ([]byte, error) {
	_ = ctx
	_ = config
	_ = request
	return nil, fmt.Errorf("media mode not supported for anthropic: %s", mode)
}

// ConvertMediaResponse is not supported for Anthropic.
func (a *AnthropicAdaptor) ConvertMediaResponse(ctx context.Context, config *ProviderConfig, mode string, body []byte) (*dto.MediaResponse, error) {
	_ = ctx
	_ = config
	_ = body
	return nil, fmt.Errorf("media mode not supported for anthropic: %s", mode)
}

// PrepareStreamRequest creates a streaming chat request body.
func (a *AnthropicAdaptor) PrepareStreamRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	streamRequest := *request
	if streamRequest.Options == nil {
		streamRequest.Options = make(map[string]interface{})
	}
	streamRequest.Stream = true
	streamRequest.Options["stream"] = true
	return a.ConvertChatRequest(ctx, config, &streamRequest)
}

// ParseStreamResponse processes a single Anthropic streaming event.
func (a *AnthropicAdaptor) ParseStreamResponse(chunk []byte) (string, error) {
	if len(strings.TrimSpace(string(chunk))) == 0 {
		return "", fmt.Errorf("empty chunk")
	}

	var event struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
		ContentBlock struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content_block"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(chunk, &event); err != nil {
		return "", fmt.Errorf("malformed response: %w", err)
	}

	if event.Error != nil && event.Error.Message != "" {
		return "", fmt.Errorf("%s", event.Error.Message)
	}

	switch event.Type {
	case "content_block_delta":
		if event.Delta.Type == "text_delta" {
			return event.Delta.Text, nil
		}
		return "", fmt.Errorf("skip token")
	case "content_block_start":
		if event.ContentBlock.Type == "text" && event.ContentBlock.Text != "" {
			return event.ContentBlock.Text, nil
		}
		return "", fmt.Errorf("skip token")
	case "message_stop":
		return "", io.EOF
	default:
		return "", fmt.Errorf("skip token")
	}
}
