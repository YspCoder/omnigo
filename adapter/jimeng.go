// Package adapter provides Volcengine Jimeng adaptor implementation.
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/YspCoder/omnigo/dto"
)

// JimengSubmitTaskRequest represents the request body for Jimeng task submission.
type JimengSubmitTaskRequest struct {
	ReqKey           string   `json:"req_key"`
	Prompt           string   `json:"prompt,omitempty"`
	BinaryDataBase64 []string `json:"binary_data_base64,omitempty"`
	ImageURLs        []string `json:"image_urls,omitempty"`
	Seed             int      `json:"seed,omitempty"`
	Frames           int      `json:"frames,omitempty"`
	AspectRatio      string   `json:"aspect_ratio,omitempty"`
}

// JimengSubmitTaskResponse represents the response body for Jimeng task submission.
type JimengSubmitTaskResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		TaskID string `json:"task_id"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

// JimengGetResultRequest represents the request body for Jimeng task result query.
type JimengGetResultRequest struct {
	ReqKey  string `json:"req_key"`
	TaskID  string `json:"task_id"`
	ReqJSON string `json:"req_json,omitempty"`
}

// JimengGetResultResponse represents the response body for Jimeng task result query.
type JimengGetResultResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Status         string `json:"status"`
		VideoURL       string `json:"video_url"`
		AIGCMetaTagged bool   `json:"aigc_meta_tagged"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

// JimengAdaptor converts requests and responses for Jimeng APIs.
type JimengAdaptor struct {
	BaseURL string
}

// GetRequestURL returns the Jimeng endpoint for the given mode.
func (a *JimengAdaptor) GetRequestURL(mode string, config *ProviderConfig) (string, error) {
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		base = "https://visual.volcengineapi.com"
	}

	switch mode {
	case ModeVideo:
		return base + "?Action=CVSync2AsyncSubmitTask&Version=2022-08-31", nil
	default:
		return "", fmt.Errorf("unsupported mode for Jimeng: %s", mode)
	}
}

// SetupHeaders sets Jimeng headers.
func (a *JimengAdaptor) SetupHeaders(req *http.Request, config *ProviderConfig, mode string) error {
	if config.APIKey != "" {
		// Note: Volcengine usually requires complex signing.
		// For now, we assume a simplified API key or a gateway that handles signing.
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")
	return nil
}

// ConvertChatRequest is not supported for Jimeng video generation.
func (a *JimengAdaptor) ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	return nil, fmt.Errorf("chat mode not supported for Jimeng")
}

// ConvertChatResponse is not supported for Jimeng video generation.
func (a *JimengAdaptor) ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error) {
	return nil, fmt.Errorf("chat mode not supported for Jimeng")
}

// ConvertMediaRequest converts a media request to Jimeng format.
func (a *JimengAdaptor) ConvertMediaRequest(ctx context.Context, config *ProviderConfig, mode string, request *dto.MediaRequest) ([]byte, error) {
	if mode != ModeVideo {
		return nil, fmt.Errorf("unsupported media mode for Jimeng: %s", mode)
	}

	reqKey := getStringExtra(request.Extra, "req_key")
	if reqKey == "" {
		reqKey = config.Model
	}
	if reqKey == "" {
		reqKey = "jimeng_ti2v_v30_pro"
	}

	payload := JimengSubmitTaskRequest{
		ReqKey:      reqKey,
		Prompt:      request.Prompt,
		Seed:        request.Seed,
		AspectRatio: request.Size, // Map size to aspect_ratio if it fits Jimeng's options
	}

	if payload.Seed == 0 {
		payload.Seed = -1
	}

	if frames, ok := request.Extra["frames"].(float64); ok {
		payload.Frames = int(frames)
	} else if frames, ok := request.Extra["frames"].(int); ok {
		payload.Frames = frames
	}

	if imgURL := getStringExtra(request.Extra, "image_url"); imgURL != "" {
		payload.ImageURLs = []string{imgURL}
	} else if len(request.Extra["image_urls"].([]string)) > 0 {
		payload.ImageURLs = request.Extra["image_urls"].([]string)
	}

	// Override with raw payload if provided
	if rawPayload := extractPayloadMap(request.Extra); rawPayload != nil {
		return json.Marshal(rawPayload)
	}

	return json.Marshal(payload)
}

// ConvertMediaResponse converts a Jimeng media response to the standardized format.
func (a *JimengAdaptor) ConvertMediaResponse(ctx context.Context, config *ProviderConfig, mode string, body []byte) (*dto.MediaResponse, error) {
	if mode != ModeVideo {
		return nil, fmt.Errorf("unsupported media mode for Jimeng: %s", mode)
	}

	var response JimengSubmitTaskResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	if response.Code != 10000 {
		return nil, &dto.LLMError{
			Code:     http.StatusBadRequest,
			Message:  response.Message,
			Provider: config.Name,
		}
	}

	return &dto.MediaResponse{
		TaskID:    response.Data.TaskID,
		Status:    "submitted",
		RequestID: response.RequestID,
	}, nil
}

// GetTaskStatusURL returns the task status endpoint for Jimeng.
func (a *JimengAdaptor) GetTaskStatusURL(taskID string, config *ProviderConfig) (string, error) {
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		base = "https://visual.volcengineapi.com"
	}
	return base + "?Action=CVSync2AsyncGetResult&Version=2022-08-31", nil
}

// PrepareTaskStatusRequest creates a POST request for Jimeng task status.
func (a *JimengAdaptor) PrepareTaskStatusRequest(ctx context.Context, config *ProviderConfig, taskID string) (string, []byte, error) {
	reqKey := config.Model
	if reqKey == "" {
		reqKey = "jimeng_ti2v_v30_pro" // Default fallback
	}
	payload := JimengGetResultRequest{
		ReqKey: reqKey,
		TaskID: taskID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	return http.MethodPost, body, nil
}

// ConvertTaskStatusResponse converts a Jimeng task status response to the standardized format.
func (a *JimengAdaptor) ConvertTaskStatusResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.TaskStatusResponse, error) {
	var response JimengGetResultResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	if response.Code != 10000 {
		return nil, &dto.LLMError{
			Code:     http.StatusBadRequest,
			Message:  response.Message,
			Provider: config.Name,
		}
	}

	result := &dto.TaskStatusResponse{
		RequestID: response.RequestID,
		Output: dto.TaskStatusOutput{
			TaskID:     "", // TaskID not returned in query body but we have it from context if needed
			TaskStatus: response.Data.Status,
			VideoURL:   response.Data.VideoURL,
		},
	}

	return result, nil
}

// PrepareStreamRequest is not supported for Jimeng.
func (a *JimengAdaptor) PrepareStreamRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	return nil, fmt.Errorf("streaming not supported for Jimeng")
}

// ParseStreamResponse is not supported for Jimeng.
func (a *JimengAdaptor) ParseStreamResponse(chunk []byte) (string, error) {
	return "", fmt.Errorf("streaming not supported for Jimeng")
}
