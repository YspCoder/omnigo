package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
)

// The new API uses model-specific create paths and a shared /tasks endpoint.
// Keep it opt-in so legacy model names and payloads retain their meaning.
func klingUsesTasks(protocol, endpoint string) bool {
	if protocol != "" {
		return strings.EqualFold(strings.TrimSpace(protocol), "tasks")
	}
	return strings.HasPrefix(endpoint, "/") && !strings.HasPrefix(endpoint, "/v1/")
}

func (a *KlingAdaptor) createNativeTask(ctx context.Context, cfg *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	endpoint, payload, err := klingNativePayload(request)
	if err != nil {
		return nil, err
	}
	var env klingEnvelope
	if err := a.doJSON(ctx, cfg, http.MethodPost, endpoint, payload, &env); err != nil {
		return nil, err
	}
	var task klingNativeTask
	if err := decodeKlingData(env.Data, &task); err != nil {
		return nil, err
	}
	if task.ID == "" {
		return nil, fmt.Errorf("kling response is missing task id")
	}
	status := task.response(env.RequestID)
	response := &dto.MediaResponse{RequestID: env.RequestID, TaskID: task.ID, Status: task.Status,
		URL: status.Output.URL, Usage: task.usage(), ErrorMessage: task.Message}
	response.Video.URL = status.Output.VideoURL
	for _, raw := range task.Outputs {
		var output klingNativeOutput
		if json.Unmarshal(raw, &output) == nil && output.Type == "image" {
			response.Data = append(response.Data, dto.ImageData{URL: output.url()})
		}
	}
	return response, nil
}

func klingNativePayload(request *dto.MediaRequest) (string, map[string]any, error) {
	payload := copyMap(utils.ExtractPayloadMap(request.Extra))
	if payload == nil {
		payload = make(map[string]any)
	}
	mergeExtraPayload(payload, request.Extra, map[string]struct{}{
		"payload": {}, "protocol": {}, "endpoint": {}, "task_type": {}, "mode": {},
		"image": {}, "images": {}, "files": {},
	})
	endpoint := utils.GetStringExtra(request.Extra, "endpoint")
	images := klingImageInputs(request)
	if endpoint == "" {
		if request.Type != dto.MediaTypeVideo || request.Model == "" {
			return "", nil, fmt.Errorf("kling tasks requires a video model or an explicit native endpoint")
		}
		mode := utils.GetStringExtra(request.Extra, "task_type")
		if mode == "" {
			mode = klingModeTextToVideo
			if len(images) > 0 || payload["contents"] != nil {
				mode = klingModeImageToVideo
			}
		}
		if mode != klingModeTextToVideo && mode != klingModeImageToVideo {
			return "", nil, fmt.Errorf("kling task type %q requires an explicit native endpoint", mode)
		}
		endpoint = "/" + mode + "/" + url.PathEscape(request.Model)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
		!strings.HasPrefix(endpoint, "/") || strings.HasPrefix(endpoint, "/v1/") || endpoint == "/tasks" {
		return "", nil, fmt.Errorf("kling tasks requires a native model endpoint path")
	}
	settings := make(map[string]any)
	if value, ok := payload["settings"]; ok {
		var valid bool
		settings, valid = customMetadataMap(value)
		if !valid {
			return "", nil, fmt.Errorf("kling settings must be an object")
		}
	}
	options := make(map[string]any)
	if value, ok := payload["options"]; ok {
		var valid bool
		options, valid = customMetadataMap(value)
		if !valid {
			return "", nil, fmt.Errorf("kling options must be an object")
		}
	}
	for _, key := range []string{"resolution", "aspect_ratio", "duration", "audio", "multi_shot"} {
		if value, ok := payload[key]; ok {
			if _, exists := settings[key]; !exists {
				settings[key] = value
			}
			delete(payload, key)
		}
	}
	for _, key := range []string{"callback_url", "external_task_id", "watermark_info"} {
		if value, ok := payload[key]; ok {
			if _, exists := options[key]; !exists {
				options[key] = value
			}
			delete(payload, key)
		}
	}
	if _, ok := settings["resolution"]; !ok && request.Resolution != "" {
		settings["resolution"] = request.Resolution
	}
	if _, ok := settings["aspect_ratio"]; !ok && request.Size != "" {
		settings["aspect_ratio"] = request.Size
	}
	if _, ok := settings["duration"]; !ok && request.Duration != nil {
		settings["duration"] = request.Duration
	}
	if value, ok := settings["duration"]; ok {
		duration, err := strconv.Atoi(fmt.Sprint(value))
		if err != nil || duration <= 0 {
			return "", nil, fmt.Errorf("kling tasks duration must be a positive integer")
		}
		settings["duration"] = duration
	}
	prompt := utils.MediaPromptWithSystem(request)
	if strings.HasPrefix(endpoint, "/text-to-video/") {
		if len(images) > 0 || klingFirstVideoURL(request.Messages) != "" {
			return "", nil, fmt.Errorf("kling text-to-video cannot include reference media")
		}
		if _, ok := payload["prompt"]; !ok && prompt != "" {
			payload["prompt"] = prompt
		}
	} else if strings.HasPrefix(endpoint, "/image-to-video/") {
		if klingFirstVideoURL(request.Messages) != "" {
			return "", nil, fmt.Errorf("kling image-to-video cannot include video references; use an omni endpoint with native contents")
		}
		if _, ok := payload["contents"]; !ok {
			if len(images) == 0 || len(images) > 2 {
				return "", nil, fmt.Errorf("kling image-to-video requires one or two images, or native contents")
			}
			contents := make([]map[string]any, 0, 3)
			if explicit := utils.GetStringExtra(payload, "prompt"); explicit != "" {
				prompt = explicit
			}
			if prompt != "" {
				contents = append(contents, map[string]any{"type": "prompt", "text": prompt})
			}
			for index, image := range images {
				contents = append(contents, map[string]any{"type": []string{"first_frame", "last_frame"}[index], "url": image})
			}
			payload["contents"] = contents
		}
		delete(payload, "prompt")
	}
	if len(settings) > 0 {
		payload["settings"] = settings
	}
	if len(options) > 0 {
		payload["options"] = options
	}
	return endpoint, payload, nil
}

type klingNativeTask struct {
	ID         string            `json:"id"`
	Status     string            `json:"status"`
	Message    string            `json:"message"`
	CreateTime int64             `json:"create_time"`
	UpdateTime int64             `json:"update_time"`
	ExternalID string            `json:"external_id"`
	Outputs    []json.RawMessage `json:"outputs"`
	Billing    json.RawMessage   `json:"billing"`
	Usage      json.RawMessage   `json:"usage"`
}

type klingNativeOutput struct {
	Type         string `json:"type"`
	URL          string `json:"url"`
	WatermarkURL string `json:"watermark_url"`
	MP3URL       string `json:"mp3_url"`
	WAVURL       string `json:"wav_url"`
}

func (output klingNativeOutput) url() string {
	return firstNonEmptyString(output.URL, output.WatermarkURL, output.MP3URL, output.WAVURL)
}

func (task klingNativeTask) usage() dto.Usage {
	usage := usageFromJSON(task.Usage)
	raw := make(map[string]json.RawMessage)
	for key, value := range map[string]json.RawMessage{"usage": task.Usage, "billing": task.Billing} {
		if trimmed := bytes.TrimSpace(value); len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
			raw[key] = value
			if usage.Billing == nil {
				usage.Billing = make(map[string]json.RawMessage)
			}
			usage.Billing[key] = value
		}
	}
	if len(raw) > 0 {
		usage.RawUsage, _ = json.Marshal(raw)
	}
	return usage
}

func (task klingNativeTask) response(requestID string) *dto.TaskStatusResponse {
	response := &dto.TaskStatusResponse{RequestID: requestID, Usage: taskUsage(task.usage()),
		Output: dto.TaskStatusOutput{TaskID: task.ID, TaskStatus: task.Status, Message: task.Message,
			SubmitTime: strconvMillis(task.CreateTime), EndTime: strconvMillis(task.UpdateTime), Outputs: task.Outputs},
	}
	for _, raw := range task.Outputs {
		var output klingNativeOutput
		if json.Unmarshal(raw, &output) != nil {
			continue
		}
		if response.Output.URL == "" {
			response.Output.URL = output.url()
		}
		if output.Type == "video" && response.Output.VideoURL == "" {
			response.Output.VideoURL = output.url()
		}
	}
	return response
}

func (a *KlingAdaptor) nativeTaskStatus(ctx context.Context, cfg *ProviderConfig, taskID string) (*dto.TaskStatusResponse, error) {
	var env klingEnvelope
	if err := a.doJSON(ctx, cfg, http.MethodGet, "/tasks?"+url.Values{"task_ids": {taskID}}.Encode(), nil, &env); err != nil {
		return nil, err
	}
	var tasks []klingNativeTask
	if err := decodeKlingData(env.Data, &tasks); err != nil {
		return nil, err
	}
	for _, task := range tasks {
		if task.ID == taskID {
			return task.response(env.RequestID), nil
		}
	}
	return nil, fmt.Errorf("kling task %q was not returned", taskID)
}

func (a *KlingAdaptor) listNativeTasks(ctx context.Context, cfg *ProviderConfig, query map[string]string) (*dto.TaskListResponse, error) {
	payload := make(map[string]any)
	for _, key := range []string{"start_time", "end_time", "cursor"} {
		if value := query[key]; value != "" {
			payload[key] = value
		}
	}
	if value := query["limit"]; value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 500 {
			return nil, fmt.Errorf("kling task list limit must be between 1 and 500")
		}
		payload["limit"] = limit
	}
	if value := query["filters"]; value != "" {
		var filters []map[string]any
		if err := json.Unmarshal([]byte(value), &filters); err != nil {
			return nil, fmt.Errorf("kling task filters: %w", err)
		}
		payload["filters"] = filters
	}
	var env klingEnvelope
	if err := a.doJSON(ctx, cfg, http.MethodPost, "/tasks", payload, &env); err != nil {
		return nil, err
	}
	var page struct {
		Result     []json.RawMessage `json:"result"`
		Count      int               `json:"count"`
		NextCursor string            `json:"next_cursor"`
		HasMore    bool              `json:"has_more"`
	}
	if err := decodeKlingData(env.Data, &page); err != nil {
		return nil, err
	}
	items := make([]dto.TaskListItem, 0, len(page.Result))
	for _, raw := range page.Result {
		var task klingNativeTask
		if err := decodeKlingData(raw, &task); err != nil {
			return nil, err
		}
		items = append(items, dto.TaskListItem{ID: task.ID, TaskID: task.ID, State: task.Status,
			CreatedAt: strconvMillis(task.CreateTime), Payload: raw, Usage: taskUsage(task.usage())})
	}
	return &dto.TaskListResponse{Items: items, Tasks: items, Total: page.Count, HasMore: page.HasMore, NextCursor: page.NextCursor}, nil
}
