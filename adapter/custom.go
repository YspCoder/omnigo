// Package adapter provides a generic full-endpoint JSON adaptor.
package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
)

// CustomAdaptor calls third-party JSON APIs using complete endpoint URLs.
type CustomAdaptor struct{}

type customAPIError struct {
	Code    interface{} `json:"code"`
	Message string      `json:"message"`
}

type customAPIData struct {
	Images   []dto.ImageData
	Response *customAPIResponse
}

func (d *customAPIData) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}

	switch trimmed[0] {
	case '[':
		return json.Unmarshal(trimmed, &d.Images)
	case '{':
		return json.Unmarshal(trimmed, &d.Response)
	default:
		return fmt.Errorf("custom API data must be an array or object")
	}
}

type customAPIResponse struct {
	ID         interface{}     `json:"id"`
	TaskID     string          `json:"task_id"`
	RequestID  string          `json:"request_id"`
	Object     string          `json:"object"`
	Model      string          `json:"model"`
	Status     string          `json:"status"`
	State      string          `json:"state"`
	URL        string          `json:"url"`
	VideoURL   string          `json:"video_url"`
	ResultURL  string          `json:"result_url"`
	Data       customAPIData   `json:"data"`
	Code       interface{}     `json:"code"`
	ErrorCode  interface{}     `json:"error_code"`
	Message    string          `json:"message"`
	FailReason string          `json:"fail_reason"`
	Error      *customAPIError `json:"error"`
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
	if request.Type == dto.MediaTypeImage && customIsImageEditEndpoint(endpoint) {
		err = a.doMultipartImage(ctx, cfg, endpoint.String(), request, payload, &out)
	} else {
		err = a.doJSON(ctx, cfg, http.MethodPost, endpoint.String(), payload, &out)
	}
	if err != nil {
		return nil, err
	}

	result := out.result()
	resultURL := firstNonEmptyString(result.URL, customFirstImageURL(result.Data.Images), result.VideoURL, result.ResultURL)
	resp := &dto.MediaResponse{
		ID:           customString(result.ID),
		Object:       result.Object,
		Model:        result.Model,
		Data:         append([]dto.ImageData(nil), result.Data.Images...),
		RequestID:    firstNonEmptyString(result.RequestID, out.RequestID),
		TaskID:       firstNonEmptyString(result.TaskID, customString(result.ID)),
		Status:       firstNonEmptyString(result.Status, result.State),
		URL:          resultURL,
		ErrorCode:    result.errorCode(),
		ErrorMessage: result.errorMessage(),
	}
	resp.Video.URL = result.VideoURL
	return resp, nil
}

func (a *CustomAdaptor) TaskStatus(ctx context.Context, cfg *ProviderConfig, taskID string, query ...map[string]string) (*dto.TaskStatusResponse, error) {
	taskQuery := firstCustomQuery(query)
	endpoint, err := customTaskEndpoint(cfg, taskID, taskQuery)
	if err != nil {
		return nil, err
	}

	var raw json.RawMessage
	if err := a.doJSON(ctx, cfg, http.MethodGet, endpoint, nil, &raw); err != nil {
		return nil, err
	}

	var out customAPIResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode custom API response: %w", err)
	}

	result := out.result()
	videoURL := result.VideoURL
	if path := strings.TrimSpace(taskQuery["video_url_path"]); path != "" {
		videoURL = customJSONPathString(raw, path)
	}
	resultURL := firstNonEmptyString(result.URL, customFirstImageURL(result.Data.Images), videoURL, result.ResultURL)
	return &dto.TaskStatusResponse{
		RequestID: firstNonEmptyString(result.RequestID, out.RequestID),
		Output: dto.TaskStatusOutput{
			TaskID:     firstNonEmptyString(result.TaskID, customString(result.ID), out.TaskID, customString(out.ID), taskID),
			TaskStatus: firstNonEmptyString(result.Status, result.State),
			URL:        resultURL,
			VideoURL:   videoURL,
			Code:       result.errorCode(),
			Message:    result.errorMessage(),
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
	return a.doRequest(ctx, cfg, method, endpoint, body, "application/json", out)
}

func (a *CustomAdaptor) doMultipartImage(ctx context.Context, cfg *ProviderConfig, endpoint string, request *dto.MediaRequest, payload map[string]interface{}, out interface{}) error {
	inputs := customImageInputs(request.Extra)
	if len(inputs) == 0 {
		return fmt.Errorf("custom image edit requires at least one image")
	}
	if len(inputs) > 9 {
		return fmt.Errorf("custom image edit supports at most 9 images, got %d", len(inputs))
	}

	readers, err := openAIImageReferenceReaders(ctx, customHTTPClient(cfg), inputs)
	if err != nil {
		return err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range payload {
		if customImageInputField(key) || key == "mask" || value == nil {
			continue
		}
		fieldValue, err := customMultipartValue(value)
		if err != nil {
			return fmt.Errorf("encode custom multipart field %q: %w", key, err)
		}
		if err := writer.WriteField(key, fieldValue); err != nil {
			return err
		}
	}
	for i, reader := range readers {
		if err := customWriteMultipartFile(writer, "image", reader, i); err != nil {
			return err
		}
	}
	if masks := utils.ContentImageURLs(request.Extra["mask"]); len(masks) > 0 {
		if len(masks) > 1 {
			return fmt.Errorf("custom image edit supports only one mask")
		}
		mask, err := openAIImageReferenceReader(ctx, customHTTPClient(cfg), masks[0], 0)
		if err != nil {
			return err
		}
		if err := customWriteMultipartFile(writer, "mask", mask, 0); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return a.doRequest(ctx, cfg, http.MethodPost, endpoint, &body, writer.FormDataContentType(), out)
}

func (a *CustomAdaptor) doRequest(ctx context.Context, cfg *ProviderConfig, method, endpoint string, body io.Reader, contentType string, out interface{}) error {

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create custom API request: %w", err)
	}
	if cfg != nil && cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
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
	customSetInt(payload, "n", request.N)
	if request.Type == dto.MediaTypeImage {
		customSetString(payload, "output_resolution", request.Resolution)
	} else {
		customSetString(payload, "resolution", request.Resolution)
		customSetString(payload, "response_format", request.ResponseFormat)
		customSetInt(payload, "duration", request.Duration)
		customSetInt(payload, "fps", request.Fps)
		customSetInt(payload, "seed", request.Seed)
	}

	for key, value := range request.Extra {
		payload[key] = value
	}
	if request.Type == dto.MediaTypeImage {
		payload["async"] = true
		if n, ok := utils.GetIntExtra(payload, "n"); ok && n > 1 {
			return nil, fmt.Errorf("custom async image requests support only n=1")
		}
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
		if !customTaskControlQuery(key) {
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

func customIsImageEditEndpoint(endpoint *url.URL) bool {
	if endpoint == nil {
		return false
	}
	return strings.HasSuffix(strings.ToLower(strings.TrimRight(endpoint.Path, "/")), "/images/edits")
}

func customImageInputs(extra map[string]interface{}) []string {
	keys := []string{"image", "images", "imageUrls", "image_urls", "reference_images", "referenceImages", "image_refs"}
	seen := make(map[string]struct{})
	var inputs []string
	for _, key := range keys {
		for _, input := range utils.ContentImageURLs(extra[key]) {
			input = strings.TrimSpace(input)
			if input == "" {
				continue
			}
			if _, exists := seen[input]; exists {
				continue
			}
			seen[input] = struct{}{}
			inputs = append(inputs, input)
		}
	}
	for _, input := range utils.ParseExtraImageInputs(map[string]interface{}{"files": extra["files"]}) {
		if _, exists := seen[input]; exists {
			continue
		}
		seen[input] = struct{}{}
		inputs = append(inputs, input)
	}
	return inputs
}

func customImageInputField(key string) bool {
	switch key {
	case "image", "images", "imageUrls", "image_urls", "reference_images", "referenceImages", "image_refs", "files":
		return true
	default:
		return false
	}
}

func customMultipartValue(value interface{}) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		return fmt.Sprint(typed), nil
	default:
		raw, err := json.Marshal(value)
		return string(raw), err
	}
}

func customWriteMultipartFile(writer *multipart.Writer, field string, reader io.Reader, index int) error {
	if sized, ok := reader.(interface{ Len() int }); ok && sized.Len() > 10<<20 {
		return fmt.Errorf("custom multipart %s exceeds 10MB", field)
	}
	filename := fmt.Sprintf("%s-%d.png", field, index+1)
	if named, ok := reader.(interface{ Filename() string }); ok && strings.TrimSpace(named.Filename()) != "" {
		filename = named.Filename()
	}
	contentType := "application/octet-stream"
	if typed, ok := reader.(interface{ ContentType() string }); ok && strings.TrimSpace(typed.ContentType()) != "" {
		contentType = typed.ContentType()
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name": field, "filename": filename,
	}))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, reader)
	return err
}

func customFirstImageURL(images []dto.ImageData) string {
	for _, image := range images {
		if strings.TrimSpace(image.URL) != "" {
			return image.URL
		}
	}
	return ""
}

func customJSONPathString(raw json.RawMessage, path string) string {
	current := raw
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return ""
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(current, &object); err != nil {
			return ""
		}
		next, ok := object[segment]
		if !ok {
			return ""
		}
		current = next
	}

	var value string
	if err := json.Unmarshal(current, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func customTaskControlQuery(key string) bool {
	switch key {
	case "endpoint", "video_url_path":
		return true
	default:
		return false
	}
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

func (r *customAPIResponse) result() *customAPIResponse {
	if r.Data.Response != nil && customString(r.ID) == "" && r.TaskID == "" &&
		r.Status == "" && r.State == "" && r.URL == "" && r.VideoURL == "" && r.ResultURL == "" {
		return r.Data.Response
	}
	return r
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
	if r.Data.Response != nil {
		if value := r.Data.Response.errorCode(); value != "" {
			return value
		}
	}
	return customString(r.Code)
}

func (r customAPIResponse) errorMessage() string {
	if r.Error != nil && r.Error.Message != "" {
		return r.Error.Message
	}
	if r.FailReason != "" {
		return r.FailReason
	}
	if r.Data.Response != nil {
		if value := r.Data.Response.errorMessage(); value != "" {
			return value
		}
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
