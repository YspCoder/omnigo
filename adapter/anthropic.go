// Package adapter provides Anthropic adaptor implementation using official/community SDK.
package adapter

import (
	"context"
	"encoding/json"
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
	params, err := anthropicRequest(request)
	if err != nil {
		return nil, err
	}
	client, err := a.getClientWithError(config)
	if err != nil {
		return nil, err
	}

	resp, err := client.CreateMessages(ctx, params)
	if err != nil && resp.ID == "" && len(resp.Content) == 0 {
		return nil, err
	}
	return anthropicResponse(resp), err
}

func anthropicResponse(resp anthropic.MessagesResponse) *dto.MediaResponse {
	var text strings.Builder
	var calls []dto.ToolCall
	structured := false
	for _, content := range resp.Content {
		text.WriteString(content.GetText())
		if tool := content.MessageContentToolUse; content.Type == anthropic.MessagesContentTypeToolUse && tool != nil {
			calls = append(calls, dto.ToolCall{ID: tool.ID, Type: "function", Function: dto.ToolCallFunction{
				Name: tool.Name, Arguments: append(json.RawMessage(nil), tool.Input...),
			}})
		} else if content.Type != anthropic.MessagesContentTypeText || len(content.Citations) > 0 {
			structured = true
		}
	}
	var content any = text.String()
	if structured {
		content = resp.Content
	}
	res := &dto.MediaResponse{
		ID:     resp.ID,
		Object: string(resp.Type),
		Model:  string(resp.Model),
		Choices: []dto.ChatChoice{{
			Message: dto.Message{
				Role:      firstNonEmptyString(string(resp.Role), "assistant"),
				Content:   content,
				ToolCalls: calls,
			},
			FinishReason: string(resp.StopReason),
		}},
		Text:  text.String(),
		Usage: anthropicUsage(resp.Usage),
	}
	return res
}

func anthropicUsage(source anthropic.MessagesUsage) dto.Usage {
	usage := usageFromValue(source)
	usage.PromptTokens = source.InputTokens + source.CacheReadInputTokens + source.CacheCreationInputTokens
	usage.CompletionTokens = source.OutputTokens
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return usage
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
