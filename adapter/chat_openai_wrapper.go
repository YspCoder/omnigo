package adapter

import (
	"context"
	"net/http"

	"github.com/YspCoder/omnigo/dto"
)

// openAIChatWrapper forces chat requests/responses to use OpenAI protocol,
// while delegating transport, headers, and non-chat modes to the base adaptor.
type openAIChatWrapper struct {
	base   Adaptor
	openai *OpenAIAdaptor
}

// WrapChatWithOpenAI returns an adaptor that uses OpenAI protocol for chat.
func WrapChatWithOpenAI(base Adaptor) Adaptor {
	if base == nil {
		return nil
	}
	if _, ok := base.(*OpenAIAdaptor); ok {
		return base
	}
	if _, ok := base.(*openAIChatWrapper); ok {
		return base
	}
	wrapper := &openAIChatWrapper{
		base:   base,
		openai: &OpenAIAdaptor{},
	}
	if _, ok := base.(StreamAdaptor); ok {
		return &openAIChatStreamWrapper{openAIChatWrapper: wrapper}
	}
	return wrapper
}

// IsOpenAIProtocol marks the wrapper as OpenAI-compatible for chat.
func (w *openAIChatWrapper) IsOpenAIProtocol() bool {
	return true
}

func (w *openAIChatWrapper) GetRequestURL(mode string, config *ProviderConfig) (string, error) {
	return w.base.GetRequestURL(mode, config)
}

func (w *openAIChatWrapper) SetupHeaders(req *http.Request, config *ProviderConfig, mode string) error {
	return w.base.SetupHeaders(req, config, mode)
}

func (w *openAIChatWrapper) ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	return w.openai.ConvertChatRequest(ctx, config, request)
}

func (w *openAIChatWrapper) ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error) {
	return w.openai.ConvertChatResponse(ctx, config, body)
}

func (w *openAIChatWrapper) ConvertImageRequest(ctx context.Context, config *ProviderConfig, request *dto.ImageRequest) ([]byte, error) {
	return w.base.ConvertImageRequest(ctx, config, request)
}

func (w *openAIChatWrapper) ConvertImageResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ImageResponse, error) {
	return w.base.ConvertImageResponse(ctx, config, body)
}

func (w *openAIChatWrapper) ConvertVideoRequest(ctx context.Context, config *ProviderConfig, request *dto.VideoRequest) ([]byte, error) {
	return w.base.ConvertVideoRequest(ctx, config, request)
}

func (w *openAIChatWrapper) ConvertVideoResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.VideoResponse, error) {
	return w.base.ConvertVideoResponse(ctx, config, body)
}

// openAIChatStreamWrapper adds OpenAI streaming support for chat.
type openAIChatStreamWrapper struct {
	*openAIChatWrapper
}

// PrepareStreamRequest uses OpenAI streaming protocol for chat.
func (w *openAIChatStreamWrapper) PrepareStreamRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	return w.openai.PrepareStreamRequest(ctx, config, request)
}

// ParseStreamResponse uses OpenAI streaming protocol for chat.
func (w *openAIChatStreamWrapper) ParseStreamResponse(chunk []byte) (string, error) {
	return w.openai.ParseStreamResponse(chunk)
}
