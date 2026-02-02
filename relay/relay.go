// Package relay provides the unified request execution layer.
package relay

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/YspCoder/omnigo/adapter"
	"github.com/YspCoder/omnigo/dto"
)

// Relay executes provider requests using a unified flow.
type Relay struct {
	Client *http.Client
}

// NewRelay creates a relay with default settings.
func NewRelay() *Relay {
	return &Relay{}
}

// Chat executes a chat completion request.
func (r *Relay) Chat(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, request *dto.ChatRequest) (*dto.ChatResponse, error) {
	if config == nil {
		return nil, fmt.Errorf("provider config is required")
	}

	convertAdaptor := adp
	if strings.EqualFold(config.ChatProtocol, "openai") {
		convertAdaptor = &adapter.OpenAIAdaptor{}
	}

	body, err := convertAdaptor.ConvertChatRequest(ctx, config, request)
	if err != nil {
		return nil, err
	}
	respBody, err := r.doRequest(ctx, adp, config, adapter.ModeChat, body)
	if err != nil {
		return nil, err
	}
	return convertAdaptor.ConvertChatResponse(ctx, config, respBody)
}

// Media executes an image/video generation request.
func (r *Relay) Media(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	if config == nil {
		return nil, fmt.Errorf("provider config is required")
	}
	if request == nil {
		return nil, fmt.Errorf("media request is required")
	}

	mode := ""
	switch request.Type {
	case dto.MediaTypeImage:
		mode = adapter.ModeImage
	case dto.MediaTypeVideo:
		mode = adapter.ModeVideo
	default:
		return nil, fmt.Errorf("unsupported media type: %s", request.Type)
	}

	body, err := adp.ConvertMediaRequest(ctx, config, mode, request)
	if err != nil {
		return nil, err
	}
	respBody, err := r.doRequest(ctx, adp, config, mode, body)
	if err != nil {
		return nil, err
	}
	return adp.ConvertMediaResponse(ctx, config, mode, respBody)
}

// TaskStatus queries a task status (e.g., async video generation).
func (r *Relay) TaskStatus(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, taskID string) (*dto.TaskStatusResponse, error) {
	if config == nil {
		return nil, fmt.Errorf("provider config is required")
	}
	taskAdaptor, ok := adp.(adapter.TaskAdaptor)
	if !ok {
		return nil, fmt.Errorf("task status not supported by adaptor")
	}
	if taskID == "" {
		return nil, fmt.Errorf("task id is required")
	}

	url, err := taskAdaptor.GetTaskStatusURL(taskID, config)
	if err != nil {
		return nil, err
	}
	if url == "" {
		return nil, fmt.Errorf("request url is empty")
	}

	method := http.MethodGet
	var body []byte
	if requestAdaptor, ok := adp.(adapter.TaskRequestAdaptor); ok {
		m, b, err := requestAdaptor.PrepareTaskStatusRequest(ctx, config, taskID)
		if err != nil {
			return nil, err
		}
		method = m
		body = b
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	if err := adp.SetupHeaders(req, config, adapter.ModeTask); err != nil {
		return nil, err
	}
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	client := config.HTTPClient
	if client == nil {
		client = r.Client
	}
	if client == nil {
		client = &http.Client{}
	}
	if config.Timeout > 0 {
		client.Timeout = config.Timeout
	} else if client.Timeout == 0 {
		client.Timeout = 60 * time.Second
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("task status request failed with status %d", resp.StatusCode)
	}

	return taskAdaptor.ConvertTaskStatusResponse(ctx, config, respBody)
}

// Stream executes a streaming chat request and returns the response body.
func (r *Relay) Stream(ctx context.Context, adp adapter.Adaptor, streamAdaptor adapter.StreamAdaptor, config *adapter.ProviderConfig, request *dto.ChatRequest) (io.ReadCloser, error) {
	if config == nil {
		return nil, fmt.Errorf("provider config is required")
	}
	if streamAdaptor == nil {
		return nil, fmt.Errorf("stream adaptor is required")
	}

	body, err := streamAdaptor.PrepareStreamRequest(ctx, config, request)
	if err != nil {
		return nil, err
	}

	url, err := adp.GetRequestURL(adapter.ModeChat, config)
	if err != nil {
		return nil, err
	}
	if url == "" {
		return nil, fmt.Errorf("request url is empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	if err := adp.SetupHeaders(req, config, adapter.ModeChat); err != nil {
		return nil, err
	}
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	client := config.HTTPClient
	if client == nil {
		client = r.Client
	}
	if client == nil {
		client = &http.Client{}
	}
	if config.Timeout > 0 {
		client.Timeout = config.Timeout
	} else if client.Timeout == 0 {
		client.Timeout = 60 * time.Second
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &dto.LLMError{
			Code:     resp.StatusCode,
			Message:  string(respBody),
			Provider: config.Name,
		}
	}

	return resp.Body, nil
}

func (r *Relay) doRequest(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, mode string, body []byte) ([]byte, error) {
	url, err := adp.GetRequestURL(mode, config)
	if err != nil {
		return nil, err
	}
	if url == "" {
		return nil, fmt.Errorf("request url is empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	if err := adp.SetupHeaders(req, config, mode); err != nil {
		return nil, err
	}
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	client := config.HTTPClient
	if client == nil {
		client = r.Client
	}
	if client == nil {
		client = &http.Client{}
	}
	if config.Timeout > 0 {
		client.Timeout = config.Timeout
	} else if client.Timeout == 0 {
		client.Timeout = 60 * time.Second
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, &dto.LLMError{
			Code:     resp.StatusCode,
			Message:  string(respBody),
			Provider: config.Name,
		}
	}
	return respBody, nil
}
