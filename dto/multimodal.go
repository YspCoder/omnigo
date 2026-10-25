// Package dto defines standardized request and response payloads.
package dto

import (
	"bytes"
	"encoding/json"
)

// Message represents a single message in a multimodal conversation.
type Message struct {
	Role        string      `json:"role"`
	Content     interface{} `json:"content"`
	ImageURL    string      `json:"image_url,omitempty"`
	ImageDetail string      `json:"image_detail,omitempty"`
	VideoURL    string      `json:"video_url,omitempty"`
	VideoFPS    float64     `json:"video_fps,omitempty"`
	FileURL     string      `json:"file_url,omitempty"`
	Name        string      `json:"name,omitempty"`
	ToolCalls   []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID  string      `json:"tool_call_id,omitempty"`
}

// ToolCall represents a function call requested by the model.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
	// Metadata preserves provider state required when replaying a tool call.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ToolCallFunction contains the function name and its JSON arguments.
type ToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Tool defines a function exposed to the model.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction contains the function metadata and JSON Schema parameters.
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// MediaType indicates the kind of media request.
type MediaType string

const (
	MediaTypeText  MediaType = "text"
	MediaTypeImage MediaType = "image"
	MediaTypeVideo MediaType = "video"
	MediaTypeAudio MediaType = "audio"
)

// MediaRequest represents a request for text/image/video generation.
// Use Metadata for structured provider fields and Extra for flat provider fields.
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
	Duration       any                    `json:"duration,omitempty"`
	Fps            int                    `json:"fps,omitempty"`
	Seed           int                    `json:"seed,omitempty"`
	ResponseFormat string                 `json:"response_format,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	Extra          map[string]interface{} `json:"extra,omitempty"`
	Prompt         string                 `json:"-"`
	Options        map[string]interface{} `json:"-"`
	Schema         interface{}            `json:"-"`
	Tools          []Tool                 `json:"tools,omitempty"`
	ToolChoice     interface{}            `json:"tool_choice,omitempty"`
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
	UsageDetails
}

// UsageDetails preserves token breakdowns and non-token billing in their native units.
type UsageDetails struct {
	PromptTokensDetails     map[string]json.RawMessage `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails map[string]json.RawMessage `json:"completion_tokens_details,omitempty"`
	Billing                 map[string]json.RawMessage `json:"billing,omitempty"`
	// RawUsage is the provider or SDK usage payload, including explicit zero values.
	// SDKs may omit fields they do not recognize; this is not the full HTTP response.
	RawUsage json.RawMessage `json:"raw_usage,omitempty"`
}

// CaptureRawUsage preserves an existing source payload when decoding normalized
// usage, or captures a native payload. Null-only objects do not report usage.
func (d *UsageDetails) CaptureRawUsage(raw json.RawMessage) {
	if len(d.RawUsage) > 0 && !bytes.Equal(bytes.TrimSpace(d.RawUsage), []byte("null")) {
		return
	}
	d.RawUsage = nil
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return
	}
	for key, value := range fields {
		if key != "raw_usage" && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			d.RawUsage = append(json.RawMessage(nil), raw...)
			return
		}
	}
}

// HasUsage distinguishes reported usage (including zero) from an absent payload.
func (u Usage) HasUsage() bool {
	raw := bytes.TrimSpace(u.RawUsage)
	return u.PromptTokens != 0 || u.CompletionTokens != 0 || u.TotalTokens != 0 ||
		(len(raw) > 0 && !bytes.Equal(raw, []byte("null"))) || len(u.PromptTokensDetails) > 0 ||
		len(u.CompletionTokensDetails) > 0 || len(u.Billing) > 0
}

func (u *Usage) UnmarshalJSON(raw []byte) error {
	type usageJSON Usage
	var decoded usageJSON
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*u = Usage(decoded)
	u.CaptureRawUsage(raw)
	return nil
}

// MediaResponse represents the unified response for text/image/video generation.
type MediaResponse struct {
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	ID           string                 `json:"id,omitempty"`
	Object       string                 `json:"object,omitempty"`
	Created      int64                  `json:"created,omitempty"`
	Model        string                 `json:"model,omitempty"`
	Choices      []ChatChoice           `json:"choices,omitempty"`
	Usage        Usage                  `json:"usage,omitempty"`
	Data         []ImageData            `json:"data,omitempty"`
	RequestID    string                 `json:"request_id,omitempty"`
	TaskID       string                 `json:"task_id,omitempty"`
	Status       string                 `json:"status,omitempty"`
	URL          string                 `json:"url,omitempty"`
	Text         string                 `json:"text,omitempty"`
	ErrorCode    string                 `json:"code,omitempty"`
	ErrorMessage string                 `json:"message,omitempty"`
	Video        struct {
		URL      string `json:"url,omitempty"`
		B64JSON  string `json:"b64_json,omitempty"`
		MIMEType string `json:"mime_type,omitempty"`
	} `json:"video,omitempty"`
}

// MarshalJSON removes duplicated URL fields and empty video object in JSON output.
func (m MediaResponse) MarshalJSON() ([]byte, error) {
	type mediaResponseJSON struct {
		Metadata     map[string]interface{} `json:"metadata,omitempty"`
		ID           string                 `json:"id,omitempty"`
		Object       string                 `json:"object,omitempty"`
		Created      int64                  `json:"created,omitempty"`
		Model        string                 `json:"model,omitempty"`
		Choices      []ChatChoice           `json:"choices,omitempty"`
		Usage        *Usage                 `json:"usage,omitempty"`
		Data         []ImageData            `json:"data,omitempty"`
		RequestID    string                 `json:"request_id,omitempty"`
		TaskID       string                 `json:"task_id,omitempty"`
		Status       string                 `json:"status,omitempty"`
		URL          string                 `json:"url,omitempty"`
		Text         string                 `json:"text,omitempty"`
		ErrorCode    string                 `json:"code,omitempty"`
		ErrorMessage string                 `json:"message,omitempty"`
		Video        *struct {
			URL      string `json:"url,omitempty"`
			B64JSON  string `json:"b64_json,omitempty"`
			MIMEType string `json:"mime_type,omitempty"`
		} `json:"video,omitempty"`
	}

	out := mediaResponseJSON{
		Metadata:     m.Metadata,
		ID:           m.ID,
		Object:       m.Object,
		Created:      m.Created,
		Model:        m.Model,
		Choices:      m.Choices,
		Data:         m.Data,
		RequestID:    m.RequestID,
		TaskID:       m.TaskID,
		Status:       m.Status,
		URL:          m.URL,
		Text:         m.Text,
		ErrorCode:    m.ErrorCode,
		ErrorMessage: m.ErrorMessage,
	}
	if m.Usage.HasUsage() {
		out.Usage = &m.Usage
	}
	if len(m.Data) > 0 && out.URL == m.Data[0].URL {
		out.URL = ""
	}
	if m.Video.URL != "" {
		out.Video = &struct {
			URL      string `json:"url,omitempty"`
			B64JSON  string `json:"b64_json,omitempty"`
			MIMEType string `json:"mime_type,omitempty"`
		}{URL: m.Video.URL, B64JSON: m.Video.B64JSON, MIMEType: m.Video.MIMEType}
	} else if m.Video.B64JSON != "" {
		out.Video = &struct {
			URL      string `json:"url,omitempty"`
			B64JSON  string `json:"b64_json,omitempty"`
			MIMEType string `json:"mime_type,omitempty"`
		}{B64JSON: m.Video.B64JSON, MIMEType: m.Video.MIMEType}
	}
	return json.Marshal(out)
}

// ImageData holds the image payload.
type ImageData struct {
	URL      string `json:"url,omitempty"`
	B64JSON  string `json:"b64_json,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
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
	URL           string `json:"url,omitempty"`
	VideoURL      string `json:"video_url,omitempty"`
	VideoB64JSON  string `json:"video_b64_json,omitempty"`
	VideoMIMEType string `json:"video_mime_type,omitempty"`
	LastFrameURL  string `json:"last_frame_url,omitempty"`
	Resolution    string `json:"resolution,omitempty"`
	Ratio         string `json:"ratio,omitempty"`
	Duration      int    `json:"duration,omitempty"`
	Seed          int    `json:"seed,omitempty"`
	ServiceTier   string `json:"service_tier,omitempty"`
	OrigPrompt    string `json:"orig_prompt,omitempty"`
	ActualPrompt  string `json:"actual_prompt,omitempty"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
	// Outputs retains every native task result when more than one is returned.
	Outputs []json.RawMessage `json:"outputs,omitempty"`
	// Result preserves structured responses from background text/image APIs.
	Result *MediaResponse `json:"result,omitempty"`
}

// TaskStatusUsage holds media and token usage details for task status response.
type TaskStatusUsage struct {
	VideoDuration    int `json:"video_duration,omitempty"`
	VideoCount       int `json:"video_count,omitempty"`
	SR               int `json:"SR,omitempty"`
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	UsageDetails
}

// TaskListResponse represents a provider task list response.
type TaskListResponse struct {
	Items      []TaskListItem `json:"items,omitempty"`
	Tasks      []TaskListItem `json:"tasks,omitempty"`
	Total      int            `json:"total,omitempty"`
	PageNum    int            `json:"page_num,omitempty"`
	PageSize   int            `json:"page_size,omitempty"`
	HasMore    bool           `json:"has_more,omitempty"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

// TaskListItem represents a single task in a task list response.
type TaskListItem struct {
	ID        string           `json:"id,omitempty"`
	TaskID    string           `json:"task_id,omitempty"`
	State     string           `json:"state,omitempty"`
	Model     string           `json:"model,omitempty"`
	CreatedAt string           `json:"created_at,omitempty"`
	Payload   interface{}      `json:"payload,omitempty"`
	Usage     *TaskStatusUsage `json:"usage,omitempty"`
}
