// Package adapter provides Anthropic adaptor implementation using official/community SDK.
package adapter

import (
	"context"
	"fmt"

	"github.com/YspCoder/omnigo/dto"
	"github.com/liushuangls/go-anthropic/v2"
)

type AnthropicAdaptor struct {
	client *anthropic.Client
}

func (a *AnthropicAdaptor) getClient(config *ProviderConfig) *anthropic.Client {
	if a.client != nil {
		return a.client
	}
	client := anthropic.NewClient(config.APIKey)
	a.client = client
	return client
}

func (a *AnthropicAdaptor) Chat(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (*dto.ChatResponse, error) {
	client := a.getClient(config)

	messages := make([]anthropic.Message, 0)
	var system string
	
	for _, m := range request.Messages {
		if m.Role == "system" {
			system = fmt.Sprint(m.Content)
			continue
		}
		messages = append(messages, anthropic.Message{
			Role:    anthropic.ChatRole(m.Role),
			Content: []anthropic.MessageContent{anthropic.NewTextMessageContent(fmt.Sprint(m.Content))},
		})
	}

	resp, err := client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model:     anthropic.Model(request.Model),
		Messages:  messages,
		System:    system,
		MaxTokens: request.MaxTokens,
	})
	if err != nil {
		return nil, err
	}

	res := &dto.ChatResponse{
		Choices: []dto.ChatChoice{{
			Message: dto.Message{
				Role:    "assistant",
				Content: resp.Content[0].GetText(),
			},
		}},
		Usage: dto.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
	return res, nil
}

func (a *AnthropicAdaptor) Stream(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("stream not implemented for Anthropic SDK yet")
}

func (a *AnthropicAdaptor) Media(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	return nil, fmt.Errorf("media generation not supported by Anthropic")
}

func (a *AnthropicAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, taskID string) (*dto.TaskStatusResponse, error) {
	return nil, fmt.Errorf("task status not supported by Anthropic")
}

func (a *AnthropicAdaptor) StreamMedia(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("streaming media not supported by Anthropic adaptor")
}
