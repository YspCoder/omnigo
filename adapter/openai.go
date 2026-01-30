// Package adapter provides OpenAI adaptor implementation.
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

	"github.com/YspCoder/omnigo/dto"
)

type openAIImagePayload struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt,omitempty"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type openAIVideoPayload struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt,omitempty"`
	Size           string `json:"size,omitempty"`
	Duration       int    `json:"duration,omitempty"`
	Fps            int    `json:"fps,omitempty"`
	Seed           int    `json:"seed,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

func openAIPayloadToMap(payload interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func openAIExtractPayloadMap(extra map[string]interface{}) map[string]interface{} {
	if extra == nil {
		return nil
	}
	raw, ok := extra["payload"]
	if !ok {
		return nil
	}
	payload, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	return payload
}

func openAIMarshalPayloadWithFallback(payload map[string]interface{}, fallback interface{}) ([]byte, error) {
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			return b, nil
		}
	}
	return json.Marshal(fallback)
}

// OpenAIAdaptor converts requests and responses to the OpenAI API format.
type OpenAIAdaptor struct {
	BaseURL string
}

// GetRequestURL returns the OpenAI endpoint for the given mode.
func (a *OpenAIAdaptor) GetRequestURL(mode string, config *ProviderConfig) (string, error) {
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		base = "https://api.openai.com/v1"
	}

	return buildOpenAIRequestURL(base, mode)
}

// SetupHeaders sets OpenAI-specific headers.
func (a *OpenAIAdaptor) SetupHeaders(req *http.Request, config *ProviderConfig, mode string) error {
	if config.AuthHeader != "" {
		req.Header.Set(config.AuthHeader, config.AuthPrefix+config.APIKey)
	} else if config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	if config.Organization != "" {
		req.Header.Set("OpenAI-Organization", config.Organization)
	}
	req.Header.Set("Content-Type", "application/json")
	return nil
}

// ConvertChatRequest marshals the OpenAI chat request.
func (a *OpenAIAdaptor) ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	payload := map[string]interface{}{
		"model":    request.Model,
		"messages": normalizeMessages(request),
	}
	if request.Stream {
		payload["stream"] = true
	}
	if request.Temperature != 0 {
		payload["temperature"] = request.Temperature
	}
	if request.MaxTokens != 0 {
		payload["max_tokens"] = request.MaxTokens
	}

	for key, value := range request.Options {
		if shouldSkipOption(key) {
			continue
		}
		payload[key] = value
	}

	if request.Schema != nil {
		schema, err := normalizeSchema(request.Schema)
		if err != nil {
			return nil, err
		}
		if _, ok := payload["response_format"]; !ok {
			payload["response_format"] = map[string]interface{}{
				"type": "json_schema",
				"json_schema": map[string]interface{}{
					"name":   "structured_response",
					"schema": cleanSchemaForOpenAI(schema),
					"strict": true,
				},
			}
		}
	}

	if _, hasMaxCompletion := payload["max_completion_tokens"]; hasMaxCompletion {
		delete(payload, "max_tokens")
	}

	return json.Marshal(payload)
}

// ConvertChatResponse unmarshals the OpenAI chat response.
func (a *OpenAIAdaptor) ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error) {
	var response dto.ChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func normalizeMessages(request *dto.ChatRequest) []dto.Message {
	messages := request.Messages
	if len(messages) == 0 && request.Prompt != "" {
		messages = []dto.Message{{Role: "user", Content: request.Prompt}}
	}

	systemPrompt, _ := request.Options["system_prompt"].(string)
	if systemPrompt != "" {
		withSystem := make([]dto.Message, 0, len(messages)+1)
		withSystem = append(withSystem, dto.Message{Role: "system", Content: systemPrompt})
		withSystem = append(withSystem, messages...)
		messages = withSystem
	}

	return messages
}

func shouldSkipOption(key string) bool {
	switch key {
	case "system_prompt", "structured_messages":
		return true
	default:
		return false
	}
}

func normalizeSchema(schema interface{}) (interface{}, error) {
	switch value := schema.(type) {
	case string:
		var result interface{}
		if err := json.Unmarshal([]byte(value), &result); err != nil {
			return nil, err
		}
		return result, nil
	case []byte:
		var result interface{}
		if err := json.Unmarshal(value, &result); err != nil {
			return nil, err
		}
		return result, nil
	case map[string]interface{}:
		return value, nil
	default:
		schemaBytes, err := json.Marshal(schema)
		if err != nil {
			return nil, err
		}
		var result interface{}
		if err := json.Unmarshal(schemaBytes, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
}

func cleanSchemaForOpenAI(schema interface{}) interface{} {
	if schemaMap, ok := schema.(map[string]interface{}); ok {
		result := make(map[string]interface{})
		for key, value := range schemaMap {
			switch key {
			case "type", "properties", "required", "items":
				if key == "properties" {
					props := make(map[string]interface{})
					if propsMap, ok := value.(map[string]interface{}); ok {
						for propName, propSchema := range propsMap {
							props[propName] = cleanSchemaForOpenAI(propSchema)
						}
					}
					result[key] = props
				} else if key == "items" {
					result[key] = cleanSchemaForOpenAI(value)
				} else {
					result[key] = value
				}
			}
		}
		if schemaMap["type"] == "object" {
			result["additionalProperties"] = false
		}
		return result
	}
	return schema
}

// ConvertMediaRequest marshals the OpenAI media request.
func (a *OpenAIAdaptor) ConvertMediaRequest(ctx context.Context, config *ProviderConfig, mode string, request *dto.MediaRequest) ([]byte, error) {
	switch mode {
	case ModeImage:
		fallback := openAIImagePayload{
			Model:          request.Model,
			Prompt:         request.Prompt,
			N:              request.N,
			Size:           request.Size,
			ResponseFormat: request.ResponseFormat,
		}
		mapped, err := openAIPayloadToMap(fallback)
		if err != nil {
			return nil, err
		}
		for k, v := range request.Extra {
			mapped[k] = v
		}
		payloadMap := openAIExtractPayloadMap(request.Extra)
		if payloadMap == nil {
			payloadMap = mapped
		}
		return openAIMarshalPayloadWithFallback(payloadMap, fallback)
	case ModeVideo:
		fallback := openAIVideoPayload{
			Model:          request.Model,
			Prompt:         request.Prompt,
			Size:           request.Size,
			Duration:       request.Duration,
			Fps:            request.Fps,
			Seed:           request.Seed,
			ResponseFormat: request.ResponseFormat,
		}
		mapped, err := openAIPayloadToMap(fallback)
		if err != nil {
			return nil, err
		}
		for k, v := range request.Extra {
			mapped[k] = v
		}
		payloadMap := openAIExtractPayloadMap(request.Extra)
		if payloadMap == nil {
			payloadMap = mapped
		}
		return openAIMarshalPayloadWithFallback(payloadMap, fallback)
	default:
		return nil, fmt.Errorf("unsupported media mode: %s", mode)
	}
}

// ConvertMediaResponse unmarshals the OpenAI media response.
func (a *OpenAIAdaptor) ConvertMediaResponse(ctx context.Context, config *ProviderConfig, mode string, body []byte) (*dto.MediaResponse, error) {
	switch mode {
	case ModeImage:
		var response dto.MediaResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, err
		}
		if response.URL == "" && len(response.Data) > 0 {
			if response.Data[0].URL != "" {
				response.URL = response.Data[0].URL
			} else if response.Data[0].B64JSON != "" {
				response.URL = response.Data[0].B64JSON
			}
		}
		return &response, nil
	case ModeVideo:
		var response dto.MediaResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, err
		}
		if response.URL == "" && response.Video.URL != "" {
			response.URL = response.Video.URL
		}
		return &response, nil
	default:
		return nil, fmt.Errorf("unsupported media mode: %s", mode)
	}
}

// PrepareStreamRequest creates a streaming chat request body.
func (a *OpenAIAdaptor) PrepareStreamRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	streamRequest := *request
	if streamRequest.Options == nil {
		streamRequest.Options = make(map[string]interface{})
	}
	streamRequest.Stream = true
	streamRequest.Options["stream"] = true
	return a.ConvertChatRequest(ctx, config, &streamRequest)
}

// ParseStreamResponse processes a single streaming chunk.
func (a *OpenAIAdaptor) ParseStreamResponse(chunk []byte) (string, error) {
	if len(bytes.TrimSpace(chunk)) == 0 {
		return "", fmt.Errorf("empty chunk")
	}
	if bytes.Equal(bytes.TrimSpace(chunk), []byte("[DONE]")) {
		return "", io.EOF
	}

	var response struct {
		Choices []struct {
			Delta struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(chunk, &response); err != nil {
		return "", fmt.Errorf("malformed response: %w", err)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	if response.Choices[0].FinishReason != "" {
		return "", io.EOF
	}
	if response.Choices[0].Delta.Role != "" && response.Choices[0].Delta.Content == "" {
		return "", fmt.Errorf("skip token")
	}
	return response.Choices[0].Delta.Content, nil
}

func buildOpenAIRequestURL(base, mode string) (string, error) {
	suffix, err := openAISuffix(mode)
	if err != nil {
		return "", err
	}

	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return buildOpenAIRequestURLFallback(base, suffix), nil
	}

	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(path, suffix) {
		return parsed.String(), nil
	}

	path = trimOpenAISuffix(path)
	path = strings.TrimRight(path, "/") + suffix
	parsed.Path = path
	return parsed.String(), nil
}

func buildOpenAIRequestURLFallback(base, suffix string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, suffix) {
		return base
	}
	base = trimOpenAISuffix(base)
	return strings.TrimRight(base, "/") + suffix
}

func openAISuffix(mode string) (string, error) {
	switch mode {
	case ModeChat:
		return "/chat/completions", nil
	case ModeImage:
		return "/images/generations", nil
	case ModeVideo:
		return "/videos/generations", nil
	default:
		return "", fmt.Errorf("unsupported mode: %s", mode)
	}
}

func trimOpenAISuffix(path string) string {
	suffixes := []string{"/chat/completions", "/images/generations", "/videos/generations"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(path, suffix) {
			return strings.TrimSuffix(path, suffix)
		}
	}
	return path
}
