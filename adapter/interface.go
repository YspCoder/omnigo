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
	ModeTask  = "task"
)

// ProviderConfig holds configuration for a specific provider.
type ProviderConfig struct {
	Name         string
	APIKey       string
	AccessKey    string
	SecretKey    string
	Model        string
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
	// GetURL returns the provider endpoint for the given mode and optional taskID.
	GetURL(mode string, config *ProviderConfig, taskID string) (string, error)

	// SetupHeaders sets authentication and content headers for the request.
	SetupHeaders(req *http.Request, config *ProviderConfig, mode string, body []byte) error

	// Chat conversions.
	ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error)
	ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error)

	// Media conversions.
	ConvertMediaRequest(ctx context.Context, config *ProviderConfig, mode string, request *dto.MediaRequest) ([]byte, error)
	ConvertMediaResponse(ctx context.Context, config *ProviderConfig, mode string, body []byte) (*dto.MediaResponse, error)
}

// StreamAdaptor defines optional streaming capabilities for adaptors.
type StreamAdaptor interface {
	PrepareStreamRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error)
	ParseStreamResponse(chunk []byte) (string, error)
}

// StreamHeadersProvider allows adaptors to inject extra headers for streaming requests.
type StreamHeadersProvider interface {
	StreamHeaders(config *ProviderConfig) map[string]string
}

// TaskAdaptor defines optional task status capabilities for adaptors.
type TaskAdaptor interface {
	ConvertTaskStatusResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.TaskStatusResponse, error)
}

// TaskRequestAdaptor allows adaptors to customize the task status request.
type TaskRequestAdaptor interface {
	PrepareTaskStatusRequest(ctx context.Context, config *ProviderConfig, taskID string) (method string, body []byte, err error)
}
