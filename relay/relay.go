// Package relay provides the unified request execution layer.
package relay

import (
	"context"
	"fmt"
	"net/http"

	"github.com/YspCoder/omnigo/adapter"
	"github.com/YspCoder/omnigo/dto"
)

// Relay executes provider requests by delegating to the specific adaptor.
type Relay struct {
	Client *http.Client
}

// NewRelay creates a relay with default settings.
func NewRelay() *Relay {
	return &Relay{}
}

// Chat executes a text-generation request through the provider adaptor.
func (r *Relay) Chat(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	if adp == nil {
		return nil, fmt.Errorf("adaptor is required")
	}
	return adp.Chat(ctx, config, request)
}

// Media executes a multimodal request through the provider adaptor.
func (r *Relay) Media(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	if adp == nil {
		return nil, fmt.Errorf("adaptor is required")
	}
	return adp.Media(ctx, config, request)
}

// TaskStatus queries a task status.
func (r *Relay) TaskStatus(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, taskID string) (*dto.TaskStatusResponse, error) {
	if adp == nil {
		return nil, fmt.Errorf("adaptor is required")
	}
	return adp.TaskStatus(ctx, config, taskID)
}

// ListTasks queries a provider task list.
func (r *Relay) ListTasks(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, query map[string]string) (*dto.TaskListResponse, error) {
	if adp == nil {
		return nil, fmt.Errorf("adaptor is required")
	}
	return adp.ListTasks(ctx, config, query)
}

// Stream executes a streaming text-generation request through the provider adaptor.
func (r *Relay) Stream(ctx context.Context, adp adapter.Adaptor, _ interface{}, config *adapter.ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	if adp == nil {
		return nil, fmt.Errorf("adaptor is required")
	}
	return adp.Stream(ctx, config, request)
}

// StreamMedia executes a streaming multimodal request through the provider adaptor.
func (r *Relay) StreamMedia(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	if adp == nil {
		return nil, fmt.Errorf("adaptor is required")
	}
	return adp.StreamMedia(ctx, config, request)
}
