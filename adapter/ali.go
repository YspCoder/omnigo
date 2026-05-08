// Package adapter provides Alibaba DashScope adaptor implementation.
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
	"time"

	"github.com/YspCoder/omnigo/dto"
)

const (
	aliCompatibleBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	aliNativeBaseURL     = "https://dashscope.aliyuncs.com"

	aliImageSyncEndpoint     = "/api/v1/services/aigc/multimodal-generation/generation"
	aliImageAsyncEndpoint    = "/api/v1/services/aigc/text2image/image-synthesis"
	aliImageNewAsyncEndpoint = "/api/v1/services/aigc/image-generation/generation"
	aliVideoEndpoint         = "/api/v1/services/aigc/video-generation/video-synthesis"
	aliImageToVideoEndpoint  = "/api/v1/services/aigc/image2video/video-synthesis"
	aliTaskEndpoint          = "/api/v1/tasks/"
)

const (
	aliImageModeSync      = "sync-image"
	aliImageModeAsyncV1   = "async-image-v1"
	aliImageModeAsyncV2   = "async-image-v2"
	aliVideoModeText      = "text-to-video"
	aliVideoModeImage     = "image-to-video"
	aliVideoModeKeyframe  = "keyframe-to-video"
	aliVideoModeReference = "reference-to-video"
	aliVideoModeAnimate   = "animate-mix"
	aliVideoModeAvatar    = "speech-to-video"
)

var aliImageModeEndpoint = map[string]string{
	aliImageModeSync:    aliImageSyncEndpoint,
	aliImageModeAsyncV1: aliImageAsyncEndpoint,
	aliImageModeAsyncV2: aliImageNewAsyncEndpoint,
}

var aliVideoModeEndpoint = map[string]string{
	aliVideoModeText:      aliVideoEndpoint,
	aliVideoModeImage:     aliImageToVideoEndpoint,
	aliVideoModeKeyframe:  aliImageToVideoEndpoint,
	aliVideoModeReference: aliVideoEndpoint,
	aliVideoModeAnimate:   aliImageToVideoEndpoint,
	aliVideoModeAvatar:    aliImageToVideoEndpoint,
}

// AliAdaptor converts requests and responses for DashScope APIs.
// Text generation uses the OpenAI-compatible endpoint, while image/video use native endpoints.
type AliAdaptor struct{}

type aliTaskCreateResponse struct {
	RequestID string `json:"request_id"`
	Output    struct {
		TaskID        string `json:"task_id"`
		TaskStatus    string `json:"task_status"`
		SubmitTime    string `json:"submit_time"`
		ScheduledTime string `json:"scheduled_time"`
		EndTime       string `json:"end_time"`
		Code          string `json:"code"`
		Message       string `json:"message"`
	} `json:"output"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type aliTaskStatusEnvelope struct {
	RequestID string `json:"request_id"`
	Output    struct {
		TaskID        string                   `json:"task_id"`
		TaskStatus    string                   `json:"task_status"`
		SubmitTime    string                   `json:"submit_time"`
		ScheduledTime string                   `json:"scheduled_time"`
		EndTime       string                   `json:"end_time"`
		Results       []map[string]interface{} `json:"results"`
		ResultURL     string                   `json:"result_url"`
		VideoURL      string                   `json:"video_url"`
		OrigPrompt    string                   `json:"orig_prompt"`
		ActualPrompt  string                   `json:"actual_prompt"`
		Code          string                   `json:"code"`
		Message       string                   `json:"message"`
	} `json:"output"`
	Usage   *dto.TaskStatusUsage `json:"usage"`
	Code    string               `json:"code"`
	Message string               `json:"message"`
}

type aliTaskListEnvelope struct {
	RequestID string            `json:"request_id"`
	Output    []aliTaskListItem `json:"output"`
	Total     int               `json:"total"`
	PageNum   int               `json:"page_num"`
	PageSize  int               `json:"page_size"`
	HasMore   bool              `json:"has_more"`
	Code      string            `json:"code"`
	Message   string            `json:"message"`
}

type aliTaskListItem struct {
	TaskID     string                 `json:"task_id"`
	TaskStatus string                 `json:"task_status"`
	Model      string                 `json:"model"`
	SubmitTime string                 `json:"submit_time"`
	Payload    map[string]interface{} `json:"payload"`
}

type aliSyncImageResponse struct {
	RequestID string `json:"request_id"`
	Output    struct {
		Choices []struct {
			Message struct {
				Content []struct {
					Image string `json:"image"`
					Text  string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	} `json:"output"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type aliPollingStream struct {
	adaptor *AliAdaptor
	cfg     *ProviderConfig
	taskID  string
	last    string
}

func (a *AliAdaptor) Chat(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	cfg := aliCompatibleConfig(config)
	return (&OpenAIAdaptor{}).Chat(ctx, cfg, request)
}

func (a *AliAdaptor) Stream(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	cfg := aliCompatibleConfig(config)
	return (&OpenAIAdaptor{}).Stream(ctx, cfg, request)
}

func (a *AliAdaptor) Media(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("ali request is required")
	}

	cfg := aliNativeConfig(config)

	switch request.Type {
	case dto.MediaTypeImage:
		endpoint, payload, async, err := a.buildImageRequest(request)
		if err != nil {
			return nil, err
		}
		if async {
			var out aliTaskCreateResponse
			headers := map[string]string{"X-DashScope-Async": "enable"}
			if err := a.doJSON(ctx, cfg, http.MethodPost, endpoint, payload, headers, &out); err != nil {
				return nil, err
			}
			return &dto.MediaResponse{
				RequestID: out.RequestID,
				TaskID:    out.Output.TaskID,
				Status:    out.Output.TaskStatus,
				ErrorCode: firstNonEmptyString(out.Output.Code, out.Code),
				ErrorMessage: firstNonEmptyString(
					out.Output.Message,
					out.Message,
				),
			}, nil
		}

		var out aliSyncImageResponse
		if err := a.doJSON(ctx, cfg, http.MethodPost, endpoint, payload, nil, &out); err != nil {
			return nil, err
		}
		resp := &dto.MediaResponse{
			RequestID:    out.RequestID,
			ErrorCode:    out.Code,
			ErrorMessage: out.Message,
		}
		for _, choice := range out.Output.Choices {
			for _, content := range choice.Message.Content {
				if content.Image != "" {
					resp.Data = append(resp.Data, dto.ImageData{URL: content.Image})
				}
				if resp.Text == "" && content.Text != "" {
					resp.Text = content.Text
				}
			}
		}
		if len(resp.Data) > 0 {
			resp.URL = resp.Data[0].URL
		}
		return resp, nil
	case dto.MediaTypeVideo:
		endpoint, payload, err := a.buildVideoRequest(request)
		if err != nil {
			return nil, err
		}
		var out aliTaskCreateResponse
		headers := map[string]string{"X-DashScope-Async": "enable"}
		if err := a.doJSON(ctx, cfg, http.MethodPost, endpoint, payload, headers, &out); err != nil {
			return nil, err
		}
		return &dto.MediaResponse{
			RequestID: out.RequestID,
			TaskID:    out.Output.TaskID,
			Status:    out.Output.TaskStatus,
			ErrorCode: firstNonEmptyString(out.Output.Code, out.Code),
			ErrorMessage: firstNonEmptyString(
				out.Output.Message,
				out.Message,
			),
		}, nil
	default:
		return (&OpenAIAdaptor{}).Media(ctx, aliCompatibleConfig(config), request)
	}
}

func (a *AliAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, taskID string, _ ...map[string]string) (*dto.TaskStatusResponse, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("task id is required")
	}

	var out aliTaskStatusEnvelope
	if err := a.doJSON(ctx, aliNativeConfig(config), http.MethodGet, aliTaskEndpoint+url.PathEscape(taskID), nil, nil, &out); err != nil {
		return nil, err
	}

	resp := &dto.TaskStatusResponse{
		RequestID: out.RequestID,
		Usage:     out.Usage,
		Output: dto.TaskStatusOutput{
			TaskID:        firstNonEmptyString(out.Output.TaskID, taskID),
			TaskStatus:    out.Output.TaskStatus,
			SubmitTime:    out.Output.SubmitTime,
			ScheduledTime: out.Output.ScheduledTime,
			EndTime:       out.Output.EndTime,
			URL:           firstNonEmptyString(out.Output.ResultURL, aliFirstResultURL(out.Output.Results)),
			VideoURL:      firstNonEmptyString(out.Output.VideoURL, aliFirstResultURL(out.Output.Results)),
			OrigPrompt:    out.Output.OrigPrompt,
			ActualPrompt:  out.Output.ActualPrompt,
			Code:          firstNonEmptyString(out.Output.Code, out.Code),
			Message: firstNonEmptyString(
				out.Output.Message,
				out.Message,
			),
		},
	}
	if resp.Output.URL == "" {
		resp.Output.URL = resp.Output.VideoURL
	}
	if resp.Output.VideoURL == "" {
		resp.Output.VideoURL = resp.Output.URL
	}
	return resp, nil
}

func (a *AliAdaptor) ListTasks(ctx context.Context, config *ProviderConfig, query map[string]string) (*dto.TaskListResponse, error) {
	var out aliTaskListEnvelope
	if err := a.doJSON(ctx, aliNativeConfig(config), http.MethodGet, aliTaskEndpoint+buildAliQuery(query), nil, nil, &out); err != nil {
		return nil, err
	}

	items := make([]dto.TaskListItem, 0, len(out.Output))
	for _, item := range out.Output {
		items = append(items, dto.TaskListItem{
			ID:        item.TaskID,
			TaskID:    item.TaskID,
			State:     item.TaskStatus,
			Model:     item.Model,
			CreatedAt: item.SubmitTime,
			Payload:   item.Payload,
		})
	}
	total := out.Total
	if total == 0 {
		total = len(items)
	}
	return &dto.TaskListResponse{
		Items:    items,
		Tasks:    items,
		Total:    total,
		PageNum:  out.PageNum,
		PageSize: out.PageSize,
		HasMore:  out.HasMore,
	}, nil
}

func (a *AliAdaptor) StreamMedia(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	resp, err := a.Media(ctx, config, request)
	if err != nil {
		return nil, err
	}
	if resp.TaskID == "" {
		if resp.URL == "" {
			return nil, fmt.Errorf("ali did not return a task id or result url")
		}
		return &klingSingleTokenStream{
			token: &dto.StreamToken{
				Type: "url",
				URL:  resp.URL,
				Text: resp.Status,
			},
		}, nil
	}
	return &aliPollingStream{adaptor: a, cfg: config, taskID: resp.TaskID, last: resp.Status}, nil
}

func (a *AliAdaptor) buildImageRequest(r *dto.MediaRequest) (string, map[string]interface{}, bool, error) {
	prompt := strings.TrimSpace(mediaPromptWithSystem(r))
	if explicitPrompt := getStringExtra(r.Extra, "prompt"); explicitPrompt != "" {
		prompt = explicitPrompt
	}
	if prompt == "" {
		return "", nil, false, fmt.Errorf("ali image request requires a prompt")
	}

	mode := aliResolveImageMode(r)
	if mode == aliImageModeSync {
		payload := map[string]interface{}{
			"model": r.Model,
			"input": map[string]interface{}{
				"messages": []map[string]interface{}{
					{
						"role": "user",
						"content": []map[string]interface{}{
							{"text": prompt},
						},
					},
				},
			},
			"parameters": aliImageParameters(r),
		}
		return aliImageModeEndpoint[mode], payload, false, nil
	}

	parameters := aliImageParameters(r)
	if len(parameters) == 0 {
		parameters = nil
	}

	if mode == aliImageModeAsyncV2 {
		payload := map[string]interface{}{
			"model": r.Model,
			"input": map[string]interface{}{
				"messages": []map[string]interface{}{
					{
						"role": "user",
						"content": []map[string]interface{}{
							{"text": prompt},
						},
					},
				},
			},
		}
		if parameters != nil {
			payload["parameters"] = parameters
		}
		return aliImageModeEndpoint[mode], payload, true, nil
	}

	payload := map[string]interface{}{
		"model": r.Model,
		"input": map[string]interface{}{
			"prompt": prompt,
		},
	}
	if parameters != nil {
		payload["parameters"] = parameters
	}
	return aliImageModeEndpoint[mode], payload, true, nil
}

func (a *AliAdaptor) buildVideoRequest(r *dto.MediaRequest) (string, map[string]interface{}, error) {
	videoMode := aliResolveVideoMode(r)
	model := strings.TrimSpace(r.Model)
	if videoMode == aliVideoModeAnimate || aliIsAnimateMixModel(model) {
		return a.buildAnimateMixVideoRequest(r, model)
	}
	if videoMode == aliVideoModeAvatar || aliIsAvatarVideoModel(model) {
		return a.buildAvatarVideoRequest(r, model)
	}

	prompt := strings.TrimSpace(mediaPromptWithSystem(r))
	input := map[string]interface{}{
		"prompt": prompt,
	}

	images := aliCollectImages(r)
	mediaProtocol := aliUsesMediaProtocol(model, r)
	if firstFrame := getStringExtra(r.Extra, "first_frame_url"); firstFrame != "" {
		if !mediaProtocol {
			input["first_frame_url"] = firstFrame
		}
	} else if len(images) > 0 && !mediaProtocol && (videoMode == aliVideoModeKeyframe || videoMode == aliVideoModeImage) {
		input["first_frame_url"] = images[0]
	}
	if lastFrame := getStringExtra(r.Extra, "last_frame_url"); lastFrame != "" {
		if !mediaProtocol {
			input["last_frame_url"] = lastFrame
		}
	} else if len(images) > 1 && !mediaProtocol && videoMode == aliVideoModeKeyframe {
		input["last_frame_url"] = images[1]
	}
	if imgURL := getStringExtra(r.Extra, "img_url"); imgURL != "" {
		if !mediaProtocol {
			input["img_url"] = imgURL
		}
	} else if input["first_frame_url"] == nil && len(images) > 0 && !mediaProtocol && videoMode == aliVideoModeImage {
		input["img_url"] = images[0]
	}
	aliCopyInputExtras(input, r.Extra)
	refImages := aliReferenceImages(r)
	if len(refImages) == 0 && videoMode == aliVideoModeReference {
		refImages = images
	}
	if media := aliMediaInput(r, videoMode); len(media) > 0 {
		input["media"] = media
	} else if len(refImages) > 0 && !mediaProtocol {
		input["reference_urls"] = refImages
	}

	payload := map[string]interface{}{
		"model": model,
		"input": input,
	}
	parameters := aliVideoParameters(r)
	if len(parameters) > 0 {
		payload["parameters"] = parameters
	}

	endpoint := aliVideoModeEndpoint[videoMode]
	if mediaProtocol {
		endpoint = aliVideoEndpoint
	}
	if path := getStringExtra(r.Extra, "endpoint"); path != "" {
		endpoint = path
	}
	return endpoint, payload, nil
}

func (a *AliAdaptor) buildAnimateMixVideoRequest(r *dto.MediaRequest, model string) (string, map[string]interface{}, error) {
	input := map[string]interface{}{}
	if imageURL := firstNonEmptyString(getStringExtra(r.Extra, "image_url"), getStringExtra(r.Extra, "image"), firstAliImageURL(r)); imageURL != "" {
		input["image_url"] = imageURL
	}
	if videoURL := firstNonEmptyString(getStringExtra(r.Extra, "video_url"), getStringExtra(r.Extra, "video"), firstAliVideoURL(r)); videoURL != "" {
		input["video_url"] = videoURL
	}
	if watermark, ok := getBoolExtra(r.Extra, "watermark"); ok {
		input["watermark"] = watermark
	}

	parameters := map[string]interface{}{}
	mode := firstNonEmptyString(getStringExtra(r.Extra, "animate_mode"), getStringExtra(r.Extra, "service_mode"))
	if rawMode := getStringExtra(r.Extra, "mode"); rawMode == "wan-std" || rawMode == "wan-pro" {
		mode = rawMode
	}
	if mode == "" {
		mode = "wan-std"
	}
	parameters["mode"] = mode

	payload := map[string]interface{}{
		"model":      model,
		"input":      input,
		"parameters": parameters,
	}
	return aliEndpointOverride(r, aliImageToVideoEndpoint), payload, nil
}

func (a *AliAdaptor) buildAvatarVideoRequest(r *dto.MediaRequest, model string) (string, map[string]interface{}, error) {
	input := map[string]interface{}{}
	if imageURL := firstNonEmptyString(getStringExtra(r.Extra, "image_url"), getStringExtra(r.Extra, "image"), firstAliImageURL(r)); imageURL != "" {
		input["image_url"] = imageURL
	}
	if audioURL := firstNonEmptyString(getStringExtra(r.Extra, "audio_url"), getStringExtra(r.Extra, "audio")); audioURL != "" {
		input["audio_url"] = audioURL
	}

	parameters := map[string]interface{}{}
	if r.Resolution != "" {
		parameters["resolution"] = r.Resolution
	}
	for _, key := range []string{"resolution", "style"} {
		if value, ok := r.Extra[key]; ok && value != nil {
			parameters[key] = value
		}
	}

	payload := map[string]interface{}{
		"model": model,
		"input": input,
	}
	if len(parameters) > 0 {
		payload["parameters"] = parameters
	}
	return aliEndpointOverride(r, aliImageToVideoEndpoint), payload, nil
}

func (a *AliAdaptor) doJSON(ctx context.Context, cfg *ProviderConfig, method, path string, payload interface{}, extraHeaders map[string]string, out interface{}) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(aliBaseURL(cfg), "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	for key, value := range cfg.Headers {
		req.Header.Set(key, value)
	}
	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}

	resp, err := aliHTTPClient(cfg).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr map[string]interface{}
		if err := json.Unmarshal(raw, &apiErr); err == nil {
			return fmt.Errorf("ali api error: status=%d code=%v message=%v request_id=%v", resp.StatusCode, apiErr["code"], apiErr["message"], apiErr["request_id"])
		}
		return fmt.Errorf("ali api error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (w *aliPollingStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	for {
		status, err := w.adaptor.TaskStatus(ctx, w.cfg, w.taskID)
		if err != nil {
			return nil, err
		}
		state := status.Output.TaskStatus
		if state == w.last && !aliTerminalTaskState(state) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				continue
			}
		}
		w.last = state

		token := &dto.StreamToken{Type: "progress", Text: state}
		switch {
		case aliTaskSucceeded(state):
			token.Type = "url"
			token.URL = firstNonEmptyString(status.Output.VideoURL, status.Output.URL)
			return token, io.EOF
		case aliTaskFailed(state):
			if status.Output.Message != "" {
				token.Text = status.Output.Message
			}
			return token, fmt.Errorf("ali task failed: %s", token.Text)
		default:
			return token, nil
		}
	}
}

func (w *aliPollingStream) Close() error { return nil }

func aliCompatibleConfig(cfg *ProviderConfig) *ProviderConfig {
	cloned := aliCloneConfig(cfg)
	cloned.BaseURL = aliNormalizeCompatibleBaseURL(cloned.BaseURL)
	return cloned
}

func aliNativeConfig(cfg *ProviderConfig) *ProviderConfig {
	cloned := aliCloneConfig(cfg)
	cloned.BaseURL = aliNormalizeNativeBaseURL(cloned.BaseURL)
	return cloned
}

func aliCloneConfig(cfg *ProviderConfig) *ProviderConfig {
	if cfg == nil {
		return &ProviderConfig{BaseURL: aliNativeBaseURL}
	}
	cloned := *cfg
	if cfg.Headers != nil {
		cloned.Headers = copyMapString(cfg.Headers)
	}
	return &cloned
}

func copyMapString(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func aliNormalizeCompatibleBaseURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch trimmed {
	case "", aliNativeBaseURL:
		return aliCompatibleBaseURL
	default:
		if strings.HasSuffix(trimmed, "/compatible-mode/v1") {
			return trimmed
		}
		return trimmed + "/compatible-mode/v1"
	}
}

func aliNormalizeNativeBaseURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch trimmed {
	case "", aliCompatibleBaseURL:
		return aliNativeBaseURL
	default:
		if strings.HasSuffix(trimmed, "/compatible-mode/v1") {
			return strings.TrimSuffix(trimmed, "/compatible-mode/v1")
		}
		return trimmed
	}
}

func aliBaseURL(cfg *ProviderConfig) string {
	if cfg != nil && cfg.BaseURL != "" {
		return cfg.BaseURL
	}
	return aliNativeBaseURL
}

func aliHTTPClient(cfg *ProviderConfig) *http.Client {
	if cfg != nil && cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}
	client := &http.Client{}
	if cfg != nil && cfg.Timeout > 0 {
		client.Timeout = cfg.Timeout
	}
	return client
}

func aliImageParameters(r *dto.MediaRequest) (parameters map[string]interface{}) {
	if r.Size != "" {
		if aliIsAspectRatio(r.Size) {
			parameters["size"] = aliAspectRatioSize(r.Size)
		} else {
			parameters["size"] = r.Size
		}
	}
	if r.N > 0 {
		parameters["n"] = r.N
	}
	if r.Seed != 0 {
		parameters["seed"] = r.Seed
	}
	if r.ResponseFormat != "" {
		parameters["response_format"] = r.ResponseFormat
	}
	if len(parameters) == 0 {
		return nil
	}
	return parameters
}

func aliVideoParameters(r *dto.MediaRequest) (parameters map[string]interface{}) {
	parameters = map[string]interface{}{}
	if r.Size != "" {
		parameters["ratio"] = r.Size
	}
	if r.Duration > 0 {
		parameters["duration"] = r.Duration
	}
	if r.Fps > 0 {
		parameters["fps"] = r.Fps
	}
	if r.Resolution != "" {
		parameters["resolution"] = r.Resolution
	}
	if r.Seed != 0 {
		parameters["seed"] = r.Seed
	}
	for _, key := range []string{
		"negative_prompt",
		"prompt_extend",
		"watermark",
		"shot_type",
		"audio",
		"template",
	} {
		if value, ok := r.Extra[key]; ok && value != nil {
			parameters[key] = value
		}
	}
	if len(parameters) == 0 {
		return nil
	}
	return parameters
}

func aliCollectImages(r *dto.MediaRequest) []string {
	out := make([]string, 0, len(r.Messages))
	for _, message := range r.Messages {
		if message.ImageURL != "" {
			out = append(out, message.ImageURL)
		}
	}
	if len(out) == 0 {
		out = append(out, getStringSliceExtra(r.Extra, "images")...)
	}
	return compactStrings(out)
}

func aliReferenceImages(r *dto.MediaRequest) []string {
	if urls := getStringSliceExtra(r.Extra, "reference_urls"); len(urls) > 0 {
		return urls
	}
	if url := getStringExtra(r.Extra, "reference_url"); url != "" {
		return []string{url}
	}
	return nil
}

const (
	aliModelFamilyUnknown            = ""
	aliModelFamilyQwenImage          = "qwen-image"
	aliModelFamilyWanImageGeneration = "wan-image-generation"
)

func aliMediaInput(r *dto.MediaRequest, videoMode string) []map[string]interface{} {
	if r == nil {
		return nil
	}
	if media := aliMediaObjectsFromExtra(r.Extra["media"]); len(media) > 0 {
		return media
	}
	if !aliUsesMediaProtocol(r.Model, r) {
		return nil
	}
	files := aliMediaObjectsFromExtra(r.Extra["files"])
	if len(files) == 0 {
		files = append(files, aliMediaObjectsFromMessages(r.Messages)...)
	}
	if len(files) == 0 {
		if firstFrame := getStringExtra(r.Extra, "first_frame_url"); firstFrame != "" {
			files = append(files, map[string]interface{}{"type": "first_frame", "url": firstFrame})
		}
		if lastFrame := getStringExtra(r.Extra, "last_frame_url"); lastFrame != "" {
			files = append(files, map[string]interface{}{"type": "last_frame", "url": lastFrame})
		}
		if imgURL := getStringExtra(r.Extra, "img_url"); imgURL != "" {
			files = append(files, map[string]interface{}{"type": "first_frame", "url": imgURL})
		}
	}
	if len(files) == 0 {
		for _, urlValue := range aliReferenceImages(r) {
			files = append(files, map[string]interface{}{"type": "image", "url": urlValue})
		}
	}
	if len(files) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(files))
	firstFrameUsed := false
	for _, file := range files {
		urlValue, _ := file["url"].(string)
		urlValue = strings.TrimSpace(urlValue)
		if urlValue == "" {
			continue
		}
		typ, _ := file["type"].(string)
		typ = strings.ToLower(strings.TrimSpace(typ))
		mediaType := ""
		switch typ {
		case "reference_image", "reference_video", "first_frame", "last_frame", "first_clip":
			mediaType = typ
		case "image":
			if videoMode == aliVideoModeImage || videoMode == aliVideoModeKeyframe {
				if firstFrameUsed && videoMode == aliVideoModeKeyframe {
					mediaType = "last_frame"
				} else if firstFrameUsed && videoMode == aliVideoModeImage {
					mediaType = "last_frame"
				} else {
					mediaType = "first_frame"
					firstFrameUsed = true
				}
			} else {
				mediaType = "reference_image"
			}
		case "video":
			if videoMode == aliVideoModeImage || videoMode == aliVideoModeKeyframe {
				if firstFrameUsed {
					continue
				}
				mediaType = "first_clip"
				firstFrameUsed = true
			} else {
				mediaType = "reference_video"
			}
		default:
			continue
		}
		item := copyMap(file)
		item["type"] = mediaType
		item["url"] = urlValue
		out = append(out, item)
	}
	return out
}

func aliMediaObjectsFromMessages(messages []dto.Message) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(messages))
	for _, message := range messages {
		if urlValue := strings.TrimSpace(message.ImageURL); urlValue != "" {
			out = append(out, map[string]interface{}{"type": "image", "url": urlValue})
		}
		if urlValue := strings.TrimSpace(message.VideoURL); urlValue != "" {
			out = append(out, map[string]interface{}{"type": "video", "url": urlValue})
		}
	}
	return out
}

func aliMediaObjectsFromExtra(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			obj, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			out = append(out, obj)
		}
		return out
	default:
		return nil
	}
}

func aliIsAspectRatio(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || !strings.Contains(value, ":") {
		return false
	}
	parts := strings.Split(value, ":")
	return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}

func aliAspectRatioSize(value string) string {
	switch strings.TrimSpace(value) {
	case "16:9":
		return "1696*960"
	case "9:16":
		return "960*1696"
	case "1:1":
		return "1280*1280"
	case "4:3":
		return "1472*1104"
	case "3:4":
		return "1104*1472"
	default:
		return value
	}
}

func aliModelFamily(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(model, "qwen-image"):
		return aliModelFamilyQwenImage
	case strings.Contains(model, "t2i"), strings.Contains(model, "text2image"):
		return aliModelFamilyWanImageGeneration
	default:
		return aliModelFamilyUnknown
	}
}

func aliResolveImageMode(r *dto.MediaRequest) string {
	if async, ok := getBoolExtra(r.Extra, "async"); ok && !async {
		return aliImageModeSync
	}
	if mode := normalizeAliImageMode(getStringExtra(r.Extra, "mode")); mode != "" {
		return mode
	}
	if mode := normalizeAliImageMode(r.Model); mode != "" {
		return mode
	}
	switch aliModelFamily(r.Model) {
	case aliModelFamilyQwenImage:
		return aliImageModeSync
	case aliModelFamilyWanImageGeneration:
		return aliImageModeAsyncV2
	default:
		return aliImageModeAsyncV1
	}
}

func normalizeAliImageMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "sync", "multimodal-generation", "qwen-image":
		return aliImageModeSync
	case "image-generation", "async-v2", "wan-t2i":
		return aliImageModeAsyncV2
	case "text2image", "image-synthesis", "async-v1":
		return aliImageModeAsyncV1
	default:
		switch {
		case strings.Contains(mode, "qwen-image"):
			return aliImageModeSync
		// wanx2.1 t2i still uses old text2image endpoint.
		case strings.Contains(mode, "wanx2.1") && (strings.Contains(mode, "t2i") || strings.Contains(mode, "text2image")):
			return aliImageModeAsyncV1
		case strings.Contains(mode, "t2i"), strings.Contains(mode, "text2image"):
			return aliImageModeAsyncV2
		}
		return ""
	}
}

func aliResolveVideoMode(r *dto.MediaRequest) string {
	if mode := normalizeAliVideoMode(getStringExtra(r.Extra, "mode")); mode != "" {
		return mode
	}
	if mode := normalizeAliVideoMode(r.Model); mode != "" {
		return mode
	}
	return aliVideoModeText
}

func normalizeAliVideoMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return ""
	}
	switch {
	case strings.Contains(mode, "animate"), strings.Contains(mode, "换人"), strings.Contains(mode, "replace-person"), strings.Contains(mode, "video-replace"):
		return aliVideoModeAnimate
	case strings.Contains(mode, "s2v"), strings.Contains(mode, "avatar"), strings.Contains(mode, "数字人"):
		return aliVideoModeAvatar
	case strings.Contains(mode, "r2v"), strings.Contains(mode, "reference"):
		return aliVideoModeReference
	case strings.Contains(mode, "kf2v"), strings.Contains(mode, "keyframe"):
		return aliVideoModeKeyframe
	case strings.Contains(mode, "i2v"), strings.Contains(mode, "img2video"), strings.Contains(mode, "image2video"):
		return aliVideoModeImage
	default:
		if strings.Contains(mode, "t2v") || strings.Contains(mode, "text-to-video") || strings.Contains(mode, "video-generation") {
			return aliVideoModeText
		}
		return ""
	}
}

func aliUsesMediaProtocol(model string, r *dto.MediaRequest) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(model, "wan2.7") {
		return true
	}
	return r != nil && r.Extra != nil && r.Extra["media"] != nil
}

func aliIsAnimateMixModel(model string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(model)), "animate-mix")
}

func aliIsAvatarVideoModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(model, "s2v") || strings.Contains(model, "avatar")
}

func aliCopyInputExtras(input, extra map[string]interface{}) {
	for _, key := range []string{"negative_prompt", "audio_url"} {
		if value, ok := extra[key]; ok && value != nil {
			input[key] = value
		}
	}
}

func aliEndpointOverride(r *dto.MediaRequest, fallback string) string {
	if path := getStringExtra(r.Extra, "endpoint"); path != "" {
		return path
	}
	return fallback
}

func firstAliImageURL(r *dto.MediaRequest) string {
	if r == nil {
		return ""
	}
	if images := aliCollectImages(r); len(images) > 0 {
		return images[0]
	}
	return ""
}

func firstAliVideoURL(r *dto.MediaRequest) string {
	if r == nil {
		return ""
	}
	for _, message := range r.Messages {
		if videoURL := strings.TrimSpace(message.VideoURL); videoURL != "" {
			return videoURL
		}
	}
	if files := aliMediaObjectsFromExtra(r.Extra["files"]); len(files) > 0 {
		for _, file := range files {
			typ, _ := file["type"].(string)
			urlValue, _ := file["url"].(string)
			if strings.EqualFold(strings.TrimSpace(typ), "video") && strings.TrimSpace(urlValue) != "" {
				return strings.TrimSpace(urlValue)
			}
		}
	}
	return ""
}

func aliFirstResultURL(results []map[string]interface{}) string {
	for _, result := range results {
		for _, key := range []string{"url", "video_url", "image_url"} {
			if value, ok := result[key].(string); ok && value != "" {
				return value
			}
		}
	}
	return ""
}

func aliTaskSucceeded(state string) bool {
	state = strings.ToUpper(strings.TrimSpace(state))
	return state == "SUCCEEDED" || state == "SUSPENDED"
}

func aliTaskFailed(state string) bool {
	state = strings.ToUpper(strings.TrimSpace(state))
	return state == "FAILED" || state == "CANCELED"
}

func aliTerminalTaskState(state string) bool {
	return aliTaskSucceeded(state) || aliTaskFailed(state)
}

func buildAliQuery(query map[string]string) string {
	if len(query) == 0 {
		return ""
	}
	values := url.Values{}
	for key, value := range query {
		if strings.TrimSpace(key) == "" || value == "" {
			continue
		}
		values.Set(key, value)
	}
	encoded := values.Encode()
	if encoded == "" {
		return ""
	}
	return "?" + encoded
}
