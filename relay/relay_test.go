package relay

import (
	"context"
	"errors"
	"testing"

	"github.com/YspCoder/omnigo/adapter"
	"github.com/YspCoder/omnigo/dto"
)

type relayCountingAdaptor struct{ calls int }

func (a *relayCountingAdaptor) Chat(context.Context, *adapter.ProviderConfig, *dto.MediaRequest) (*dto.MediaResponse, error) {
	a.calls++
	return &dto.MediaResponse{}, nil
}
func (a *relayCountingAdaptor) Stream(context.Context, *adapter.ProviderConfig, *dto.MediaRequest) (dto.TokenStream, error) {
	a.calls++
	return nil, nil
}
func (a *relayCountingAdaptor) Media(context.Context, *adapter.ProviderConfig, *dto.MediaRequest) (*dto.MediaResponse, error) {
	a.calls++
	return &dto.MediaResponse{}, nil
}
func (a *relayCountingAdaptor) TaskStatus(context.Context, *adapter.ProviderConfig, string, ...map[string]string) (*dto.TaskStatusResponse, error) {
	a.calls++
	return &dto.TaskStatusResponse{}, nil
}
func (a *relayCountingAdaptor) ListTasks(context.Context, *adapter.ProviderConfig, map[string]string) (*dto.TaskListResponse, error) {
	a.calls++
	return &dto.TaskListResponse{}, nil
}
func (a *relayCountingAdaptor) StreamMedia(context.Context, *adapter.ProviderConfig, *dto.MediaRequest) (dto.TokenStream, error) {
	a.calls++
	return nil, nil
}

func TestRelayDoesNotDispatchCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	adaptor := &relayCountingAdaptor{}
	relay := NewRelay()

	if _, err := relay.Chat(ctx, adaptor, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Chat error = %v, want context.Canceled", err)
	}
	if _, err := relay.Stream(ctx, adaptor, nil, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream error = %v, want context.Canceled", err)
	}
	if adaptor.calls != 0 {
		t.Fatalf("adaptor calls = %d, want 0 for canceled context", adaptor.calls)
	}
}
