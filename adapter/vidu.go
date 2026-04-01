// Package adapter provides Vidu adaptor implementation.
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
	viduModeText           = "text-to-video"
	viduModeImage          = "image-to-video"
	viduModeReference      = "reference-to-video"
	viduModeStartEnd       = "start-end-to-video"
	viduModeMulti          = "multi-frame"
	viduModeReferenceImage = "reference-to-image"
	viduModeTextAudio      = "text-to-audio"
	viduModeTimingAudio    = "timing-to-audio"
	viduModeTextToSpeech   = "text-to-speech"
	viduModeVoiceClone     = "voice-clone"
	viduModeLipSync        = "lip-sync"
)

var viduModeEndpoint = map[string]string{
	viduModeText:           "/ent/v2/text2video",
	viduModeImage:          "/ent/v2/img2video",
	viduModeReference:      "/ent/v2/reference2video",
	viduModeStartEnd:       "/ent/v2/start-end2video",
	viduModeMulti:          "/ent/v2/multiframe",
	viduModeReferenceImage: "/ent/v2/reference2image",
	viduModeTextAudio:      "/ent/v2/text2audio",
	viduModeTimingAudio:    "/ent/v2/timing2audio",
	viduModeTextToSpeech:   "/ent/v2/audio-tts",
	viduModeVoiceClone:     "/ent/v2/audio-clone",
	viduModeLipSync:        "/ent/v2/lip-sync",
}

var (
	viduVideoModels = map[string]struct{}{
		"viduq2":  {},
		"viduq1":  {},
		"vidu1.5": {},
		"vidu2.0": {},
	}
	viduImageModels = map[string]struct{}{
		"viduq1":  {},
		"vidu2.0": {},
	}
	viduTextAudioModels = map[string]struct{}{
		"audio1.0": {},
	}
	viduExactModelModes = map[string]string{
		"text-to-audio":   viduModeTextAudio,
		"timing-to-audio": viduModeTimingAudio,
		"text-to-speech":  viduModeTextToSpeech,
		"audio-tts":       viduModeTextToSpeech,
		"voice-clone":     viduModeVoiceClone,
		"audio-clone":     viduModeVoiceClone,
		"lip-sync":        viduModeLipSync,
	}
)

type ViduAdaptor struct{}

type viduErrorResponse struct {
	Code     interface{} `json:"code"`
	Reason   string      `json:"reason"`
	Message  string      `json:"message"`
	Metadata struct {
		TraceID string `json:"trace_id"`
	} `json:"metadata"`
}

type viduCreateResponse struct {
	TaskID            string      `json:"task_id"`
	ID                string      `json:"id"`
	State             string      `json:"state"`
	Model             string      `json:"model"`
	Prompt            string      `json:"prompt"`
	Images            []string    `json:"images"`
	Duration          int         `json:"duration"`
	Seed              int         `json:"seed"`
	Resolution        string      `json:"resolution"`
	BGM               bool        `json:"bgm"`
	MovementAmplitude string      `json:"movement_amplitude"`
	Payload           interface{} `json:"payload"`
	CreatedAt         string      `json:"created_at"`
}

type viduTaskResponse struct {
	ID        string               `json:"id"`
	State     string               `json:"state"`
	ErrCode   interface{}          `json:"err_code"`
	Message   string               `json:"message"`
	Credits   int                  `json:"credits"`
	Payload   interface{}          `json:"payload"`
	Creations []viduCreationResult `json:"creations"`
	CreatedAt string               `json:"created_at"`
}

type viduTaskListResponse struct {
	Items    []dto.TaskListItem `json:"items"`
	Tasks    []dto.TaskListItem `json:"tasks"`
	Total    int                `json:"total"`
	PageNum  int                `json:"page_num"`
	PageSize int                `json:"page_size"`
	HasMore  bool               `json:"has_more"`
}

type viduCreationResult struct {
	ID             string `json:"id"`
	URL            string `json:"url"`
	CoverURL       string `json:"cover_url"`
	WatermarkedURL string `json:"watermarked_url"`
}

type viduPollingStream struct {
	adaptor *ViduAdaptor
	cfg     *ProviderConfig
	taskID  string
	last    string
}

func (a *ViduAdaptor) Chat(context.Context, *ProviderConfig, *dto.MediaRequest) (*dto.MediaResponse, error) {
	return nil, fmt.Errorf("vidu adaptor does not support chat")
}

func (a *ViduAdaptor) Stream(context.Context, *ProviderConfig, *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("vidu adaptor does not support chat stream")
}

func (a *ViduAdaptor) Media(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (*dto.MediaResponse, error) {
	if r == nil {
		return nil, fmt.Errorf("vidu request is required")
	}

	mode, endpoint, err := a.resolveMode(r)
	if err != nil {
		return nil, err
	}

	payload, err := a.buildPayload(mode, r)
	if err != nil {
		return nil, err
	}

	var out viduCreateResponse
	if err := a.doJSON(ctx, cfg, http.MethodPost, endpoint, payload, &out); err != nil {
		return nil, err
	}

	taskID := out.TaskID
	if taskID == "" {
		taskID = out.ID
	}
	return &dto.MediaResponse{
		TaskID: taskID,
		Status: out.State,
		Model:  out.Model,
		Text:   out.Prompt,
	}, nil
}

func (a *ViduAdaptor) StreamMedia(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (dto.TokenStream, error) {
	resp, err := a.Media(ctx, cfg, r)
	if err != nil {
		return nil, err
	}
	if resp.TaskID == "" {
		return nil, fmt.Errorf("vidu did not return a task id")
	}
	return &viduPollingStream{adaptor: a, cfg: cfg, taskID: resp.TaskID, last: resp.Status}, nil
}

func (a *ViduAdaptor) TaskStatus(ctx context.Context, cfg *ProviderConfig, taskID string, _ ...map[string]string) (*dto.TaskStatusResponse, error) {
	var out viduTaskResponse
	if err := a.doJSON(ctx, cfg, http.MethodGet, "/ent/v2/tasks/"+taskID+"/creations", nil, &out); err != nil {
		return nil, err
	}

	res := &dto.TaskStatusResponse{
		Output: dto.TaskStatusOutput{
			TaskID:     out.ID,
			TaskStatus: out.State,
			Code:       stringifyViduCode(out.ErrCode),
			Message:    out.Message,
		},
	}
	if res.Output.TaskID == "" {
		res.Output.TaskID = taskID
	}
	if len(out.Creations) > 0 {
		res.Output.URL = firstNonEmptyString(
			out.Creations[0].URL,
			out.Creations[0].WatermarkedURL,
		)
		res.Output.VideoURL = res.Output.URL
	}
	return res, nil
}

func (a *ViduAdaptor) ListTasks(ctx context.Context, cfg *ProviderConfig, query map[string]string) (*dto.TaskListResponse, error) {
	var out viduTaskListResponse
	if err := a.doJSON(ctx, cfg, http.MethodGet, "/ent/v2/tasks"+buildViduQuery(query), nil, &out); err != nil {
		return nil, err
	}
	return &dto.TaskListResponse{
		Items:    out.Items,
		Tasks:    out.Tasks,
		Total:    out.Total,
		PageNum:  out.PageNum,
		PageSize: out.PageSize,
		HasMore:  out.HasMore,
	}, nil
}

func (a *ViduAdaptor) resolveMode(r *dto.MediaRequest) (string, string, error) {
	if mode := normalizeViduMode(getStringExtra(r.Extra, "mode")); mode != "" {
		return mode, viduModeEndpoint[mode], nil
	}
	if mode := normalizeViduMode(getStringExtra(r.Extra, "task_type")); mode != "" {
		return mode, viduModeEndpoint[mode], nil
	}
	if mode := normalizeViduMode(getStringExtra(r.Extra, "endpoint")); mode != "" {
		return mode, viduModeEndpoint[mode], nil
	}
	if mode := viduModelMode(r.Model, r.Type); mode != "" {
		return mode, viduModeEndpoint[mode], nil
	}
	switch r.Type {
	case dto.MediaTypeVideo:
		if a.looksLikeLipSync(r) {
			return viduModeLipSync, viduModeEndpoint[viduModeLipSync], nil
		}
		if rawSubjects, ok := r.Extra["subjects"]; ok && rawSubjects != nil {
			return viduModeReference, viduModeEndpoint[viduModeReference], nil
		}
		if rawFrames, ok := r.Extra["frames"]; ok && rawFrames != nil {
			return viduModeMulti, viduModeEndpoint[viduModeMulti], nil
		}
		if getStringExtra(r.Extra, "start_image") != "" || getStringExtra(r.Extra, "end_image") != "" {
			return viduModeStartEnd, viduModeEndpoint[viduModeStartEnd], nil
		}
		images := a.collectImages(r)
		switch {
		case len(images) >= 2:
			return viduModeStartEnd, viduModeEndpoint[viduModeStartEnd], nil
		case len(images) == 1:
			return viduModeImage, viduModeEndpoint[viduModeImage], nil
		default:
			return viduModeText, viduModeEndpoint[viduModeText], nil
		}
	case dto.MediaTypeImage:
		return viduModeReferenceImage, viduModeEndpoint[viduModeReferenceImage], nil
	case dto.MediaTypeAudio:
		if a.looksLikeTimingAudio(r) {
			return viduModeTimingAudio, viduModeEndpoint[viduModeTimingAudio], nil
		}
		if a.looksLikeTextToSpeech(r) {
			return viduModeTextToSpeech, viduModeEndpoint[viduModeTextToSpeech], nil
		}
		if a.looksLikeVoiceClone(r) {
			return viduModeVoiceClone, viduModeEndpoint[viduModeVoiceClone], nil
		}
		return viduModeTextAudio, viduModeEndpoint[viduModeTextAudio], nil
	case dto.MediaTypeText:
		if a.looksLikeTimingAudio(r) {
			return viduModeTimingAudio, viduModeEndpoint[viduModeTimingAudio], nil
		}
		if a.looksLikeTextToSpeech(r) {
			return viduModeTextToSpeech, viduModeEndpoint[viduModeTextToSpeech], nil
		}
		if a.looksLikeVoiceClone(r) {
			return viduModeVoiceClone, viduModeEndpoint[viduModeVoiceClone], nil
		}
		return viduModeTextAudio, viduModeEndpoint[viduModeTextAudio], nil
	default:
		return "", "", fmt.Errorf("vidu does not support media type: %s", r.Type)
	}
}

func normalizeViduMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "text", "text2video", "text-to-video":
		return viduModeText
	case "image", "img2video", "image-to-video":
		return viduModeImage
	case "reference", "reference2video", "reference-to-video":
		return viduModeReference
	case "start-end", "start_end", "start-end2video", "start-end-to-video":
		return viduModeStartEnd
	case "multi", "multiframe", "multi-frame":
		return viduModeMulti
	case "reference-image", "reference2image", "reference-to-image":
		return viduModeReferenceImage
	case "audio", "text2audio", "text-to-audio":
		return viduModeTextAudio
	case "timing-audio", "timing2audio", "timing-to-audio":
		return viduModeTimingAudio
	case "tts", "audio-tts", "text-to-speech":
		return viduModeTextToSpeech
	case "clone", "audio-clone", "voice-clone":
		return viduModeVoiceClone
	case "lip-sync", "lipsync":
		return viduModeLipSync
	default:
		return ""
	}
}

func viduModelMode(model string, mediaType dto.MediaType) string {
	normalized := strings.TrimSpace(strings.ToLower(model))
	if mode, ok := viduExactModelModes[normalized]; ok {
		return mode
	}
	switch mediaType {
	case dto.MediaTypeVideo:
		if _, ok := viduVideoModels[normalized]; ok {
			return ""
		}
	case dto.MediaTypeImage:
		if _, ok := viduImageModels[normalized]; ok {
			return viduModeReferenceImage
		}
	case dto.MediaTypeAudio, dto.MediaTypeText:
		if _, ok := viduTextAudioModels[normalized]; ok {
			return viduModeTextAudio
		}
	}
	return ""
}

func (a *ViduAdaptor) looksLikeTimingAudio(r *dto.MediaRequest) bool {
	if r == nil || r.Extra == nil {
		return false
	}
	if _, ok := r.Extra["segments"]; ok {
		return true
	}
	if _, ok := r.Extra["timings"]; ok {
		return true
	}
	return false
}

func (a *ViduAdaptor) looksLikeTextToSpeech(r *dto.MediaRequest) bool {
	if r == nil || r.Extra == nil {
		return false
	}
	if getStringExtra(r.Extra, "text") == "" {
		return false
	}
	for _, key := range []string{"voice", "voice_id", "speaker", "speaker_id"} {
		if getStringExtra(r.Extra, key) != "" {
			return true
		}
	}
	return false
}

func (a *ViduAdaptor) looksLikeVoiceClone(r *dto.MediaRequest) bool {
	if r == nil || r.Extra == nil {
		return false
	}
	for _, key := range []string{"audio_url", "audio_file_url", "sample_audio_url", "reference_audio_url"} {
		if getStringExtra(r.Extra, key) != "" {
			return true
		}
	}
	return false
}

func (a *ViduAdaptor) looksLikeLipSync(r *dto.MediaRequest) bool {
	if r == nil || r.Extra == nil {
		return false
	}
	hasVideo := getStringExtra(r.Extra, "video_url") != "" || getStringExtra(r.Extra, "input_video") != ""
	hasAudio := getStringExtra(r.Extra, "audio_url") != "" || getStringExtra(r.Extra, "voice_url") != ""
	return hasVideo && hasAudio
}

func (a *ViduAdaptor) buildPayload(mode string, r *dto.MediaRequest) (map[string]interface{}, error) {
	payload := make(map[string]interface{})
	consumed := map[string]struct{}{"mode": {}, "task_type": {}, "endpoint": {}}
	if r.Model != "" {
		payload["model"] = r.Model
	}
	prompt := strings.TrimSpace(mediaPromptWithSystem(r))
	if explicitPrompt := getStringExtra(r.Extra, "prompt"); explicitPrompt != "" {
		prompt = explicitPrompt
	}
	if prompt != "" && mode != viduModeMulti {
		payload["prompt"] = prompt
	}
	if r.Duration > 0 {
		payload["duration"] = r.Duration
	}
	if r.Seed != 0 {
		payload["seed"] = r.Seed
	}
	if r.Resolution != "" {
		payload["resolution"] = r.Resolution
	}
	if r.Size != "" {
		payload["aspect_ratio"] = r.Size
	}
	if style := getStringExtra(r.Extra, "style"); style != "" {
		payload["style"] = style
		consumed["style"] = struct{}{}
	}
	if movement := getStringExtra(r.Extra, "movement_amplitude"); movement != "" {
		payload["movement_amplitude"] = movement
		consumed["movement_amplitude"] = struct{}{}
	}
	if callbackURL := getStringExtra(r.Extra, "callback_url"); callbackURL != "" {
		payload["callback_url"] = callbackURL
		consumed["callback_url"] = struct{}{}
	}
	if metaData, ok := r.Extra["meta_data"]; ok && metaData != nil {
		payload["meta_data"] = stringifyViduValue(metaData)
		consumed["meta_data"] = struct{}{}
	}
	if passthrough, ok := r.Extra["payload"]; ok && passthrough != nil {
		payload["payload"] = stringifyViduValue(passthrough)
		consumed["payload"] = struct{}{}
	}
	for _, key := range []string{"bgm", "off_peak", "watermark"} {
		if value, ok := getBoolExtra(r.Extra, key); ok {
			payload[key] = value
			consumed[key] = struct{}{}
		}
	}
	if pos, ok := intValue(r.Extra["wm_position"]); ok {
		payload["wm_position"] = pos
		consumed["wm_position"] = struct{}{}
	}
	if wmURL := getStringExtra(r.Extra, "wm_url"); wmURL != "" {
		payload["wm_url"] = wmURL
		consumed["wm_url"] = struct{}{}
	}

	switch mode {
	case viduModeText:
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case viduModeImage:
		images := a.collectImages(r)
		if len(images) == 0 {
			return nil, fmt.Errorf("vidu image-to-video requires image input")
		}
		payload["images"] = images
		consumed["image"] = struct{}{}
		consumed["images"] = struct{}{}
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case viduModeStartEnd:
		images := a.collectStartEndImages(r)
		if len(images) < 2 {
			return nil, fmt.Errorf("vidu start-end-to-video requires start and end images")
		}
		payload["images"] = images[:2]
		consumed["image"] = struct{}{}
		consumed["images"] = struct{}{}
		consumed["start_image"] = struct{}{}
		consumed["end_image"] = struct{}{}
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case viduModeReference:
		subjects := a.collectSubjects(r)
		if len(subjects) == 0 {
			return nil, fmt.Errorf("vidu reference-to-video requires subjects")
		}
		payload["subjects"] = subjects
		consumed["image"] = struct{}{}
		consumed["images"] = struct{}{}
		consumed["subjects"] = struct{}{}
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case viduModeMulti:
		frames := a.collectFrames(r)
		if len(frames) == 0 {
			return nil, fmt.Errorf("vidu multi-frame requires frames")
		}
		payload["frames"] = frames
		if prompt != "" {
			payload["prompt"] = prompt
		}
		consumed["frames"] = struct{}{}
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case viduModeReferenceImage:
		subjects := a.collectSubjects(r)
		if len(subjects) == 0 {
			return nil, fmt.Errorf("vidu reference-to-image requires subjects")
		}
		payload["subjects"] = subjects
		if r.N > 0 {
			payload["count"] = r.N
		}
		consumed["image"] = struct{}{}
		consumed["images"] = struct{}{}
		consumed["subjects"] = struct{}{}
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case viduModeTextAudio:
		if prompt != "" {
			payload["prompt"] = prompt
		}
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case viduModeTimingAudio:
		if prompt != "" {
			payload["prompt"] = prompt
		}
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case viduModeTextToSpeech:
		if text := getStringExtra(r.Extra, "text"); text != "" {
			payload["text"] = text
			consumed["text"] = struct{}{}
		} else if prompt != "" {
			payload["text"] = prompt
		}
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case viduModeVoiceClone:
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	case viduModeLipSync:
		mergeExtraPayload(payload, r.Extra, consumed)
		return payload, nil
	default:
		return nil, fmt.Errorf("unsupported vidu mode: %s", mode)
	}
}

func (a *ViduAdaptor) collectImages(r *dto.MediaRequest) []string {
	if urls := contentImageURLs(r.Extra["images"]); len(urls) > 0 {
		return urls
	}
	if url, ok := contentImageURL(r.Extra["image"]); ok {
		return []string{url}
	}
	var out []string
	for _, message := range r.Messages {
		if message.ImageURL != "" {
			out = append(out, message.ImageURL)
		}
	}
	return out
}

func (a *ViduAdaptor) collectStartEndImages(r *dto.MediaRequest) []string {
	start := getStringExtra(r.Extra, "start_image")
	end := getStringExtra(r.Extra, "end_image")
	if start != "" || end != "" {
		return compactStrings([]string{start, end})
	}
	return a.collectImages(r)
}

func (a *ViduAdaptor) collectSubjects(r *dto.MediaRequest) []map[string]interface{} {
	if raw, ok := r.Extra["subjects"]; ok && raw != nil {
		switch typed := raw.(type) {
		case []map[string]interface{}:
			return typed
		case []interface{}:
			out := make([]map[string]interface{}, 0, len(typed))
			for _, item := range typed {
				if subject, ok := item.(map[string]interface{}); ok && len(subject) > 0 {
					out = append(out, subject)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	if images := a.collectImages(r); len(images) > 0 {
		return []map[string]interface{}{{"images": images}}
	}
	return nil
}

func (a *ViduAdaptor) collectFrames(r *dto.MediaRequest) []map[string]interface{} {
	raw, ok := r.Extra["frames"]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			switch frame := item.(type) {
			case map[string]interface{}:
				if len(frame) > 0 {
					out = append(out, frame)
				}
			case string:
				if frame != "" {
					out = append(out, map[string]interface{}{"image": frame})
				}
			}
		}
		return out
	default:
		return nil
	}
}

func (a *ViduAdaptor) doJSON(ctx context.Context, cfg *ProviderConfig, method, path string, payload interface{}, out interface{}) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(viduBaseURL(cfg), "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	for key, value := range cfg.Headers {
		req.Header.Set(key, value)
	}

	resp, err := viduHTTPClient(cfg).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr viduErrorResponse
		if err := json.Unmarshal(raw, &apiErr); err == nil && (apiErr.Message != "" || apiErr.Reason != "" || apiErr.Code != nil) {
			return fmt.Errorf("vidu api error: code=%s reason=%s message=%s trace_id=%s", stringifyViduCode(apiErr.Code), apiErr.Reason, apiErr.Message, apiErr.Metadata.TraceID)
		}
		return fmt.Errorf("vidu api error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (w *viduPollingStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	for {
		status, err := w.adaptor.TaskStatus(ctx, w.cfg, w.taskID)
		if err != nil {
			return nil, err
		}
		state := status.Output.TaskStatus
		if state == w.last && state != "success" && state != "failed" {
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
		case "success":
			token.Type = "url"
			token.URL = status.Output.VideoURL
			return token, io.EOF
		case "failed":
			if status.Output.Message != "" {
				token.Text = status.Output.Message
			}
			return token, fmt.Errorf("vidu task failed: %s", token.Text)
		default:
			return token, nil
		}
	}
}

func (w *viduPollingStream) Close() error { return nil }

func viduBaseURL(cfg *ProviderConfig) string {
	if cfg != nil && cfg.BaseURL != "" {
		return cfg.BaseURL
	}
	return "https://api.vidu.cn"
}

func viduHTTPClient(cfg *ProviderConfig) *http.Client {
	if cfg != nil && cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}
	client := &http.Client{}
	if cfg != nil && cfg.Timeout > 0 {
		client.Timeout = cfg.Timeout
	}
	return client
}

func stringifyViduCode(v interface{}) string {
	return strings.TrimSpace(fmt.Sprint(v))
}

func stringifyViduValue(v interface{}) string {
	switch typed := v.(type) {
	case string:
		return typed
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(data)
	}
}

func compactStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			out = append(out, item)
		}
	}
	return out
}

func mergeExtraPayload(dst, extra map[string]interface{}, consumed map[string]struct{}) {
	for key, value := range extra {
		if _, ok := consumed[key]; ok {
			continue
		}
		if value == nil {
			continue
		}
		if _, exists := dst[key]; !exists {
			dst[key] = value
		}
	}
}

func firstNonEmptyString(items ...string) string {
	for _, item := range items {
		if item != "" {
			return item
		}
	}
	return ""
}

func buildViduQuery(query map[string]string) string {
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
