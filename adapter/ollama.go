// Package adapter provides Ollama adaptor implementation.
package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/YspCoder/omnigo/dto"
)

// OllamaAdaptor converts requests and responses for Ollama's API.
type OllamaAdaptor struct {
	BaseURL string
}

// GetRequestURL returns the Ollama endpoint for chat mode.
func (a *OllamaAdaptor) GetRequestURL(mode string, config *ProviderConfig) (string, error) {
	if mode != ModeChat {
		return "", fmt.Errorf("unsupported mode for ollama adaptor: %s", mode)
	}
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		base = "http://localhost:11434/api/generate"
	}
	return base, nil
}

// SetupHeaders sets Ollama-specific headers.
func (a *OllamaAdaptor) SetupHeaders(req *http.Request, config *ProviderConfig, mode string) error {
	req.Header.Set("Content-Type", "application/json")
	return nil
}

// ConvertChatRequest converts a chat request to Ollama format.
func (a *OllamaAdaptor) ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	payload := map[string]interface{}{
		"model":  request.Model,
		"prompt": request.Prompt,
	}
	if request.Prompt == "" && len(request.Messages) > 0 {
		payload["prompt"] = flattenMessages(request.Messages)
	}

	for key, value := range request.Options {
		payload[key] = value
	}

	return json.Marshal(payload)
}

// ConvertChatResponse converts Ollama responses to the standardized format.
func (a *OllamaAdaptor) ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error) {
	var full strings.Builder
	decoder := json.NewDecoder(bytes.NewReader(body))
	for decoder.More() {
		var response struct {
			Response string `json:"response"`
			Done     bool   `json:"done"`
		}
		if err := decoder.Decode(&response); err != nil {
			return nil, fmt.Errorf("error parsing ollama response: %w", err)
		}
		full.WriteString(response.Response)
		if response.Done {
			break
		}
	}

	return &dto.ChatResponse{
		Choices: []dto.ChatChoice{{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: full.String(),
			},
		}},
	}, nil
}

// ConvertImageRequest returns an error because Ollama does not support images.
func (a *OllamaAdaptor) ConvertImageRequest(ctx context.Context, config *ProviderConfig, request *dto.ImageRequest) ([]byte, error) {
	return nil, fmt.Errorf("image mode not supported for ollama adaptor")
}

// ConvertImageResponse returns an error because Ollama does not support images.
func (a *OllamaAdaptor) ConvertImageResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ImageResponse, error) {
	return nil, fmt.Errorf("image mode not supported for ollama adaptor")
}

// ConvertVideoRequest returns an error because Ollama does not support video.
func (a *OllamaAdaptor) ConvertVideoRequest(ctx context.Context, config *ProviderConfig, request *dto.VideoRequest) ([]byte, error) {
	return nil, fmt.Errorf("video mode not supported for ollama adaptor")
}

// ConvertVideoResponse returns an error because Ollama does not support video.
func (a *OllamaAdaptor) ConvertVideoResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.VideoResponse, error) {
	return nil, fmt.Errorf("video mode not supported for ollama adaptor")
}

// PrepareStreamRequest creates a streaming chat request body.
func (a *OllamaAdaptor) PrepareStreamRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	streamRequest := *request
	if streamRequest.Options == nil {
		streamRequest.Options = make(map[string]interface{})
	}
	streamRequest.Options["stream"] = true
	return a.ConvertChatRequest(ctx, config, &streamRequest)
}

// ParseStreamResponse parses a single chunk from a streaming response.
func (a *OllamaAdaptor) ParseStreamResponse(chunk []byte) (string, error) {
	var response struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}
	if err := json.Unmarshal(chunk, &response); err != nil {
		return "", err
	}
	return response.Response, nil
}

func flattenMessages(messages []dto.Message) string {
	var builder strings.Builder
	for _, msg := range messages {
		builder.WriteString(msg.Role)
		builder.WriteString(": ")
		builder.WriteString(fmt.Sprint(msg.Content))
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}
