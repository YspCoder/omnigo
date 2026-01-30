// Package adapter provides Alibaba DashScope adaptor implementation.
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/YspCoder/omnigo/dto"
)

// AliAdaptor converts requests and responses for DashScope APIs.
type AliAdaptor struct {
	BaseURL string
}

// GetRequestURL returns the DashScope endpoint for the given mode.
func (a *AliAdaptor) GetRequestURL(mode string, config *ProviderConfig) (string, error) {
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		base = "https://dashscope.aliyuncs.com"
	}

	switch mode {
	case ModeChat:
		return base + "/api/v1/services/aigc/text-generation/generation", nil
	case ModeVideo:
		return base + "/api/v1/services/aigc/video-generation/generation", nil
	case ModeImage:
		return "", fmt.Errorf("image mode not supported for ali adaptor")
	default:
		return "", fmt.Errorf("unsupported mode: %s", mode)
	}
}

// SetupHeaders sets DashScope headers.
func (a *AliAdaptor) SetupHeaders(req *http.Request, config *ProviderConfig, mode string) error {
	if config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")
	if mode == ModeVideo {
		req.Header.Set("X-DashScope-Async", "enable")
	}
	return nil
}

// ConvertChatRequest converts a chat request to DashScope format.
func (a *AliAdaptor) ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	payload := struct {
		Model string `json:"model"`
		Input struct {
			Messages []dto.Message `json:"messages"`
		} `json:"input"`
		Parameters map[string]interface{} `json:"parameters,omitempty"`
	}{
		Model: request.Model,
	}
	payload.Input.Messages = request.Messages

	if request.Stream {
		payload.Parameters = map[string]interface{}{"incremental_output": true}
	}

	return json.Marshal(payload)
}

// ConvertChatResponse converts a DashScope chat response to the standardized format.
func (a *AliAdaptor) ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error) {
	var response struct {
		Output struct {
			Text    string `json:"text"`
			Choices []struct {
				Message dto.Message `json:"message"`
			} `json:"choices"`
		} `json:"output"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if response.Code != "" {
		return nil, &dto.LLMError{
			Code:     http.StatusBadRequest,
			Message:  response.Message,
			Provider: config.Name,
		}
	}

	chatResponse := &dto.ChatResponse{}
	if len(response.Output.Choices) > 0 {
		chatResponse.Choices = []dto.ChatChoice{{
			Index:   0,
			Message: response.Output.Choices[0].Message,
		}}
	} else if response.Output.Text != "" {
		chatResponse.Choices = []dto.ChatChoice{{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: response.Output.Text,
			},
		}}
	}
	chatResponse.Usage = dto.Usage{
		PromptTokens:     response.Usage.InputTokens,
		CompletionTokens: response.Usage.OutputTokens,
		TotalTokens:      response.Usage.TotalTokens,
	}
	return chatResponse, nil
}

// ConvertImageRequest returns an error because DashScope image is not implemented.
func (a *AliAdaptor) ConvertImageRequest(ctx context.Context, config *ProviderConfig, request *dto.ImageRequest) ([]byte, error) {
	return nil, fmt.Errorf("image mode not supported for ali adaptor")
}

// ConvertImageResponse returns an error because DashScope image is not implemented.
func (a *AliAdaptor) ConvertImageResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ImageResponse, error) {
	return nil, fmt.Errorf("image mode not supported for ali adaptor")
}

// ConvertVideoRequest converts a video request to DashScope format.
func (a *AliAdaptor) ConvertVideoRequest(ctx context.Context, config *ProviderConfig, request *dto.VideoRequest) ([]byte, error) {
	payload := struct {
		Model string `json:"model"`
		Input struct {
			Prompt string `json:"prompt"`
		} `json:"input"`
		Parameters map[string]interface{} `json:"parameters,omitempty"`
	}{
		Model: request.Model,
	}
	payload.Input.Prompt = request.Prompt
	params := map[string]interface{}{}
	if request.Size != "" {
		params["size"] = request.Size
	}
	if request.Duration != 0 {
		params["duration"] = request.Duration
	}
	if request.Fps != 0 {
		params["fps"] = request.Fps
	}
	if request.Seed != 0 {
		params["seed"] = request.Seed
	}
	if len(params) > 0 {
		payload.Parameters = params
	}

	return json.Marshal(payload)
}

// ConvertVideoResponse converts a DashScope video response to the standardized format.
func (a *AliAdaptor) ConvertVideoResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.VideoResponse, error) {
	var response struct {
		Output struct {
			TaskID     string `json:"task_id"`
			TaskStatus string `json:"task_status"`
			VideoURL   string `json:"video_url"`
		} `json:"output"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if response.Code != "" {
		return nil, &dto.LLMError{
			Code:     http.StatusBadRequest,
			Message:  response.Message,
			Provider: config.Name,
		}
	}

	videoResponse := &dto.VideoResponse{
		ID:     response.Output.TaskID,
		Status: strings.ToLower(response.Output.TaskStatus),
	}
	if response.Output.VideoURL != "" {
		videoResponse.Video.URL = response.Output.VideoURL
	}
	return videoResponse, nil
}
