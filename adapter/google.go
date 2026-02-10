// Package adapter provides Google Gemini adaptor implementation using official SDK.
package adapter

import (
	"context"
	"fmt"

	"github.com/YspCoder/omnigo/dto"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type GoogleAdaptor struct {
	client *genai.Client
}

func (a *GoogleAdaptor) getClient(ctx context.Context, config *ProviderConfig) (*genai.Client, error) {
	if a.client != nil {
		return a.client, nil
	}
	client, err := genai.NewClient(ctx, option.WithAPIKey(config.APIKey))
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

	model := client.GenerativeModel(request.Model)
	if request.Temperature != 0 {
		model.SetTemperature(float32(request.Temperature))
	}
	if request.MaxTokens != 0 {
		model.SetMaxOutputTokens(int32(request.MaxTokens))
	}

	cs := model.StartChat()
	
	msg := fmt.Sprint(request.Prompt)
	if len(request.Messages) > 0 {
		msg = fmt.Sprint(request.Messages[len(request.Messages)-1].Content)
	}

	resp, err := cs.SendMessage(ctx, genai.Text(msg))
	if err != nil {
		return nil, err
	}

	res := &dto.ChatResponse{}
	for _, cand := range resp.Candidates {
		if cand.Content != nil && len(cand.Content.Parts) > 0 {
			res.Choices = append(res.Choices, dto.ChatChoice{
				Message: dto.Message{
					Role:    "assistant",
					Content: fmt.Sprint(cand.Content.Parts[0]),
				},
			})
		}
	}
	return res, nil
}

func (a *GoogleAdaptor) Stream(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("stream not implemented for Google SDK yet")
}

func (a *GoogleAdaptor) Media(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	return nil, fmt.Errorf("media generation not supported by Google Gemini SDK in this adaptor")
}

func (a *GoogleAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, taskID string) (*dto.TaskStatusResponse, error) {
	return nil, fmt.Errorf("task status not supported by Google")
}
