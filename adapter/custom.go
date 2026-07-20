// Package adapter provides a generic full-endpoint JSON adaptor.
package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/YspCoder/omnigo/dto"
)

// CustomAdaptor calls third-party JSON APIs using complete endpoint URLs.
type CustomAdaptor struct{}

type customAPIError struct {
	Code    interface{} `json:"code"`
	Message string      `json:"message"`
}

type customAPIResponse struct {
	ID        string          `json:"id"`
	TaskID    string          `json:"task_id"`
	RequestID string          `json:"request_id"`
	Status    string          `json:"status"`
	State     string          `json:"state"`
	URL       string          `json:"url"`
	VideoURL  string          `json:"video_url"`
	Code      interface{}     `json:"code"`
	ErrorCode interface{}     `json:"error_code"`
	Message   string          `json:"message"`
	Error     *customAPIError `json:"error"`
}

func (a *CustomAdaptor) Chat(context.Context, *ProviderConfig, *dto.MediaRequest) (*dto.MediaResponse, error) {
	return nil, fmt.Errorf("custom adaptor does not support chat")
}

func (a *CustomAdaptor) Stream(context.Context, *ProviderConfig, *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("custom adaptor does not support streaming")
}

func (a *CustomAdaptor) Media(ctx context.Context, cfg *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	endpoint, err := customAbsoluteEndpoint(customBaseURL(cfg))
	if err != nil {
		return nil, err
	}
	payload, err := customPayload(cfg, request)
	if err != nil {
		return nil, err
	}

	var out customAPIResponse
	if err := a.doJSON(ctx, cfg, http.MethodPost, endpoint.String(), payload, &out); err != nil {
		return nil, err
	}

	videoURL := firstNonEmptyString(out.VideoURL, out.URL)
	resp := &dto.MediaResponse{
		ID:           out.ID,
		RequestID:    out.RequestID,
		TaskID:       firstNonEmptyString(out.TaskID, out.ID),
		Status:       firstNonEmptyString(out.Status, out.State),
		URL:          videoURL,
		ErrorCode:    out.errorCode(),
		ErrorMessage: out.errorMessage(),
	}
	resp.Video.URL = videoURL
	return resp, nil
}

func (a *CustomAdaptor) TaskStatus(ctx context.Context, cfg *ProviderConfig, taskID string, query ...map[string]string) (*dto.TaskStatusResponse, error) {
	endpoint, err := customTaskEndpoint(cfg, taskID, firstCustomQuery(query))
	if err != nil {
		return nil, err
	}

	var out customAPIResponse
	if err := a.doJSON(ctx, cfg, http.MethodGet, endpoint, nil, &out); err != nil {
		return nil, err
	}

	videoURL := firstNonEmptyString(out.VideoURL, out.URL)
	return &dto.TaskStatusResponse{
		RequestID: out.RequestID,
		Output: dto.TaskStatusOutput{
			TaskID:     firstNonEmptyString(out.TaskID, out.ID, taskID),
			TaskStatus: firstNonEmptyString(out.Status, out.State),
			URL:        videoURL,
			VideoURL:   videoURL,
			Code:       out.errorCode(),
			Message:    out.errorMessage(),
		},
	}, nil
}

func (a *CustomAdaptor) ListTasks(context.Context, *ProviderConfig, map[string]string) (*dto.TaskListResponse, error) {
	return nil, fmt.Errorf("custom adaptor does not support task lists")
}

func (a *CustomAdaptor) StreamMedia(context.Context, *ProviderConfig, *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("custom adaptor does not support streaming media")
}

func (a *CustomAdaptor) doJSON(ctx context.Context, cfg *ProviderConfig, method, endpoint string, payload interface{}, out interface{}) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode custom API request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create custom API request: %w", err)
	}
	if cfg != nil && cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg != nil {
		for key, value := range cfg.Headers {
			req.Header.Set(key, value)
		}
	}

	resp, err := customHTTPClient(cfg).Do(req)
	if err != nil {
		return fmt.Errorf("call custom API: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read custom API response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return customResponseError(resp.StatusCode, raw)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode custom API response: %w", err)
	}
	return nil
}

func customPayload(cfg *ProviderConfig, request *dto.MediaRequest) (map[string]interface{}, error) {
	if request == nil {
		return nil, fmt.Errorf("custom media request is required")
	}

	payload := make(map[string]interface{}, len(request.Extra)+8)
	model := request.Model
	if model == "" && cfg != nil {
		model = cfg.Model
	}
	customSetString(payload, "model", model)
	customSetString(payload, "prompt", customPrompt(request))
	customSetString(payload, "size", request.Size)
	customSetString(payload, "resolution", request.Resolution)
	customSetString(payload, "response_format", request.ResponseFormat)
	customSetInt(payload, "duration", request.Duration)
	customSetInt(payload, "fps", request.Fps)
	customSetInt(payload, "seed", request.Seed)
	customSetInt(payload, "n", request.N)

	for key, value := range request.Extra {
		payload[key] = value
	}
	return payload, nil
}

func customPrompt(request *dto.MediaRequest) string {
	if request == nil || request.Prompt != "" {
		if request == nil {
			return ""
		}
		return request.Prompt
	}

	parts := make([]string, 0, len(request.Messages))
	for _, message := range request.Messages {
		if message.Role == "system" || message.Content == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(message.Content))
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func customTaskEndpoint(cfg *ProviderConfig, taskID string, query map[string]string) (string, error) {
	if strings.TrimSpace(taskID) == "" {
		return "", fmt.Errorf("custom task ID is required")
	}

	rawEndpoint := customBaseURL(cfg)
	if endpoint := strings.TrimSpace(query["endpoint"]); endpoint != "" {
		rawEndpoint = endpoint
	}
	escapedTaskID := url.PathEscape(taskID)
	hasTaskTemplate := strings.Contains(rawEndpoint, "{task_id}")
	if hasTaskTemplate {
		rawEndpoint = strings.ReplaceAll(rawEndpoint, "{task_id}", escapedTaskID)
	}

	endpoint, err := customAbsoluteEndpoint(rawEndpoint)
	if err != nil {
		return "", err
	}
	if !hasTaskTemplate {
		basePath := strings.TrimRight(endpoint.Path, "/")
		baseEscapedPath := strings.TrimRight(endpoint.EscapedPath(), "/")
		endpoint.Path = basePath + "/" + taskID
		endpoint.RawPath = baseEscapedPath + "/" + escapedTaskID
	}

	values := endpoint.Query()
	for key, value := range query {
		if key != "endpoint" {
			values.Set(key, value)
		}
	}
	endpoint.RawQuery = values.Encode()
	return endpoint.String(), nil
}

func customAbsoluteEndpoint(raw string) (*url.URL, error) {
	endpoint, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, fmt.Errorf("custom endpoint must be an absolute http(s) URL: %q", raw)
	}
	return endpoint, nil
}

func customBaseURL(cfg *ProviderConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.BaseURL
}

func customHTTPClient(cfg *ProviderConfig) *http.Client {
	if cfg != nil && cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}
	client := &http.Client{}
	if cfg != nil && cfg.Timeout > 0 {
		client.Timeout = cfg.Timeout
	}
	return client
}

func customResponseError(statusCode int, raw []byte) error {
	var parsed customAPIResponse
	if json.Unmarshal(raw, &parsed) == nil {
		if code, message := parsed.errorCode(), parsed.errorMessage(); code != "" || message != "" {
			return fmt.Errorf("custom API error: status=%d code=%s message=%s", statusCode, code, message)
		}
	}
	return fmt.Errorf("custom API error: status=%d body=%s", statusCode, strings.TrimSpace(string(raw)))
}

func (r customAPIResponse) errorCode() string {
	if value := customString(r.ErrorCode); value != "" {
		return value
	}
	if r.Error != nil {
		if value := customString(r.Error.Code); value != "" {
			return value
		}
	}
	return customString(r.Code)
}

func (r customAPIResponse) errorMessage() string {
	if r.Error != nil && r.Error.Message != "" {
		return r.Error.Message
	}
	return r.Message
}

func customString(value interface{}) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func customSetString(payload map[string]interface{}, key, value string) {
	if value != "" {
		payload[key] = value
	}
}

func customSetInt(payload map[string]interface{}, key string, value int) {
	if value != 0 {
		payload[key] = value
	}
}

func firstCustomQuery(query []map[string]string) map[string]string {
	if len(query) == 0 || query[0] == nil {
		return map[string]string{}
	}
	return query[0]
}
