// Package relay provides the unified request execution layer.
package relay

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
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
	body, err := adp.ConvertChatRequest(ctx, config, request)
	if err != nil {
		return nil, err
	}
	respBody, err := r.doRequest(ctx, adp, config, adapter.ModeChat, body)
	if err != nil {
		return nil, err
	}
	return adp.ConvertChatResponse(ctx, config, respBody)
}

// Image executes an image generation request.
func (r *Relay) Image(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, request *dto.ImageRequest) (*dto.ImageResponse, error) {
	if config == nil {
		return nil, fmt.Errorf("provider config is required")
	}
	body, err := adp.ConvertImageRequest(ctx, config, request)
	if err != nil {
		return nil, err
	}
	respBody, err := r.doRequest(ctx, adp, config, adapter.ModeImage, body)
	if err != nil {
		return nil, err
	}
	return adp.ConvertImageResponse(ctx, config, respBody)
}

// Video executes a video generation request.
func (r *Relay) Video(ctx context.Context, adp adapter.Adaptor, config *adapter.ProviderConfig, request *dto.VideoRequest) (*dto.VideoResponse, error) {
	if config == nil {
		return nil, fmt.Errorf("provider config is required")
	}
	body, err := adp.ConvertVideoRequest(ctx, config, request)
	if err != nil {
		return nil, err
	}
	respBody, err := r.doRequest(ctx, adp, config, adapter.ModeVideo, body)
	if err != nil {
		return nil, err
	}
	return adp.ConvertVideoResponse(ctx, config, respBody)
}

// Stream executes a streaming chat request and returns the response body.
func (r *Relay) Stream(ctx context.Context, adp interface {
	adapter.Adaptor
	adapter.StreamAdaptor
}, config *adapter.ProviderConfig, request *dto.ChatRequest) (io.ReadCloser, error) {
	if config == nil {
		return nil, fmt.Errorf("provider config is required")
	}
	body, err := adp.PrepareStreamRequest(ctx, config, request)
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
