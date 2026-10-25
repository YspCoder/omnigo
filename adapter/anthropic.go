// Package adapter provides Anthropic adaptor implementation using official/community SDK.
package adapter

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
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
	httpClient = httpClientWithHeaders(httpClient, config.Headers)
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

	for _, m := range request.Messages {
		if m.Role == "system" {
			continue
		}
		messages = append(messages, anthropic.Message{
			Role:    anthropic.ChatRole(m.Role),
			Content: []anthropic.MessageContent{anthropic.NewTextMessageContent(fmt.Sprint(m.Content))},
		})
	}

	params := anthropic.MessagesRequest{
		Model:     anthropic.Model(request.Model),
		Messages:  messages,
		System:    utils.FirstSystemMessage(request.Messages),
		MaxTokens: request.MaxTokens,
	}
	if temperature, ok := requestTemperature(request); ok {
		params.SetTemperature(float32(temperature))
	}
	resp, err := client.CreateMessages(ctx, params)
	if err != nil {
		return nil, err
	}

	var text strings.Builder
	for _, content := range resp.Content {
		text.WriteString(content.GetText())
	}
	usage := usageFromValue(resp.Usage)
	usage.PromptTokens = resp.Usage.InputTokens + resp.Usage.CacheReadInputTokens + resp.Usage.CacheCreationInputTokens
	usage.CompletionTokens = resp.Usage.OutputTokens
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	res := &dto.MediaResponse{
		ID:     resp.ID,
		Object: string(resp.Type),
		Model:  string(resp.Model),
		Choices: []dto.ChatChoice{{
			Message: dto.Message{
				Role:    firstNonEmptyString(string(resp.Role), "assistant"),
				Content: text.String(),
			},
			FinishReason: string(resp.StopReason),
		}},
		Text:  text.String(),
		Usage: usage,
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
