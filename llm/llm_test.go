package llm

import (
	"context"
	"testing"

	"github.com/YspCoder/omnigo/adapter"
	"github.com/YspCoder/omnigo/config"
	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/relay"
	"github.com/YspCoder/omnigo/utils"
)

func TestNewLLM_AllowsAKSKWithoutAPIKey(t *testing.T) {
	cfg := &config.Config{
		Provider:  "ark",
		Model:     "doubao-seed-1-6-250615",
		APIKeys:   map[string]string{},
		AccessKey: "test-ak",
		SecretKey: "test-sk",
	}

	logger := utils.NewLogger(utils.LogLevelOff)
	registry := adapter.NewRegistry("ark")

	_, err := NewLLM(cfg, logger, registry)
	if err != nil {
		t.Fatalf("expected NewLLM success with ak/sk, got error: %v", err)
	}
}

func TestNewLLMPassesExtraHeadersToAdaptor(t *testing.T) {
	cfg := &config.Config{
		Provider:     "custom",
		Model:        "test-model",
		Endpoint:     "https://example.com/v1/videos",
		APIKeys:      map[string]string{"custom": "test-api-key"},
		ExtraHeaders: map[string]string{"X-Tenant-ID": "tenant-1"},
	}

	client, err := NewLLM(cfg, utils.NewLogger(utils.LogLevelOff), adapter.NewRegistry("custom"))
	if err != nil {
		t.Fatalf("NewLLM error = %v", err)
	}
	impl := client.(*LLMImpl)
	if got := impl.adaptorCfg.Headers["X-Tenant-ID"]; got != "tenant-1" {
		t.Fatalf("X-Tenant-ID = %q, want tenant-1", got)
	}
}

func TestEffectivePromptUsesConfiguredSystemPrompt(t *testing.T) {
	client := &LLMImpl{
		config: &config.Config{
			SystemPrompt:          "You are a helpful assistant.",
			SystemPromptCacheType: "ephemeral",
		},
	}

	prompt := NewPrompt("hello")
	effective := client.effectivePrompt(prompt)

	if effective.SystemPrompt != "You are a helpful assistant." {
		t.Fatalf("expected configured system prompt, got %q", effective.SystemPrompt)
	}

	if effective.SystemCacheType != CacheTypeEphemeral {
		t.Fatalf("expected configured cache type, got %q", effective.SystemCacheType)
	}

	if prompt.SystemPrompt != "" {
		t.Fatalf("expected original prompt to remain unchanged, got %q", prompt.SystemPrompt)
	}
}

func TestEffectivePromptKeepsExplicitSystemPrompt(t *testing.T) {
	client := &LLMImpl{
		config: &config.Config{
			SystemPrompt: "configured prompt",
		},
	}

	prompt := NewPrompt("hello", WithSystemPrompt("explicit prompt", CacheTypeEphemeral))
	effective := client.effectivePrompt(prompt)

	if effective.SystemPrompt != "explicit prompt" {
		t.Fatalf("expected explicit system prompt to win, got %q", effective.SystemPrompt)
	}
}

func TestEffectiveMediaRequestUsesConfiguredSystemPrompt(t *testing.T) {
	client := &LLMImpl{
		config: &config.Config{
			SystemPrompt: "configured media prompt",
		},
	}

	request := &dto.MediaRequest{
		Messages: []dto.Message{{
			Role:    "user",
			Content: "draw a cat",
		}},
	}
	effective := client.effectiveMediaRequest(request)

	if len(effective.Messages) != 2 {
		t.Fatalf("expected injected system message plus original user message, got %d", len(effective.Messages))
	}

	if effective.Messages[0].Role != "system" {
		t.Fatalf("expected injected role system, got %q", effective.Messages[0].Role)
	}

	if effective.Messages[0].Content != "configured media prompt" {
		t.Fatalf("expected injected system prompt, got %#v", effective.Messages[0].Content)
	}

	if len(request.Messages) != 1 {
		t.Fatalf("expected original request to remain unchanged, got %d messages", len(request.Messages))
	}
}

func TestEffectiveMediaRequestKeepsExplicitSystemPrompt(t *testing.T) {
	client := &LLMImpl{
		config: &config.Config{
			SystemPrompt: "configured media prompt",
		},
	}

	request := &dto.MediaRequest{
		Messages: []dto.Message{
			{
				Role:    "system",
				Content: "explicit media prompt",
			},
			{
				Role:    "user",
				Content: "draw a cat",
			},
		},
	}
	effective := client.effectiveMediaRequest(request)

	if effective.Messages[0].Content != "explicit media prompt" {
		t.Fatalf("expected explicit system prompt to win, got %#v", effective.Messages[0].Content)
	}
}

func TestGenerateWithResponseReturnsPrimaryChoiceContent(t *testing.T) {
	client := &LLMImpl{
		config:     &config.Config{Model: "test-model"},
		relay:      relay.NewRelay(),
		adaptor:    stubAdaptor{response: &dto.MediaResponse{Choices: []dto.ChatChoice{{Message: dto.Message{Content: "hello"}}}, Text: "hello"}},
		adaptorCfg: &adapter.ProviderConfig{},
	}

	resp, err := client.GenerateWithResponse(nil, NewPrompt("hello"))
	if err != nil {
		t.Fatalf("GenerateWithResponse error = %v", err)
	}
	if resp.Text != "hello" {
		t.Fatalf("text = %q, want hello", resp.Text)
	}
	if resp.Raw == nil || len(resp.Raw.Choices) != 1 {
		t.Fatalf("raw response not preserved: %#v", resp.Raw)
	}
}

type stubAdaptor struct {
	response *dto.MediaResponse
	stream   dto.TokenStream
	media    *dto.MediaResponse
}

func (s stubAdaptor) Chat(_ context.Context, _ *adapter.ProviderConfig, _ *dto.MediaRequest) (*dto.MediaResponse, error) {
	return s.response, nil
}

func (s stubAdaptor) Stream(_ context.Context, _ *adapter.ProviderConfig, _ *dto.MediaRequest) (dto.TokenStream, error) {
	return s.stream, nil
}

func (s stubAdaptor) Media(_ context.Context, _ *adapter.ProviderConfig, _ *dto.MediaRequest) (*dto.MediaResponse, error) {
	return s.media, nil
}

func (s stubAdaptor) TaskStatus(_ context.Context, _ *adapter.ProviderConfig, _ string, _ ...map[string]string) (*dto.TaskStatusResponse, error) {
	return nil, nil
}

func (s stubAdaptor) ListTasks(_ context.Context, _ *adapter.ProviderConfig, _ map[string]string) (*dto.TaskListResponse, error) {
	return nil, nil
}

func (s stubAdaptor) StreamMedia(_ context.Context, _ *adapter.ProviderConfig, _ *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, nil
}

func TestMediaTypeTextUsesChat(t *testing.T) {
	client := &LLMImpl{
		config:     &config.Config{Model: "test-model"},
		relay:      relay.NewRelay(),
		adaptor:    stubAdaptor{response: &dto.MediaResponse{Choices: []dto.ChatChoice{{Message: dto.Message{Content: "hello from chat"}}}, Text: "hello from chat"}},
		adaptorCfg: &adapter.ProviderConfig{},
	}

	resp, err := client.Media(context.Background(), &dto.MediaRequest{
		Type: dto.MediaTypeText,
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if resp.Text != "hello from chat" {
		t.Fatalf("text = %q, want hello from chat", resp.Text)
	}
}

func TestStreamMediaTypeTextUsesChatStream(t *testing.T) {
	expected := &stubTokenStream{}
	client := &LLMImpl{
		config:     &config.Config{Model: "test-model"},
		relay:      relay.NewRelay(),
		adaptor:    stubAdaptor{stream: expected},
		adaptorCfg: &adapter.ProviderConfig{},
	}

	stream, err := client.StreamMedia(context.Background(), &dto.MediaRequest{
		Type: dto.MediaTypeText,
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("StreamMedia error = %v", err)
	}
	if stream != expected {
		t.Fatalf("stream = %#v, want %#v", stream, expected)
	}
}

func TestMediaTypeTextUsesProviderMediaForVidu(t *testing.T) {
	client := &LLMImpl{
		providerName: "vidu",
		config:       &config.Config{Model: "text-to-audio"},
		relay:        relay.NewRelay(),
		adaptor:      stubAdaptor{media: &dto.MediaResponse{TaskID: "task-1", Status: "created"}},
		adaptorCfg:   &adapter.ProviderConfig{},
	}

	resp, err := client.Media(context.Background(), &dto.MediaRequest{
		Type: dto.MediaTypeText,
		Messages: []dto.Message{
			{Role: "user", Content: "rain ambience"},
		},
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if resp.TaskID != "task-1" {
		t.Fatalf("task id = %q, want task-1", resp.TaskID)
	}
}

type stubTokenStream struct{}

func (s *stubTokenStream) Next(context.Context) (*dto.StreamToken, error) { return nil, nil }
func (s *stubTokenStream) Close() error                                   { return nil }
