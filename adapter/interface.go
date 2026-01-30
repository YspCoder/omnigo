// Package adapter defines provider-specific adaptors for unified DTOs.
package adapter

import (
	"context"
	"net/http"
	"time"

	"github.com/YspCoder/omnigo/dto"
)

const (
	ModeChat  = "chat"
	ModeImage = "image"
	ModeVideo = "video"
)

// ProviderConfig holds configuration for a specific provider.
type ProviderConfig struct {
	Name         string
	APIKey       string
	BaseURL      string
	Organization string
	AuthHeader   string
	AuthPrefix   string
	Headers      map[string]string
	HTTPClient   *http.Client
	Timeout      time.Duration
	ChatProtocol string
}

// Adaptor defines the interface for provider-specific conversions and routing.
type Adaptor interface {
	// GetRequestURL returns the provider endpoint for the given mode.
	GetRequestURL(mode string, config *ProviderConfig) (string, error)

	// SetupHeaders sets authentication and content headers for the request.
	SetupHeaders(req *http.Request, config *ProviderConfig, mode string) error

	// Chat conversions.
	ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error)
	ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error)

	// Image conversions.
	ConvertImageRequest(ctx context.Context, config *ProviderConfig, request *dto.ImageRequest) ([]byte, error)
	ConvertImageResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ImageResponse, error)

	// Video conversions.
	ConvertVideoRequest(ctx context.Context, config *ProviderConfig, request *dto.VideoRequest) ([]byte, error)
	ConvertVideoResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.VideoResponse, error)
}

// StreamAdaptor defines optional streaming capabilities for adaptors.
type StreamAdaptor interface {
	PrepareStreamRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error)
	ParseStreamResponse(chunk []byte) (string, error)
}
