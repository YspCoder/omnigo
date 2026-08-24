// Package llm provides a unified interface for interacting with various Language Learning Model providers.
package llm

import (
	"context"
	"fmt"
	"sync"

	"github.com/YspCoder/omnigo/adapter"
	"github.com/YspCoder/omnigo/config"
	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/relay"
	"github.com/YspCoder/omnigo/utils"
)

// LLM interface defines the methods for LLM interaction.
type LLM interface {
	Generate(ctx context.Context, prompt *Prompt, opts ...GenerateOption) (string, error)
	GenerateWithResponse(ctx context.Context, prompt *Prompt, opts ...GenerateOption) (*dto.GenerateResponse, error)
	Stream(ctx context.Context, prompt *Prompt, opts ...StreamOption) (dto.TokenStream, error)
	Media(ctx context.Context, request *dto.MediaRequest) (*dto.MediaResponse, error)
	StreamMedia(ctx context.Context, request *dto.MediaRequest) (dto.TokenStream, error)
	TaskStatus(ctx context.Context, taskID string, query ...map[string]string) (*dto.TaskStatusResponse, error)
	ListTasks(ctx context.Context, query map[string]string) (*dto.TaskListResponse, error)
	SetOption(key string, value interface{})
	SetLogLevel(level utils.LogLevel)
	NewPrompt(input string) *Prompt
}

// LLMImpl implements the LLM interface and manages interactions with specific providers.
type LLMImpl struct {
	providerName string
	Options      map[string]interface{}
	optionsMutex sync.RWMutex
	logger       utils.Logger
	config       *config.Config
	relay        *relay.Relay
	adaptor      adapter.Adaptor
	adaptorCfg   *adapter.ProviderConfig
}

// GenerateOption is a function type for configuring generation behavior.
type GenerateOption func(*GenerateConfig)

// GenerateConfig holds configuration options for text generation.
type GenerateConfig struct {
	UseJSONSchema bool
}

// NewLLM creates a new LLM instance with the specified configuration.
func NewLLM(cfg *config.Config, logger utils.Logger, registry *adapter.Registry) (LLM, error) {
	adp, spec, err := registry.BuildAdaptor(cfg.Provider)
	if err != nil {
		return nil, err
	}

	llmClient := &LLMImpl{
		providerName: spec.Name,
		logger:       logger,
		config:       cfg,
		Options:      make(map[string]interface{}),
		relay:        relay.NewRelay(),
		adaptor:      adp,
	}
	baseURL := cfg.Endpoint
	if baseURL == "" {
		baseURL = spec.Endpoint
	}
	if spec.EndpointRequired && baseURL == "" {
		return nil, fmt.Errorf("provider %s requires an endpoint", spec.Name)
	}
	headers := make(map[string]string, len(spec.Headers)+len(cfg.ExtraHeaders))
	for key, value := range spec.Headers {
		headers[key] = value
	}
	for key, value := range cfg.ExtraHeaders {
		headers[key] = value
	}

	llmClient.adaptorCfg = &adapter.ProviderConfig{
		Name:         spec.Name,
		APIKey:       cfg.APIKeys[cfg.Provider],
		AccessKey:    cfg.AccessKey,
		SecretKey:    cfg.SecretKey,
		Model:        cfg.Model,
		BaseURL:      baseURL,
		PollingURL:   cfg.PollingURL,
		Organization: cfg.APIKeys["organization"],
		Headers:      headers,
		Proxy:        cfg.Proxy,
		HTTPClient:   cfg.HTTPClient,
		ChatProtocol: cfg.ChatProtocol,
		Timeout:      cfg.Timeout,
	}

	return llmClient, nil
}

// SetOption sets a provider-specific option.
func (l *LLMImpl) SetOption(key string, value interface{}) {
	l.optionsMutex.Lock()
	defer l.optionsMutex.Unlock()
	l.Options[key] = value
}

// SetLogLevel updates the logging verbosity level.
func (l *LLMImpl) SetLogLevel(level utils.LogLevel) {
	l.logger.SetLevel(level)
}

// NewPrompt creates a new prompt instance.
func (l *LLMImpl) NewPrompt(prompt string) *Prompt {
	return &Prompt{Input: prompt}
}

func (l *LLMImpl) effectivePrompt(prompt *Prompt) *Prompt {
	if prompt == nil {
		return &Prompt{}
	}
	if prompt.SystemPrompt != "" || l.config == nil || l.config.SystemPrompt == "" {
		return prompt
	}

	cloned := *prompt
	cloned.SystemPrompt = l.config.SystemPrompt
	cloned.SystemCacheType = CacheType(l.config.SystemPromptCacheType)
	if prompt.Messages != nil {
		cloned.Messages = append([]PromptMessage(nil), prompt.Messages...)
	}
	return &cloned
}

func (l *LLMImpl) effectiveMediaRequest(request *dto.MediaRequest) *dto.MediaRequest {
	if request == nil {
		return &dto.MediaRequest{}
	}
	if l.config == nil || l.config.SystemPrompt == "" {
		return request
	}
	for _, message := range request.Messages {
		if message.Role == "system" {
			return request
		}
	}

	cloned := *request
	cloned.Messages = make([]dto.Message, 0, len(request.Messages)+1)
	cloned.Messages = append(cloned.Messages, dto.Message{
		Role:    "system",
		Content: l.config.SystemPrompt,
	})
	cloned.Messages = append(cloned.Messages, request.Messages...)
	return &cloned
}

func (l *LLMImpl) textRequest(prompt *Prompt, stream bool, options map[string]interface{}) *dto.MediaRequest {
	messages := toDTOMessages(prompt)
	return &dto.MediaRequest{
		Type:        dto.MediaTypeText,
		Model:       l.config.Model,
		Messages:    messages,
		Prompt:      mediaPromptString(messages),
		Temperature: l.config.Temperature,
		MaxTokens:   l.config.MaxTokens,
		Stream:      stream,
		Options:     options,
		Tools:       toDTOTools(prompt.Tools),
		ToolChoice:  prompt.ToolChoice,
	}
}

// Generate produces text based on the given prompt and routes it through the unified request pipeline.
func (l *LLMImpl) Generate(ctx context.Context, prompt *Prompt, opts ...GenerateOption) (string, error) {
	resp, err := l.GenerateWithResponse(ctx, prompt, opts...)
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

// GenerateWithResponse produces text and preserves the raw unified provider response.
func (l *LLMImpl) GenerateWithResponse(ctx context.Context, prompt *Prompt, opts ...GenerateOption) (*dto.GenerateResponse, error) {
	prompt = l.effectivePrompt(prompt)

	options := make(map[string]interface{})
	l.optionsMutex.RLock()
	for k, v := range l.Options {
		options[k] = v
	}
	l.optionsMutex.RUnlock()

	request := l.textRequest(prompt, false, options)

	resp, err := l.relay.Chat(ctx, l.adaptor, l.adaptorCfg, request)
	if err != nil {
		return nil, err
	}

	if resp.Text != "" {
		return &dto.GenerateResponse{Text: resp.Text, Raw: resp}, nil
	}
	if len(resp.Choices) > 0 {
		content := fmt.Sprint(resp.Choices[0].Message.Content)
		resp.Text = content
		return &dto.GenerateResponse{Text: content, Raw: resp}, nil
	}
	return nil, fmt.Errorf("no choices in response")
}

// Stream initiates a streaming text response through the unified request pipeline.
func (l *LLMImpl) Stream(ctx context.Context, prompt *Prompt, opts ...StreamOption) (dto.TokenStream, error) {
	prompt = l.effectivePrompt(prompt)

	request := l.textRequest(prompt, true, nil)
	return l.relay.Stream(ctx, l.adaptor, nil, l.adaptorCfg, request)
}

// Media executes a unified multimodal request.
func (l *LLMImpl) Media(ctx context.Context, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	request = l.effectiveMediaRequest(request)
	if request.Type == dto.MediaTypeText && !l.textMediaUsesProviderMedia(request) {
		resp, err := l.relay.Chat(ctx, l.adaptor, l.adaptorCfg, request)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
	return l.relay.Media(ctx, l.adaptor, l.adaptorCfg, request)
}

func (l *LLMImpl) StreamMedia(ctx context.Context, request *dto.MediaRequest) (dto.TokenStream, error) {
	request = l.effectiveMediaRequest(request)
	if request.Type == dto.MediaTypeText && !l.textMediaUsesProviderMedia(request) {
		request.Stream = true
		return l.relay.Stream(ctx, l.adaptor, nil, l.adaptorCfg, request)
	}
	return l.relay.StreamMedia(ctx, l.adaptor, l.adaptorCfg, request)
}

func (l *LLMImpl) textMediaUsesProviderMedia(request *dto.MediaRequest) bool {
	if request == nil {
		return false
	}
	switch l.providerName {
	case "vidu":
		return true
	default:
		return false
	}
}

// TaskStatus queries a provider task status.
func (l *LLMImpl) TaskStatus(ctx context.Context, taskID string, query ...map[string]string) (*dto.TaskStatusResponse, error) {
	return l.relay.TaskStatus(ctx, l.adaptor, l.adaptorCfg, taskID, query...)
}

func (l *LLMImpl) ListTasks(ctx context.Context, query map[string]string) (*dto.TaskListResponse, error) {
	return l.relay.ListTasks(ctx, l.adaptor, l.adaptorCfg, query)
}

func toDTOMessages(prompt *Prompt) []dto.Message {
	var msgs []dto.Message
	if prompt.SystemPrompt != "" {
		msgs = append(msgs, dto.Message{Role: "system", Content: prompt.SystemPrompt})
	}
	for _, m := range prompt.Messages {
		message := dto.Message{
			Role: m.Role, Content: m.Content, Name: m.Name, ToolCallID: m.ToolCallID,
		}
		for _, call := range m.ToolCalls {
			message.ToolCalls = append(message.ToolCalls, dto.ToolCall{
				ID: call.ID, Type: call.Type,
				Function: dto.ToolCallFunction{Name: call.Function.Name, Arguments: call.Function.Arguments},
			})
		}
		msgs = append(msgs, message)
	}
	if len(msgs) == 0 && prompt.Input != "" {
		msgs = append(msgs, dto.Message{Role: "user", Content: prompt.Input})
	}
	return msgs
}

func toDTOTools(tools []utils.Tool) []dto.Tool {
	result := make([]dto.Tool, 0, len(tools))
	for _, tool := range tools {
		result = append(result, dto.Tool{
			Type: tool.Type,
			Function: dto.ToolFunction{
				Name: tool.Function.Name, Description: tool.Function.Description,
				Parameters: tool.Function.Parameters,
			},
		})
	}
	return result
}

func mediaPromptString(messages []dto.Message) string {
	var prompt string
	for _, message := range messages {
		if prompt != "" {
			prompt += "\n\n"
		}
		prompt += fmt.Sprint(message.Content)
	}
	return prompt
}
