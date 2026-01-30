// Package adapter provides Cohere adaptor implementation.
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
)

// CohereAdaptor converts requests and responses for Cohere's chat API.
type CohereAdaptor struct {
	BaseURL string
}

// GetRequestURL returns the Cohere endpoint for chat mode.
func (a *CohereAdaptor) GetRequestURL(mode string, config *ProviderConfig) (string, error) {
	if mode != ModeChat {
		return "", fmt.Errorf("unsupported mode for cohere adaptor: %s", mode)
	}
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		base = "https://api.cohere.ai/v2/chat"
	}
	return base, nil
}

// SetupHeaders sets Cohere-specific headers.
func (a *CohereAdaptor) SetupHeaders(req *http.Request, config *ProviderConfig, mode string) error {
	if config.AuthHeader != "" {
		req.Header.Set(config.AuthHeader, config.AuthPrefix+config.APIKey)
	} else if config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")
	return nil
}

// ConvertChatRequest converts a chat request to Cohere format.
func (a *CohereAdaptor) ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	payload := map[string]interface{}{
		"model": request.Model,
	}
	if len(request.Messages) > 0 {
		payload["messages"] = toSimpleMessages(request.Messages)
	} else {
		payload["messages"] = []map[string]interface{}{{"role": "user", "content": request.Prompt}}
	}

	if request.Schema != nil {
		payload["response_format"] = map[string]interface{}{
			"type":        "json_object",
			"json_schema": request.Schema,
		}
	}

	for key, value := range request.Options {
		if shouldSkipCohereOption(key) {
			continue
		}
		payload[key] = value
	}

	return json.Marshal(payload)
}

// ConvertChatResponse converts Cohere responses to the standardized format.
func (a *CohereAdaptor) ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error) {
	var response struct {
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			ToolCalls []struct {
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	if len(response.Message.Content) == 0 {
		return nil, fmt.Errorf("empty response from cohere")
	}

	var final strings.Builder
	for _, content := range response.Message.Content {
		if content.Type == "text" {
			final.WriteString(content.Text)
		}
	}

	for _, toolCall := range response.Message.ToolCalls {
		var args interface{}
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			return nil, fmt.Errorf("error parsing function arguments: %w", err)
		}
		call, err := utils.FormatFunctionCall(toolCall.Function.Name, args)
		if err != nil {
			return nil, fmt.Errorf("error formatting function call: %w", err)
		}
		if final.Len() > 0 {
			final.WriteString("\n")
		}
		final.WriteString(call)
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

// ConvertImageRequest returns an error because Cohere does not support images.
func (a *CohereAdaptor) ConvertImageRequest(ctx context.Context, config *ProviderConfig, request *dto.ImageRequest) ([]byte, error) {
	return nil, fmt.Errorf("image mode not supported for cohere adaptor")
}

// ConvertImageResponse returns an error because Cohere does not support images.
func (a *CohereAdaptor) ConvertImageResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ImageResponse, error) {
	return nil, fmt.Errorf("image mode not supported for cohere adaptor")
}

// ConvertVideoRequest returns an error because Cohere does not support video.
func (a *CohereAdaptor) ConvertVideoRequest(ctx context.Context, config *ProviderConfig, request *dto.VideoRequest) ([]byte, error) {
	return nil, fmt.Errorf("video mode not supported for cohere adaptor")
}

// ConvertVideoResponse returns an error because Cohere does not support video.
func (a *CohereAdaptor) ConvertVideoResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.VideoResponse, error) {
	return nil, fmt.Errorf("video mode not supported for cohere adaptor")
}

// PrepareStreamRequest creates a streaming chat request body.
func (a *CohereAdaptor) PrepareStreamRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	streamRequest := *request
	streamRequest.Options = copyOptions(streamRequest.Options)
	streamRequest.Options["stream"] = true
	return a.ConvertChatRequest(ctx, config, &streamRequest)
}

// ParseStreamResponse parses a single chunk from a streaming response.
func (a *CohereAdaptor) ParseStreamResponse(chunk []byte) (string, error) {
	var response struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(chunk, &response); err != nil {
		return "", err
	}
	return response.Text, nil
}

func toSimpleMessages(messages []dto.Message) []map[string]interface{} {
	converted := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		converted = append(converted, map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}
	return converted
}

func shouldSkipCohereOption(key string) bool {
	switch key {
	case "system_prompt", "structured_messages":
		return true
	default:
		return false
	}
}
