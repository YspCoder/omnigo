package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestAliChatUsesCompatibleModeBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/compatible-mode/v1/chat/completions" {
			t.Fatalf("path = %q, want /compatible-mode/v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chat-1","created":1,"model":"qwen-plus","choices":[{"message":{"role":"assistant","content":"hello"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	adaptor := &AliAdaptor{}
	resp, err := adaptor.Chat(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeText,
		Model: "qwen-plus",
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("Chat error = %v", err)
	}
	if resp.Text != "hello" {
		t.Fatalf("text = %q, want hello", resp.Text)
	}
}

func TestAliMediaSyncImageMapsURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != aliImageSyncEndpoint {
			t.Fatalf("path = %q, want %q", r.URL.Path, aliImageSyncEndpoint)
		}
		if got := r.Header.Get("X-DashScope-Async"); got != "" {
			t.Fatalf("X-DashScope-Async = %q, want empty", got)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["model"] != "qwen-image-max" {
			t.Fatalf("model = %#v, want qwen-image-max", body["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req-img","output":{"choices":[{"message":{"content":[{"image":"https://cdn.example.com/image.png"}]}}]}}`))
	}))
	defer server.Close()

	adaptor := &AliAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeImage,
		Model: "qwen-image-max",
		Messages: []dto.Message{
			{Role: "user", Content: "一只猫"},
		},
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if resp.URL != "https://cdn.example.com/image.png" {
		t.Fatalf("url = %q, want parsed image url", resp.URL)
	}
}

func TestAliMediaVideoUsesAsyncTaskEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != aliImageToVideoEndpoint {
			t.Fatalf("path = %q, want %q", r.URL.Path, aliImageToVideoEndpoint)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("authorization = %q, want Bearer secret", got)
		}
		if got := r.Header.Get("X-DashScope-Async"); got != "enable" {
			t.Fatalf("X-DashScope-Async = %q, want enable", got)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		input := body["input"].(map[string]interface{})
		if input["first_frame_url"] != "https://example.com/start.png" {
			t.Fatalf("first_frame_url = %#v, want start image", input["first_frame_url"])
		}
		if input["last_frame_url"] != "https://example.com/end.png" {
			t.Fatalf("last_frame_url = %#v, want end image", input["last_frame_url"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req-vid","output":{"task_id":"task-123","task_status":"PENDING"}}`))
	}))
	defer server.Close()

	adaptor := &AliAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Model: "wan2.2-kf2v-flash",
		Messages: []dto.Message{
			{Role: "user", Content: "镜头推进", ImageURL: "https://example.com/start.png"},
			{Role: "user", ImageURL: "https://example.com/end.png"},
		},
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if resp.TaskID != "task-123" {
		t.Fatalf("task id = %q, want task-123", resp.TaskID)
	}
	if resp.Status != "PENDING" {
		t.Fatalf("status = %q, want PENDING", resp.Status)
	}
}

func TestAliBuildVideoRequestUsesModelToInferReferenceMode(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Model: "wan2.2-r2v-plus",
		Messages: []dto.Message{
			{Role: "user", Content: "保持主体一致", ImageURL: "https://example.com/ref-1.png"},
			{Role: "user", ImageURL: "https://example.com/ref-2.png"},
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	if endpoint != aliVideoEndpoint {
		t.Fatalf("endpoint = %q, want %q", endpoint, aliVideoEndpoint)
	}
	input := payload["input"].(map[string]interface{})
	if _, ok := input["reference_urls"]; !ok {
		t.Fatalf("reference_urls missing from payload: %#v", input)
	}
	if _, ok := input["first_frame_url"]; ok {
		t.Fatalf("first_frame_url = %#v, want omitted for reference mode", input["first_frame_url"])
	}
}

func TestAliBuildVideoRequestUsesModelToInferImageMode(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, payload, err := adaptor.buildVideoRequest(&dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Model: "wan2.2-i2v-plus",
		Messages: []dto.Message{
			{Role: "user", Content: "镜头推进", ImageURL: "https://example.com/input.png"},
		},
	})
	if err != nil {
		t.Fatalf("buildVideoRequest error = %v", err)
	}
	if endpoint != aliImageToVideoEndpoint {
		t.Fatalf("endpoint = %q, want %q", endpoint, aliImageToVideoEndpoint)
	}
	input := payload["input"].(map[string]interface{})
	if input["first_frame_url"] != "https://example.com/input.png" {
		t.Fatalf("first_frame_url = %#v, want single input image", input["first_frame_url"])
	}
}

func TestAliBuildImageRequestUsesModelToInferAsyncImageGeneration(t *testing.T) {
	adaptor := &AliAdaptor{}
	endpoint, _, async, err := adaptor.buildImageRequest(&dto.MediaRequest{
		Type:  dto.MediaTypeImage,
		Model: "wan2.2-t2i-plus",
		Messages: []dto.Message{
			{Role: "user", Content: "一只猫"},
		},
	})
	if err != nil {
		t.Fatalf("buildImageRequest error = %v", err)
	}
	if endpoint != aliImageNewAsyncEndpoint && endpoint != aliImageAsyncEndpoint {
		t.Fatalf("endpoint = %q, want an async image endpoint", endpoint)
	}
	if !async {
		t.Fatalf("async = false, want true")
	}
}

func TestAliResolveVideoModePrefersModelRouteTable(t *testing.T) {
	mode := aliResolveVideoMode(&dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Model: "wanx2.1-kf2v-plus",
	})
	if mode != aliVideoModeKeyframe {
		t.Fatalf("mode = %q, want %q", mode, aliVideoModeKeyframe)
	}
}

func TestAliTaskStatusMapsResultURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks/task-123" {
			t.Fatalf("path = %q, want /api/v1/tasks/task-123", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req-task","output":{"task_id":"task-123","task_status":"SUCCEEDED","results":[{"video_url":"https://cdn.example.com/video.mp4"}]}}`))
	}))
	defer server.Close()

	adaptor := &AliAdaptor{}
	resp, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, "task-123")
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if resp.Output.TaskStatus != "SUCCEEDED" {
		t.Fatalf("task status = %q, want SUCCEEDED", resp.Output.TaskStatus)
	}
	if resp.Output.VideoURL != "https://cdn.example.com/video.mp4" {
		t.Fatalf("video url = %q, want parsed url", resp.Output.VideoURL)
	}
}

func TestAliListTasksBuildsQueryString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks/" {
			t.Fatalf("path = %q, want /api/v1/tasks/", r.URL.Path)
		}
		if r.URL.Query().Get("page_size") != "10" || r.URL.Query().Get("status") != "SUCCEEDED" {
			t.Fatalf("query = %v, want page_size/status", r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"task_id":"task-1","task_status":"SUCCEEDED","model":"wan2.2-t2v","submit_time":"2025-03-20T10:00:00Z"}],"total":1,"page_num":1,"page_size":10}`))
	}))
	defer server.Close()

	adaptor := &AliAdaptor{}
	resp, err := adaptor.ListTasks(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, map[string]string{
		"page_size": "10",
		"status":    "SUCCEEDED",
	})
	if err != nil {
		t.Fatalf("ListTasks error = %v", err)
	}
	if resp.Total != 1 || resp.PageSize != 10 {
		t.Fatalf("resp = %#v, want parsed pagination", resp)
	}
	if len(resp.Items) != 1 || resp.Items[0].TaskID != "task-1" {
		t.Fatalf("items = %#v, want one task item", resp.Items)
	}
}
