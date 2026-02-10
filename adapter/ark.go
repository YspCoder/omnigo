// Package adapter provides Volcengine Ark (火山方舟) adaptor implementation.
package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/YspCoder/omnigo/dto"
)

// ArkAdaptor converts requests and responses for Volcengine Ark APIs.
type ArkAdaptor struct {
	BaseURL string
}

// GetURL returns the Ark endpoint for the given mode and optional taskID.
func (a *ArkAdaptor) GetURL(mode string, config *ProviderConfig, taskID string) (string, error) {
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		// Default to beijing region if not specified
		base = "https://ark.cn-beijing.volces.com/api/v3"
	}

	switch mode {
	case ModeChat:
		return base + "/chat/completions", nil
	case ModeImage:
		return base + "/image/generations", nil
	case ModeVideo:
		return base + "/video/generations", nil
	case ModeTask:
		if taskID == "" {
			return "", errors.New("task_id is required for task query")
		}
		return base + "/tasks/" + taskID, nil
	default:
		return "", fmt.Errorf("unsupported mode for Ark: %s", mode)
	}
}

// SetupHeaders sets Ark headers.
func (a *ArkAdaptor) SetupHeaders(req *http.Request, config *ProviderConfig, mode string, body []byte) error {
	if config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")
	return nil
}

// ConvertChatRequest handles chat completion requests (OpenAI compatible).
func (a *ArkAdaptor) ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	return (&OpenAIAdaptor{}).ConvertChatRequest(ctx, config, request)
}

// ConvertChatResponse handles chat completion responses (OpenAI compatible).
func (a *ArkAdaptor) ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error) {
	return (&OpenAIAdaptor{}).ConvertChatResponse(ctx, config, body)
}

// ConvertMediaRequest handles image/video generation requests.
func (a *ArkAdaptor) ConvertMediaRequest(ctx context.Context, config *ProviderConfig, mode string, request *dto.MediaRequest) ([]byte, error) {
	switch mode {
	case ModeImage:
		payload := map[string]interface{}{
			"model":  request.Model,
			"prompt": request.Prompt,
		}
		if request.Size != "" {
			payload["size"] = request.Size
		}
		if request.N > 0 {
			payload["n"] = request.N
		}
		for k, v := range request.Extra {
			payload[k] = v
		}
		return json.Marshal(payload)
	case ModeVideo:
		payload := map[string]interface{}{
			"model":  request.Model,
			"prompt": request.Prompt,
		}
		if request.Size != "" {
			payload["size"] = request.Size
		}
		for k, v := range request.Extra {
			payload[k] = v
		}
		return json.Marshal(payload)
	default:
		return nil, fmt.Errorf("unsupported media mode for Ark: %s", mode)
	}
}

// ConvertMediaResponse handles image/video generation responses.
func (a *ArkAdaptor) ConvertMediaResponse(ctx context.Context, config *ProviderConfig, mode string, body []byte) (*dto.MediaResponse, error) {
	var response struct {
		ID      string `json:"id"`
		TaskID  string `json:"task_id"`
		Created int64  `json:"created"`
		Data    []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, &dto.LLMError{
			Code:     http.StatusBadRequest,
			Message:  response.Error.Message,
			Provider: "ark",
		}
	}

	res := &dto.MediaResponse{
		Created: response.Created,
		TaskID:  response.TaskID,
	}

	if res.TaskID == "" {
		res.TaskID = response.ID
	}

	for _, item := range response.Data {
		res.Data = append(res.Data, dto.ImageData{
			URL:     item.URL,
			B64JSON: item.B64JSON,
		})
	}

	if len(res.Data) > 0 {
		res.URL = res.Data[0].URL
	}

	return res, nil
}

// ConvertTaskStatusResponse handles the async task query response.
func (a *ArkAdaptor) ConvertTaskStatusResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.TaskStatusResponse, error) {
	var response struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Result struct {
			Video struct {
				URL string `json:"url"`
			} `json:"video"`
			Images []struct {
				URL string `json:"url"`
			} `json:"images"`
		} `json:"result"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	res := &dto.TaskStatusResponse{
		Output: dto.TaskStatusOutput{
			TaskID:     response.ID,
			TaskStatus: response.Status,
		},
	}

	if response.Error != nil {
		res.Output.Code = response.Error.Code
		res.Output.Message = response.Error.Message
	}

	if response.Result.Video.URL != "" {
		res.Output.VideoURL = response.Result.Video.URL
	} else if len(response.Result.Images) > 0 {
		// Just take the first image URL if it's an image task
		res.Output.VideoURL = response.Result.Images[0].URL
	}

	return res, nil
}

// PrepareStreamRequest handles streaming setup (OpenAI compatible).
func (a *ArkAdaptor) PrepareStreamRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	return (&OpenAIAdaptor{}).PrepareStreamRequest(ctx, config, request)
}

// ParseStreamResponse handles streaming chunks (OpenAI compatible).
func (a *ArkAdaptor) ParseStreamResponse(chunk []byte) (string, error) {
	return (&OpenAIAdaptor{}).ParseStreamResponse(chunk)
}
