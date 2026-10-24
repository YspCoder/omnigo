// Package llm provides a unified interface for interacting with various Language Learning Model providers.
package llm

import (
	"context"
	"fmt"
	"maps"
	"strings"
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
	Options      map[string]interface{}
	optionsMutex sync.RWMutex
	logger       utils.Logger
	config       *config.Config
	configMutex  sync.RWMutex
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
	baseURL := strings.TrimSpace(cfg.Endpoint)
	if baseURL == "" {
		baseURL = spec.Endpoint
	}

	llmClient := &LLMImpl{
		logger:  logger,
		config:  cfg,
		Options: make(map[string]interface{}),
		relay:   relay.NewRelay(),
		adaptor: adp,
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
		Headers:      maps.Clone(cfg.ExtraHeaders),
		Proxy:        cfg.Proxy,
		HTTPClient:   cfg.HTTPClient,
		ChatProtocol: cfg.ChatProtocol,
		Timeout:      cfg.Timeout,
		MaxRetries:   cfg.MaxRetries,
		RetryDelay:   cfg.RetryDelay,
	}
	if err := adapter.PrepareProviderConfig(llmClient.adaptorCfg); err != nil {
		return nil, fmt.Errorf("prepare provider client: %w", err)
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
	return NewPrompt(prompt)
}

// SetSystemPrompt updates the default prompt atomically with its cache setting.
func (l *LLMImpl) SetSystemPrompt(prompt string, cacheType CacheType) {
	l.configMutex.Lock()
	defer l.configMutex.Unlock()
	l.config.SystemPrompt = prompt
	l.config.SystemPromptCacheType = string(cacheType)
}

func (l *LLMImpl) effectivePrompt(prompt *Prompt) *Prompt {
	l.configMutex.RLock()
	defer l.configMutex.RUnlock()
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

// normalizeMediaRequest creates the canonical request passed to every adaptor.
// It applies client defaults without mutating the caller's request, so provider
// code only needs to handle provider-specific fields and protocols.
func (l *LLMImpl) normalizeMediaRequest(request *dto.MediaRequest) (*dto.MediaRequest, error) {
	if request == nil {
		return nil, fmt.Errorf("media request is required")
	}

	l.configMutex.RLock()
	var defaultModel, defaultSystemPrompt string
	if l.config != nil {
		defaultModel = l.config.Model
		defaultSystemPrompt = l.config.SystemPrompt
	}
	l.configMutex.RUnlock()

	cloned := cloneMediaRequest(request)
	cloned.Type = dto.MediaType(strings.ToLower(strings.TrimSpace(string(cloned.Type))))
	if cloned.Type == "" {
		cloned.Type = dto.MediaTypeText
	}
	cloned.Model = strings.TrimSpace(cloned.Model)
	if cloned.Model == "" {
		cloned.Model = strings.TrimSpace(defaultModel)
	}
	cloned.Prompt = strings.TrimSpace(cloned.Prompt)
	cloned.Size = strings.TrimSpace(cloned.Size)
	cloned.Resolution = strings.TrimSpace(cloned.Resolution)
	cloned.ResponseFormat = strings.TrimSpace(cloned.ResponseFormat)

	hasSystemPrompt := false
	for _, message := range request.Messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "system") {
			hasSystemPrompt = true
			break
		}
	}
	for index := range cloned.Messages {
		cloned.Messages[index].Role = strings.ToLower(strings.TrimSpace(cloned.Messages[index].Role))
	}
	if !hasSystemPrompt && strings.TrimSpace(defaultSystemPrompt) != "" {
		messages := cloned.Messages
		cloned.Messages = make([]dto.Message, 0, len(request.Messages)+1)
		cloned.Messages = append(cloned.Messages, dto.Message{
			Role:    "system",
			Content: strings.TrimSpace(defaultSystemPrompt),
		})
		cloned.Messages = append(cloned.Messages, messages...)
	}
	return cloned, nil
}

// effectiveMediaRequest is retained for internal callers and older tests. New
// request paths should use normalizeMediaRequest so validation errors propagate.
func (l *LLMImpl) effectiveMediaRequest(request *dto.MediaRequest) *dto.MediaRequest {
	normalized, err := l.normalizeMediaRequest(request)
	if err != nil {
		return &dto.MediaRequest{Type: dto.MediaTypeText}
	}
	return normalized
}

func cloneMediaRequest(request *dto.MediaRequest) *dto.MediaRequest {
	cloned := *request
	cloned.Messages = append([]dto.Message(nil), request.Messages...)
	for index := range cloned.Messages {
		cloned.Messages[index].ToolCalls = append([]dto.ToolCall(nil), request.Messages[index].ToolCalls...)
	}
	cloned.Extra = maps.Clone(request.Extra)
	cloned.Metadata = maps.Clone(request.Metadata)
	cloned.Options = maps.Clone(request.Options)
	cloned.Tools = append([]dto.Tool(nil), request.Tools...)
	return &cloned
}

func (l *LLMImpl) textRequest(prompt *Prompt, stream bool, options map[string]interface{}) *dto.MediaRequest {
	messages := toDTOMessages(prompt)
	request := &dto.MediaRequest{
		Type:        dto.MediaTypeText,
		Model:       l.config.Model,
		Messages:    messages,
		Prompt:      mediaPromptString(messages),
		Temperature: l.config.Temperature,
		MaxTokens:   l.config.MaxTokens,
		Stream:      stream,
		Options:     options,
		Tools:       toDTOTools(prompt.Tools),
	}
	if len(prompt.ToolChoice) > 0 {
		request.ToolChoice = prompt.ToolChoice
	}
	return request
}

func (l *LLMImpl) generationOptions() map[string]interface{} {
	options := map[string]interface{}{"temperature": l.config.Temperature, "top_p": l.config.TopP}
	if l.config.FrequencyPenalty != 0 {
		options["frequency_penalty"] = l.config.FrequencyPenalty
	}
	if l.config.PresencePenalty != 0 {
		options["presence_penalty"] = l.config.PresencePenalty
	}
	if l.config.Seed != nil {
		options["seed"] = *l.config.Seed
	}
	l.optionsMutex.RLock()
	defer l.optionsMutex.RUnlock()
	for key, value := range l.Options {
		options[key] = value
	}
	return options
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

	request := l.textRequest(prompt, false, l.generationOptions())

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

	request := l.textRequest(prompt, true, l.generationOptions())
	return l.relay.Stream(ctx, l.adaptor, nil, l.adaptorCfg, request)
}

// Media executes a unified multimodal request.
func (l *LLMImpl) Media(ctx context.Context, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	var err error
	request, err = l.normalizeMediaRequest(request)
	if err != nil {
		return nil, err
	}
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
	var err error
	request, err = l.normalizeMediaRequest(request)
	if err != nil {
		return nil, err
	}
	if request.Type == dto.MediaTypeText && !l.textMediaUsesProviderMedia(request) {
		cloned := *request
		cloned.Stream = true
		return l.relay.Stream(ctx, l.adaptor, nil, l.adaptorCfg, &cloned)
	}
	return l.relay.StreamMedia(ctx, l.adaptor, l.adaptorCfg, request)
}

func (l *LLMImpl) textMediaUsesProviderMedia(request *dto.MediaRequest) bool {
	router, ok := l.adaptor.(adapter.MediaRequestRouter)
	return ok && router.UsesMediaRequest(request)
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
	if len(prompt.Messages) == 0 {
		if content := prompt.userContent(prompt.Input); content != "" {
			msgs = append(msgs, dto.Message{Role: "user", Content: content})
		}
	} else {
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == "user" {
				msgs[i].Content = prompt.userContent(fmt.Sprint(msgs[i].Content))
				break
			}
		}
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
	var prompt strings.Builder
	for i, message := range messages {
		if i > 0 {
			prompt.WriteString("\n\n")
		}
		fmt.Fprint(&prompt, message.Content)
	}
	return prompt.String()
}
