package llm

import (
	"testing"

	"github.com/YspCoder/omnigo/adapter"
	"github.com/YspCoder/omnigo/config"
	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
)

func TestNewLLM_AllowsAKSKWithoutAPIKey(t *testing.T) {
	cfg := &config.Config{
		Provider:  "jimeng",
		Model:     "jimeng_ti2v_v30_pro",
		APIKeys:   map[string]string{},
		AccessKey: "test-ak",
		SecretKey: "test-sk",
	}

	logger := utils.NewLogger(utils.LogLevelOff)
	registry := adapter.NewRegistry("jimeng")

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
