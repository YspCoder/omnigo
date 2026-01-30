// Package adapter provides Anthropic adaptor implementation.
package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
)

// AnthropicAdaptor converts requests and responses for Anthropic's Messages API.
type AnthropicAdaptor struct {
	BaseURL string
}

// GetRequestURL returns the Anthropic endpoint for chat mode.
func (a *AnthropicAdaptor) GetRequestURL(mode string, config *ProviderConfig) (string, error) {
	if mode != ModeChat {
		return "", fmt.Errorf("unsupported mode for anthropic adaptor: %s", mode)
	}
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		base = "https://api.anthropic.com/v1/messages"
	}
	return base, nil
}

// SetupHeaders sets Anthropic-specific headers.
func (a *AnthropicAdaptor) SetupHeaders(req *http.Request, config *ProviderConfig, mode string) error {
	if config.AuthHeader != "" {
		req.Header.Set(config.AuthHeader, config.AuthPrefix+config.APIKey)
	} else if config.APIKey != "" {
		req.Header.Set("x-api-key", config.APIKey)
	}
	req.Header.Set("content-type", "application/json")
	if _, ok := req.Header["anthropic-version"]; !ok {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	return nil
}

// ConvertChatRequest converts a chat request to Anthropic format.
func (a *AnthropicAdaptor) ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	payload := map[string]interface{}{
		"model":      request.Model,
		"max_tokens": defaultOption(request.Options, "max_tokens", request.MaxTokens),
		"system":     []map[string]interface{}{},
		"messages":   []map[string]interface{}{},
	}

	systemPrompt, _ := request.Options["system_prompt"].(string)

	if tools, ok := request.Options["tools"].([]utils.Tool); ok && len(tools) > 0 {
		anthropicTools := make([]map[string]interface{}, len(tools))
		for i, tool := range tools {
			anthropicTools[i] = map[string]interface{}{
				"name":         tool.Function.Name,
				"description":  tool.Function.Description,
				"input_schema": tool.Function.Parameters,
			}
		}
		payload["tools"] = anthropicTools

		if len(tools) > 1 {
			toolUsagePrompt := "When multiple tools are needed to answer a question, you should identify all required tools upfront and use them all at once in your response, rather than using them sequentially. Do not wait for tool results before calling other tools."
			payload["system"] = append(payload["system"].([]map[string]interface{}), map[string]interface{}{
				"type": "text",
				"text": toolUsagePrompt,
			})
		}

		if toolChoice, ok := request.Options["tool_choice"].(string); ok {
			payload["tool_choice"] = map[string]interface{}{"type": toolChoice}
		} else if toolChoice, ok := request.Options["tool_choice"].(map[string]interface{}); ok {
			payload["tool_choice"] = toolChoice
		} else {
			payload["tool_choice"] = map[string]interface{}{"type": "auto"}
		}
	}

	if systemPrompt != "" {
		parts := splitSystemPrompt(systemPrompt, 3)
		for i, part := range parts {
			systemMessage := map[string]interface{}{
				"type": "text",
				"text": part,
			}
			if i > 0 {
				systemMessage["cache_control"] = map[string]string{"type": "ephemeral"}
			}
			payload["system"] = append(payload["system"].([]map[string]interface{}), systemMessage)
		}
	}

	messages := request.Messages
	if len(messages) == 0 && request.Prompt != "" {
		messages = []dto.Message{{Role: "user", Content: request.Prompt}}
	}

	for _, msg := range messages {
		payload["messages"] = append(payload["messages"].([]map[string]interface{}), map[string]interface{}{
			"role":    msg.Role,
			"content": anthropicContent(msg.Content, request.Options),
		})
	}

	for key, value := range request.Options {
		if shouldSkipAnthropicOption(key) {
			continue
		}
		payload[key] = value
	}

	return json.Marshal(payload)
}

// ConvertChatResponse converts Anthropic responses to the standardized format.
func (a *AnthropicAdaptor) ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error) {
	var response struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text,omitempty"`
			Name  string          `json:"name,omitempty"`
			Input json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if len(response.Content) == 0 {
		return nil, fmt.Errorf("empty response from anthropic")
	}

	var final strings.Builder
	var functionCalls []string
	var pending strings.Builder
	lastType := ""

	for _, content := range response.Content {
		switch content.Type {
		case "text":
			if lastType == "text" && pending.Len() > 0 {
				pending.WriteString(" ")
			}
			pending.WriteString(content.Text)
		case "tool_use", "tool_calls":
			if pending.Len() > 0 {
				if final.Len() > 0 {
					final.WriteString("\n")
				}
				final.WriteString(pending.String())
				pending.Reset()
			}
			var args interface{}
			if err := json.Unmarshal(content.Input, &args); err != nil {
				return nil, fmt.Errorf("error parsing tool input: %w", err)
			}
			call, err := utils.FormatFunctionCall(content.Name, args)
			if err != nil {
				return nil, fmt.Errorf("error formatting function call: %w", err)
			}
			functionCalls = append(functionCalls, call)
		}
		lastType = content.Type
	}

	if pending.Len() > 0 {
		if final.Len() > 0 {
			final.WriteString("\n")
		}
		final.WriteString(pending.String())
	}

	if len(functionCalls) > 0 {
		if final.Len() > 0 {
			final.WriteString("\n")
		}
		final.WriteString(strings.Join(functionCalls, "\n"))
	}

	return &dto.ChatResponse{
		Choices: []dto.ChatChoice{{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: final.String(),
			},
		}},
	}, nil
}

// ConvertImageRequest returns an error because Anthropic does not support images here.
func (a *AnthropicAdaptor) ConvertImageRequest(ctx context.Context, config *ProviderConfig, request *dto.ImageRequest) ([]byte, error) {
	return nil, fmt.Errorf("image mode not supported for anthropic adaptor")
}

// ConvertImageResponse returns an error because Anthropic does not support images here.
func (a *AnthropicAdaptor) ConvertImageResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ImageResponse, error) {
	return nil, fmt.Errorf("image mode not supported for anthropic adaptor")
}

// ConvertVideoRequest returns an error because Anthropic does not support video.
func (a *AnthropicAdaptor) ConvertVideoRequest(ctx context.Context, config *ProviderConfig, request *dto.VideoRequest) ([]byte, error) {
	return nil, fmt.Errorf("video mode not supported for anthropic adaptor")
}

// ConvertVideoResponse returns an error because Anthropic does not support video.
func (a *AnthropicAdaptor) ConvertVideoResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.VideoResponse, error) {
	return nil, fmt.Errorf("video mode not supported for anthropic adaptor")
}

// PrepareStreamRequest creates a streaming chat request body.
func (a *AnthropicAdaptor) PrepareStreamRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	streamRequest := *request
	streamRequest.Options = copyOptions(streamRequest.Options)
	streamRequest.Options["stream"] = true
	return a.ConvertChatRequest(ctx, config, &streamRequest)
}

// ParseStreamResponse processes a single streaming chunk.
func (a *AnthropicAdaptor) ParseStreamResponse(chunk []byte) (string, error) {
	if len(bytes.TrimSpace(chunk)) == 0 {
		return "", fmt.Errorf("empty chunk")
	}
	if bytes.Equal(bytes.TrimSpace(chunk), []byte("[DONE]")) {
		return "", io.EOF
	}

	var event struct {
		Type  string `json:"type"`
		Index int    `json:"index"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	}
	if err := json.Unmarshal(chunk, &event); err != nil {
		return "", fmt.Errorf("malformed event: %w", err)
	}

	switch event.Type {
	case "content_block_delta":
		if event.Delta.Type == "text_delta" {
			if event.Delta.Text == "" {
				return "", fmt.Errorf("skip token")
			}
			return event.Delta.Text, nil
		}
		return "", fmt.Errorf("skip token")
	case "message_stop":
		return "", io.EOF
	default:
		return "", fmt.Errorf("skip token")
	}
}

func defaultOption(options map[string]interface{}, key string, fallback int) int {
	if options == nil {
		return fallback
	}
	if value, ok := options[key]; ok {
		switch typed := value.(type) {
		case int:
			return typed
		case int64:
			return int(typed)
		case float64:
			return int(typed)
		}
	}
	return fallback
}

func anthropicContent(content interface{}, options map[string]interface{}) []map[string]interface{} {
	text := ""
	switch value := content.(type) {
	case string:
		text = value
	default:
		text = fmt.Sprint(value)
	}
	part := map[string]interface{}{
		"type": "text",
		"text": text,
	}
	if caching, ok := options["enable_caching"].(bool); ok && caching {
		part["cache_control"] = map[string]string{"type": "ephemeral"}
	}
	return []map[string]interface{}{part}
}

func shouldSkipAnthropicOption(key string) bool {
	switch key {
	case "system_prompt", "max_tokens", "tools", "tool_choice", "enable_caching", "structured_messages":
		return true
	default:
		return false
	}
}

func splitSystemPrompt(prompt string, parts int) []string {
	if parts <= 1 || prompt == "" {
		return []string{prompt}
	}
	words := strings.Fields(prompt)
	if len(words) <= 1 {
		return []string{prompt}
	}
	size := (len(words) + parts - 1) / parts
	result := make([]string, 0, parts)
	for i := 0; i < len(words); i += size {
		end := i + size
		if end > len(words) {
			end = len(words)
		}
		result = append(result, strings.Join(words[i:end], " "))
	}
	return result
}
