// Package adapter provides Google Gemini adaptor implementation using the new official genai SDK.
package adapter

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"google.golang.org/genai"
)

type GoogleAdaptor struct {
	client *genai.Client
}

func (a *GoogleAdaptor) getClient(ctx context.Context, cfg *ProviderConfig) (*genai.Client, error) {
	if a.client != nil {
		return a.client, nil
	}
	cc := &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	}
	if cfg.Proxy != "" {
		proxy := cfg.Proxy
		if !strings.HasPrefix(proxy, "http") {
			proxy = "http://" + proxy
		}
		pURL, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy: %w", err)
		}
		cc.HTTPClient = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(pURL),
			},
		}
	}
	c, err := genai.NewClient(ctx, cc)
	if err != nil {
		return nil, err
	}
	a.client = c
	return c, nil
}

func (a *GoogleAdaptor) Chat(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (*dto.MediaResponse, error) {
	c, err := a.getClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	resp, err := c.Models.GenerateContent(ctx, r.Model, a.toContents(r), a.toGenCfg(r))
	if err != nil {
		return nil, err
	}
	res := &dto.MediaResponse{}
	for _, cand := range resp.Candidates {
		if cand.Content != nil && len(cand.Content.Parts) > 0 {
			res.Choices = append(res.Choices, dto.ChatChoice{Message: dto.Message{Role: cand.Content.Role, Content: cand.Content.Parts[0].Text}, FinishReason: string(cand.FinishReason)})
		}
	}
	if len(res.Choices) > 0 {
		res.Text = fmt.Sprint(res.Choices[0].Message.Content)
	}
	return res, nil
}

func (a *GoogleAdaptor) Stream(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (dto.TokenStream, error) {
	c, err := a.getClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	seq := c.Models.GenerateContentStream(ctx, r.Model, a.toContents(r), a.toGenCfg(r))
	next, stop := iter.Pull2(seq)
	return &googleStream{next: next, stop: stop}, nil
}

func (a *GoogleAdaptor) Media(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (*dto.MediaResponse, error) {
	c, err := a.getClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	switch r.Type {
	case dto.MediaTypeImage:
		model := normalizeGoogleMediaModel(r.Model)
		if isGoogleGenerateContentImageModel(model) {
			resp, err := c.Models.GenerateContent(ctx, model, genai.Text(mediaPromptWithSystem(r)), a.toImgContentCfg(r))
			if err != nil {
				return nil, err
			}
			return a.toImgContentResp(resp), nil
		}
		resp, err := c.Models.GenerateImages(ctx, model, mediaPromptWithSystem(r), a.toImgCfg(r))
		if err != nil {
			return nil, err
		}
		res := &dto.MediaResponse{}
		for _, img := range resp.GeneratedImages {
			if img.Image == nil {
				continue
			}
			data := dto.ImageData{}
			if img.Image.GCSURI != "" {
				data.URL = img.Image.GCSURI
			}
			if len(img.Image.ImageBytes) > 0 {
				data.B64JSON = base64.StdEncoding.EncodeToString(img.Image.ImageBytes)
			}
			res.Data = append(res.Data, data)
		}
		if len(res.Data) > 0 {
			res.URL = res.Data[0].URL
		}
		return res, nil
	case dto.MediaTypeVideo:
		op, err := c.Models.GenerateVideos(ctx, r.Model, mediaPromptWithSystem(r), nil, a.toVidCfg(r))
		if err != nil {
			return nil, err
		}
		return a.toVidResp(op), nil
	default:
		return nil, fmt.Errorf("unsupported media type for Google: %s", r.Type)
	}
}

func (a *GoogleAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, id string, _ ...map[string]string) (*dto.TaskStatusResponse, error) {
	c, err := a.getClient(ctx, config)
	if err != nil {
		return nil, err
	}
	op, err := c.Operations.GetVideosOperation(ctx, &genai.GenerateVideosOperation{Name: id}, nil)
	if err != nil {
		return nil, err
	}

	status := "RUNNING"
	videoURL := ""
	videoB64JSON := ""
	videoMIMEType := ""
	code := ""
	message := ""
	if op.Done {
		if len(op.Error) > 0 {
			status = "FAILED"
			code, message = mapErr(op.Error)
		} else {
			status = "SUCCEEDED"
			videoURL, videoB64JSON, videoMIMEType = a.googleVideoPayload(ctx, c, op.Response)
		}
	}

	return &dto.TaskStatusResponse{
		Output: dto.TaskStatusOutput{
			TaskID:        op.Name,
			TaskStatus:    status,
			VideoURL:      videoURL,
			VideoB64JSON:  videoB64JSON,
			VideoMIMEType: videoMIMEType,
			Code:          code,
			Message:       message,
		},
	}, nil
}

func (a *GoogleAdaptor) ListTasks(ctx context.Context, config *ProviderConfig, query map[string]string) (*dto.TaskListResponse, error) {
	return nil, fmt.Errorf("task list not supported by Google adaptor")
}

func (a *GoogleAdaptor) StreamMedia(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("streaming media not supported by Google adaptor")
}

// Helpers
func (a *GoogleAdaptor) toContents(r *dto.MediaRequest) []*genai.Content {
	messages := nonSystemMessages(r.Messages)
	res := make([]*genai.Content, 0, len(messages))
	for _, m := range messages {
		parts := make([]*genai.Part, 0, 1+len(googleImageInputs(r)))
		if text := strings.TrimSpace(fmt.Sprint(m.Content)); text != "" {
			parts = append(parts, &genai.Part{Text: text})
		}
		if m.Role == "user" {
			parts = append(parts, googleImageParts(r)...)
		}
		if len(parts) == 0 {
			continue
		}
		res = append(res, &genai.Content{Role: m.Role, Parts: parts})
	}
	return res
}

func (a *GoogleAdaptor) toGenCfg(r *dto.MediaRequest) *genai.GenerateContentConfig {
	cfg := &genai.GenerateContentConfig{}
	if systemPrompt := firstSystemMessage(r.Messages); systemPrompt != "" {
		cfg.SystemInstruction = &genai.Content{
			Role:  "system",
			Parts: []*genai.Part{{Text: systemPrompt}},
		}
	}
	if r.Temperature != 0 {
		cfg.Temperature = genai.Ptr(float32(r.Temperature))
	}
	if r.MaxTokens != 0 {
		cfg.MaxOutputTokens = int32(r.MaxTokens)
	}
	return cfg
}

func (a *GoogleAdaptor) toImgCfg(r *dto.MediaRequest) *genai.GenerateImagesConfig {
	cfg := &genai.GenerateImagesConfig{}
	if r.N > 0 {
		cfg.NumberOfImages = int32(r.N)
	}
	if r.ResponseFormat != "" {
		cfg.OutputMIMEType = r.ResponseFormat
	}
	cfg.AspectRatio, cfg.ImageSize = normalizeGoogleImageShape(r.Size, r.Resolution)
	return cfg
}

func (a *GoogleAdaptor) toImgContentCfg(r *dto.MediaRequest) *genai.GenerateContentConfig {
	cfg := a.toGenCfg(r)
	cfg.ResponseModalities = []string{"TEXT", "IMAGE"}
	cfg.ImageConfig = &genai.ImageConfig{}
	cfg.ImageConfig.AspectRatio, cfg.ImageConfig.ImageSize = normalizeGoogleImageShape(r.Size, r.Resolution)
	if r.Extra != nil {
		if v, ok := r.Extra["person_generation"].(string); ok {
			cfg.ImageConfig.PersonGeneration = v
		}
	}
	return cfg
}

func (a *GoogleAdaptor) toVidCfg(r *dto.MediaRequest) *genai.GenerateVideosConfig {
	cfg := &genai.GenerateVideosConfig{}
	if r.N > 0 {
		cfg.NumberOfVideos = int32(r.N)
	}
	if r.Size != "" {
		cfg.AspectRatio = r.Size
	}
	if r.Resolution != "" {
		cfg.Resolution = r.Resolution
	}
	if r.Duration > 0 {
		cfg.DurationSeconds = genai.Ptr(int32(r.Duration))
	}
	if r.Fps > 0 {
		cfg.FPS = genai.Ptr(int32(r.Fps))
	}
	if r.Seed != 0 {
		cfg.Seed = genai.Ptr(int32(r.Seed))
	}
	if r.Extra != nil {
		if v, ok := r.Extra["negative_prompt"].(string); ok {
			cfg.NegativePrompt = v
		}
		if v, ok := r.Extra["person_generation"].(string); ok {
			cfg.PersonGeneration = v
		}
		if v, ok := r.Extra["output_gcs_uri"].(string); ok {
			cfg.OutputGCSURI = v
		}
		if v, ok := r.Extra["enhance_prompt"].(bool); ok {
			cfg.EnhancePrompt = v
		}
		if v, ok := r.Extra["generate_audio"].(bool); ok {
			cfg.GenerateAudio = genai.Ptr(v)
		}
	}
	return cfg
}

func (a *GoogleAdaptor) toVidResp(op *genai.GenerateVideosOperation) *dto.MediaResponse {
	res := &dto.MediaResponse{
		TaskID: op.Name,
	}
	if !op.Done {
		res.Status = "RUNNING"
		return res
	}
	if len(op.Error) > 0 {
		res.Status = "FAILED"
		res.ErrorCode, res.ErrorMessage = mapErr(op.Error)
		return res
	}
	res.Status = "SUCCEEDED"
	res.Video.URL, res.Video.B64JSON, res.Video.MIMEType = a.googleVideoPayload(context.Background(), a.client, op.Response)
	res.URL = res.Video.URL
	return res
}

func firstGeneratedVideo(resp *genai.GenerateVideosResponse) *genai.GeneratedVideo {
	if resp == nil || len(resp.GeneratedVideos) == 0 {
		return nil
	}
	v := resp.GeneratedVideos[0]
	if v == nil || v.Video == nil {
		return nil
	}
	return v
}

func (a *GoogleAdaptor) googleVideoPayload(ctx context.Context, c *genai.Client, resp *genai.GenerateVideosResponse) (url, b64JSON, mimeType string) {
	v := firstGeneratedVideo(resp)
	if v == nil || v.Video == nil {
		return "", "", ""
	}

	url = v.Video.URI
	mimeType = v.Video.MIMEType
	if mimeType == "" {
		mimeType = "video/mp4"
	}

	videoBytes := v.Video.VideoBytes
	if len(videoBytes) == 0 && c != nil && c.ClientConfig().Backend != genai.BackendVertexAI {
		if data, err := c.Files.Download(ctx, genai.NewDownloadURIFromGeneratedVideo(v), nil); err == nil {
			videoBytes = data
		}
	}
	if len(videoBytes) > 0 {
		b64JSON = base64.StdEncoding.EncodeToString(videoBytes)
	}
	return url, b64JSON, mimeType
}

func mapErr(errMap map[string]any) (code, message string) {
	if v, ok := errMap["code"]; ok {
		code = fmt.Sprint(v)
	}
	if v, ok := errMap["message"]; ok {
		message = fmt.Sprint(v)
	}
	return code, message
}

func normalizeGoogleMediaModel(model string) string {
	return strings.TrimSpace(model)
}

func isGoogleGenerateContentImageModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return false
	}
	if strings.HasPrefix(model, "imagen-") {
		return false
	}
	return strings.Contains(model, "image")
}

func googleImageParts(r *dto.MediaRequest) []*genai.Part {
	urls := googleImageInputs(r)
	if len(urls) == 0 {
		return nil
	}
	parts := make([]*genai.Part, 0, len(urls))
	for _, raw := range urls {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts = append(parts, genai.NewPartFromURI(raw, googleImageMIMEType(raw)))
	}
	return parts
}

func googleImageInputs(r *dto.MediaRequest) []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, 4)
	appendURL := func(v string) {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	for _, msg := range r.Messages {
		appendURL(msg.ImageURL)
	}
	appendURL(getStringExtra(r.Extra, "image"))
	for _, item := range getStringSliceExtra(r.Extra, "images") {
		appendURL(item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func googleImageMIMEType(raw string) string {
	if parsed, err := url.Parse(raw); err == nil {
		raw = parsed.Path
	}
	switch strings.ToLower(filepath.Ext(raw)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	default:
		return "image/png"
	}
}

func normalizeGoogleImageShape(size, resolution string) (aspectRatio, imageSize string) {
	for _, value := range []string{size, resolution} {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		switch {
		case isGoogleImageAspectRatio(v):
			aspectRatio = v
		case isGoogleImageSize(v):
			imageSize = strings.ToUpper(v)
		}
	}
	return aspectRatio, imageSize
}

func isGoogleImageAspectRatio(value string) bool {
	switch strings.TrimSpace(value) {
	case "1:1", "9:16", "16:9", "4:3", "3:4":
		return true
	default:
		return false
	}
}

func isGoogleImageSize(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "1K", "2K", "4K":
		return true
	default:
		return false
	}
}

func (a *GoogleAdaptor) toImgContentResp(resp *genai.GenerateContentResponse) *dto.MediaResponse {
	res := &dto.MediaResponse{}
	if resp == nil {
		return res
	}

	for _, cand := range resp.Candidates {
		if cand == nil || cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if part == nil {
				continue
			}
			if text := strings.TrimSpace(part.Text); text != "" {
				res.Choices = append(res.Choices, dto.ChatChoice{
					Message:      dto.Message{Role: cand.Content.Role, Content: text},
					FinishReason: string(cand.FinishReason),
				})
				if res.Text == "" {
					res.Text = text
				}
			}
			if part.InlineData != nil && len(part.InlineData.Data) > 0 {
				res.Data = append(res.Data, dto.ImageData{
					B64JSON: base64.StdEncoding.EncodeToString(part.InlineData.Data),
				})
			}
		}
	}
	return res
}

type googleStream struct {
	stop func()
	next func() (*genai.GenerateContentResponse, error, bool)
}

func (w *googleStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	resp, err, ok := w.next()
	if !ok {
		if err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		return &dto.StreamToken{Type: "text"}, nil
	}
	return &dto.StreamToken{Text: resp.Candidates[0].Content.Parts[0].Text, Type: "text"}, nil
}

func (w *googleStream) Close() error {
	w.stop()
	return nil
}
