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
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/utils"
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
	resp, err := a.getClient(config).CreateChatCompletion(ctx, a.toChatReq(r))
	if err != nil {
		return nil, err
	}
	res := &dto.MediaResponse{Usage: dto.Usage{PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens, TotalTokens: resp.Usage.TotalTokens}}
	for _, c := range resp.Choices {
		msg := ""
		if c.Message.Content != nil && c.Message.Content.StringValue != nil {
			msg = *c.Message.Content.StringValue
		}
		res.Choices = append(res.Choices, dto.ChatChoice{Index: c.Index, Message: dto.Message{Role: c.Message.Role, Content: msg}, FinishReason: string(c.FinishReason)})
	}
	if len(res.Choices) > 0 {
		res.Text = fmt.Sprint(res.Choices[0].Message.Content)
	}
	return res, nil
}

func (a *ArkAdaptor) Stream(ctx context.Context, config *ProviderConfig, r *dto.MediaRequest) (dto.TokenStream, error) {
	s, err := a.getClient(config).CreateChatCompletionStream(ctx, a.toChatReq(r))
	if err != nil {
		return nil, err
	}
	return &arkStream{s}, nil
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

func (a *ArkAdaptor) TaskStatus(ctx context.Context, cfg *ProviderConfig, id string) (*dto.TaskStatusResponse, error) {
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

func (a *ArkAdaptor) toChatReq(r *dto.MediaRequest) model.ChatRequest {
	req := &arkChatRequest{Model: r.Model}
	if r.MaxTokens > 0 {
		req.MaxTokens = &r.MaxTokens
	}
	if r.Temperature != 0 {
		t := float32(r.Temperature)
		req.Temperature = &t
	}
	for _, m := range r.Messages {
		req.Messages = append(req.Messages, &arkChatMessage{
			Role:    m.Role,
			Content: toArkChatMessageContent(m),
		})
	}
	return req
}

type arkChatRequest struct {
	Model       string            `json:"model"`
	Messages    []*arkChatMessage `json:"messages"`
	MaxTokens   *int              `json:"max_tokens,omitempty"`
	Temperature *float32          `json:"temperature,omitempty"`
	Stream      *bool             `json:"stream,omitempty"`
}

func (r *arkChatRequest) MarshalJSON() ([]byte, error) {
	type alias arkChatRequest
	return json.Marshal((*alias)(r))
}

func (r *arkChatRequest) WithStream(stream bool) model.ChatRequest {
	r.Stream = &stream
	return r
}

func (r *arkChatRequest) IsStream() bool {
	return r.Stream != nil && *r.Stream
}

func (r *arkChatRequest) GetModel() string {
	return r.Model
}

type arkChatMessage struct {
	Role    string                 `json:"role"`
	Content *arkChatMessageContent `json:"content"`
}

type arkChatMessageContent struct {
	StringValue *string
	ListValue   []*arkChatMessageContentPart
}

func (c arkChatMessageContent) MarshalJSON() ([]byte, error) {
	if c.StringValue != nil {
		return json.Marshal(c.StringValue)
	}
	if c.ListValue != nil {
		return json.Marshal(c.ListValue)
	}
	return json.Marshal(nil)
}

type arkChatMessageContentPart struct {
	Type     string           `json:"type,omitempty"`
	Text     string           `json:"text,omitempty"`
	ImageURL *arkImageURLPart `json:"image_url,omitempty"`
	VideoURL *arkVideoURLPart `json:"video_url,omitempty"`
	FileURL  *arkFileURLPart  `json:"file_url,omitempty"`
}

type arkImageURLPart struct {
	URL    string `json:"url,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type arkVideoURLPart struct {
	URL string   `json:"url,omitempty"`
	FPS *float64 `json:"fps,omitempty"`
}

type arkFileURLPart struct {
	URL      string `json:"url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	FileName string `json:"file_name,omitempty"`
	Name     string `json:"name,omitempty"`
}

func toArkChatMessageContent(m dto.Message) *arkChatMessageContent {
	parts := make([]*arkChatMessageContentPart, 0, 4)
	if text, ok := messageContentText(m.Content); ok && text != "" {
		parts = append(parts, &arkChatMessageContentPart{
			Type: "text",
			Text: text,
		})
	}
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
	if m.FileURL != "" || m.FileID != "" {
		parts = append(parts, &arkChatMessageContentPart{
			Type: "file_url",
			FileURL: &arkFileURLPart{
				URL:      m.FileURL,
				FileID:   m.FileID,
				FileName: m.FileName,
				Name:     m.Name,
			},
		})
	}
	if len(parts) > 0 {
		return &arkChatMessageContent{ListValue: parts}
	}
	text, _ := messageContentText(m.Content)
	return &arkChatMessageContent{StringValue: &text}
}

func messageContentText(v interface{}) (string, bool) {
	if v == nil {
		return "", false
	}
	switch text := v.(type) {
	case string:
		return text, true
	default:
		return fmt.Sprint(v), true
	}
}

func (a *ArkAdaptor) toImgReq(r *dto.MediaRequest) model.GenerateImagesRequest {
	req := model.GenerateImagesRequest{Model: r.Model, Prompt: mediaPromptWithSystem(r)}
	if r.Size != "" {
		req.Size = &r.Size
	}
	if r.Seed != 0 {
		req.Seed = volcengine.Int64(int64(r.Seed))
	}
	if r.ResponseFormat != "" {
		req.ResponseFormat = &r.ResponseFormat
	}
	if r.Extra != nil {
		images := contentImageURLs(r.Extra["images"])
		if len(images) > 0 {
			req.Image = images
		} else if image, ok := contentImageURL(r.Extra["image"]); ok {
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
	if maxImages > 0 && req.Image != nil {
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
	hasImageInput := false
	for _, message := range r.Messages {
		text, ok := messageContentText(message.Content)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		content = append(content, &model.CreateContentGenerationContentItem{
			Type: model.ContentGenerationContentItemTypeText,
			Text: volcengine.String(text),
		})
	}

	req := model.CreateContentGenerationTaskRequest{
		Model:     r.Model,
		Ratio:     volcengine.String("adaptive"),
		Content:   content,
		ExtraBody: make(model.ExtraBody),
	}
	appendImageContent := func(url, role string) {
		if url == "" {
			return
		}
		hasImageInput = true
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
	appendReferenceImages := func(urls []string) {
		for _, url := range urls {
			appendImageContent(url, "reference_image")
		}
	}
	if r.Duration > 0 {
		req.Duration = volcengine.Int64(int64(r.Duration))
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
			if url, ok := contentImageURL(v); ok {
				appendImageContent(url, "first_frame")
			}
		case "images":
			urls := contentImageURLs(v)
			if len(urls) > 0 {
				appendImageContent(urls[0], "first_frame")
			}
			if len(urls) > 1 {
				appendImageContent(urls[1], "last_frame")
			}
		case "reference_image":
			if url, ok := contentImageURL(v); ok {
				appendReferenceImages([]string{url})
			}
		case "reference_images":
			urls := contentImageURLs(v)
			appendReferenceImages(urls)
		case "draft_task_id":
			if id, ok := v.(string); ok && id != "" {
				req.Content = append(req.Content, &model.CreateContentGenerationContentItem{
					Type:      model.ContentGenerationContentItemTypeDraftTask,
					DraftTask: &model.DraftTask{ID: id},
				})
			}
		case "draft_task":
			if id, ok := contentDraftTaskID(v); ok {
				req.Content = append(req.Content, &model.CreateContentGenerationContentItem{
					Type:      model.ContentGenerationContentItemTypeDraftTask,
					DraftTask: &model.DraftTask{ID: id},
				})
			}
		default:
			req.ExtraBody[k] = v
		}
	}
	if r.Resolution != "" && !hasImageInput {
		req.Resolution = &r.Resolution
	}
	return req
}

func contentImageURLs(v interface{}) []string {
	switch arr := v.(type) {
	case []string:
		var out []string
		for _, item := range arr {
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []interface{}:
		var out []string
		for _, item := range arr {
			if url, ok := contentImageURL(item); ok {
				out = append(out, url)
			}
		}
		return out
	default:
		if url, ok := contentImageURL(v); ok {
			return []string{url}
		}
	}
	return nil
}

func contentImageURL(v interface{}) (string, bool) {
	switch item := v.(type) {
	case string:
		if item == "" {
			return "", false
		}
		return item, true
	case map[string]interface{}:
		if url, ok := item["url"].(string); ok && url != "" {
			return url, true
		}
		if imageURL, ok := item["image_url"].(map[string]interface{}); ok {
			if url, ok := imageURL["url"].(string); ok && url != "" {
				return url, true
			}
		}
	case map[string]string:
		if url, ok := item["url"]; ok && url != "" {
			return url, true
		}
	}
	return "", false
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
	r *utils.ChatCompletionStreamReader
}

func (w *arkStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	resp, err := w.r.Recv()
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, io.EOF
	}
	return &dto.StreamToken{Text: resp.Choices[0].Delta.Content, Type: "text", Index: resp.Choices[0].Index}, nil
}
func (w *arkStream) Close() error { return w.r.Close() }

type arkMediaStream struct {
	r *utils.ImageGenerationStreamReader
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
