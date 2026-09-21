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

// contextErr avoids dispatching work that the caller has already cancelled.
// A nil context is kept compatible with existing callers and is left for the
// adaptor to handle as before.
func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
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
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	return adp.Chat(ctx, config, request)
}

// Media executes a multimodal request through the provider adaptor.
func (r *Relay) Media(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	if adp == nil {
		return nil, fmt.Errorf("adaptor is required")
	}
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	return adp.Media(ctx, config, request)
}

// TaskStatus queries a task status.
func (r *Relay) TaskStatus(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, taskID string, query ...map[string]string) (*dto.TaskStatusResponse, error) {
	if adp == nil {
		return nil, fmt.Errorf("adaptor is required")
	}
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	return adp.TaskStatus(ctx, config, taskID, query...)
}

// ListTasks queries a provider task list.
func (r *Relay) ListTasks(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, query map[string]string) (*dto.TaskListResponse, error) {
	if adp == nil {
		return nil, fmt.Errorf("adaptor is required")
	}
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	return adp.ListTasks(ctx, config, query)
}

// Stream executes a streaming text-generation request through the provider adaptor.
func (r *Relay) Stream(ctx context.Context, adp adapter.Adaptor, _ interface{}, config *adapter.ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	if adp == nil {
		return nil, fmt.Errorf("adaptor is required")
	}
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	return adp.Stream(ctx, config, request)
}

// StreamMedia executes a streaming multimodal request through the provider adaptor.
func (r *Relay) StreamMedia(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	if adp == nil {
		return nil, fmt.Errorf("adaptor is required")
	}
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	return adp.StreamMedia(ctx, config, request)
}
