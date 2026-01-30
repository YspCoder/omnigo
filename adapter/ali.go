// Package adapter provides Alibaba DashScope adaptor implementation.
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/YspCoder/omnigo/dto"
)

// DashScope multimodal generation request/response.
// Endpoint: /v1/services/aigc/multimodal-generation/generation
type AliMultimodalGenerationRequest struct {
	Model string `json:"model,omitempty"`
	Input struct {
		Messages []struct {
			Role    string `json:"role,omitempty"`
			Content []struct {
				Text string `json:"text,omitempty"`
			} `json:"content,omitempty"`
		} `json:"messages,omitempty"`
	} `json:"input,omitempty"`
	Parameters struct {
		NegativePrompt string `json:"negative_prompt,omitempty"`
		Size           string `json:"size,omitempty"`
		N              int    `json:"n,omitempty"`
		PromptExtend   bool   `json:"prompt_extend,omitempty"`
		Watermark      bool   `json:"watermark,omitempty"`
		Seed           int    `json:"seed,omitempty"`
	} `json:"parameters,omitempty"`
}

type AliMultimodalGenerationResponse struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Output  struct {
		Choices []struct {
			FinishReason string `json:"finish_reason,omitempty"`
			Message      struct {
				Role    string `json:"role,omitempty"`
				Content []struct {
					Image string `json:"image,omitempty"`
				} `json:"content,omitempty"`
			} `json:"message,omitempty"`
		} `json:"choices,omitempty"`
		TaskMetric struct {
			Total     int `json:"TOTAL,omitempty"`
			Failed    int `json:"FAILED,omitempty"`
			Succeeded int `json:"SUCCEEDED,omitempty"`
		} `json:"task_metric,omitempty"`
		TaskStatus string `json:"task_status,omitempty"`
		TaskID     string `json:"task_id,omitempty"`
	} `json:"output,omitempty"`
	Usage struct {
		Width      int `json:"width,omitempty"`
		ImageCount int `json:"image_count,omitempty"`
		Height     int `json:"height,omitempty"`
	} `json:"usage,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// DashScope video-generation request/response.
// Endpoint: /api/v1/services/aigc/video-generation/video-synthesis
type AliVideoGenerationRequest struct {
	Model string `json:"model"`
	Input struct {
		Prompt         string   `json:"prompt"`
		NegativePrompt string   `json:"negative_prompt,omitempty"`
		ImgURL         string   `json:"img_url,omitempty"`
		AudioURL       string   `json:"audio_url,omitempty"`
		Template       string   `json:"template,omitempty"`
		ReferenceURLs  []string `json:"reference_urls,omitempty"`
	} `json:"input"`
	Parameters struct {
		Resolution   string `json:"resolution,omitempty"`
		PromptExtend bool   `json:"prompt_extend,omitempty"`
		Duration     int    `json:"duration,omitempty"`
		ShotType     string `json:"shot_type,omitempty"`
		Audio        bool   `json:"audio,omitempty"`
		Watermark    bool   `json:"watermark,omitempty"`
		Seed         int    `json:"seed,omitempty"`
	} `json:"parameters"`
}

type AliVideoGenerationResponse struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Output  struct {
		TaskStatus string `json:"task_status,omitempty"`
		TaskID     string `json:"task_id,omitempty"`
	} `json:"output,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// DashScope image2video request/response.
// Endpoint: /api/v1/services/aigc/image2video/video-synthesis
type AliImage2VideoRequest struct {
	Model string `json:"model"`
	Input struct {
		FirstFrameURL string `json:"first_frame_url"`
		LastFrameURL  string `json:"last_frame_url"`
		Prompt        string `json:"prompt"`
	} `json:"input"`
	Parameters struct {
		Resolution   string `json:"resolution,omitempty"`
		PromptExtend bool   `json:"prompt_extend,omitempty"`
	} `json:"parameters"`
}

type AliImage2VideoResponse struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Output  struct {
		TaskStatus string `json:"task_status,omitempty"`
		TaskID     string `json:"task_id,omitempty"`
	} `json:"output,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// DashScope task status response.
// Endpoint: /api/v1/tasks/{task_id}
type AliTaskStatusResponse struct {
	RequestID string `json:"request_id,omitempty"`
	Output    struct {
		TaskID        string `json:"task_id,omitempty"`
		TaskStatus    string `json:"task_status,omitempty"`
		SubmitTime    string `json:"submit_time,omitempty"`
		ScheduledTime string `json:"scheduled_time,omitempty"`
		EndTime       string `json:"end_time,omitempty"`
		OrigPrompt    string `json:"orig_prompt,omitempty"`
		VideoURL      string `json:"video_url,omitempty"`
		Code          string `json:"code,omitempty"`
		Message       string `json:"message,omitempty"`
	} `json:"output,omitempty"`
	Usage struct {
		Duration            float64 `json:"duration,omitempty"`
		Size                string  `json:"size,omitempty"`
		InputVideoDuration  int     `json:"input_video_duration,omitempty"`
		OutputVideoDuration int     `json:"output_video_duration,omitempty"`
		VideoCount          int     `json:"video_count,omitempty"`
		SR                  int     `json:"SR,omitempty"`
	} `json:"usage,omitempty"`
}

// AliAdaptor converts requests and responses for DashScope APIs.
type AliAdaptor struct {
	BaseURL string
}

// GetRequestURL returns the DashScope endpoint for the given mode.
func (a *AliAdaptor) GetRequestURL(mode string, config *ProviderConfig) (string, error) {
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		base = "https://dashscope.aliyuncs.com"
	}

	switch mode {
	case ModeChat:
		return base + "/compatible-mode/v1/chat/completions", nil
	case ModeVideo:
		return base + aliVideoEndpointForModel(config.Model), nil
	case ModeImage:
		return base + "/api/v1/services/aigc/multimodal-generation/generation", nil
	default:
		return "", fmt.Errorf("unsupported mode: %s", mode)
	}
}

const (
	aliVideoEndpointImage2Video   = "/api/v1/services/aigc/image2video/video-synthesis"
	aliVideoEndpointVideoGenerate = "/api/v1/services/aigc/video-generation/video-synthesis"
)

// 模型映射
var aliVideoEndpointByModel = map[string]string{
	"wan2.2-kf2v-flash": aliVideoEndpointImage2Video,
	"wanx2.1-kf2v-plus": aliVideoEndpointImage2Video,
}

type aliImage2VideoPayload struct {
	AliImage2VideoRequest
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

type aliVideoPayload struct {
	AliVideoGenerationRequest
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

func aliVideoEndpointForModel(model string) string {
	if endpoint, ok := aliVideoEndpointByModel[model]; ok {
		return endpoint
	}
	return aliVideoEndpointVideoGenerate
}

func getStringExtra(extra map[string]interface{}, key string) string {
	if extra == nil {
		return ""
	}
	if value, ok := extra[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

func getBoolExtra(extra map[string]interface{}, key string) (bool, bool) {
	if extra == nil {
		return false, false
	}
	value, ok := extra[key]
	if !ok {
		return false, false
	}
	typed, ok := value.(bool)
	return typed, ok
}

func aliExtractPayloadMap(extra map[string]interface{}) map[string]interface{} {
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

func aliMarshalPayloadWithFallback(payload map[string]interface{}, fallback interface{}) ([]byte, error) {
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			return b, nil
		}
	}
	return json.Marshal(fallback)
}

// SetupHeaders sets DashScope headers.
func (a *AliAdaptor) SetupHeaders(req *http.Request, config *ProviderConfig, mode string) error {
	if config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")
	if mode == ModeVideo {
		req.Header.Set("X-DashScope-Async", "enable")
	}
	return nil
}

// PrepareStreamRequest creates a streaming chat request body.
func (a *AliAdaptor) PrepareStreamRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	streamRequest := *request
	if streamRequest.Options == nil {
		streamRequest.Options = make(map[string]interface{})
	}
	streamRequest.Stream = true
	streamRequest.Options["stream"] = true
	return a.ConvertChatRequest(ctx, config, &streamRequest)
}

// ParseStreamResponse processes a single streaming chunk.
func (a *AliAdaptor) ParseStreamResponse(chunk []byte) (string, error) {
	return (&OpenAIAdaptor{}).ParseStreamResponse(chunk)
}

// ConvertChatRequest converts a chat request to DashScope format.
func (a *AliAdaptor) ConvertChatRequest(ctx context.Context, config *ProviderConfig, request *dto.ChatRequest) ([]byte, error) {
	payload := struct {
		Model string `json:"model"`
		Input struct {
			Messages []dto.Message `json:"messages"`
		} `json:"input"`
		Parameters map[string]interface{} `json:"parameters,omitempty"`
	}{
		Model: request.Model,
	}
	payload.Input.Messages = request.Messages

	if request.Stream {
		payload.Parameters = map[string]interface{}{"incremental_output": true}
	}

	return json.Marshal(payload)
}

// ConvertChatResponse converts a DashScope chat response to the standardized format.
func (a *AliAdaptor) ConvertChatResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.ChatResponse, error) {
	var response struct {
		Output struct {
			Text    string `json:"text"`
			Choices []struct {
				Message dto.Message `json:"message"`
			} `json:"choices"`
		} `json:"output"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if response.Code != "" {
		return nil, &dto.LLMError{
			Code:     http.StatusBadRequest,
			Message:  response.Message,
			Provider: config.Name,
		}
	}

	chatResponse := &dto.ChatResponse{}
	if len(response.Output.Choices) > 0 {
		chatResponse.Choices = []dto.ChatChoice{{
			Index:   0,
			Message: response.Output.Choices[0].Message,
		}}
	} else if response.Output.Text != "" {
		chatResponse.Choices = []dto.ChatChoice{{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: response.Output.Text,
			},
		}}
	}
	chatResponse.Usage = dto.Usage{
		PromptTokens:     response.Usage.InputTokens,
		CompletionTokens: response.Usage.OutputTokens,
		TotalTokens:      response.Usage.TotalTokens,
	}
	return chatResponse, nil
}

// ConvertMediaRequest converts a media request to DashScope format.
func (a *AliAdaptor) ConvertMediaRequest(ctx context.Context, config *ProviderConfig, mode string, request *dto.MediaRequest) ([]byte, error) {
	if mode == ModeImage {
		return nil, fmt.Errorf("image mode not supported for ali adaptor")
	}
	if mode != ModeVideo {
		return nil, fmt.Errorf("unsupported media mode: %s", mode)
	}

	if aliVideoEndpointForModel(request.Model) == aliVideoEndpointImage2Video {
		fallback := aliImage2VideoPayload{
			AliImage2VideoRequest: AliImage2VideoRequest{
				Model: request.Model,
			},
		}
		fallback.Input.FirstFrameURL = getStringExtra(request.Extra, "first_frame_url")
		fallback.Input.LastFrameURL = getStringExtra(request.Extra, "last_frame_url")
		fallback.Input.Prompt = request.Prompt
		if prompt := getStringExtra(request.Extra, "prompt"); prompt != "" {
			fallback.Input.Prompt = prompt
		}

		params := map[string]interface{}{}
		if resolution := getStringExtra(request.Extra, "resolution"); resolution != "" {
			params["resolution"] = resolution
		}
		if promptExtend, ok := getBoolExtra(request.Extra, "prompt_extend"); ok {
			params["prompt_extend"] = promptExtend
		}
		if request.Seed != 0 {
			params["seed"] = request.Seed
		}
		for k, v := range request.Extra {
			params[k] = v
		}
		if len(params) > 0 {
			fallback.Parameters = params
		}

		payloadMap := aliExtractPayloadMap(request.Extra)
		return aliMarshalPayloadWithFallback(payloadMap, fallback)
	}

	fallback := aliVideoPayload{
		AliVideoGenerationRequest: AliVideoGenerationRequest{
			Model: request.Model,
		},
	}
	fallback.Input.Prompt = request.Prompt
	if prompt := getStringExtra(request.Extra, "prompt"); prompt != "" {
		fallback.Input.Prompt = prompt
	}
	params := map[string]interface{}{}
	if request.Size != "" {
		params["size"] = request.Size
	}
	if request.Duration != 0 {
		params["duration"] = request.Duration
	}
	if request.Fps != 0 {
		params["fps"] = request.Fps
	}
	if request.Seed != 0 {
		params["seed"] = request.Seed
	}
	for k, v := range request.Extra {
		params[k] = v
	}
	if len(params) > 0 {
		fallback.Parameters = params
	}

	payloadMap := aliExtractPayloadMap(request.Extra)
	return aliMarshalPayloadWithFallback(payloadMap, fallback)
}

// ConvertMediaResponse converts a DashScope media response to the standardized format.
func (a *AliAdaptor) ConvertMediaResponse(ctx context.Context, config *ProviderConfig, mode string, body []byte) (*dto.MediaResponse, error) {
	if mode == ModeImage {
		return nil, fmt.Errorf("image mode not supported for ali adaptor")
	}
	if mode != ModeVideo {
		return nil, fmt.Errorf("unsupported media mode: %s", mode)
	}

	var response struct {
		Output struct {
			TaskID     string `json:"task_id"`
			TaskStatus string `json:"task_status"`
			VideoURL   string `json:"video_url"`
		} `json:"output"`
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
		Message   string `json:"message"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if response.Code != "" {
		return nil, &dto.LLMError{
			Code:     http.StatusBadRequest,
			Message:  response.Message,
			Provider: config.Name,
		}
	}

	videoResponse := &dto.MediaResponse{
		Status:    strings.ToLower(response.Output.TaskStatus),
		RequestID: response.RequestID,
		TaskID:    response.Output.TaskID,
	}
	if response.Output.VideoURL != "" {
		videoResponse.Video.URL = response.Output.VideoURL
		videoResponse.URL = response.Output.VideoURL
	}
	return videoResponse, nil
}

// GetTaskStatusURL returns the task status endpoint for DashScope.
func (a *AliAdaptor) GetTaskStatusURL(taskID string, config *ProviderConfig) (string, error) {
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(a.BaseURL, "/")
	}
	if base == "" {
		base = "https://dashscope.aliyuncs.com"
	}
	return base + "/api/v1/tasks/" + taskID, nil
}

// ConvertTaskStatusResponse converts a DashScope task status response to the standardized format.
func (a *AliAdaptor) ConvertTaskStatusResponse(ctx context.Context, config *ProviderConfig, body []byte) (*dto.TaskStatusResponse, error) {
	var response struct {
		RequestID string `json:"request_id"`
		Output    struct {
			TaskID        string `json:"task_id"`
			TaskStatus    string `json:"task_status"`
			SubmitTime    string `json:"submit_time"`
			ScheduledTime string `json:"scheduled_time"`
			EndTime       string `json:"end_time"`
			VideoURL      string `json:"video_url"`
			OrigPrompt    string `json:"orig_prompt"`
			ActualPrompt  string `json:"actual_prompt"`
			Code          string `json:"code"`
			Message       string `json:"message"`
		} `json:"output"`
		Usage struct {
			VideoDuration int `json:"video_duration"`
			VideoCount    int `json:"video_count"`
			SR            int `json:"SR"`
		} `json:"usage"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if response.Code != "" {
		return nil, &dto.LLMError{
			Code:     http.StatusBadRequest,
			Message:  response.Message,
			Provider: config.Name,
		}
	}

	result := &dto.TaskStatusResponse{
		RequestID: response.RequestID,
		Output: dto.TaskStatusOutput{
			TaskID:        response.Output.TaskID,
			TaskStatus:    response.Output.TaskStatus,
			SubmitTime:    response.Output.SubmitTime,
			ScheduledTime: response.Output.ScheduledTime,
			EndTime:       response.Output.EndTime,
			VideoURL:      response.Output.VideoURL,
			OrigPrompt:    response.Output.OrigPrompt,
			ActualPrompt:  response.Output.ActualPrompt,
			Code:          response.Output.Code,
			Message:       response.Output.Message,
		},
	}

	if response.Usage.VideoDuration != 0 || response.Usage.VideoCount != 0 || response.Usage.SR != 0 {
		result.Usage = &dto.TaskStatusUsage{
			VideoDuration: response.Usage.VideoDuration,
			VideoCount:    response.Usage.VideoCount,
			SR:            response.Usage.SR,
		}
	}

	return result, nil
}
