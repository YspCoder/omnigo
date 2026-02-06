// Package adapter provides Google Gemini adaptor implementation.
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/YspCoder/omnigo/dto"
)

// Google Gemini REST API structures
type googleGeminiPart struct {
	Text string `json:"text,omitempty"`
}

type googleGeminiContent struct {
	Role  string             `json:"role,omitempty"`
	Parts []googleGeminiPart `json:"parts"`
}

type googleGeminiGenerationConfig struct {
	Temperature     float64  `json:"temperature,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	TopP            float64  `json:"topP,omitempty"`
	TopK            int      `json:"topK,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type googleGeminiChatRequest struct {
	Contents         []googleGeminiContent        `json:"contents"`
	SystemInstruction *googleGeminiContent        `json:"system_instruction,omitempty"`
	GenerationConfig *googleGeminiGenerationConfig `json:"generationConfig,omitempty"`
}

type googleGeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []googleGeminiPart `json:"parts"`
			Role  string             `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

// GoogleAdaptor converts requests and responses for Google Gemini API.
type GoogleAdaptor struct {
	BaseURL string
}

// GetRequestURL returns the Google Gemini endpoint for the given mode.
func (a *GoogleAdaptor) GetRequestURL(mode string, config *ProviderConfig) (string, error) {
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		base = "https://generativelanguage.googleapis.com/v1beta"
	}

	action := "generateContent"
	if mode == ModeChat && config.Headers["X-Stream"] == "true" {
		action = "streamGenerateContent"
	}
	if mode == ModeImage || mode == ModeVideo {
		// Image and Video generation often use the predict endpoint for Imagen/Veo
		action = "predict"
	}

	// Format: models/{model}:{action}?key={api_key}
	url := fmt.Sprintf("%s/models/%s:%s", base, config.Model, action)
	if config.APIKey != "" {
		url += "?key=" + config.APIKey
	}
	return url, nil
}

// SetupHeaders sets Google-specific headers.
func (a *GoogleAdaptor) SetupHeaders(req *http.Request, config *ProviderConfig, mode string) error {
	// API Key is usually passed in the URL for Gemini, but we can set content type.
	req.Header.Set("Content-Type", "application/json")
	return nil
}

// ConvertChatRequest marshals the Google Gemini chat request.
func (a *GoogleAdaptor) ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	contents := make([]googleGeminiContent, 0, len(request.Messages))
	for _, m := range request.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		if role == "system" {
			// System prompt is handled separately in Gemini v1beta
			continue
		}
		contents = append(contents, googleGeminiContent{
			Role: role,
			Parts: []googleGeminiPart{
				{Text: fmt.Sprint(m.Content)},
			},
		})
	}

	payload := googleGeminiChatRequest{
		Contents: contents,
		GenerationConfig: &googleGeminiGenerationConfig{
			Temperature:     request.Temperature,
			MaxOutputTokens: request.MaxTokens,
		},
	}

	// Handle System Prompt
	if sysPrompt, ok := request.Options["system_prompt"].(string); ok && sysPrompt != "" {
		payload.SystemInstruction = &googleGeminiContent{
			Parts: []googleGeminiPart{{Text: sysPrompt}},
		}
	}

	// Map other options
	if topP, ok := request.Options["top_p"].(float64); ok {
		payload.GenerationConfig.TopP = topP
	}
	if topK, ok := request.Options["top_k"].(int); ok {
		payload.GenerationConfig.TopK = topK
	}

	return json.Marshal(payload)
}

// ConvertChatResponse unmarshals the Google Gemini chat response.
func (a *GoogleAdaptor) ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error) {
	var gResp googleGeminiResponse
	if err := json.Unmarshal(body, &gResp); err != nil {
		return nil, err
	}

	if len(gResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in google response")
	}

	candidate := gResp.Candidates[0]
	var content string
	if len(candidate.Content.Parts) > 0 {
		content = candidate.Content.Parts[0].Text
	}

	resp := &dto.ChatResponse{
		Choices: []dto.ChatChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: candidate.FinishReason,
			},
		},
		Usage: dto.Usage{
			PromptTokens:     gResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: gResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gResp.UsageMetadata.TotalTokenCount,
		},
	}

	return resp, nil
}

// ConvertMediaRequest marshals the Google Media request (Imagen/Video).
func (a *GoogleAdaptor) ConvertMediaRequest(ctx context.Context, config *ProviderConfig, mode string, request *dto.MediaRequest) ([]byte, error) {
	// For Imagen 3 / Veo via predict
	payload := map[string]interface{}{
		"instances": []map[string]interface{}{
			{
				"prompt": request.Prompt,
			},
		},
		"parameters": map[string]interface{}{
			"sampleCount": request.N,
		},
	}

	if request.Size != "" {
		payload["parameters"].(map[string]interface{})["aspectRatio"] = request.Size
	}

	// For Video
	if mode == ModeVideo {
		if request.Duration > 0 {
			payload["parameters"].(map[string]interface{})["durationSeconds"] = request.Duration
		}
	}

	return json.Marshal(payload)
}

// ConvertMediaResponse unmarshals the Google Media response.
func (a *GoogleAdaptor) ConvertMediaResponse(ctx context.Context, config *ProviderConfig, mode string, body []byte) (*dto.MediaResponse, error) {
	// Initial response for async tasks might be an Operation object
	var op struct {
		Name string `json:"name"`
		Done bool   `json:"done"`
	}
	if err := json.Unmarshal(body, &op); err == nil && op.Name != "" {
		return &dto.MediaResponse{
			TaskID: op.Name,
			Status: "processing",
		}, nil
	}

	// Direct response
	var gResp struct {
		Predictions []struct {
			BytesBase64Encoded string `json:"bytesBase64Encoded"`
			MimeType           string `json:"mimeType"`
			URL                string `json:"url"`
		} `json:"predictions"`
	}

	if err := json.Unmarshal(body, &gResp); err != nil {
		return nil, err
	}

	if len(gResp.Predictions) > 0 {
		prediction := gResp.Predictions[0]
		url := prediction.URL
		if url == "" && prediction.BytesBase64Encoded != "" {
			url = "data:" + prediction.MimeType + ";base64," + prediction.BytesBase64Encoded
		}
		return &dto.MediaResponse{
			URL:    url,
			Status: "completed",
		}, nil
	}

	return nil, fmt.Errorf("empty google media response")
}

// GetTaskStatusURL returns the Google endpoint for operations.
func (a *GoogleAdaptor) GetTaskStatusURL(taskID string, config *ProviderConfig) (string, error) {
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		base = "https://generativelanguage.googleapis.com/v1beta"
	}
	// taskID is usually the full operation name 'operations/xxx'
	url := fmt.Sprintf("%s/%s", base, taskID)
	if config.APIKey != "" {
		url += "?key=" + config.APIKey
	}
	return url, nil
}

// ConvertTaskStatusResponse converts a Google operation response to TaskStatusResponse.
func (a *GoogleAdaptor) ConvertTaskStatusResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.TaskStatusResponse, error) {
	var op struct {
		Name     string `json:"name"`
		Done     bool   `json:"done"`
		Response struct {
			Predictions []struct {
				URL                string `json:"url"`
				BytesBase64Encoded string `json:"bytesBase64Encoded"`
				MimeType           string `json:"mimeType"`
			} `json:"predictions"`
		} `json:"response"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &op); err != nil {
		return nil, err
	}

	status := "processing"
	if op.Done {
		status = "completed"
	}
	if op.Error.Message != "" {
		status = "failed"
	}

	result := &dto.TaskStatusResponse{
		Output: dto.TaskStatusOutput{
			TaskStatus: status,
		},
	}

	if len(op.Response.Predictions) > 0 {
		pred := op.Response.Predictions[0]
		url := pred.URL
		if url == "" && pred.BytesBase64Encoded != "" {
			url = "data:" + pred.MimeType + ";base64," + pred.BytesBase64Encoded
		}
		result.Output.VideoURL = url // maps both video and image results to this field for simplicity in DTO
	}

	return result, nil
}

// PrepareStreamRequest creates a streaming chat request body.
func (a *GoogleAdaptor) PrepareStreamRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	if config.Headers == nil {
		config.Headers = make(map[string]string)
	}
	config.Headers["X-Stream"] = "true"
	return a.ConvertChatRequest(ctx, config, request)
}

// ParseStreamResponse processes a single streaming chunk for Google.
// Note: Google's stream is a JSON array of objects, or individual objects depending on the endpoint.
func (a *GoogleAdaptor) ParseStreamResponse(chunk []byte) (string, error) {
	// Google v1beta streamGenerateContent returns a JSON array of candidates.
	// However, usually it's wrapped in a response object.
	var gResp googleGeminiResponse
	if err := json.Unmarshal(chunk, &gResp); err != nil {
		// Might be a partial chunk or SSE format
		return "", fmt.Errorf("malformed chunk: %w", err)
	}

	if len(gResp.Candidates) > 0 && len(gResp.Candidates[0].Content.Parts) > 0 {
		return gResp.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", nil
}
