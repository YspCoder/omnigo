package adapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
	"google.golang.org/genai"
)

// Interactions uses the current steps protocol. The pinned genai SDK has no
// Interactions service, so this transport follows /api/interactions-api directly.
func googleUsesInteractions(cfg *ProviderConfig, r *dto.MediaRequest) bool {
	return cfg != nil && strings.EqualFold(cfg.ChatProtocol, "interactions") || r != nil && r.Options["api"] == "interactions"
}

type googleInteractionContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	URI      string `json:"uri,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
}

type googleInteractionStep struct {
	Type      string                     `json:"type"`
	ID        string                     `json:"id,omitempty"`
	Name      string                     `json:"name,omitempty"`
	Arguments json.RawMessage            `json:"arguments,omitempty"`
	Content   []googleInteractionContent `json:"content,omitempty"`
}

type googleInteraction struct {
	ID      string            `json:"id"`
	Model   string            `json:"model"`
	Status  string            `json:"status"`
	Created string            `json:"created"`
	Updated string            `json:"updated"`
	Steps   []json.RawMessage `json:"steps"`
	Usage   json.RawMessage   `json:"usage"`
	Error   struct {
		Code    interface{} `json:"code"`
		Message string      `json:"message"`
	} `json:"error"`
}

func googleInteractionEndpoint(cfg *ProviderConfig, version string) string {
	if cfg == nil {
		cfg = &ProviderConfig{}
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}
	if strings.HasSuffix(base, "/interactions") {
		return base
	}
	for _, known := range []string{"v1beta", "v1beta2", "v1"} {
		if strings.HasSuffix(base, "/"+known) {
			if version == "" {
				return base + "/interactions"
			}
			base = strings.TrimSuffix(base, "/"+known)
			break
		}
	}
	if version == "" {
		version = "v1beta"
	}
	return base + "/" + version + "/interactions"
}

func googleInteractionVersion(r *dto.MediaRequest) (string, error) {
	version, _ := r.Extra["api_version"].(string)
	if version != "" && version != "v1beta" && version != "v1beta2" && version != "v1" {
		return "", fmt.Errorf("unsupported google interactions API version %q", version)
	}
	return version, nil
}

func googleInteractionPayload(r *dto.MediaRequest, stream bool) (map[string]interface{}, error) {
	config := &genai.GenerateContentConfig{}
	if err := googleApplyTools(config, r); err != nil {
		return nil, err
	}
	payload := map[string]interface{}{"model": normalizeGoogleMediaModel(r.Model), "stream": stream}
	if system := utils.FirstSystemMessage(r.Messages); system != "" {
		payload["system_instruction"] = system
	}
	var steps []interface{}
	messages := utils.NonSystemMessages(r.Messages)
	if len(messages) == 0 && r.Prompt != "" {
		messages = []dto.Message{{Role: "user", Content: r.Prompt}}
	}
	lastUser := lastGoogleUserMessageIndex(messages)
	toolNames := make(map[string]string)
	for index, message := range messages {
		if message.Role == "tool" {
			if message.ToolCallID == "" {
				return nil, fmt.Errorf("google interaction tool result requires tool_call_id")
			}
			step := map[string]interface{}{"type": "function_result", "call_id": message.ToolCallID, "result": message.Content}
			if name := firstNonEmptyString(message.Name, toolNames[message.ToolCallID]); name != "" {
				step["name"] = name
			}
			steps = append(steps, step)
			continue
		}
		var content []googleInteractionContent
		if text, ok := messageContentText(message.Content); ok && text != "" {
			content = append(content, googleInteractionContent{Type: "text", Text: text})
		}
		images := googleMessageImageInputs(message)
		if index == lastUser {
			images = append(images, googleExtraImageInputs(r)...)
		}
		for _, image := range images {
			part := googleInteractionContent{Type: "image", URI: image}
			if strings.HasPrefix(image, "data:") {
				mime, data, err := decodeDataURL(image)
				if err != nil {
					return nil, err
				}
				part.URI, part.MIMEType, part.Data = "", mime, base64.StdEncoding.EncodeToString(data)
			}
			content = append(content, part)
		}
		if message.VideoURL != "" {
			content = append(content, googleInteractionContent{Type: "video", URI: message.VideoURL, MIMEType: "video/mp4"})
		}
		if message.FileURL != "" {
			content = append(content, googleInteractionContent{Type: "document", URI: message.FileURL})
		}
		if len(content) > 0 {
			kind := "user_input"
			if message.Role == "assistant" || message.Role == "model" {
				kind = "model_output"
			}
			steps = append(steps, googleInteractionStep{Type: kind, Content: content})
		}
		for _, call := range message.ToolCalls {
			if call.ID == "" || !json.Valid(call.Function.Arguments) {
				return nil, fmt.Errorf("google interaction tool history requires ID and JSON arguments")
			}
			steps = append(steps, googleInteractionStep{Type: "function_call", ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
			toolNames[call.ID] = call.Function.Name
		}
	}
	payload["input"] = steps
	if len(r.Tools) > 0 {
		payload["tools"] = toOpenAIResponsesTools(r.Tools)
	}
	generation := map[string]interface{}{}
	if temperature, ok := requestTemperature(r); ok {
		generation["temperature"] = temperature
	}
	if r.MaxTokens > 0 {
		generation["max_output_tokens"] = r.MaxTokens
	}
	if r.Seed != 0 {
		generation["seed"] = r.Seed
	}
	if config.ToolConfig != nil {
		choice := config.ToolConfig.FunctionCallingConfig
		mode := strings.ToLower(string(choice.Mode))
		if len(choice.AllowedFunctionNames) > 0 {
			generation["tool_choice"] = map[string]interface{}{"allowed_tools": map[string]interface{}{"mode": mode, "tools": choice.AllowedFunctionNames}}
		} else {
			generation["tool_choice"] = mode
		}
	}
	if len(generation) > 0 {
		payload["generation_config"] = generation
	}
	if r.Type == dto.MediaTypeImage {
		ratio, size := normalizeGoogleImageShape(r.Size, r.Resolution)
		format := map[string]interface{}{"type": "image"}
		if ratio != "" {
			format["aspect_ratio"] = ratio
		}
		if size != "" {
			format["image_size"] = size
		}
		payload["response_format"] = []interface{}{map[string]interface{}{"type": "text"}, format}
	} else if r.Schema != nil {
		payload["response_format"] = map[string]interface{}{"type": "text", "mime_type": "application/json", "schema": r.Schema}
	}
	for _, key := range []string{"input", "previous_interaction_id", "store", "background", "generation_config", "response_format", "tools", "system_instruction", "agent", "agent_config"} {
		if value, ok := r.Extra[key]; ok {
			payload[key] = value
		}
	}
	if payload["agent"] != nil {
		if r.Extra["generation_config"] != nil {
			return nil, fmt.Errorf("google interaction agent cannot use generation_config")
		}
		delete(payload, "model")
		delete(payload, "generation_config")
	}
	return payload, nil
}

func googleInteractionRequest(ctx context.Context, cfg *ProviderConfig, method, endpoint string, payload interface{}) (*http.Response, error) {
	if cfg == nil {
		cfg = &ProviderConfig{}
	}
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-goog-api-key", cfg.APIKey)
	for key, value := range cfg.Headers {
		request.Header.Set(key, value)
	}
	client, err := providerHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return nil, fmt.Errorf("google interactions: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	return response, nil
}

func (a *GoogleAdaptor) interaction(ctx context.Context, cfg *ProviderConfig, r *dto.MediaRequest) (*dto.MediaResponse, error) {
	payload, err := googleInteractionPayload(r, false)
	if err != nil {
		return nil, err
	}
	version, err := googleInteractionVersion(r)
	if err != nil {
		return nil, err
	}
	response, err := googleInteractionRequest(ctx, cfg, http.MethodPost, googleInteractionEndpoint(cfg, version), payload)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var interaction googleInteraction
	if err := json.NewDecoder(response.Body).Decode(&interaction); err != nil {
		return nil, err
	}
	result := googleInteractionResponse(interaction)
	if interaction.Status == "in_progress" {
		result.TaskID = "interactions/" + interaction.ID
		if version != "" {
			result.TaskID = "interactions/" + version + "/" + interaction.ID
		}
	}
	return result, googleInteractionError(interaction)
}

func googleInteractionUsage(raw json.RawMessage) dto.Usage {
	usage := usageFromJSON(raw)
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return usage
	}
	usage.PromptTokens, _ = usageInt(fields, "total_input_tokens")
	output, _ := usageInt(fields, "total_output_tokens")
	thoughts, _ := usageInt(fields, "total_thought_tokens")
	usage.CompletionTokens = output + thoughts
	if _, reported := usageInt(fields, "total_tokens"); !reported {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	for _, field := range []struct{ source, target string }{{"total_cached_tokens", "cached_tokens"}, {"input_tokens_by_modality", "modalities"}, {"cached_tokens_by_modality", "cache_modalities"}, {"total_tool_use_tokens", "tool_tokens"}, {"tool_use_tokens_by_modality", "tool_modalities"}} {
		copyUsageField(&usage.PromptTokensDetails, fields, field.source, field.target)
	}
	copyUsageField(&usage.CompletionTokensDetails, fields, "total_thought_tokens", "reasoning_tokens")
	copyUsageField(&usage.CompletionTokensDetails, fields, "output_tokens_by_modality", "modalities")
	return usage
}

func googleInteractionResponse(interaction googleInteraction) *dto.MediaResponse {
	result := &dto.MediaResponse{ID: interaction.ID, Model: interaction.Model, Status: interaction.Status, Usage: googleInteractionUsage(interaction.Usage), ErrorMessage: interaction.Error.Message}
	if len(interaction.Steps) > 0 {
		result.Metadata = map[string]interface{}{"steps": interaction.Steps}
	}
	if interaction.Error.Code != nil {
		result.ErrorCode = fmt.Sprint(interaction.Error.Code)
	}
	if created, err := time.Parse(time.RFC3339, interaction.Created); err == nil {
		result.Created = created.Unix()
	}
	message := dto.Message{Role: "assistant"}
	var text strings.Builder
	for _, raw := range interaction.Steps {
		var step googleInteractionStep
		if json.Unmarshal(raw, &step) != nil {
			continue
		}
		switch step.Type {
		case "function_call":
			message.ToolCalls = append(message.ToolCalls, dto.ToolCall{ID: step.ID, Type: "function", Function: dto.ToolCallFunction{Name: step.Name, Arguments: step.Arguments}})
		case "model_output":
			for _, content := range step.Content {
				switch content.Type {
				case "text":
					text.WriteString(content.Text)
				case "image":
					result.Data = append(result.Data, dto.ImageData{URL: content.URI, B64JSON: content.Data, MIMEType: content.MIMEType})
				case "video":
					result.Video.URL, result.Video.B64JSON, result.Video.MIMEType = content.URI, content.Data, content.MIMEType
				}
			}
		}
	}
	result.Text, message.Content = text.String(), text.String()
	finish := interaction.Status
	if len(message.ToolCalls) > 0 {
		finish = "tool_calls"
	}
	result.Choices = []dto.ChatChoice{{Message: message, FinishReason: finish}}
	if len(result.Data) > 0 {
		result.URL = result.Data[0].URL
	} else {
		result.URL = result.Video.URL
	}
	return result
}

func googleInteractionError(interaction googleInteraction) error {
	if interaction.Status == "failed" || interaction.Status == "cancelled" || interaction.Error.Message != "" {
		return fmt.Errorf("google interaction %s: %s", interaction.Status, interaction.Error.Message)
	}
	return nil
}

func (a *GoogleAdaptor) interactionStatus(ctx context.Context, cfg *ProviderConfig, id string) (*dto.TaskStatusResponse, error) {
	taskID := id
	id = strings.TrimPrefix(id, "interactions/")
	version := ""
	if prefix, rest, found := strings.Cut(id, "/"); found && (prefix == "v1beta" || prefix == "v1beta2" || prefix == "v1") {
		version, id = prefix, rest
	}
	response, err := googleInteractionRequest(ctx, cfg, http.MethodGet, googleInteractionEndpoint(cfg, version)+"/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var interaction googleInteraction
	if err := json.NewDecoder(response.Body).Decode(&interaction); err != nil {
		return nil, err
	}
	result := googleInteractionResponse(interaction)
	state := strings.ToUpper(interaction.Status)
	if interaction.Status == "completed" {
		state = "SUCCEEDED"
	}
	if interaction.Status == "in_progress" {
		state = "RUNNING"
	}
	result.TaskID = taskID
	return &dto.TaskStatusResponse{Usage: taskUsage(result.Usage), Output: dto.TaskStatusOutput{TaskID: taskID, TaskStatus: state, SubmitTime: interaction.Created, EndTime: interaction.Updated, URL: result.URL, VideoURL: result.Video.URL, VideoB64JSON: result.Video.B64JSON, VideoMIMEType: result.Video.MIMEType, Code: result.ErrorCode, Message: result.ErrorMessage, Result: result}}, nil
}
