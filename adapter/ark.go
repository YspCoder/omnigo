// Package adapter provides Volcengine Ark (火山方舟) adaptor implementation.
package adapter

import (
	"context"
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
	if a.client != nil { return a.client }
	opts := []arkruntime.ConfigOption{arkruntime.WithRegion(config.Region)}
	if config.BaseURL != "" { opts = append(opts, arkruntime.WithBaseUrl(config.BaseURL)) }
	if config.HTTPClient != nil { opts = append(opts, arkruntime.WithHTTPClient(config.HTTPClient)) }
	if config.APIKey != "" {
		a.client = arkruntime.NewClientWithApiKey(config.APIKey, opts...)
	} else {
		a.client = arkruntime.NewClientWithAkSk(config.AccessKey, config.SecretKey, opts...)
	}
	return a.client
}

func (a *ArkAdaptor) Chat(ctx context.Context, config *ProviderConfig, r *dto.ChatRequest) (*dto.ChatResponse, error) {
	resp, err := a.getClient(config).CreateChatCompletion(ctx, a.toChatReq(r))
	if err != nil { return nil, err }
	res := &dto.ChatResponse{Usage: dto.Usage{PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens, TotalTokens: resp.Usage.TotalTokens}}
	for _, c := range resp.Choices {
		msg := ""
		if c.Message.Content != nil && c.Message.Content.StringValue != nil { msg = *c.Message.Content.StringValue }
		res.Choices = append(res.Choices, dto.ChatChoice{Index: c.Index, Message: dto.Message{Role: c.Message.Role, Content: msg}, FinishReason: string(c.FinishReason)})
	}
	return res, nil
}

func (a *ArkAdaptor) Stream(ctx context.Context, config *ProviderConfig, r *dto.ChatRequest) (dto.TokenStream, error) {
	s, err := a.getClient(config).CreateChatCompletionStream(ctx, a.toChatReq(r))
	if err != nil { return nil, err }
	return &arkStream{s}, nil
}

func (a *ArkAdaptor) Media(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (*dto.MediaResponse, error) {
	c := a.getClient(cfg)
	if r.Type == dto.MediaTypeImage {
		resp, err := c.GenerateImages(ctx, a.toImgReq(r))
		if err != nil { return nil, err }
		res := &dto.MediaResponse{Created: resp.Created}
		for _, d := range resp.Data {
			res.Data = append(res.Data, dto.ImageData{URL: volcengine.StringValue(d.Url), B64JSON: volcengine.StringValue(d.B64Json)})
		}
		if len(res.Data) > 0 { res.URL = res.Data[0].URL }
		return res, nil
	}
	resp, err := c.CreateContentGenerationTask(ctx, a.toVidReq(r))
	if err != nil { return nil, err }
	return &dto.MediaResponse{TaskID: resp.ID}, nil
}

func (a *ArkAdaptor) StreamMedia(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (dto.TokenStream, error) {
	c := a.getClient(cfg)
	if r.Type == dto.MediaTypeImage {
		s, err := c.GenerateImagesStreaming(ctx, a.toImgReq(r))
		return &arkMediaStream{s}, err
	}
	resp, err := c.CreateContentGenerationTask(ctx, a.toVidReq(r))
	if err != nil { return nil, err }
	return &arkVidStream{c, resp.ID, ""}, nil
}

func (a *ArkAdaptor) TaskStatus(ctx context.Context, cfg *ProviderConfig, id string) (*dto.TaskStatusResponse, error) {
	resp, err := a.getClient(cfg).GetContentGenerationTask(ctx, model.GetContentGenerationTaskRequest{ID: id})
	if err != nil { return nil, err }
	res := &dto.TaskStatusResponse{Output: dto.TaskStatusOutput{TaskID: resp.ID, TaskStatus: resp.Status, VideoURL: resp.Content.VideoURL}}
	if res.Output.VideoURL == "" { res.Output.VideoURL = resp.Content.FileURL }
	if resp.Error != nil { res.Output.Code, res.Output.Message = resp.Error.Code, resp.Error.Message }
	return res, nil
}

func (a *ArkAdaptor) toChatReq(r *dto.ChatRequest) model.CreateChatCompletionRequest {
	req := model.CreateChatCompletionRequest{Model: r.Model}
	for _, m := range r.Messages {
		req.Messages = append(req.Messages, &model.ChatCompletionMessage{Role: m.Role, Content: &model.ChatCompletionMessageContent{StringValue: volcengine.String(fmt.Sprint(m.Content))}})
	}
	return req
}

func (a *ArkAdaptor) toImgReq(r *dto.MediaRequest) model.GenerateImagesRequest {
	req := model.GenerateImagesRequest{Model: r.Model, Prompt: r.Prompt}
	if r.Size != "" { req.Size = &r.Size }
	if r.Seed != 0 { req.Seed = volcengine.Int64(int64(r.Seed)) }
	if r.ResponseFormat != "" { req.ResponseFormat = &r.ResponseFormat }
	if r.Extra != nil {
		if v, ok := r.Extra["guidance_scale"].(float64); ok { req.GuidanceScale = &v }
		if v, ok := r.Extra["watermark"].(bool); ok { req.Watermark = &v }
		if v, ok := r.Extra["optimize_prompt"].(bool); ok { req.OptimizePrompt = &v }
	}
	return req
}

func (a *ArkAdaptor) toVidReq(r *dto.MediaRequest) model.CreateContentGenerationTaskRequest {
	req := model.CreateContentGenerationTaskRequest{Model: r.Model, Content: []*model.CreateContentGenerationContentItem{{Type: model.ContentGenerationContentItemTypeText, Text: &r.Prompt}}, ExtraBody: make(model.ExtraBody)}
	if r.Duration > 0 { req.Duration = volcengine.Int64(int64(r.Duration)) }
	if r.Seed != 0 { req.Seed = volcengine.Int64(int64(r.Seed)) }
	if r.Size != "" {
		if strings.Contains(r.Size, "p") || strings.Contains(r.Size, "x") { req.Resolution = &r.Size } else { req.Ratio = &r.Size }
	}
	if r.Resolution != "" { req.Resolution = &r.Resolution }
	for k, v := range r.Extra {
		switch k {
		case "service_tier": if s, ok := v.(string); ok { req.ServiceTier = &s }
		case "watermark": if b, ok := v.(bool); ok { req.Watermark = &b }
		case "frames": if f, ok := v.(float64); ok { req.Frames = volcengine.Int64(int64(f)) }
		default: req.ExtraBody[k] = v
		}
	}
	return req
}

type arkStream struct{ r *utils.ChatCompletionStreamReader }
func (w *arkStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	resp, err := w.r.Recv()
	if err != nil { return nil, err }
	if len(resp.Choices) == 0 { return nil, io.EOF }
	return &dto.StreamToken{Text: resp.Choices[0].Delta.Content, Type: "text", Index: resp.Choices[0].Index}, nil
}
func (w *arkStream) Close() error { return w.r.Close() }

type arkMediaStream struct{ r *utils.ImageGenerationStreamReader }
func (w *arkMediaStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	resp, err := w.r.Recv()
	if err != nil { return nil, err }
	return &dto.StreamToken{Type: resp.Type, Index: int(resp.ImageIndex), URL: volcengine.StringValue(resp.Url), Text: volcengine.StringValue(resp.B64Json)}, nil
}
func (w *arkMediaStream) Close() error { return w.r.Close() }

type arkVidStream struct {
	c  *arkruntime.Client
	id string
	last string
}
func (w *arkVidStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	for {
		resp, err := w.c.GetContentGenerationTask(ctx, model.GetContentGenerationTaskRequest{ID: w.id})
		if err != nil { return nil, err }
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
			if res.URL == "" { res.URL = resp.Content.FileURL }
			return res, io.EOF
		} else if resp.Status == model.StatusFailed {
			if resp.Error != nil { res.Text = resp.Error.Message }
			return res, fmt.Errorf("vid failed: %s", res.Text)
		}
		return res, nil
	}
}
func (w *arkVidStream) Close() error { return nil }
