// Package adapter provides Google Gemini adaptor implementation using the new official genai SDK.
package adapter

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"iter"

	"github.com/YspCoder/omnigo/dto"
	"google.golang.org/genai"
)

type GoogleAdaptor struct {
	client *genai.Client
}

func (a *GoogleAdaptor) getClient(ctx context.Context, cfg *ProviderConfig) (*genai.Client, error) {
	if a.client != nil { return a.client, nil }
	c, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: cfg.APIKey, Backend: genai.BackendGeminiAPI})
	if err != nil { return nil, err }
	a.client = c
	return c, nil
}

func (a *GoogleAdaptor) Chat(ctx context.Context, cfg *ProviderConfig, r *dto.ChatRequest) (*dto.ChatResponse, error) {
	c, err := a.getClient(ctx, cfg)
	if err != nil { return nil, err }
	resp, err := c.Models.GenerateContent(ctx, r.Model, a.toContents(r), a.toGenCfg(r))
	if err != nil { return nil, err }
	res := &dto.ChatResponse{}
	for _, cand := range resp.Candidates {
		if cand.Content != nil && len(cand.Content.Parts) > 0 {
			res.Choices = append(res.Choices, dto.ChatChoice{Message: dto.Message{Role: cand.Content.Role, Content: cand.Content.Parts[0].Text}, FinishReason: string(cand.FinishReason)})
		}
	}
	return res, nil
}

func (a *GoogleAdaptor) Stream(ctx context.Context, cfg *ProviderConfig, r *dto.ChatRequest) (dto.TokenStream, error) {
	c, err := a.getClient(ctx, cfg)
	if err != nil { return nil, err }
	seq := c.Models.GenerateContentStream(ctx, r.Model, a.toContents(r), a.toGenCfg(r))
	next, stop := iter.Pull2(seq)
	return &googleStream{next: next, stop: stop}, nil
}

func (a *GoogleAdaptor) Media(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (*dto.MediaResponse, error) {
	if r.Type != dto.MediaTypeImage { return nil, fmt.Errorf("only image generation supported for Google") }
	c, err := a.getClient(ctx, cfg)
	if err != nil { return nil, err }
	resp, err := c.Models.GenerateImages(ctx, r.Model, r.Prompt, a.toImgCfg(r))
	if err != nil { return nil, err }
	res := &dto.MediaResponse{}
	for _, img := range resp.GeneratedImages {
		if img.Image == nil { continue }
		data := dto.ImageData{}
		if img.Image.GCSURI != "" { data.URL = img.Image.GCSURI }
		if len(img.Image.ImageBytes) > 0 { data.B64JSON = base64.StdEncoding.EncodeToString(img.Image.ImageBytes) }
		res.Data = append(res.Data, data)
	}
	if len(res.Data) > 0 { res.URL = res.Data[0].URL }
	return res, nil
}

func (a *GoogleAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, id string) (*dto.TaskStatusResponse, error) {
	return nil, fmt.Errorf("task status not supported by Google")
}

func (a *GoogleAdaptor) StreamMedia(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("streaming media not supported by Google adaptor")
}

// Helpers
func (a *GoogleAdaptor) toContents(r *dto.ChatRequest) []*genai.Content {
	res := make([]*genai.Content, 0, len(r.Messages))
	for _, m := range r.Messages {
		res = append(res, &genai.Content{Role: m.Role, Parts: []*genai.Part{{Text: fmt.Sprint(m.Content)}}})
	}
	return res
}

func (a *GoogleAdaptor) toGenCfg(r *dto.ChatRequest) *genai.GenerateContentConfig {
	cfg := &genai.GenerateContentConfig{}
	if r.Temperature != 0 { cfg.Temperature = genai.Ptr(float32(r.Temperature)) }
	if r.MaxTokens != 0 { cfg.MaxOutputTokens = int32(r.MaxTokens) }
	return cfg
}

func (a *GoogleAdaptor) toImgCfg(r *dto.MediaRequest) *genai.GenerateImagesConfig {
	cfg := &genai.GenerateImagesConfig{}
	if r.N > 0 { cfg.NumberOfImages = int32(r.N) }
	if r.ResponseFormat != "" { cfg.OutputMIMEType = r.ResponseFormat }
	if r.Size != "" { cfg.AspectRatio = r.Size }
	return cfg
}

type googleStream struct {
	stop func()
	next func() (*genai.GenerateContentResponse, error, bool)
}

func (w *googleStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	resp, err, ok := w.next()
	if !ok {
		if err != nil { return nil, err }
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
