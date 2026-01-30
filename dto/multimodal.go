// Package dto defines standardized request and response payloads.
package dto

// ImageRequest represents a request for image generation (e.g., DALL-E).
type ImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"` // url or b64_json
}

// ImageResponse represents the response for image generation.
type ImageResponse struct {
	Created int64       `json:"created,omitempty"`
	Data    []ImageData `json:"data,omitempty"`
}

// ImageData holds the image payload.
type ImageData struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
}

// VideoRequest represents a request for video generation.
type VideoRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	Size           string `json:"size,omitempty"`
	Duration       int    `json:"duration,omitempty"`
	Fps            int    `json:"fps,omitempty"`
	Seed           int    `json:"seed,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

// VideoResponse represents the response for video generation.
type VideoResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"` // queued, processing, succeeded, failed
	Video  struct {
		URL string `json:"url,omitempty"`
	} `json:"video,omitempty"`
}
