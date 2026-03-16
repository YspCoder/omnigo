package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestKlingBuildPayloadMapsImageMessages(t *testing.T) {
	adaptor := &KlingAdaptor{}
	req := &dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Model:    "kling-v2-6",
		Size:     "16:9",
		Duration: 5,
		Messages: []dto.Message{
			{Role: "user", Content: "镜头拉远，女生微笑", ImageURL: "https://example.com/start.png"},
			{Role: "user", ImageURL: "https://example.com/end.png"},
		},
	}

	mode, _, err := adaptor.resolveMode(req)
	if err != nil {
		t.Fatalf("resolveMode error = %v", err)
	}
	if mode != klingModeMultiImageToVideo {
		t.Fatalf("mode = %q, want %q", mode, klingModeMultiImageToVideo)
	}

	payload, err := adaptor.buildPayload(mode, req)
	if err != nil {
		t.Fatalf("buildPayload error = %v", err)
	}
	if payload["model_name"] != "kling-v2-6" {
		t.Fatalf("model_name = %#v, want kling-v2-6", payload["model_name"])
	}
	if payload["aspect_ratio"] != "16:9" {
		t.Fatalf("aspect_ratio = %#v, want 16:9", payload["aspect_ratio"])
	}
	if payload["duration"] != "5" {
		t.Fatalf("duration = %#v, want 5", payload["duration"])
	}

	imageList, ok := payload["image_list"].([]map[string]interface{})
	if !ok || len(imageList) != 2 {
		t.Fatalf("image_list = %#v, want two wrapped images", payload["image_list"])
	}
}

func TestKlingResolveModeUsesModelInference(t *testing.T) {
	adaptor := &KlingAdaptor{}

	mode, _, err := adaptor.resolveMode(&dto.MediaRequest{
		Type:  dto.MediaTypeText,
		Model: "kling-v2-6",
	})
	if err != nil {
		t.Fatalf("resolveMode text error = %v", err)
	}
	if mode != klingModeTextToVideo {
		t.Fatalf("text mode = %q, want %q", mode, klingModeTextToVideo)
	}

	mode, _, err = adaptor.resolveMode(&dto.MediaRequest{
		Type:  dto.MediaTypeImage,
		Model: "kling-image-o1",
	})
	if err != nil {
		t.Fatalf("resolveMode image error = %v", err)
	}
	if mode != klingModeOmniImage {
		t.Fatalf("image mode = %q, want %q", mode, klingModeOmniImage)
	}

	mode, _, err = adaptor.resolveMode(&dto.MediaRequest{
		Type:  dto.MediaTypeText,
		Model: "kling-v2-new",
	})
	if err != nil {
		t.Fatalf("resolveMode text image error = %v", err)
	}
	if mode != klingModeImageGeneration {
		t.Fatalf("text image mode = %q, want %q", mode, klingModeImageGeneration)
	}
}

func TestKlingMediaParsesTTSDirectResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/tts" {
			t.Fatalf("path = %q, want /v1/audio/tts", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("authorization = %q, want Bearer secret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","request_id":"req-1","data":{"task_id":"task-tts","task_status":"succeed","task_result":{"audios":[{"id":"audio-1","url":"https://cdn.example.com/output.mp3","duration":"3"}]}}}`))
	}))
	defer server.Close()

	adaptor := &KlingAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeAudio,
		Model: "tts-model",
		Extra: map[string]interface{}{
			"mode":     "tts",
			"text":     "hello world",
			"voice_id": "voice-1",
		},
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if resp.TaskID != "task-tts" {
		t.Fatalf("task id = %q, want task-tts", resp.TaskID)
	}
	if resp.URL != "https://cdn.example.com/output.mp3" {
		t.Fatalf("url = %q, want parsed audio url", resp.URL)
	}
}

func TestKlingTaskStatusFallsBackAcrossEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/videos/text2video/task-123":
			http.NotFound(w, r)
		case "/v1/videos/image2video/task-123":
			http.NotFound(w, r)
		case "/v1/videos/multi-image2video/task-123":
			http.NotFound(w, r)
		case "/v1/videos/omni-video/task-123":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"ok","request_id":"req-2","data":{"task_id":"task-123","task_status":"succeed","task_status_msg":"","created_at":1722769557708,"updated_at":1722769558708,"task_result":{"videos":[{"id":"vid-1","url":"https://cdn.example.com/video.mp4","watermark_url":"https://cdn.example.com/video-watermark.mp4"}]}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adaptor := &KlingAdaptor{}
	resp, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, "task-123")
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if resp.Output.TaskStatus != "succeed" {
		t.Fatalf("task status = %q, want succeed", resp.Output.TaskStatus)
	}
	if resp.Output.VideoURL != "https://cdn.example.com/video.mp4" {
		t.Fatalf("video url = %q, want parsed url", resp.Output.VideoURL)
	}
}
