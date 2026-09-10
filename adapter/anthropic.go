// Package adapter provides Anthropic adaptor implementation using official/community SDK.
package adapter

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/YspCoder/omnigo/dto"
	"github.com/liushuangls/go-anthropic/v2"
)

type AnthropicAdaptor struct {
	client      *anthropic.Client
	clientErr   error
	clientMutex sync.Mutex
}

func (a *AnthropicAdaptor) getClient(config *ProviderConfig) *anthropic.Client {
	client, _ := a.getClientWithError(config)
	return client
}

func (a *AnthropicAdaptor) getClientWithError(config *ProviderConfig) (*anthropic.Client, error) {
	a.clientMutex.Lock()
	defer a.clientMutex.Unlock()
	if a.client != nil {
		return a.client, nil
	}
	if a.clientErr != nil {
		return nil, a.clientErr
	}

	if config == nil {
		config = &ProviderConfig{}
	}
	httpClient, err := providerHTTPClient(config)
	if err != nil {
		a.clientErr = err
		return nil, err
	}
	opts := []anthropic.ClientOption{anthropic.WithHTTPClient(httpClient)}
	if baseURL := strings.TrimSpace(config.BaseURL); baseURL != "" {
		opts = append(opts, anthropic.WithBaseURL(baseURL))
	}
	client := anthropic.NewClient(config.APIKey, opts...)
	a.client = client
	return client, nil
}

func (a *AnthropicAdaptor) Chat(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	client, err := a.getClientWithError(config)
	if err != nil {
		return nil, err
	}

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

	res := &dto.MediaResponse{
		Choices: []dto.ChatChoice{{
			Message: dto.Message{
				Role:    "assistant",
				Content: resp.Content[0].GetText(),
			},
		}},
		Text: resp.Content[0].GetText(),
		Usage: dto.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
	return res, nil
}

func (a *AnthropicAdaptor) Stream(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("stream not implemented for Anthropic SDK yet")
}

func (a *AnthropicAdaptor) Media(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	return nil, fmt.Errorf("media generation not supported by Anthropic")
}

func (a *AnthropicAdaptor) TaskStatus(ctx context.Context, config *ProviderConfig, taskID string, _ ...map[string]string) (*dto.TaskStatusResponse, error) {
	return nil, fmt.Errorf("task status not supported by Anthropic")
}

func (a *AnthropicAdaptor) ListTasks(ctx context.Context, config *ProviderConfig, query map[string]string) (*dto.TaskListResponse, error) {
	return nil, fmt.Errorf("task list not supported by Anthropic")
}

func (a *AnthropicAdaptor) StreamMedia(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("streaming media not supported by Anthropic adaptor")
}
