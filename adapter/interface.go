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
	Region       string
	Organization string
	Proxy        string // Proxy URL
	AuthHeader   string
	AuthPrefix   string
	Headers      map[string]string
	HTTPClient   *http.Client
	Timeout      time.Duration
	ChatProtocol string
}

// Adaptor defines the interface for provider-specific implementations using their SDKs.
type Adaptor interface {
	// Chat executes a chat completion request.
	Chat(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (*dto.ChatResponse, error)

	// Stream executes a streaming chat completion request.
	Stream(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) (dto.TokenStream, error)

	// Media executes an image or video generation request.
	Media(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error)

	// TaskStatus queries a background task status (mostly for video).
	TaskStatus(ctx context.Context, config *ProviderConfig, taskID string) (*dto.TaskStatusResponse, error)
}
