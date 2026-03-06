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
	TaskStatus(ctx context.Context, taskID string) (*dto.TaskStatusResponse, error)
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

	llmClient.adaptorCfg = &adapter.ProviderConfig{
		Name:         spec.Name,
		APIKey:       cfg.APIKeys[cfg.Provider],
		AccessKey:    cfg.AccessKey,
		SecretKey:    cfg.SecretKey,
		Model:        cfg.Model,
		BaseURL:      cfg.Endpoint,
		Organization: cfg.APIKeys["organization"],
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

// Generate produces text based on the given prompt and options.
func (l *LLMImpl) Generate(ctx context.Context, prompt *Prompt, opts ...GenerateOption) (string, error) {
	resp, err := l.GenerateWithResponse(ctx, prompt, opts...)
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

// GenerateWithResponse produces text and preserves the raw provider response.
func (l *LLMImpl) GenerateWithResponse(ctx context.Context, prompt *Prompt, opts ...GenerateOption) (*dto.GenerateResponse, error) {
	prompt = l.effectivePrompt(prompt)

	options := make(map[string]interface{})
	l.optionsMutex.RLock()
	for k, v := range l.Options {
		options[k] = v
	}
	l.optionsMutex.RUnlock()

	request := &dto.ChatRequest{
		Model:       l.config.Model,
		Prompt:      prompt.String(),
		Messages:    toDTOMessages(prompt),
		Temperature: l.config.Temperature,
		MaxTokens:   l.config.MaxTokens,
		Options:     options,
	}

	resp, err := l.relay.Chat(ctx, l.adaptor, l.adaptorCfg, request)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) > 0 {
		content := fmt.Sprint(resp.Choices[0].Message.Content)
		return &dto.GenerateResponse{Text: content, Raw: resp}, nil
	}
	return nil, fmt.Errorf("no choices in response")
}

// Stream initiates a streaming response.
func (l *LLMImpl) Stream(ctx context.Context, prompt *Prompt, opts ...StreamOption) (dto.TokenStream, error) {
	prompt = l.effectivePrompt(prompt)

	request := &dto.ChatRequest{
		Model:    l.config.Model,
		Prompt:   prompt.String(),
		Messages: toDTOMessages(prompt),
		Stream:   true,
	}
	return l.relay.Stream(ctx, l.adaptor, nil, l.adaptorCfg, request)
}

// Media initiates an image/video generation request.
func (l *LLMImpl) Media(ctx context.Context, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	request = l.effectiveMediaRequest(request)
	return l.relay.Media(ctx, l.adaptor, l.adaptorCfg, request)
}

func (l *LLMImpl) StreamMedia(ctx context.Context, request *dto.MediaRequest) (dto.TokenStream, error) {
	request = l.effectiveMediaRequest(request)
	return l.relay.StreamMedia(ctx, l.adaptor, l.adaptorCfg, request)
}

// TaskStatus queries a provider task status.
func (l *LLMImpl) TaskStatus(ctx context.Context, taskID string) (*dto.TaskStatusResponse, error) {
	return l.relay.TaskStatus(ctx, l.adaptor, l.adaptorCfg, taskID)
}

func toDTOMessages(prompt *Prompt) []dto.Message {
	var msgs []dto.Message
	if prompt.SystemPrompt != "" {
		msgs = append(msgs, dto.Message{Role: "system", Content: prompt.SystemPrompt})
	}
	for _, m := range prompt.Messages {
		msgs = append(msgs, dto.Message{Role: m.Role, Content: m.Content})
	}
	if len(msgs) == 0 && prompt.Input != "" {
		msgs = append(msgs, dto.Message{Role: "user", Content: prompt.Input})
	}
	return msgs
}
