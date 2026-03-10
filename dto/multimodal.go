// Package dto defines standardized request and response payloads.
package dto

import "encoding/json"

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
	N              int                    `json:"n,omitempty"`
	Size           string                 `json:"size,omitempty"`
	Resolution     string                 `json:"resolution,omitempty"`
	Duration       int                    `json:"duration,omitempty"`
	Fps            int                    `json:"fps,omitempty"`
	Seed           int                    `json:"seed,omitempty"`
	ResponseFormat string                 `json:"response_format,omitempty"`
	Extra          map[string]interface{} `json:"extra,omitempty"`
}

// MediaResponse represents the response for image/video generation.
type MediaResponse struct {
	Created      int64       `json:"created,omitempty"`
	Data         []ImageData `json:"data,omitempty"`
	RequestID    string      `json:"request_id,omitempty"`
	TaskID       string      `json:"task_id,omitempty"`
	Status       string      `json:"status,omitempty"`
	URL          string      `json:"url,omitempty"`
	Text         string      `json:"text,omitempty"`
	ErrorCode    string      `json:"code,omitempty"`
	ErrorMessage string      `json:"message,omitempty"`
	Video        struct {
		URL string `json:"url,omitempty"`
	} `json:"video,omitempty"`
}

// MarshalJSON removes duplicated URL fields and empty video object in JSON output.
func (m MediaResponse) MarshalJSON() ([]byte, error) {
	type mediaResponseJSON struct {
		Created      int64       `json:"created,omitempty"`
		Data         []ImageData `json:"data,omitempty"`
		RequestID    string      `json:"request_id,omitempty"`
		TaskID       string      `json:"task_id,omitempty"`
		Status       string      `json:"status,omitempty"`
		URL          string      `json:"url,omitempty"`
		Text         string      `json:"text,omitempty"`
		ErrorCode    string      `json:"code,omitempty"`
		ErrorMessage string      `json:"message,omitempty"`
		Video        *struct {
			URL string `json:"url,omitempty"`
		} `json:"video,omitempty"`
	}

	out := mediaResponseJSON{
		Created:      m.Created,
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
