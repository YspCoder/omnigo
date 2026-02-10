// Package adapter provides Google Gemini adaptor implementation using the new official genai SDK.
package adapter

import (
	"context"
	"fmt"

	"github.com/YspCoder/omnigo/dto"
	"google.golang.org/genai"
)

type GoogleAdaptor struct {
	client *genai.Client
}

func (a *GoogleAdaptor) getClient(ctx context.Context, config *ProviderConfig) (*genai.Client, error) {
	if a.client != nil {
		return a.client, nil
	}
	
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  config.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}
	a.client = client
	return client, nil
}

func (a *GoogleAdaptor) Chat(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (*dto.ChatResponse, error) {
	client, err := a.getClient(ctx, config)
	if err != nil {
		return nil, err
	}

	cfg := &genai.GenerateContentConfig{}
	if request.Temperature != 0 {
		cfg.Temperature = genai.Ptr(float32(request.Temperature))
	}
	if request.MaxTokens != 0 {
		cfg.MaxOutputTokens = int32(request.MaxTokens)
	}
	
	// Convert messages to genai.Content
	contents := make([]*genai.Content, 0, len(request.Messages))
	for _, m := range request.Messages {
		contents = append(contents, &genai.Content{
			Role: m.Role,
			Parts: []*genai.Part{{
				Text: fmt.Sprint(m.Content),
			}},
		})
	}

	resp, err := client.Models.GenerateContent(ctx, request.Model, contents, cfg)
	if err != nil {
		return nil, err
	}

	res := &dto.ChatResponse{}
	for _, cand := range resp.Candidates {
		if cand.Content != nil && len(cand.Content.Parts) > 0 {
			res.Choices = append(res.Choices, dto.ChatChoice{
				Message: dto.Message{
					Role:    cand.Content.Role,
					Content: cand.Content.Parts[0].Text,
				},
				FinishReason: string(cand.FinishReason),
			})
		}
	}
	return res, nil
}

func (a *GoogleAdaptor) Stream(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("stream not implemented for Google genai SDK in this adaptor")
}

func (a *GoogleAdaptor) Media(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	return nil, fmt.Errorf("media mode %s not supported by Google genai SDK adaptor", request.Type)
}

func (a *GoogleAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, taskID string) (*dto.TaskStatusResponse, error) {
	return nil, fmt.Errorf("task status not supported by Google")
}
