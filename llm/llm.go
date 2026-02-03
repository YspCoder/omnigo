// Package llm provides a unified interface for interacting with various Language Learning Model providers.
// It abstracts away provider-specific implementations and provides a consistent API for text generation,
// prompt management, and error handling.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/YspCoder/omnigo/adapter"
	"github.com/YspCoder/omnigo/config"
	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/relay"
	"github.com/YspCoder/omnigo/utils"
)

// LLM interface defines the methods that our internal language model should implement.
// It provides a unified way to interact with different LLM providers while abstracting
// away provider-specific details.
type LLM interface {
	// Generate produces text based on the given prompt and options.
	// Returns ErrorTypeRequest for request preparation failures,
	// ErrorTypeAPI for provider API errors, or ErrorTypeResponse for response processing issues.
	Generate(ctx context.Context, prompt *Prompt, opts ...GenerateOption) (response string, err error)

	// GenerateWithSchema generates text that conforms to a specific JSON schema.
	// Returns ErrorTypeInvalidInput for schema validation failures,
	// or other error types as per Generate.
	GenerateWithSchema(ctx context.Context, prompt *Prompt, schema interface{}, opts ...GenerateOption) (string, error)

	// Stream initiates a streaming response from the LLM.
	// Returns ErrorTypeUnsupported if the provider doesn't support streaming.
	Stream(ctx context.Context, prompt *Prompt, opts ...StreamOption) (TokenStream, error)

	// Media initiates an image/video generation request.
	Media(ctx context.Context, request *dto.MediaRequest) (*dto.MediaResponse, error)

	// TaskStatus queries a provider task status.
	TaskStatus(ctx context.Context, taskID string) (*dto.TaskStatusResponse, error)

	// SupportsStreaming checks if the provider supports streaming responses.
	SupportsStreaming() bool

	// SetOption configures a provider-specific option.
	// Returns ErrorTypeInvalidInput if the option is not supported.
	SetOption(key string, value interface{})

	// SetLogLevel adjusts the logging verbosity.
	SetLogLevel(level utils.LogLevel)

	// NewPrompt creates a new prompt instance.
	NewPrompt(input string) *Prompt

	// GetLogger returns the current logger instance.
	GetLogger() utils.Logger

	// SupportsJSONSchema checks if the provider supports JSON schema validation.
	SupportsJSONSchema() bool
}

// LLMImpl implements the LLM interface and manages interactions with specific providers.
// It handles provider communication, error management, and logging.
type LLMImpl struct {
	providerName      string                 // Provider identifier
	supportsSchema    bool                   // Supports JSON schema validation
	supportsStreaming bool                   // Supports streaming responses
	chatProtocol      string                 // Chat protocol format (e.g., openai)
	Options           map[string]interface{} // Provider-specific options
	optionsMutex      sync.RWMutex           // Mutex to protect concurrent access to Options map
	client            *http.Client           // HTTP client for API requests
	logger            utils.Logger           // Logger for debugging and monitoring
	config            *config.Config         // Configuration settings
	MaxRetries        int                    // Maximum number of retry attempts
	RetryDelay        time.Duration          // Delay between retry attempts
	relay             *relay.Relay
	adaptor           adapter.Adaptor
	adaptorCfg        *adapter.ProviderConfig
}

// GenerateOption is a function type for configuring generation behavior.
type GenerateOption func(*GenerateConfig)

// GenerateConfig holds configuration options for text generation.
type GenerateConfig struct {
	UseJSONSchema bool // Whether to use JSON schema validation
}

// NewLLM creates a new LLM instance with the specified configuration.
// It initializes the appropriate provider and sets up logging and HTTP clients.
//
// Returns:
//   - Configured LLM instance
//   - ErrorTypeProvider if provider initialization fails
//   - ErrorTypeAuthentication if API key validation fails
func NewLLM(cfg *config.Config, logger utils.Logger, registry *adapter.Registry) (LLM, error) {
	// Check if API key is empty for providers that need it
	apiKey := cfg.APIKeys[cfg.Provider]
	if apiKey == "" && cfg.Provider != "ollama" {
		return nil, NewLLMError(ErrorTypeAuthentication, "empty API key", nil)
	}

	adp, spec, err := registry.BuildAdaptor(cfg.Provider)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string)
	for key, value := range spec.RequiredHeaders {
		headers[key] = value
	}
	for key, value := range cfg.ExtraHeaders {
		if isReservedHeaderKey(key) {
			continue
		}
		headers[key] = value
	}

	baseURL := spec.Endpoint
	if cfg.Endpoint != "" {
		baseURL = cfg.Endpoint
	} else {
		if endpoint, ok := cfg.ExtraHeaders["azure_endpoint"]; ok && endpoint != "" {
			baseURL = endpoint
		}
		if endpoint, ok := cfg.ExtraHeaders["endpoint"]; ok && endpoint != "" {
			baseURL = endpoint
		}
	}

	llmClient := &LLMImpl{
		providerName:      spec.Name,
		supportsSchema:    spec.SupportsSchema,
		supportsStreaming: spec.SupportsStreaming,
		chatProtocol:      "openai",
		client:            &http.Client{Timeout: cfg.Timeout},
		logger:            logger,
		config:            cfg,
		MaxRetries:        cfg.MaxRetries,
		RetryDelay:        cfg.RetryDelay,
		Options:           make(map[string]interface{}),
	}

	llmClient.adaptor = adp
	llmClient.adaptorCfg = &adapter.ProviderConfig{
		Name:         spec.Name,
		APIKey:       apiKey,
		BaseURL:      baseURL,
		AuthHeader:   spec.AuthHeader,
		AuthPrefix:   spec.AuthPrefix,
		Headers:      headers,
		HTTPClient:   llmClient.client,
		Timeout:      cfg.Timeout,
		ChatProtocol: llmClient.chatProtocol,
	}
	llmClient.relay = relay.NewRelay()

	return llmClient, nil
}

func isReservedHeaderKey(key string) bool {
	switch strings.ToLower(key) {
	case "endpoint", "azure_endpoint":
		return true
	default:
		return false
	}
}

// SetOption sets a provider-specific option with the given key and value.
// The option is logged at debug level for troubleshooting.
func (l *LLMImpl) SetOption(key string, value interface{}) {
	l.optionsMutex.Lock()
	defer l.optionsMutex.Unlock()

	l.Options[key] = value
	l.logger.Debug("Option set", key, value)
}

// SetLogLevel updates the logging verbosity level.
func (l *LLMImpl) SetLogLevel(level utils.LogLevel) {
	l.logger.Debug("Setting internal LLM log level", "new_level", level)
	l.logger.SetLevel(level)
}

// GetLogger returns the current logger instance.
func (l *LLMImpl) GetLogger() utils.Logger {
	return l.logger
}

// NewPrompt creates a new prompt instance with the given input text.
func (l *LLMImpl) NewPrompt(prompt string) *Prompt {
	return &Prompt{Input: prompt}
}

// SupportsJSONSchema checks if the current provider supports JSON schema validation.
func (l *LLMImpl) SupportsJSONSchema() bool {
	return l.supportsSchema
}

// Generate produces text based on the given prompt and options.
// It handles retries, logging, and error management.
//
// Returns:
//   - Generated text response
//   - ErrorTypeRequest for request preparation failures
//   - ErrorTypeAPI for provider API errors
//   - ErrorTypeResponse for response processing issues
//   - ErrorTypeRateLimit if provider rate limit is exceeded
func (l *LLMImpl) Generate(ctx context.Context, prompt *Prompt, opts ...GenerateOption) (string, error) {
	config := &GenerateConfig{}
	for _, opt := range opts {
		opt(config)
	}
	// Set the system prompt in the LLM's options
	if prompt.SystemPrompt != "" {
		l.SetOption("system_prompt", prompt.SystemPrompt)
	}
	for attempt := 0; attempt <= l.MaxRetries; attempt++ {
		l.logger.Debug("Generating text", "provider", l.providerName, "prompt", prompt.String(), "system_prompt", prompt.SystemPrompt, "attempt", attempt+1)
		// Pass the entire Prompt struct to attemptGenerate
		result, err := l.attemptGenerate(ctx, prompt)
		if err == nil {
			return result, nil
		}
		l.logger.Warn("Generation attempt failed", "error", err, "attempt", attempt+1)
		if attempt < l.MaxRetries {
			l.logger.Debug("Retrying", "delay", l.RetryDelay)
			if err := l.wait(ctx); err != nil {
				return "", err
			}
		}
	}
	return "", fmt.Errorf("failed to generate after %d attempts", l.MaxRetries+1)
}

// wait implements a cancellable delay between retry attempts.
// Returns context.Canceled if the context is cancelled during the wait.
func (l *LLMImpl) wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(l.RetryDelay):
		return nil
	}
}

// attemptGenerate makes a single attempt to generate text using the provider.
// It handles request preparation, API communication, and response processing.
//
// Returns:
//   - Generated text response
//   - ErrorTypeRequest for request preparation failures
//   - ErrorTypeAPI for provider API errors
//   - ErrorTypeResponse for response processing issues
//   - ErrorTypeRateLimit if provider rate limit is exceeded
func (l *LLMImpl) attemptGenerate(ctx context.Context, prompt *Prompt) (string, error) {
	// Create a new options map that includes both l.Options and prompt-specific options
	options := make(map[string]interface{})

	// Safely read from the Options map
	l.optionsMutex.RLock()
	for k, v := range l.Options {
		options[k] = v
	}
	l.optionsMutex.RUnlock()

	// Add Tools and ToolChoice to options
	if len(prompt.Tools) > 0 {
		options["tools"] = prompt.Tools
	}
	if len(prompt.ToolChoice) > 0 {
		options["tool_choice"] = prompt.ToolChoice
	}

	options = applyDefaultOptions(options, l.config)

	messages := toDTOMessages(prompt.Messages)
	if l.useOpenAIProtocol() {
		options = filterOptions(options, "structured_messages")
	}

	request := &dto.ChatRequest{
		Model:    l.config.Model,
		Messages: messages,
		Prompt:   prompt.String(),
		Options:  options,
	}
	response, err := l.relay.Chat(ctx, l.adaptor, l.adaptorCfg, request)
	if err != nil {
		return "", NewLLMError(ErrorTypeAPI, "relay chat request failed", err)
	}

	result, err := firstChoiceContent(response)
	if err != nil {
		return "", NewLLMError(ErrorTypeResponse, "failed to parse response", err)
	}

	l.logger.Debug("Text generated successfully", "result", result)
	return result, nil
}

// GenerateWithSchema generates text that conforms to a specific JSON schema.
// It handles retries, logging, and error management.
//
// Returns:
//   - Generated text response
//   - ErrorTypeInvalidInput for schema validation failures
//   - Other error types as per Generate
func (l *LLMImpl) GenerateWithSchema(ctx context.Context, prompt *Prompt, schema interface{}, opts ...GenerateOption) (string, error) {
	config := &GenerateConfig{}
	for _, opt := range opts {
		opt(config)
	}

	var result string
	var lastErr error

	for attempt := 0; attempt <= l.MaxRetries; attempt++ {
		l.logger.Debug("Generating text with schema", "provider", l.providerName, "prompt", prompt.String(), "attempt", attempt+1)

		result, _, lastErr = l.attemptGenerateWithSchema(ctx, prompt.String(), schema)
		if lastErr == nil {
			return result, nil
		}

		l.logger.Warn("Generation attempt with schema failed", "error", lastErr, "attempt", attempt+1)

		if attempt < l.MaxRetries {
			l.logger.Debug("Retrying", "delay", l.RetryDelay)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(l.RetryDelay):
				// Continue to next attempt
			}
		}
	}

	return "", fmt.Errorf("failed to generate with schema after %d attempts: %w", l.MaxRetries+1, lastErr)
}

// attemptGenerateWithSchema makes a single attempt to generate text using the provider and a JSON schema.
// It handles request preparation, API communication, and response processing.
//
// Returns:
//   - Generated text response
//   - Full prompt used for generation
//   - ErrorTypeInvalidInput for schema validation failures
//   - Other error types as per attemptGenerate
func (l *LLMImpl) attemptGenerateWithSchema(ctx context.Context, prompt string, schema interface{}) (string, string, error) {
	var fullPrompt string

	l.optionsMutex.RLock()
	options := make(map[string]interface{})
	for k, v := range l.Options {
		options[k] = v
	}
	l.optionsMutex.RUnlock()

	if l.SupportsJSONSchema() {
		fullPrompt = prompt
	} else {
		fullPrompt = l.preparePromptWithSchema(prompt, schema)
	}

	options = applyDefaultOptions(options, l.config)
	if l.useOpenAIProtocol() {
		options = filterOptions(options, "structured_messages")
	}

	request := &dto.ChatRequest{
		Model:    l.config.Model,
		Messages: toDTOMessages([]PromptMessage{{Role: "user", Content: fullPrompt}}),
		Prompt:   fullPrompt,
		Options:  options,
	}
	if l.SupportsJSONSchema() {
		request.Schema = schema
	}

	response, err := l.relay.Chat(ctx, l.adaptor, l.adaptorCfg, request)
	if err != nil {
		return "", fullPrompt, NewLLMError(ErrorTypeAPI, "relay chat request failed", err)
	}

	result, err := firstChoiceContent(response)
	if err != nil {
		return "", fullPrompt, NewLLMError(ErrorTypeResponse, "failed to parse response", err)
	}

	// Validate the result against the schema
	if err := ValidateAgainstSchema(result, schema); err != nil {
		return "", fullPrompt, NewLLMError(ErrorTypeResponse, "response does not match schema", err)
	}

	l.logger.Debug("Text generated successfully", "result", result)
	return result, fullPrompt, nil
}

// preparePromptWithSchema prepares a prompt with a JSON schema for providers that do not support JSON schema validation.
// Returns the original prompt if schema marshaling fails (with a warning log).
func (l *LLMImpl) preparePromptWithSchema(prompt string, schema interface{}) string {
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		l.logger.Warn("Failed to marshal schema", "error", err)
		return prompt
	}

	return fmt.Sprintf("%s\n\nPlease provide your response in JSON format according to this schema:\n%s", prompt, string(schemaJSON))
}

// Stream initiates a streaming response from the LLM.
func (l *LLMImpl) Stream(ctx context.Context, prompt *Prompt, opts ...StreamOption) (TokenStream, error) {
	if !l.SupportsStreaming() {
		return nil, NewLLMError(ErrorTypeUnsupported, "streaming not supported by provider", nil)
	}

	// Apply stream options
	config := &StreamConfig{
		BufferSize: 100,
		RetryStrategy: &DefaultRetryStrategy{
			MaxRetries:  l.MaxRetries,
			InitialWait: l.RetryDelay,
			MaxWait:     l.RetryDelay * 10,
		},
	}
	for _, opt := range opts {
		opt(config)
	}

	var streamAdaptor adapter.StreamAdaptor
	if l.useOpenAIProtocol() {
		streamAdaptor = &adapter.OpenAIAdaptor{}
	} else if adaptor, ok := l.adaptor.(adapter.StreamAdaptor); ok {
		streamAdaptor = adaptor
	} else {
		return nil, NewLLMError(ErrorTypeUnsupported, "streaming not supported by adaptor", nil)
	}

	options := make(map[string]interface{})
	l.optionsMutex.RLock()
	for k, v := range l.Options {
		options[k] = v
	}
	l.optionsMutex.RUnlock()

	if len(prompt.Tools) > 0 {
		options["tools"] = prompt.Tools
	}
	if len(prompt.ToolChoice) > 0 {
		options["tool_choice"] = prompt.ToolChoice
	}
	options = applyDefaultOptions(options, l.config)
	options["stream"] = true
	options["stream_options"] = map[string]interface{}{
		"include_usage": true,
	}

	messages := toDTOMessages(prompt.Messages)
	if l.useOpenAIProtocol() {
		options = filterOptions(options, "structured_messages")
	}

	request := &dto.ChatRequest{
		Model:    l.config.Model,
		Messages: messages,
		Prompt:   prompt.String(),
		Options:  options,
	}

	adaptorCfg := *l.adaptorCfg
	if headerProvider, ok := l.adaptor.(adapter.StreamHeadersProvider); ok {
		extraHeaders := headerProvider.StreamHeaders(&adaptorCfg)
		if len(extraHeaders) > 0 {
			headers := make(map[string]string, len(adaptorCfg.Headers)+len(extraHeaders))
			for k, v := range adaptorCfg.Headers {
				headers[k] = v
			}
			for k, v := range extraHeaders {
				headers[k] = v
			}
			adaptorCfg.Headers = headers
		}
	}
	body, err := l.relay.Stream(ctx, l.adaptor, streamAdaptor, &adaptorCfg, request)
	if err != nil {
		return nil, NewLLMError(ErrorTypeAPI, "relay stream request failed", err)
	}

	return newProviderStream(body, streamAdaptor, config), nil
}

// Image initiates an image generation request.
// Media initiates an image/video generation request.
func (l *LLMImpl) Media(ctx context.Context, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	if request == nil {
		return nil, NewLLMError(ErrorTypeInvalidInput, "media request is nil", nil)
	}
	if request.Model == "" {
		request.Model = l.config.Model
	}

	adaptorCfg := *l.adaptorCfg
	adaptorCfg.Model = request.Model
	response, err := l.relay.Media(ctx, l.adaptor, &adaptorCfg, request)
	if err != nil {
		return nil, NewLLMError(ErrorTypeAPI, "relay media request failed", err)
	}

	return response, nil
}

// TaskStatus queries a provider task status.
func (l *LLMImpl) TaskStatus(ctx context.Context, taskID string) (*dto.TaskStatusResponse, error) {
	response, err := l.relay.TaskStatus(ctx, l.adaptor, l.adaptorCfg, taskID)
	if err != nil {
		return nil, NewLLMError(ErrorTypeAPI, "relay task status request failed", err)
	}
	return response, nil
}

func toDTOMessages(messages []PromptMessage) []dto.Message {
	if len(messages) == 0 {
		return nil
	}
	converted := make([]dto.Message, 0, len(messages))
	for _, msg := range messages {
		converted = append(converted, dto.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	return converted
}

func firstChoiceContent(response *dto.ChatResponse) (string, error) {
	if response == nil || len(response.Choices) == 0 {
		return "", fmt.Errorf("empty response choices")
	}
	content := response.Choices[0].Message.Content
	if content == nil {
		return "", fmt.Errorf("empty response content")
	}
	return fmt.Sprint(content), nil
}

func filterOptions(options map[string]interface{}, keys ...string) map[string]interface{} {
	if len(options) == 0 || len(keys) == 0 {
		return options
	}
	filtered := make(map[string]interface{}, len(options))
	skip := map[string]struct{}{}
	for _, key := range keys {
		skip[key] = struct{}{}
	}
	for key, value := range options {
		if _, ok := skip[key]; ok {
			continue
		}
		filtered[key] = value
	}
	return filtered
}

func (l *LLMImpl) useOpenAIProtocol() bool {
	return l.chatProtocol == "openai"
}

func applyDefaultOptions(options map[string]interface{}, cfg *config.Config) map[string]interface{} {
	if options == nil {
		options = make(map[string]interface{})
	}
	if _, ok := options["temperature"]; !ok {
		options["temperature"] = cfg.Temperature
	}
	if _, ok := options["max_tokens"]; !ok {
		options["max_tokens"] = cfg.MaxTokens
	}
	if _, ok := options["top_p"]; !ok && cfg.TopP != 0 {
		options["top_p"] = cfg.TopP
	}
	if _, ok := options["frequency_penalty"]; !ok && cfg.FrequencyPenalty != 0 {
		options["frequency_penalty"] = cfg.FrequencyPenalty
	}
	if _, ok := options["presence_penalty"]; !ok && cfg.PresencePenalty != 0 {
		options["presence_penalty"] = cfg.PresencePenalty
	}
	if _, ok := options["seed"]; !ok && cfg.Seed != nil {
		options["seed"] = *cfg.Seed
	}
	if _, ok := options["min_p"]; !ok && cfg.MinP != nil {
		options["min_p"] = *cfg.MinP
	}
	if _, ok := options["repeat_penalty"]; !ok && cfg.RepeatPenalty != nil {
		options["repeat_penalty"] = *cfg.RepeatPenalty
	}
	if _, ok := options["repeat_last_n"]; !ok && cfg.RepeatLastN != nil {
		options["repeat_last_n"] = *cfg.RepeatLastN
	}
	if _, ok := options["mirostat"]; !ok && cfg.Mirostat != nil {
		options["mirostat"] = *cfg.Mirostat
	}
	if _, ok := options["mirostat_eta"]; !ok && cfg.MirostatEta != nil {
		options["mirostat_eta"] = *cfg.MirostatEta
	}
	if _, ok := options["mirostat_tau"]; !ok && cfg.MirostatTau != nil {
		options["mirostat_tau"] = *cfg.MirostatTau
	}
	if _, ok := options["tfs_z"]; !ok && cfg.TfsZ != nil {
		options["tfs_z"] = *cfg.TfsZ
	}
	return options
}

// SupportsStreaming checks if the provider supports streaming responses.
func (l *LLMImpl) SupportsStreaming() bool {
	if _, ok := l.adaptor.(adapter.StreamAdaptor); ok {
		return true
	}
	return l.supportsStreaming
}

// providerStream implements TokenStream for a specific provider
type providerStream struct {
	decoder       *SSEDecoder
	parser        interface{ ParseStreamResponse([]byte) (string, error) }
	config        *StreamConfig
	buffer        []byte
	currentIndex  int
	retryStrategy RetryStrategy
	reader        io.ReadCloser
}

func newProviderStream(reader io.ReadCloser, parser interface{ ParseStreamResponse([]byte) (string, error) }, config *StreamConfig) *providerStream {
	return &providerStream{
		decoder:       NewSSEDecoder(reader),
		parser:        parser,
		config:        config,
		buffer:        make([]byte, 0, 4096),
		currentIndex:  0,
		retryStrategy: config.RetryStrategy,
		reader:        reader,
	}
}

func (s *providerStream) Next(ctx context.Context) (*StreamToken, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			if !s.decoder.Next() {
				if err := s.decoder.Err(); err != nil {
					if s.retryStrategy.ShouldRetry(err) {
						time.Sleep(s.retryStrategy.NextDelay())
						continue
					}
					return nil, err
				}
				return nil, io.EOF
			}

			event := s.decoder.Event()
			if len(event.Data) == 0 {
				continue
			}

			// Process the event
			token, err := s.parser.ParseStreamResponse(event.Data)
			if err != nil {
				if err.Error() == "skip token" {
					continue
				}
				if err == io.EOF {
					return nil, io.EOF
				}
				continue // Not enough data or malformed
			}

			// Create and return token
			return &StreamToken{
				Text:  token,
				Type:  event.Type,
				Index: s.currentIndex,
			}, nil
		}
	}
}

func (s *providerStream) Close() error {
	if s.reader == nil {
		return nil
	}
	return s.reader.Close()
}
