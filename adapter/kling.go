// Package adapter provides Kling AI adaptor implementation.
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
	klingModeOmniVideo         = "omni-video"
	klingModeTextToVideo       = "text-to-video"
	klingModeImageToVideo      = "image-to-video"
	klingModeMultiImageToVideo = "multi-image-to-video"
	klingModeTextToAudio       = "text-to-audio"
	klingModeVideoToAudio      = "video-to-audio"
	klingModeTTS               = "tts"
	klingModeCustomVoices      = "custom-voices"
	klingModeIdentifyFace      = "identify-face"
	klingModeLipSync           = "lip-sync"
	klingModeOmniImage         = "omni-image"
	klingModeImageGeneration   = "image-generation"
	klingModeMultiImageToImage = "multi-image-to-image"
)

var klingModeEndpoint = map[string]string{
	klingModeOmniVideo:         "/v1/videos/omni-video",
	klingModeTextToVideo:       "/v1/videos/text2video",
	klingModeImageToVideo:      "/v1/videos/image2video",
	klingModeMultiImageToVideo: "/v1/videos/multi-image2video",
	klingModeTextToAudio:       "/v1/audio/text-to-audio",
	klingModeVideoToAudio:      "/v1/audio/video-to-audio",
	klingModeTTS:               "/v1/audio/tts",
	klingModeCustomVoices:      "/v1/general/custom-voices",
	klingModeIdentifyFace:      "/v1/videos/identify-face",
	klingModeLipSync:           "/v1/videos/advanced-lip-sync",
	klingModeOmniImage:         "/v1/images/omni-image",
	klingModeImageGeneration:   "/v1/images/generations",
	klingModeMultiImageToImage: "/v1/images/multi-image2image",
}

var klingTaskModes = []string{
	klingModeTextToVideo,
	klingModeImageToVideo,
	klingModeMultiImageToVideo,
	klingModeOmniVideo,
	klingModeTextToAudio,
	klingModeVideoToAudio,
	klingModeCustomVoices,
	klingModeLipSync,
	klingModeOmniImage,
	klingModeImageGeneration,
	klingModeMultiImageToImage,
}

var klingVideoModels = map[string]struct{}{
	"kling-v1":          {},
	"kling-v1-5":        {},
	"kling-v1-6":        {},
	"kling-v2":          {},
	"kling-v2-1":        {},
	"kling-v2-1-master": {},
	"kling-v2-5":        {},
	"kling-v2-5-turbo":  {},
	"kling-v2-6":        {},
	"kling-v2-master":   {},
	"kling-v3":          {},
	"kling-v3-omni":     {},
	"kling-video-o1":    {},
}

var klingImageModels = map[string]struct{}{
	"kling-2":        {},
	"kling-image-o1": {},
	"kling-v1":       {},
	"kling-v1-5":     {},
	"kling-v2":       {},
	"kling-v2-1":     {},
	"kling-v2-new":   {},
	"kling-v3":       {},
	"kling-v3-omni":  {},
}

type KlingAdaptor struct{}

type klingEnvelope struct {
	Code      interface{}     `json:"code"`
	Message   string          `json:"message"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
}

type klingTaskData struct {
	TaskID        string                 `json:"task_id"`
	TaskStatus    string                 `json:"task_status"`
	TaskStatusMsg string                 `json:"task_status_msg"`
	TaskInfo      map[string]interface{} `json:"task_info"`
	TaskResult    klingTaskResult        `json:"task_result"`
	CreatedAt     int64                  `json:"created_at"`
	UpdatedAt     int64                  `json:"updated_at"`
}

type klingTaskResult struct {
	Videos       []klingVideoResult `json:"videos"`
	Audios       []klingAudioResult `json:"audios"`
	Images       []klingImageResult `json:"images"`
	SeriesImages []klingImageResult `json:"series_images"`
	Voices       []klingVoiceResult `json:"voices"`
}

type klingVideoResult struct {
	ID           string `json:"id"`
	URL          string `json:"url"`
	WatermarkURL string `json:"watermark_url"`
	Duration     string `json:"duration"`
}

type klingAudioResult struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	URLMP3      string `json:"url_mp3"`
	URLWAV      string `json:"url_wav"`
	Duration    string `json:"duration"`
	DurationMP3 string `json:"duration_mp3"`
	DurationWAV string `json:"duration_wav"`
}

type klingImageResult struct {
	Index        int    `json:"index"`
	URL          string `json:"url"`
	WatermarkURL string `json:"watermark_url"`
}

type klingVoiceResult struct {
	VoiceID   string `json:"voice_id"`
	VoiceName string `json:"voice_name"`
	TrialURL  string `json:"trial_url"`
}

type klingIdentifyFaceData struct {
	SessionID string                   `json:"session_id"`
	FaceData  []map[string]interface{} `json:"face_data"`
}

type klingSingleTokenStream struct {
	token *dto.StreamToken
	done  bool
}

type klingPollingStream struct {
	adaptor *KlingAdaptor
	cfg     *ProviderConfig
	taskID  string
	last    string
}

func (a *KlingAdaptor) Chat(context.Context, *ProviderConfig, *dto.MediaRequest) (*dto.MediaResponse, error) {
	return nil, fmt.Errorf("kling adaptor does not support chat")
}

func (a *KlingAdaptor) Stream(context.Context, *ProviderConfig, *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("kling adaptor does not support chat stream")
}

func (a *KlingAdaptor) Media(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (*dto.MediaResponse, error) {
	if r == nil {
		return nil, fmt.Errorf("kling request is required")
	}

	mode, endpoint, err := a.resolveMode(r)
	if err != nil {
		return nil, err
	}

	payload, err := a.buildPayload(mode, r)
	if err != nil {
		return nil, err
	}

	var env klingEnvelope
	if err := a.doJSON(ctx, cfg, http.MethodPost, endpoint, payload, &env); err != nil {
		return nil, err
	}
	return klingMediaResponseFromEnvelope(mode, &env), nil
}

func (a *KlingAdaptor) StreamMedia(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (dto.TokenStream, error) {
	resp, err := a.Media(ctx, cfg, r)
	if err != nil {
		return nil, err
	}
	if resp.TaskID == "" {
		if resp.URL == "" && resp.Video.URL == "" {
			return nil, fmt.Errorf("kling did not return a task id or result url")
		}
		return &klingSingleTokenStream{
			token: &dto.StreamToken{
				Type: "url",
				URL:  firstNonEmptyString(resp.Video.URL, resp.URL),
				Text: resp.Status,
			},
		}, nil
	}
	return &klingPollingStream{adaptor: a, cfg: cfg, taskID: resp.TaskID, last: resp.Status}, nil
}

func (a *KlingAdaptor) TaskStatus(ctx context.Context, cfg *ProviderConfig, taskID string) (*dto.TaskStatusResponse, error) {
	var lastErr error
	for _, mode := range klingTaskModes {
		endpoint := klingModeEndpoint[mode] + "/" + url.PathEscape(taskID)
		var env klingEnvelope
		status, err := a.doJSONWithStatus(ctx, cfg, http.MethodGet, endpoint, nil, &env)
		if err == nil {
			resp := klingTaskStatusFromEnvelope(taskID, &env)
			if resp.Output.TaskID == "" {
				resp.Output.TaskID = taskID
			}
			return resp, nil
		}
		if status == http.StatusNotFound || status == http.StatusBadRequest {
			lastErr = err
			continue
		}
		return nil, err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("kling task status not found for task id: %s", taskID)
}

func (a *KlingAdaptor) ListTasks(ctx context.Context, cfg *ProviderConfig, query map[string]string) (*dto.TaskListResponse, error) {
	if mode := normalizeKlingMode(query["mode"]); mode != "" && mode != klingModeTTS && mode != klingModeIdentifyFace {
		return a.listTasksForMode(ctx, cfg, mode, query)
	}
	if mode := normalizeKlingMode(query["task_type"]); mode != "" && mode != klingModeTTS && mode != klingModeIdentifyFace {
		return a.listTasksForMode(ctx, cfg, mode, query)
	}

	items := make([]dto.TaskListItem, 0)
	for _, mode := range klingTaskModes {
		resp, err := a.listTasksForMode(ctx, cfg, mode, query)
		if err != nil {
			continue
		}
		items = append(items, resp.Items...)
	}
	return &dto.TaskListResponse{
		Items: items,
		Tasks: items,
		Total: len(items),
	}, nil
}

func (a *KlingAdaptor) listTasksForMode(ctx context.Context, cfg *ProviderConfig, mode string, query map[string]string) (*dto.TaskListResponse, error) {
	endpoint, ok := klingModeEndpoint[mode]
	if !ok {
		return nil, fmt.Errorf("unsupported kling list mode: %s", mode)
	}

	var env klingEnvelope
	if err := a.doJSON(ctx, cfg, http.MethodGet, endpoint+buildKlingQuery(query), nil, &env); err != nil {
		return nil, err
	}

	var tasks []klingTaskData
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &tasks); err != nil {
			return nil, err
		}
	}

	items := make([]dto.TaskListItem, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, dto.TaskListItem{
			ID:        task.TaskID,
			TaskID:    task.TaskID,
			State:     task.TaskStatus,
			CreatedAt: strconvMillis(task.CreatedAt),
			Payload:   task.TaskInfo,
		})
	}
	return &dto.TaskListResponse{
		Items: items,
		Tasks: items,
		Total: len(items),
	}, nil
}

func (a *KlingAdaptor) resolveMode(r *dto.MediaRequest) (string, string, error) {
	if mode := normalizeKlingMode(getStringExtra(r.Extra, "mode")); mode != "" {
		return mode, klingModeEndpoint[mode], nil
	}
	if mode := normalizeKlingMode(getStringExtra(r.Extra, "task_type")); mode != "" {
		return mode, klingModeEndpoint[mode], nil
	}
	if path := getStringExtra(r.Extra, "endpoint"); path != "" {
		mode := normalizeKlingMode(path)
		if mode != "" {
			return mode, klingModeEndpoint[mode], nil
		}
		if strings.HasPrefix(path, "/v1/") {
			return path, path, nil
		}
	}
	if mode := inferKlingDedicatedModeFromModel(r.Model); mode != "" {
		if endpoint, ok := klingModeEndpoint[mode]; ok {
			return mode, endpoint, nil
		}
	}

	switch r.Type {
	case dto.MediaTypeVideo:
		switch {
		case r.Extra["session_id"] != nil || r.Extra["face_choose"] != nil:
			return klingModeLipSync, klingModeEndpoint[klingModeLipSync], nil
		case klingFirstImageURL(r.Messages) != "" || klingFirstVideoURL(r.Messages) != "":
			if len(allImageURLs(r.Messages)) > 1 || r.Extra["image_list"] != nil {
				return klingModeMultiImageToVideo, klingModeEndpoint[klingModeMultiImageToVideo], nil
			}
			return klingModeImageToVideo, klingModeEndpoint[klingModeImageToVideo], nil
		case r.Extra["video_list"] != nil || r.Extra["element_list"] != nil:
			return klingModeOmniVideo, klingModeEndpoint[klingModeOmniVideo], nil
		default:
			return klingModeTextToVideo, klingModeEndpoint[klingModeTextToVideo], nil
		}
	case dto.MediaTypeText:
		if mode := inferKlingModeFromModel(r.Model, r.Type); mode != "" {
			return mode, klingModeEndpoint[mode], nil
		}
		return "", "", fmt.Errorf("kling text type requires a recognizable model")
	case dto.MediaTypeAudio:
		switch {
		case r.Extra["video_url"] != nil:
			return klingModeVideoToAudio, klingModeEndpoint[klingModeVideoToAudio], nil
		case r.Extra["voice_url"] != nil || r.Extra["voice_name"] != nil:
			return klingModeCustomVoices, klingModeEndpoint[klingModeCustomVoices], nil
		case r.Extra["voice_id"] != nil || r.Extra["text"] != nil:
			return klingModeTTS, klingModeEndpoint[klingModeTTS], nil
		default:
			return klingModeTextToAudio, klingModeEndpoint[klingModeTextToAudio], nil
		}
	case dto.MediaTypeImage:
		switch {
		case r.Extra["element_list"] != nil || r.Extra["image_list"] != nil:
			return klingModeOmniImage, klingModeEndpoint[klingModeOmniImage], nil
		case r.Extra["subject_image_list"] != nil || r.Extra["scene_image"] != nil || r.Extra["style_image"] != nil:
			return klingModeMultiImageToImage, klingModeEndpoint[klingModeMultiImageToImage], nil
		default:
			return klingModeImageGeneration, klingModeEndpoint[klingModeImageGeneration], nil
		}
	default:
		return "", "", fmt.Errorf("kling does not support media type: %s", r.Type)
	}
}

func normalizeKlingMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "omni-video", "omnivideo", "/v1/videos/omni-video":
		return klingModeOmniVideo
	case "text", "text2video", "text-to-video", "/v1/videos/text2video":
		return klingModeTextToVideo
	case "image", "image2video", "image-to-video", "/v1/videos/image2video":
		return klingModeImageToVideo
	case "multi-image", "multi-image2video", "multi-image-to-video", "/v1/videos/multi-image2video":
		return klingModeMultiImageToVideo
	case "text2audio", "text-to-audio", "/v1/audio/text-to-audio":
		return klingModeTextToAudio
	case "video2audio", "video-to-audio", "/v1/audio/video-to-audio":
		return klingModeVideoToAudio
	case "tts", "/v1/audio/tts":
		return klingModeTTS
	case "custom-voices", "voice-clone", "customvoices", "/v1/general/custom-voices":
		return klingModeCustomVoices
	case "identify-face", "identifyface", "/v1/videos/identify-face":
		return klingModeIdentifyFace
	case "lip-sync", "lipsync", "advanced-lip-sync", "/v1/videos/advanced-lip-sync":
		return klingModeLipSync
	case "omni-image", "omniimage", "/v1/images/omni-image":
		return klingModeOmniImage
	case "image-generation", "generations", "imagegeneration", "/v1/images/generations":
		return klingModeImageGeneration
	case "multi-image-to-image", "multi-image2image", "/v1/images/multi-image2image":
		return klingModeMultiImageToImage
	default:
		return ""
	}
}

func inferKlingModeFromModel(model string, reqType dto.MediaType) string {
	name := strings.TrimSpace(strings.ToLower(model))
	if name == "" {
		return ""
	}

	switch {
	case strings.HasPrefix(name, "kling-video-o"):
		return klingModeOmniVideo
	case strings.HasPrefix(name, "kling-image-o"):
		return klingModeOmniImage
	case strings.Contains(name, "tts"):
		return klingModeTTS
	case strings.Contains(name, "lip"):
		return klingModeLipSync
	case strings.Contains(name, "voice"):
		return klingModeCustomVoices
	}

	if isKnownKlingVideoModel(name) {
		switch reqType {
		case dto.MediaTypeVideo, dto.MediaTypeText:
			return klingModeTextToVideo
		}
	}
	if isKnownKlingImageModel(name) {
		switch reqType {
		case dto.MediaTypeImage, dto.MediaTypeText:
			return klingModeImageGeneration
		}
	}

	return ""
}

func inferKlingDedicatedModeFromModel(model string) string {
	name := strings.TrimSpace(strings.ToLower(model))
	if name == "" {
		return ""
	}

	switch {
	case strings.HasPrefix(name, "kling-video-o"):
		return klingModeOmniVideo
	case strings.HasPrefix(name, "kling-image-o"):
		return klingModeOmniImage
	case strings.Contains(name, "tts"):
		return klingModeTTS
	case strings.Contains(name, "lip"):
		return klingModeLipSync
	case strings.Contains(name, "voice"):
		return klingModeCustomVoices
	default:
		return ""
	}
}

func isKnownKlingVideoModel(model string) bool {
	_, ok := klingVideoModels[model]
	return ok
}

func isKnownKlingImageModel(model string) bool {
	_, ok := klingImageModels[model]
	return ok
}

func (a *KlingAdaptor) buildPayload(mode string, r *dto.MediaRequest) (map[string]interface{}, error) {
	payload := copyMap(extractPayloadMap(r.Extra))
	if payload == nil {
		payload = map[string]interface{}{}
	}
	mergeExtraPayload(payload, r.Extra, map[string]struct{}{
		"payload":   {},
		"mode":      {},
		"task_type": {},
		"endpoint":  {},
	})

	prompt := strings.TrimSpace(mediaPromptWithSystem(r))
	if explicitPrompt := getStringExtra(r.Extra, "prompt"); explicitPrompt != "" {
		prompt = explicitPrompt
	}
	if text := getStringExtra(r.Extra, "text"); text != "" && mode == klingModeTTS {
		payload["text"] = text
	}

	if r.Model != "" {
		if _, ok := payload["model_name"]; !ok {
			payload["model_name"] = r.Model
		}
	}
	if r.Size != "" {
		if _, ok := payload["aspect_ratio"]; !ok {
			payload["aspect_ratio"] = r.Size
		}
	}
	if r.Resolution != "" {
		if _, ok := payload["resolution"]; !ok {
			payload["resolution"] = r.Resolution
		}
	}
	if r.N > 0 {
		if _, ok := payload["n"]; !ok && supportsKlingN(mode) {
			payload["n"] = r.N
		}
	}

	switch mode {
	case klingModeTextToVideo, klingModeOmniVideo, klingModeImageToVideo, klingModeMultiImageToVideo, klingModeOmniImage, klingModeImageGeneration, klingModeMultiImageToImage:
		if prompt != "" {
			if _, ok := payload["prompt"]; !ok {
				payload["prompt"] = prompt
			}
		}
	}

	switch mode {
	case klingModeTextToVideo, klingModeOmniVideo, klingModeImageToVideo, klingModeMultiImageToVideo:
		if r.Duration > 0 {
			if _, ok := payload["duration"]; !ok {
				payload["duration"] = fmt.Sprintf("%d", r.Duration)
			}
		}
	case klingModeTextToAudio:
		if prompt != "" {
			payload["prompt"] = prompt
		}
		if r.Duration > 0 {
			if _, ok := payload["duration"]; !ok {
				payload["duration"] = r.Duration
			}
		}
	case klingModeTTS:
		if _, ok := payload["text"]; !ok && prompt != "" {
			payload["text"] = prompt
		}
	}

	switch mode {
	case klingModeImageToVideo:
		images := allImageURLs(r.Messages)
		if _, ok := payload["image"]; !ok && len(images) > 0 {
			payload["image"] = images[0]
		}
		if _, ok := payload["image_tail"]; !ok && len(images) > 1 {
			payload["image_tail"] = images[1]
		}
	case klingModeMultiImageToVideo:
		if _, ok := payload["image_list"]; !ok {
			payload["image_list"] = wrapImages(allImageURLs(r.Messages), "image")
		}
	case klingModeOmniVideo:
		if _, ok := payload["image_list"]; !ok {
			payload["image_list"] = wrapImages(allImageURLs(r.Messages), "image_url")
		}
		if _, ok := payload["video_list"]; !ok {
			payload["video_list"] = wrapVideos(allVideoURLs(r.Messages))
		}
	case klingModeOmniImage:
		if _, ok := payload["image_list"]; !ok {
			payload["image_list"] = wrapImages(allImageURLs(r.Messages), "image")
		}
	case klingModeImageGeneration:
		if _, ok := payload["image"]; !ok {
			if image := klingFirstImageURL(r.Messages); image != "" {
				payload["image"] = image
			}
		}
	case klingModeMultiImageToImage:
		if _, ok := payload["subject_image_list"]; !ok {
			payload["subject_image_list"] = wrapImages(allImageURLs(r.Messages), "subject_image")
		}
	}

	if mode == klingModeIdentifyFace {
		if _, ok := payload["video_url"]; !ok {
			if video := klingFirstVideoURL(r.Messages); video != "" {
				payload["video_url"] = video
			}
		}
	}
	if mode == klingModeVideoToAudio {
		if _, ok := payload["video_url"]; !ok {
			if video := klingFirstVideoURL(r.Messages); video != "" {
				payload["video_url"] = video
			}
		}
	}

	return payload, nil
}

func (a *KlingAdaptor) doJSON(ctx context.Context, cfg *ProviderConfig, method, path string, payload interface{}, out interface{}) error {
	_, err := a.doJSONWithStatus(ctx, cfg, method, path, payload, out)
	return err
}

func (a *KlingAdaptor) doJSONWithStatus(ctx context.Context, cfg *ProviderConfig, method, path string, payload interface{}, out interface{}) (int, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(klingBaseURL(cfg), "/")+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	for key, value := range cfg.Headers {
		req.Header.Set(key, value)
	}

	resp, err := klingHTTPClient(cfg).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr klingEnvelope
		if err := json.Unmarshal(raw, &apiErr); err == nil {
			return resp.StatusCode, fmt.Errorf("kling api error: status=%d code=%s message=%s request_id=%s", resp.StatusCode, strings.TrimSpace(fmt.Sprint(apiErr.Code)), apiErr.Message, apiErr.RequestID)
		}
		return resp.StatusCode, fmt.Errorf("kling api error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func (w *klingSingleTokenStream) Next(context.Context) (*dto.StreamToken, error) {
	if w.done {
		return nil, io.EOF
	}
	w.done = true
	return w.token, io.EOF
}

func (w *klingSingleTokenStream) Close() error { return nil }

func (w *klingPollingStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	for {
		status, err := w.adaptor.TaskStatus(ctx, w.cfg, w.taskID)
		if err != nil {
			return nil, err
		}
		state := status.Output.TaskStatus
		if state == w.last && state != "succeed" && state != "failed" {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				continue
			}
		}
		w.last = state
		token := &dto.StreamToken{Type: "progress", Text: state}
		switch state {
		case "succeed":
			token.Type = "url"
			token.URL = firstNonEmptyString(status.Output.URL, status.Output.VideoURL)
			return token, io.EOF
		case "failed":
			if status.Output.Message != "" {
				token.Text = status.Output.Message
			}
			return token, fmt.Errorf("kling task failed: %s", token.Text)
		default:
			return token, nil
		}
	}
}

func (w *klingPollingStream) Close() error { return nil }

func klingMediaResponseFromEnvelope(mode string, env *klingEnvelope) *dto.MediaResponse {
	resp := &dto.MediaResponse{
		RequestID:    env.RequestID,
		ErrorCode:    strings.TrimSpace(fmt.Sprint(env.Code)),
		ErrorMessage: env.Message,
	}
	if len(env.Data) == 0 {
		return resp
	}

	if mode == klingModeIdentifyFace {
		var data klingIdentifyFaceData
		if err := json.Unmarshal(env.Data, &data); err == nil {
			resp.TaskID = data.SessionID
			resp.Status = "succeed"
			if len(data.FaceData) > 0 {
				resp.Text = "identify-face"
			}
			return resp
		}
	}

	var data klingTaskData
	if err := json.Unmarshal(env.Data, &data); err == nil {
		resp.TaskID = data.TaskID
		resp.Status = data.TaskStatus
		resp.URL = firstKlingResultURL(data.TaskResult)
		if len(data.TaskResult.Videos) > 0 {
			resp.Video.URL = firstNonEmptyString(data.TaskResult.Videos[0].URL, data.TaskResult.Videos[0].WatermarkURL)
		}
		if mode == klingModeCustomVoices && len(data.TaskResult.Voices) > 0 {
			resp.Text = data.TaskResult.Voices[0].VoiceID
		}
		return resp
	}

	return resp
}

func klingTaskStatusFromEnvelope(taskID string, env *klingEnvelope) *dto.TaskStatusResponse {
	resp := &dto.TaskStatusResponse{
		RequestID: env.RequestID,
		Output: dto.TaskStatusOutput{
			TaskID:  taskID,
			Code:    strings.TrimSpace(fmt.Sprint(env.Code)),
			Message: env.Message,
		},
	}

	var data klingTaskData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return resp
	}

	resp.Output.TaskID = firstNonEmptyString(data.TaskID, taskID)
	resp.Output.TaskStatus = data.TaskStatus
	resp.Output.Message = firstNonEmptyString(data.TaskStatusMsg, env.Message)
	resp.Output.URL = firstKlingResultURL(data.TaskResult)
	if len(data.TaskResult.Videos) > 0 {
		resp.Output.VideoURL = firstNonEmptyString(data.TaskResult.Videos[0].URL, data.TaskResult.Videos[0].WatermarkURL)
	}
	resp.Output.SubmitTime = strconvMillis(data.CreatedAt)
	resp.Output.EndTime = strconvMillis(data.UpdatedAt)
	if externalID, ok := data.TaskInfo["external_task_id"].(string); ok {
		resp.Output.OrigPrompt = externalID
	}
	return resp
}

func firstKlingResultURL(result klingTaskResult) string {
	if len(result.Videos) > 0 {
		return firstNonEmptyString(result.Videos[0].URL, result.Videos[0].WatermarkURL)
	}
	if len(result.Audios) > 0 {
		return firstNonEmptyString(result.Audios[0].URL, result.Audios[0].URLMP3, result.Audios[0].URLWAV)
	}
	if len(result.Images) > 0 {
		return firstNonEmptyString(result.Images[0].URL, result.Images[0].WatermarkURL)
	}
	if len(result.SeriesImages) > 0 {
		return firstNonEmptyString(result.SeriesImages[0].URL, result.SeriesImages[0].WatermarkURL)
	}
	if len(result.Voices) > 0 {
		return result.Voices[0].TrialURL
	}
	return ""
}

func klingBaseURL(cfg *ProviderConfig) string {
	if cfg != nil && cfg.BaseURL != "" {
		return cfg.BaseURL
	}
	return "https://api-beijing.klingai.com"
}

func klingHTTPClient(cfg *ProviderConfig) *http.Client {
	if cfg != nil && cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}
	client := &http.Client{}
	if cfg != nil && cfg.Timeout > 0 {
		client.Timeout = cfg.Timeout
	}
	return client
}

func buildKlingQuery(query map[string]string) string {
	if len(query) == 0 {
		return ""
	}
	values := url.Values{}
	for key, value := range query {
		if value == "" {
			continue
		}
		switch key {
		case "mode", "task_type", "endpoint":
			continue
		default:
			values.Set(key, value)
		}
	}
	encoded := values.Encode()
	if encoded == "" {
		return ""
	}
	return "?" + encoded
}

func wrapImages(urls []string, field string) []map[string]interface{} {
	if len(urls) == 0 {
		return nil
	}
	items := make([]map[string]interface{}, 0, len(urls))
	for _, imageURL := range urls {
		if imageURL == "" {
			continue
		}
		items = append(items, map[string]interface{}{field: imageURL})
	}
	return items
}

func wrapVideos(urls []string) []map[string]interface{} {
	if len(urls) == 0 {
		return nil
	}
	items := make([]map[string]interface{}, 0, len(urls))
	for _, videoURL := range urls {
		if videoURL == "" {
			continue
		}
		items = append(items, map[string]interface{}{"video_url": videoURL})
	}
	return items
}

func klingFirstImageURL(messages []dto.Message) string {
	for _, message := range messages {
		if message.ImageURL != "" {
			return message.ImageURL
		}
	}
	return ""
}

func allImageURLs(messages []dto.Message) []string {
	out := make([]string, 0, len(messages))
	for _, message := range messages {
		if message.ImageURL != "" {
			out = append(out, message.ImageURL)
		}
	}
	return out
}

func klingFirstVideoURL(messages []dto.Message) string {
	for _, message := range messages {
		if message.VideoURL != "" {
			return message.VideoURL
		}
	}
	return ""
}

func allVideoURLs(messages []dto.Message) []string {
	out := make([]string, 0, len(messages))
	for _, message := range messages {
		if message.VideoURL != "" {
			out = append(out, message.VideoURL)
		}
	}
	return out
}

func copyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func supportsKlingN(mode string) bool {
	switch mode {
	case klingModeOmniImage, klingModeImageGeneration, klingModeMultiImageToImage:
		return true
	default:
		return false
	}
}

func strconvMillis(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.UnixMilli(ts).UTC().Format(time.RFC3339)
}
