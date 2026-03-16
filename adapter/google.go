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
		resp, err := c.Models.GenerateImages(ctx, r.Model, mediaPromptWithSystem(r), a.toImgCfg(r))
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

func (a *GoogleAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, id string) (*dto.TaskStatusResponse, error) {
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
	code := ""
	message := ""
	if op.Done {
		if len(op.Error) > 0 {
			status = "FAILED"
			code, message = mapErr(op.Error)
		} else {
			status = "SUCCEEDED"
			videoURL = firstVideoURL(op.Response)
		}
	}

	return &dto.TaskStatusResponse{
		Output: dto.TaskStatusOutput{
			TaskID:     op.Name,
			TaskStatus: status,
			VideoURL:   videoURL,
			Code:       code,
			Message:    message,
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
		res = append(res, &genai.Content{Role: m.Role, Parts: []*genai.Part{{Text: fmt.Sprint(m.Content)}}})
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
	if r.Size != "" {
		cfg.AspectRatio = r.Size
	}
	if r.Resolution != "" {
		cfg.ImageSize = r.Resolution
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
	res.Video.URL = firstVideoURL(op.Response)
	res.URL = res.Video.URL
	return res
}

func firstVideoURL(resp *genai.GenerateVideosResponse) string {
	if resp == nil || len(resp.GeneratedVideos) == 0 {
		return ""
	}
	v := resp.GeneratedVideos[0]
	if v == nil || v.Video == nil {
		return ""
	}
	return v.Video.URI
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
