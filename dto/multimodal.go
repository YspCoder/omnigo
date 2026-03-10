// Package dto defines standardized request and response payloads.
package dto

import "encoding/json"

// Message represents a single message in a multimodal conversation.
type Message struct {
	Role        string      `json:"role"`
	Content     interface{} `json:"content"`
	ImageURL    string      `json:"image_url,omitempty"`
	ImageDetail string      `json:"image_detail,omitempty"`
	VideoURL    string      `json:"video_url,omitempty"`
	VideoFPS    float64     `json:"video_fps,omitempty"`
	FileURL     string      `json:"file_url,omitempty"`
	FileID      string      `json:"file_id,omitempty"`
	FileName    string      `json:"file_name,omitempty"`
	Name        string      `json:"name,omitempty"`
}

// MediaType indicates the kind of media request.
type MediaType string

const (
	MediaTypeText  MediaType = "text"
	MediaTypeImage MediaType = "image"
	MediaTypeVideo MediaType = "video"
)

// MediaRequest represents a request for text/image/video generation.
// Use Extra for provider- or model-specific fields.
type MediaRequest struct {
	Type           MediaType              `json:"-"`
	Model          string                 `json:"model"`
	Messages       []Message              `json:"messages,omitempty"`
	Stream         bool                   `json:"stream,omitempty"`
	Temperature    float64                `json:"temperature,omitempty"`
	MaxTokens      int                    `json:"max_tokens,omitempty"`
	N              int                    `json:"n,omitempty"`
	Size           string                 `json:"size,omitempty"`
	Resolution     string                 `json:"resolution,omitempty"`
	Duration       int                    `json:"duration,omitempty"`
	Fps            int                    `json:"fps,omitempty"`
	Seed           int                    `json:"seed,omitempty"`
	ResponseFormat string                 `json:"response_format,omitempty"`
	Extra          map[string]interface{} `json:"extra,omitempty"`
	Prompt         string                 `json:"-"`
	Options        map[string]interface{} `json:"-"`
	Schema         interface{}            `json:"-"`
}

// GenerateResponse preserves both the extracted text and the raw multimodal response.
type GenerateResponse struct {
	Text string         `json:"text,omitempty"`
	Raw  *MediaResponse `json:"raw,omitempty"`
}

// Choice represents a single text-generation candidate within a multimodal response.
type ChatChoice struct {
	Index        int     `json:"index,omitempty"`
	Message      Message `json:"message,omitempty"`
	FinishReason string  `json:"finish_reason,omitempty"`
}

// Usage represents token usage statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

// MediaResponse represents the unified response for text/image/video generation.
type MediaResponse struct {
	ID           string       `json:"id,omitempty"`
	Object       string       `json:"object,omitempty"`
	Created      int64        `json:"created,omitempty"`
	Model        string       `json:"model,omitempty"`
	Choices      []ChatChoice `json:"choices,omitempty"`
	Usage        Usage        `json:"usage,omitempty"`
	Data         []ImageData  `json:"data,omitempty"`
	RequestID    string       `json:"request_id,omitempty"`
	TaskID       string       `json:"task_id,omitempty"`
	Status       string       `json:"status,omitempty"`
	URL          string       `json:"url,omitempty"`
	Text         string       `json:"text,omitempty"`
	ErrorCode    string       `json:"code,omitempty"`
	ErrorMessage string       `json:"message,omitempty"`
	Video        struct {
		URL string `json:"url,omitempty"`
	} `json:"video,omitempty"`
}

// MarshalJSON removes duplicated URL fields and empty video object in JSON output.
func (m MediaResponse) MarshalJSON() ([]byte, error) {
	type mediaResponseJSON struct {
		ID           string       `json:"id,omitempty"`
		Object       string       `json:"object,omitempty"`
		Created      int64        `json:"created,omitempty"`
		Model        string       `json:"model,omitempty"`
		Choices      []ChatChoice `json:"choices,omitempty"`
		Usage        Usage        `json:"usage,omitempty"`
		Data         []ImageData  `json:"data,omitempty"`
		RequestID    string       `json:"request_id,omitempty"`
		TaskID       string       `json:"task_id,omitempty"`
		Status       string       `json:"status,omitempty"`
		URL          string       `json:"url,omitempty"`
		Text         string       `json:"text,omitempty"`
		ErrorCode    string       `json:"code,omitempty"`
		ErrorMessage string       `json:"message,omitempty"`
		Video        *struct {
			URL string `json:"url,omitempty"`
		} `json:"video,omitempty"`
	}

	out := mediaResponseJSON{
		ID:           m.ID,
		Object:       m.Object,
		Created:      m.Created,
		Model:        m.Model,
		Choices:      m.Choices,
		Usage:        m.Usage,
		Data:         m.Data,
		RequestID:    m.RequestID,
		TaskID:       m.TaskID,
		Status:       m.Status,
		URL:          m.URL,
		Text:         m.Text,
		ErrorCode:    m.ErrorCode,
		ErrorMessage: m.ErrorMessage,
	}
	if len(m.Data) > 0 && out.URL == m.Data[0].URL {
		out.URL = ""
	}
	if m.Video.URL != "" {
		out.Video = &struct {
			URL string `json:"url,omitempty"`
		}{URL: m.Video.URL}
	}
	return json.Marshal(out)
}

// ImageData holds the image payload.
type ImageData struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
}

// TaskStatusResponse represents the task status query response.
type TaskStatusResponse struct {
	RequestID string           `json:"request_id,omitempty"`
	Output    TaskStatusOutput `json:"output,omitempty"`
	Usage     *TaskStatusUsage `json:"usage,omitempty"`
}

// TaskStatusOutput holds task status details.
type TaskStatusOutput struct {
	TaskID        string `json:"task_id,omitempty"`
	TaskStatus    string `json:"task_status,omitempty"`
	SubmitTime    string `json:"submit_time,omitempty"`
	ScheduledTime string `json:"scheduled_time,omitempty"`
	EndTime       string `json:"end_time,omitempty"`
	VideoURL      string `json:"video_url,omitempty"`
	OrigPrompt    string `json:"orig_prompt,omitempty"`
	ActualPrompt  string `json:"actual_prompt,omitempty"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}

// TaskStatusUsage holds usage details for task status response.
type TaskStatusUsage struct {
	VideoDuration int `json:"video_duration,omitempty"`
	VideoCount    int `json:"video_count,omitempty"`
	SR            int `json:"SR,omitempty"`
}
