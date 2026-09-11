package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type vectorHTTP struct {
	config VectorConfig
	client *retryHTTPClient
}

// VectorAPIError preserves the HTTP status and a bounded response body so
// callers can distinguish authentication, validation, and server failures.
type VectorAPIError struct {
	Provider   string
	StatusCode int
	Body       string
}

func (e *VectorAPIError) Error() string {
	if e.Provider == "" {
		return fmt.Sprintf("vector database request failed with status %d: %s", e.StatusCode, e.Body)
	}
	return fmt.Sprintf("%s request failed with status %d: %s", e.Provider, e.StatusCode, e.Body)
}

func newVectorHTTP(config *VectorConfig) (*vectorHTTP, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: config is required", ErrVectorConfig)
	}
	copyConfig := *config
	if strings.TrimSpace(copyConfig.Endpoint) == "" {
		copyConfig.Endpoint = copyConfig.BaseURL
	}
	if strings.TrimSpace(copyConfig.Endpoint) == "" {
		copyConfig.Endpoint = firstConfiguredOperationURL(copyConfig)
	}
	if strings.TrimSpace(copyConfig.Endpoint) == "" {
		return nil, fmt.Errorf("%w: endpoint is required", ErrVectorConfig)
	}
	if err := validateOperationURLs(copyConfig); err != nil {
		return nil, err
	}
	parsed, err := url.Parse(strings.TrimRight(copyConfig.Endpoint, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("%w: endpoint must be an absolute http(s) URL", ErrVectorConfig)
	}
	copyConfig.Endpoint = strings.TrimRight(parsed.String(), "/")
	providerConfig := &ProviderConfig{
		HTTPClient: copyConfig.HTTPClient,
		Proxy:      copyConfig.Proxy,
		Timeout:    copyConfig.Timeout,
		MaxRetries: copyConfig.MaxRetries,
		RetryDelay: copyConfig.RetryDelay,
	}
	client, err := providerHTTPClient(providerConfig)
	if err != nil {
		return nil, err
	}
	return &vectorHTTP{config: copyConfig, client: &retryHTTPClient{client: client, maxRetries: copyConfig.MaxRetries, retryDelay: copyConfig.RetryDelay}}, nil
}

func (h *vectorHTTP) endpoint(path string) string {
	base := strings.TrimRight(h.config.Endpoint, "/")
	if path == "" {
		return base
	}
	return base + "/" + strings.TrimLeft(path, "/")
}

func (h *vectorHTTP) do(ctx context.Context, provider, method, path string, payload interface{}, out interface{}, auth func(*http.Request)) error {
	return h.doURL(ctx, provider, method, h.endpoint(path), payload, out, auth)
}

func (h *vectorHTTP) doOperation(ctx context.Context, provider, operation, method, path string, payload interface{}, out interface{}, auth func(*http.Request)) error {
	endpoint := h.endpoint(path)
	if configured := operationURL(h.config, operation); configured != "" {
		endpoint = configured
	}
	return h.doURL(ctx, provider, method, endpoint, payload, out, auth)
}

func (h *vectorHTTP) doURL(ctx context.Context, provider, method, endpoint string, payload interface{}, out interface{}, auth func(*http.Request)) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode %s request: %w", provider, err)
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create %s request: %w", provider, err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range h.config.Headers {
		request.Header.Set(key, value)
	}
	if auth != nil {
		auth(request)
	}
	response, err := h.client.Do(request)
	if err != nil {
		return fmt.Errorf("call %s: %w", provider, err)
	}
	if response == nil {
		return fmt.Errorf("call %s: empty HTTP response", provider)
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if readErr != nil {
		return fmt.Errorf("read %s response: %w", provider, readErr)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &VectorAPIError{Provider: provider, StatusCode: response.StatusCode, Body: strings.TrimSpace(string(data))}
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s response: %w", provider, err)
	}
	return nil
}

func operationURL(config VectorConfig, operation string) string {
	keys := []string{operation}
	if operation == "upsert" {
		keys = append(keys, "push", "insert", "write")
	}
	for _, key := range keys {
		if value := strings.TrimSpace(config.OperationURLs[key]); value != "" {
			return value
		}
	}
	switch operation {
	case "upsert":
		if strings.TrimSpace(config.UpsertURL) != "" {
			return strings.TrimSpace(config.UpsertURL)
		}
		if strings.TrimSpace(config.PushURL) != "" {
			return strings.TrimSpace(config.PushURL)
		}
		return strings.TrimSpace(config.InsertURL)
	case "search":
		return strings.TrimSpace(config.SearchURL)
	case "delete":
		return strings.TrimSpace(config.DeleteURL)
	case "create_collection":
		return strings.TrimSpace(config.CreateCollectionURL)
	default:
		return ""
	}
}

func firstConfiguredOperationURL(config VectorConfig) string {
	for _, operation := range []string{"upsert", "search", "delete", "create_collection"} {
		if value := operationURL(config, operation); value != "" {
			return value
		}
	}
	return ""
}

func validateOperationURLs(config VectorConfig) error {
	values := []string{config.UpsertURL, config.PushURL, config.InsertURL, config.SearchURL, config.DeleteURL, config.CreateCollectionURL}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		parsed, err := url.Parse(strings.TrimSpace(value))
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("%w: operation URL must be an absolute http(s) URL", ErrVectorConfig)
		}
	}
	for _, value := range config.OperationURLs {
		if strings.TrimSpace(value) == "" {
			continue
		}
		parsed, err := url.Parse(strings.TrimSpace(value))
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("%w: operation URL must be an absolute http(s) URL", ErrVectorConfig)
		}
	}
	return nil
}

func bearerAuth(config VectorConfig) func(*http.Request) {
	return func(request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			return
		}
		token := config.Token
		if token == "" {
			token = config.APIKey
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}
}

func basicAuth(config VectorConfig) func(*http.Request) {
	return func(request *http.Request) {
		if request.Header.Get("Authorization") == "" && (config.Username != "" || config.Password != "") {
			request.SetBasicAuth(config.Username, config.Password)
		}
	}
}

func defaultTopK(value int) int {
	if value <= 0 {
		return 10
	}
	return value
}

func distanceName(distance VectorDistance, fallback string) string {
	if distance == "" {
		return fallback
	}
	return string(distance)
}
