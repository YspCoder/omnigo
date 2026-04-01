// Package adapter defines provider-specific adaptors for unified DTOs.
package adapter

import (
	"context"
	"net/http"
	"time"

	"github.com/YspCoder/omnigo/dto"
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

// Adaptor defines provider-specific implementations for unified multimodal requests.
type Adaptor interface {
	// Chat executes a text-generation request using the unified multimodal DTO.
	Chat(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error)

	// Stream executes a streaming text-generation request using the unified multimodal DTO.
	Stream(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error)

	// Media executes a unified multimodal request, typically for image or video generation.
	Media(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error)

	// TaskStatus queries a background task status (mostly for video).
	TaskStatus(ctx context.Context, config *ProviderConfig, taskID string, query ...map[string]string) (*dto.TaskStatusResponse, error)

	// ListTasks queries a provider task list.
	ListTasks(ctx context.Context, config *ProviderConfig, query map[string]string) (*dto.TaskListResponse, error)

	// StreamMedia executes a streaming multimodal request.
	StreamMedia(ctx context.Context, config *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error)
}
