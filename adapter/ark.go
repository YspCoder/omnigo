// Package adapter provides Volcengine Ark (火山方舟) adaptor implementation.
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model/responses"
	arkutils "github.com/volcengine/volcengine-go-sdk/service/arkruntime/utils"
	"github.com/volcengine/volcengine-go-sdk/volcengine"
)

type ArkAdaptor struct {
	client *arkruntime.Client
}

func (a *ArkAdaptor) getClient(config *ProviderConfig) *arkruntime.Client {
	if a.client != nil {
		return a.client
	}
	opts := []arkruntime.ConfigOption{arkruntime.WithRegion(config.Region)}
	if config.BaseURL != "" {
		opts = append(opts, arkruntime.WithBaseUrl(config.BaseURL))
	}
	if config.HTTPClient != nil {
		opts = append(opts, arkruntime.WithHTTPClient(config.HTTPClient))
	}
	if config.APIKey != "" {
		a.client = arkruntime.NewClientWithApiKey(config.APIKey, opts...)
	} else {
		a.client = arkruntime.NewClientWithAkSk(config.AccessKey, config.SecretKey, opts...)
	}
	return a.client
}

func (a *ArkAdaptor) Chat(ctx context.Context, config *ProviderConfig, r *dto.MediaRequest) (*dto.MediaResponse, error) {
	chatReq := a.toChatReq(r)
	resp, err := a.getClient(config).CreateResponses(ctx, chatReq)
	if err != nil {
		return nil, err
	}
	if resp.GetError() != nil && strings.TrimSpace(resp.GetError().GetMessage()) != "" {
		return nil, fmt.Errorf("ark responses api error: %s", resp.GetError().GetMessage())
	}
	text := arkResponsesText(resp)
	msg := dto.Message{
		Role:    arkResponsesRole(resp),
		Content: text,
	}
	res := &dto.MediaResponse{
		ID:      resp.GetId(),
		Created: resp.GetCreatedAt(),
		Model:   resp.GetModel(),
		Text:    text,
		Choices: []dto.ChatChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: arkResponsesFinishReason(resp),
		}},
		Usage: arkResponsesUsage(resp.GetUsage()),
	}
	return res, nil
}

func (a *ArkAdaptor) Stream(ctx context.Context, config *ProviderConfig, r *dto.MediaRequest) (dto.TokenStream, error) {
	chatReq := a.toChatReq(r)
	s, err := a.getClient(config).CreateResponsesStream(ctx, chatReq)
	if err != nil {
		return nil, err
	}
	return &arkStream{s}, nil
}

type arkUploadedFile struct {
	ID       string
	FileName string
}

func (a *ArkAdaptor) Media(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (*dto.MediaResponse, error) {
	c := a.getClient(cfg)
	if r.Type == dto.MediaTypeImage {
		resp, err := c.GenerateImages(ctx, a.toImgReq(r))
		if err != nil {
			return nil, err
		}
		res := &dto.MediaResponse{Created: resp.Created}
		for _, d := range resp.Data {
			res.Data = append(res.Data, dto.ImageData{URL: volcengine.StringValue(d.Url), B64JSON: volcengine.StringValue(d.B64Json)})
		}
		if len(res.Data) > 0 {
			res.URL = res.Data[0].URL
		}
		return res, nil
	}
	resp, err := c.CreateContentGenerationTask(ctx, a.toVidReq(r))
	if err != nil {
		return nil, err
	}
	return &dto.MediaResponse{TaskID: resp.ID}, nil
}

func (a *ArkAdaptor) StreamMedia(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (dto.TokenStream, error) {
	c := a.getClient(cfg)
	if r.Type == dto.MediaTypeImage {
		s, err := c.GenerateImagesStreaming(ctx, a.toImgReq(r))
		return &arkMediaStream{s}, err
	}
	resp, err := c.CreateContentGenerationTask(ctx, a.toVidReq(r))
	if err != nil {
		return nil, err
	}
	return &arkVidStream{c, resp.ID, ""}, nil
}

func (a *ArkAdaptor) TaskStatus(ctx context.Context, cfg *ProviderConfig, id string, _ ...map[string]string) (*dto.TaskStatusResponse, error) {
	resp, err := a.getClient(cfg).GetContentGenerationTask(ctx, model.GetContentGenerationTaskRequest{ID: id})
	if err != nil {
		return nil, err
	}
	res := &dto.TaskStatusResponse{
		Usage: &dto.TaskStatusUsage{},
		Output: dto.TaskStatusOutput{
			TaskID:       resp.ID,
			TaskStatus:   resp.Status,
			SubmitTime:   formatUnixMillis(resp.CreatedAt),
			EndTime:      formatUnixMillis(resp.UpdatedAt),
			VideoURL:     resp.Content.VideoURL,
			LastFrameURL: resp.Content.LastFrameURL,
			ActualPrompt: volcengine.StringValue(resp.RevisedPrompt),
			Resolution:   volcengine.StringValue(resp.Resolution),
			Ratio:        volcengine.StringValue(resp.Ratio),
			Duration:     int64ToInt(resp.Duration),
			Seed:         int64ToInt(resp.Seed),
			ServiceTier:  volcengine.StringValue(resp.ServiceTier),
		},
	}
	if res.Output.VideoURL == "" {
		res.Output.VideoURL = resp.Content.FileURL
	}
	res.Output.URL = firstNonEmptyString(res.Output.VideoURL, res.Output.LastFrameURL)
	if resp.Error != nil {
		res.Output.Code, res.Output.Message = resp.Error.Code, resp.Error.Message
	}
	res.Usage.VideoDuration = int64ToInt(resp.Duration)
	res.Usage.VideoCount = 1
	if resp.Status == model.StatusFailed {
		res.Usage = nil
	}
	return res, nil
}

func (a *ArkAdaptor) ListTasks(ctx context.Context, cfg *ProviderConfig, query map[string]string) (*dto.TaskListResponse, error) {
	return nil, fmt.Errorf("task list not supported by Ark adaptor")
}

type arkResponsesRequest struct {
	Model           string                     `json:"model"`
	Input           []arkResponsesInputMessage `json:"input"`
	MaxOutputTokens *int64                     `json:"max_output_tokens,omitempty"`
	Temperature     *float64                   `json:"temperature,omitempty"`
}

type arkResponsesInputMessage struct {
	Role    string                   `json:"role"`
	Content []map[string]interface{} `json:"content"`
}

func (a *ArkAdaptor) toChatReq(r *dto.MediaRequest) *responses.ResponsesRequest {
	req := &arkResponsesRequest{Model: r.Model}
	if r.MaxTokens > 0 {
		maxOutputTokens := int64(r.MaxTokens)
		req.MaxOutputTokens = &maxOutputTokens
	}
	if r.Temperature != 0 {
		t := r.Temperature
		req.Temperature = &t
	}
	for _, m := range r.Messages {
		content := toArkResponsesMessageContent(m)
		if len(content) == 0 {
			continue
		}
		req.Input = append(req.Input, arkResponsesInputMessage{
			Role:    arkNormalizeMessageRole(m.Role),
			Content: content,
		})
	}
	data, err := json.Marshal(req)
	if err != nil {
		return &responses.ResponsesRequest{Model: r.Model}
	}
	typed := &responses.ResponsesRequest{}
	if err := json.Unmarshal(data, typed); err != nil {
		return &responses.ResponsesRequest{Model: r.Model}
	}
	return typed
}

type arkChatMessageContentPart struct {
	Type     string           `json:"type,omitempty"`
	Text     string           `json:"text,omitempty"`
	ImageURL *arkImageURLPart `json:"image_url,omitempty"`
	VideoURL *arkVideoURLPart `json:"video_url,omitempty"`
	FileURL  *string          `json:"file_url,omitempty"`
}

type arkImageURLPart struct {
	URL    string `json:"url,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type arkVideoURLPart struct {
	URL string   `json:"url,omitempty"`
	FPS *float64 `json:"fps,omitempty"`
}

func toArkResponsesMessageContent(m dto.Message) []map[string]interface{} {
	parts := arkContentPartsFromValue(m.Content)
	if m.ImageURL != "" {
		parts = append(parts, &arkChatMessageContentPart{
			Type: "image_url",
			ImageURL: &arkImageURLPart{
				URL:    m.ImageURL,
				Detail: m.ImageDetail,
			},
		})
	}
	if m.VideoURL != "" {
		video := &arkVideoURLPart{URL: m.VideoURL}
		if m.VideoFPS > 0 {
			video.FPS = &m.VideoFPS
		}
		parts = append(parts, &arkChatMessageContentPart{
			Type:     "video_url",
			VideoURL: video,
		})
	}
	if m.FileURL != "" {
		parts = append(parts, &arkChatMessageContentPart{
			Type:    "input_file",
			FileURL: &m.FileURL,
		})
	}

	items := make([]map[string]interface{}, 0, len(parts))
	for _, part := range parts {
		if part == nil {
			continue
		}
		switch part.Type {
		case "text", "input_text":
			if strings.TrimSpace(part.Text) != "" {
				items = append(items, map[string]interface{}{
					"type": "input_text",
					"text": part.Text,
				})
			}
		case "image_url", "input_image":
			if part.ImageURL != nil && strings.TrimSpace(part.ImageURL.URL) != "" {
				item := map[string]interface{}{
					"type":      "input_image",
					"image_url": part.ImageURL.URL,
				}
				if part.ImageURL.Detail != "" {
					item["detail"] = part.ImageURL.Detail
				}
				items = append(items, item)
			}
		case "video_url", "input_video":
			if part.VideoURL != nil && strings.TrimSpace(part.VideoURL.URL) != "" {
				item := map[string]interface{}{
					"type":      "input_video",
					"video_url": part.VideoURL.URL,
				}
				if part.VideoURL.FPS != nil {
					item["fps"] = *part.VideoURL.FPS
				}
				items = append(items, item)
			}
		case "file_url", "input_file", "input_url":
			if part.FileURL != nil && strings.TrimSpace(*part.FileURL) != "" {
				items = append(items, map[string]interface{}{
					"type":     "input_file",
					"file_url": *part.FileURL,
				})
			}
		}
	}
	if len(items) > 0 {
		return items
	}
	if text, ok := messageContentText(m.Content); ok && strings.TrimSpace(text) != "" {
		return []map[string]interface{}{{
			"type": "input_text",
			"text": text,
		}}
	}
	return nil
}

func arkNormalizeMessageRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant", "system", "developer", "user":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return "user"
	}
}

func arkResponsesUsage(usage *responses.Usage) dto.Usage {
	if usage == nil {
		return dto.Usage{}
	}
	return dto.Usage{
		PromptTokens:     int(usage.GetInputTokens()),
		CompletionTokens: int(usage.GetOutputTokens()),
		TotalTokens:      int(usage.GetTotalTokens()),
	}
}

func arkResponsesFinishReason(resp *responses.ResponseObject) string {
	if resp == nil {
		return ""
	}
	status := strings.TrimSpace(resp.GetStatus().String())
	if status == "" || status == "unspecified" {
		return ""
	}
	return status
}

func arkResponsesRole(resp *responses.ResponseObject) string {
	if resp == nil {
		return "assistant"
	}
	for _, item := range resp.GetOutput() {
		message := item.GetOutputMessage()
		if message == nil {
			continue
		}
		role := strings.TrimSpace(message.GetRole().String())
		if role == "" || role == "unspecified" {
			continue
		}
		return role
	}
	return "assistant"
}

func arkResponsesText(resp *responses.ResponseObject) string {
	if resp == nil {
		return ""
	}
	texts := make([]string, 0, len(resp.GetOutput()))
	for _, item := range resp.GetOutput() {
		message := item.GetOutputMessage()
		if message == nil {
			continue
		}
		for _, content := range message.GetContent() {
			text := content.GetText()
			if text == nil || strings.TrimSpace(text.GetText()) == "" {
				continue
			}
			texts = append(texts, strings.TrimSpace(text.GetText()))
		}
	}
	return strings.Join(texts, "\n")
}

func messageContentText(v interface{}) (string, bool) {
	if v == nil {
		return "", false
	}
	switch text := v.(type) {
	case string:
		return text, true
	case []interface{}, []map[string]interface{}:
		return arkContentText(v), true
	default:
		return fmt.Sprint(v), true
	}
}

func arkContentPartsFromValue(v interface{}) []*arkChatMessageContentPart {
	switch content := v.(type) {
	case nil:
		return nil
	case string:
		if content == "" {
			return nil
		}
		return []*arkChatMessageContentPart{{Type: "text", Text: content}}
	case []map[string]interface{}:
		parts := make([]*arkChatMessageContentPart, 0, len(content))
		for _, item := range content {
			parts = append(parts, arkContentPartsFromMap(item)...)
		}
		return parts
	case []interface{}:
		parts := make([]*arkChatMessageContentPart, 0, len(content))
		for _, item := range content {
			switch typed := item.(type) {
			case map[string]interface{}:
				parts = append(parts, arkContentPartsFromMap(typed)...)
			case string:
				if typed != "" {
					parts = append(parts, &arkChatMessageContentPart{Type: "text", Text: typed})
				}
			}
		}
		return parts
	case map[string]interface{}:
		return arkContentPartsFromMap(content)
	default:
		if text := strings.TrimSpace(fmt.Sprint(v)); text != "" && text != "<nil>" {
			return []*arkChatMessageContentPart{{Type: "text", Text: text}}
		}
		return nil
	}
}

func arkContentPartsFromMap(item map[string]interface{}) []*arkChatMessageContentPart {
	if len(item) == 0 {
		return nil
	}
	partType, _ := item["type"].(string)
	switch partType {
	case "image_url", "input_image":
		if imageURL, detail, ok := arkMapImage(item); ok {
			return []*arkChatMessageContentPart{{
				Type:     "image_url",
				ImageURL: &arkImageURLPart{URL: imageURL, Detail: detail},
			}}
		}
	case "video_url", "input_video":
		if videoURL, fps, ok := arkMapVideo(item); ok {
			part := &arkChatMessageContentPart{
				Type:     "video_url",
				VideoURL: &arkVideoURLPart{URL: videoURL},
			}
			if fps != nil {
				part.VideoURL.FPS = fps
			}
			return []*arkChatMessageContentPart{part}
		}
	case "file_url", "input_file":
		if file, ok := arkMapFileURL(item); ok {
			return []*arkChatMessageContentPart{{
				Type:    "input_file",
				FileURL: &file,
			}}
		}
	}

	parts := make([]*arkChatMessageContentPart, 0, 4)
	if text, ok := item["text"].(string); ok && text != "" {
		parts = append(parts, &arkChatMessageContentPart{Type: "text", Text: text})
	}
	if imageURL, detail, ok := arkMapImage(item); ok {
		parts = append(parts, &arkChatMessageContentPart{
			Type: "image_url",
			ImageURL: &arkImageURLPart{
				URL:    imageURL,
				Detail: detail,
			},
		})
	}
	if videoURL, fps, ok := arkMapVideo(item); ok {
		part := &arkChatMessageContentPart{
			Type:     "video_url",
			VideoURL: &arkVideoURLPart{URL: videoURL},
		}
		if fps != nil {
			part.VideoURL.FPS = fps
		}
		parts = append(parts, part)
	}

	if file, ok := arkMapFileURL(item); ok {
		parts = append(parts, &arkChatMessageContentPart{
			Type:    "input_file",
			FileURL: &file,
		})
	}
	return parts
}

func arkMapImage(item map[string]interface{}) (url, detail string, ok bool) {
	switch image := item["image_url"].(type) {
	case string:
		if image != "" {
			return image, stringValue(item["detail"]), true
		}
	case map[string]interface{}:
		url = stringValue(image["url"])
		if url != "" {
			return url, firstNonEmptyString(stringValue(image["detail"]), stringValue(item["detail"])), true
		}
	}
	if url = stringValue(item["url"]); url != "" && (item["image_url"] != nil || item["type"] == "image_url" || item["type"] == "input_image") {
		return url, stringValue(item["detail"]), true
	}
	return "", "", false
}

func arkMapVideo(item map[string]interface{}) (url string, fps *float64, ok bool) {
	switch video := item["video_url"].(type) {
	case string:
		if video != "" {
			return video, arkFPSValue(item["fps"]), true
		}
	case map[string]interface{}:
		url = stringValue(video["url"])
		if url != "" {
			return url, arkFPSValue(video["fps"]), true
		}
	}
	if url = stringValue(item["url"]); url != "" && (item["video_url"] != nil || item["type"] == "video_url" || item["type"] == "input_video") {
		return url, arkFPSValue(item["fps"]), true
	}
	return "", nil, false
}

func arkMapFileURL(item map[string]interface{}) (url string, ok bool) {
	switch file := item["file_url"].(type) {
	case string:
		if strings.TrimSpace(file) != "" {
			return file, true
		}
	case map[string]interface{}:
		if parsed := stringValue(file["url"]); strings.TrimSpace(parsed) != "" {
			return parsed, true
		}
	}
	if parsed := stringValue(item["url"]); strings.TrimSpace(parsed) != "" && (item["type"] == "file_url" || item["type"] == "input_file") {
		return parsed, true
	}
	return "", false
}

func arkFPSValue(v interface{}) *float64 {
	switch fps := v.(type) {
	case float64:
		return &fps
	case float32:
		value := float64(fps)
		return &value
	case int:
		value := float64(fps)
		return &value
	case int64:
		value := float64(fps)
		return &value
	default:
		return nil
	}
}

func arkContentText(v interface{}) string {
	parts := arkContentPartsFromValue(v)
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func arkResponseText(content *model.ChatCompletionMessageContent) string {
	if content == nil {
		return ""
	}
	if content.StringValue != nil {
		return *content.StringValue
	}
	if content.ListValue == nil {
		return ""
	}
	texts := make([]string, 0, len(content.ListValue))
	for _, part := range content.ListValue {
		if part != nil && part.Type == model.ChatCompletionMessageContentPartTypeText && strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func arkResponseFirstImage(content *model.ChatCompletionMessageContent) (url, detail string) {
	if content == nil || content.ListValue == nil {
		return "", ""
	}
	for _, part := range content.ListValue {
		if part != nil && part.Type == model.ChatCompletionMessageContentPartTypeImageURL && part.ImageURL != nil && part.ImageURL.URL != "" {
			return part.ImageURL.URL, string(part.ImageURL.Detail)
		}
	}
	return "", ""
}

func stringValue(v interface{}) string {
	s, _ := v.(string)
	return s
}

func (a *ArkAdaptor) toImgReq(r *dto.MediaRequest) model.GenerateImagesRequest {
	req := model.GenerateImagesRequest{Model: r.Model, Prompt: utils.MediaPromptWithSystem(r)}
	if r.Size != "" {
		req.Size = &r.Resolution
	}
	if r.Seed != 0 {
		req.Seed = volcengine.Int64(int64(r.Seed))
	}
	if r.ResponseFormat != "" {
		req.ResponseFormat = &r.ResponseFormat
	}
	if r.Extra != nil {
		images := utils.ContentImageURLs(r.Extra["images"])
		if len(images) > 0 {
			req.Image = images
		} else if image, ok := utils.ContentImageURL(r.Extra["image"]); ok {
			req.Image = image
		}
		if v, ok := r.Extra["guidance_scale"].(float64); ok {
			req.GuidanceScale = &v
		}
		if v, ok := r.Extra["watermark"].(bool); ok {
			req.Watermark = &v
		}
		if v, ok := r.Extra["optimize_prompt"].(bool); ok {
			req.OptimizePrompt = &v
		}
	}

	if req.Watermark == nil {
		v := false
		req.Watermark = &v
	}

	maxImages := r.N
	if maxImages <= 0 && r.Extra != nil {
		if n, ok := intValue(r.Extra["n"]); ok {
			maxImages = n
		}
	}
	if maxImages > 0 {
		sequential := model.SequentialImageGeneration(model.SequentialImageGenerationAuto)
		req.SequentialImageGeneration = &sequential
		req.SequentialImageGenerationOptions = &model.SequentialImageGenerationOptions{
			MaxImages: &maxImages,
		}
	}
	return req
}

func (a *ArkAdaptor) toVidReq(r *dto.MediaRequest) model.CreateContentGenerationTaskRequest {
	content := make([]*model.CreateContentGenerationContentItem, 0, len(r.Messages))
	customContent := make([]map[string]interface{}, 0, len(r.Messages))
	for _, message := range r.Messages {
		text, ok := messageContentText(message.Content)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		content = append(content, &model.CreateContentGenerationContentItem{
			Type: model.ContentGenerationContentItemTypeText,
			Text: volcengine.String(text),
		})
		customContent = append(customContent, map[string]interface{}{
			"type": "text",
			"text": text,
		})
	}

	req := model.CreateContentGenerationTaskRequest{
		Model:     r.Model,
		Ratio:     volcengine.String("adaptive"),
		Content:   content,
		ExtraBody: make(model.ExtraBody),
	}
	appendCustomMediaContent := func(contentType, role, fieldName, url string) {
		if url == "" {
			return
		}
		if contentType == "image_url" {
			item := &model.CreateContentGenerationContentItem{
				Type: model.ContentGenerationContentItemTypeImage,
				ImageURL: &model.ImageURL{
					URL: url,
				},
			}
			if role != "" {
				item.Role = volcengine.String(role)
			}
			req.Content = append(req.Content, item)
		}
		customItem := map[string]interface{}{
			"type": contentType,
			fieldName: map[string]interface{}{
				"url": url,
			},
		}
		if role != "" {
			customItem["role"] = role
		}
		customContent = append(customContent, customItem)
	}
	appendReferenceImages := func(urls []string) {
		for _, url := range urls {
			appendCustomMediaContent("image_url", "reference_image", "image_url", url)
		}
	}
	if duration, ok := int64Value(r.Duration); ok && duration > 0 {
		req.Duration = volcengine.Int64(duration)
	}
	if r.Seed != 0 {
		req.Seed = volcengine.Int64(int64(r.Seed))
	}
	if r.Size != "" {
		req.Ratio = &r.Size
	}
	for k, v := range r.Extra {
		switch k {
		case "callback_url":
			if s, ok := v.(string); ok && s != "" {
				req.CallbackUrl = &s
			}
		case "return_last_frame":
			if b, ok := v.(bool); ok {
				req.ReturnLastFrame = &b
			}
		case "service_tier":
			if s, ok := v.(string); ok {
				req.ServiceTier = &s
			}
		case "execution_expires_after":
			if n, ok := int64Value(v); ok {
				req.ExecutionExpiresAfter = volcengine.Int64(n)
			}
		case "generate_audio":
			if b, ok := v.(bool); ok {
				req.GenerateAudio = &b
			}
		case "draft":
			if b, ok := v.(bool); ok {
				req.Draft = &b
			}
		case "camera_fixed":
			if b, ok := v.(bool); ok {
				req.CameraFixed = &b
			}
		case "watermark":
			if b, ok := v.(bool); ok {
				req.Watermark = &b
			} else {
				wm := false
				req.Watermark = &wm
			}
		case "frames":
			if n, ok := int64Value(v); ok {
				req.Frames = volcengine.Int64(n)
			}
		case "image":
			if url, ok := utils.ContentImageURL(v); ok {
				appendCustomMediaContent("image_url", "first_frame", "image_url", url)
			}
		case "images":
			urls := utils.ContentImageURLs(v)
			if len(urls) > 0 {
				appendCustomMediaContent("image_url", "first_frame", "image_url", urls[0])
			}
			if len(urls) > 1 {
				appendCustomMediaContent("image_url", "last_frame", "image_url", urls[1])
			}
		case "reference_image":
			if url, ok := utils.ContentImageURL(v); ok {
				appendReferenceImages([]string{url})
			}
		case "reference_images":
			urls := utils.ContentImageURLs(v)
			appendReferenceImages(urls)
		case "files":
			files := contentReferenceFiles(v)
			for _, file := range files {
				switch file.Type {
				case "image":
					appendCustomMediaContent("image_url", "reference_image", "image_url", file.URL)
				case "video":
					appendCustomMediaContent("video_url", "reference_video", "video_url", file.URL)
				case "audio":
					appendCustomMediaContent("audio_url", "reference_audio", "audio_url", file.URL)
				}
			}
		case "draft_task_id":
			if id, ok := v.(string); ok && id != "" {
				req.Content = append(req.Content, &model.CreateContentGenerationContentItem{
					Type:      model.ContentGenerationContentItemTypeDraftTask,
					DraftTask: &model.DraftTask{ID: id},
				})
				customContent = append(customContent, map[string]interface{}{
					"type": "draft_task",
					"draft_task": map[string]interface{}{
						"id": id,
					},
				})
			}
		case "draft_task":
			if id, ok := contentDraftTaskID(v); ok {
				req.Content = append(req.Content, &model.CreateContentGenerationContentItem{
					Type:      model.ContentGenerationContentItemTypeDraftTask,
					DraftTask: &model.DraftTask{ID: id},
				})
				customContent = append(customContent, map[string]interface{}{
					"type": "draft_task",
					"draft_task": map[string]interface{}{
						"id": id,
					},
				})
			}
		default:
			req.ExtraBody[k] = v
		}
	}
	if len(customContent) > 0 {
		req.ExtraBody["content"] = customContent
	}
	if r.Resolution != "" {
		req.Resolution = &r.Resolution
	}
	return req
}

type contentReferenceFile struct {
	URL   string
	Type  string
	Index int
}

func contentReferenceFiles(v interface{}) []contentReferenceFile {
	switch items := v.(type) {
	case []interface{}:
		out := make([]contentReferenceFile, 0, len(items))
		for _, item := range items {
			file, ok := contentReferenceFileFromValue(item)
			if ok {
				out = append(out, file)
			}
		}
		return out
	case []map[string]interface{}:
		out := make([]contentReferenceFile, 0, len(items))
		for _, item := range items {
			file, ok := contentReferenceFileFromMap(item)
			if ok {
				out = append(out, file)
			}
		}
		return out
	default:
		if file, ok := contentReferenceFileFromValue(v); ok {
			return []contentReferenceFile{file}
		}
	}
	return nil
}

func contentReferenceFileFromValue(v interface{}) (contentReferenceFile, bool) {
	switch item := v.(type) {
	case map[string]interface{}:
		return contentReferenceFileFromMap(item)
	case map[string]string:
		mapped := make(map[string]interface{}, len(item))
		for k, val := range item {
			mapped[k] = val
		}
		return contentReferenceFileFromMap(mapped)
	default:
		return contentReferenceFile{}, false
	}
}

func contentReferenceFileFromMap(item map[string]interface{}) (contentReferenceFile, bool) {
	url := stringValue(item["url"])
	typ := strings.ToLower(strings.TrimSpace(stringValue(item["type"])))
	if url == "" || typ == "" {
		return contentReferenceFile{}, false
	}
	switch typ {
	case "image", "video", "audio":
	default:
		return contentReferenceFile{}, false
	}
	file := contentReferenceFile{URL: url, Type: typ}
	if n, ok := int64Value(item["index"]); ok {
		file.Index = int(n)
	}
	return file, true
}

func intValue(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	default:
		return 0, false
	}
}

func int64Value(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	default:
		return 0, false
	}
}

func int64ToInt(v *int64) int {
	if v == nil {
		return 0
	}
	return int(*v)
}

func contentDraftTaskID(v interface{}) (string, bool) {
	switch item := v.(type) {
	case string:
		if item == "" {
			return "", false
		}
		return item, true
	case map[string]interface{}:
		if id, ok := item["id"].(string); ok && id != "" {
			return id, true
		}
		if task, ok := item["draft_task"].(map[string]interface{}); ok {
			if id, ok := task["id"].(string); ok && id != "" {
				return id, true
			}
		}
	case map[string]string:
		if id, ok := item["id"]; ok && id != "" {
			return id, true
		}
	}
	return "", false
}

func formatUnixMillis(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.UnixMilli(ts).UTC().Format(time.RFC3339)
}

type arkStream struct {
	r *arkutils.ResponsesStreamReader
}

func (w *arkStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	for {
		resp, err := w.r.Recv()
		if err != nil {
			return nil, err
		}
		if resp == nil {
			continue
		}
		if evtErr := resp.GetError(); evtErr != nil {
			return nil, fmt.Errorf("ark responses stream error: %s", evtErr.GetMessage())
		}
		if text := resp.GetText(); text != nil {
			delta := firstNonEmptyString(text.GetDelta(), text.GetText())
			if strings.TrimSpace(delta) == "" {
				continue
			}
			return &dto.StreamToken{
				Text:  delta,
				Type:  "text",
				Index: int(text.GetOutputIndex()),
			}, nil
		}
		if failed := resp.GetResponseFailed(); failed != nil {
			if failed.GetResponse() != nil && failed.GetResponse().GetError() != nil && strings.TrimSpace(failed.GetResponse().GetError().GetMessage()) != "" {
				return nil, fmt.Errorf("ark responses stream failed: %s", failed.GetResponse().GetError().GetMessage())
			}
			return nil, fmt.Errorf("ark responses stream failed")
		}
		if resp.GetResponseCompleted() != nil {
			return nil, io.EOF
		}
	}
}
func (w *arkStream) Close() error { return w.r.Close() }

type arkMediaStream struct {
	r *arkutils.ImageGenerationStreamReader
}

func (w *arkMediaStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	resp, err := w.r.Recv()
	if err != nil {
		return nil, err
	}
	return &dto.StreamToken{Type: resp.Type, Index: int(resp.ImageIndex), URL: volcengine.StringValue(resp.Url), Text: volcengine.StringValue(resp.B64Json)}, nil
}
func (w *arkMediaStream) Close() error { return w.r.Close() }

type arkVidStream struct {
	c    *arkruntime.Client
	id   string
	last string
}

func (w *arkVidStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	for {
		resp, err := w.c.GetContentGenerationTask(ctx, model.GetContentGenerationTaskRequest{ID: w.id})
		if err != nil {
			return nil, err
		}
		if resp.Status == w.last && resp.Status != model.StatusSucceeded && resp.Status != model.StatusFailed {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				continue
			}
		}
		w.last = resp.Status
		res := &dto.StreamToken{Type: "progress", Text: resp.Status}
		if resp.Status == model.StatusSucceeded {
			res.Type, res.URL = "url", resp.Content.VideoURL
			if res.URL == "" {
				res.URL = resp.Content.FileURL
			}
			return res, io.EOF
		} else if resp.Status == model.StatusFailed {
			if resp.Error != nil {
				res.Text = resp.Error.Message
			}
			return res, fmt.Errorf("vid failed: %s", res.Text)
		}
		return res, nil
	}
}
func (w *arkVidStream) Close() error { return nil }
