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

	request := &dto.MediaRequest{Prompt: "draw a cat"}
	effective := client.effectiveMediaRequest(request)

	if len(effective.Messages) != 1 {
		t.Fatalf("expected one injected system message, got %d", len(effective.Messages))
	}

	if effective.Messages[0].Role != "system" {
		t.Fatalf("expected injected role system, got %q", effective.Messages[0].Role)
	}

	if effective.Messages[0].Content != "configured media prompt" {
		t.Fatalf("expected injected system prompt, got %#v", effective.Messages[0].Content)
	}

	if len(request.Messages) != 0 {
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
		Prompt: "draw a cat",
		Messages: []dto.Message{{
			Role:    "system",
			Content: "explicit media prompt",
		}},
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
		adaptor:    stubAdaptor{response: &dto.ChatResponse{Choices: []dto.ChatChoice{{Message: dto.Message{Content: "hello"}}}}},
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
	response *dto.ChatResponse
}

func (s stubAdaptor) Chat(_ context.Context, _ *adapter.ProviderConfig, _ *dto.ChatRequest) (*dto.ChatResponse, error) {
	return s.response, nil
}

func (s stubAdaptor) Stream(_ context.Context, _ *adapter.ProviderConfig, _ *dto.ChatRequest) (dto.TokenStream, error) {
	return nil, nil
}

func (s stubAdaptor) Media(_ context.Context, _ *adapter.ProviderConfig, _ *dto.MediaRequest) (*dto.MediaResponse, error) {
	return nil, nil
}

func (s stubAdaptor) TaskStatus(_ context.Context, _ *adapter.ProviderConfig, _ string) (*dto.TaskStatusResponse, error) {
	return nil, nil
}

func (s stubAdaptor) StreamMedia(_ context.Context, _ *adapter.ProviderConfig, _ *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, nil
}
