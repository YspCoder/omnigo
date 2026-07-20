package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestCustomAdaptorMediaUsesFullEndpoint(t *testing.T) {
	t.Helper()

	var gotPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/videos" {
			t.Fatalf("path = %s, want /v1/videos", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q, want Bearer test-key", got)
		}
		if got := r.Header.Get("X-Tenant-ID"); got != "tenant-1" {
			t.Fatalf("X-Tenant-ID = %q, want tenant-1", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"task-1","status":"queued"}`))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1/videos",
		Headers: map[string]string{"X-Tenant-ID": "tenant-1"},
	}, &dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Model:    "seedance-2.0-fast-480p",
		Prompt:   "rainy neon street",
		Duration: 8,
		Extra: map[string]interface{}{
			"aspect_ratio": "16:9",
			"image_url":    "https://cdn.example.com/photo.jpg",
		},
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if resp.TaskID != "task-1" || resp.Status != "queued" {
		t.Fatalf("response = %#v, want task-1/queued", resp)
	}

	want := map[string]interface{}{
		"aspect_ratio": "16:9",
		"duration":     float64(8),
		"image_url":    "https://cdn.example.com/photo.jpg",
		"model":        "seedance-2.0-fast-480p",
		"prompt":       "rainy neon street",
	}
	if got := mustJSON(t, gotPayload); got != mustJSON(t, want) {
		t.Fatalf("payload = %s, want %s", got, mustJSON(t, want))
	}
}

func TestCustomAdaptorMediaUsesConfiguredModelAndMessagePrompt(t *testing.T) {
	var gotPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"task_id":"task-2","state":"submitted"}`))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		Model:   "configured-model",
		BaseURL: server.URL + "/generate",
	}, &dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Messages: []dto.Message{{Role: "user", Content: "make it move"}},
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if gotPayload["model"] != "configured-model" || gotPayload["prompt"] != "make it move" {
		t.Fatalf("payload = %#v", gotPayload)
	}
	if resp.TaskID != "task-2" || resp.Status != "submitted" {
		t.Fatalf("response = %#v", resp)
	}
}

func TestCustomAdaptorTaskStatusAppendsTaskID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/videos/task-1" {
			t.Fatalf("path = %s, want /v1/videos/task-1", r.URL.Path)
		}
		if got := r.URL.Query().Get("locale"); got != "zh-CN" {
			t.Fatalf("locale = %q, want zh-CN", got)
		}
		_, _ = w.Write([]byte(`{"id":"task-1","status":"completed","video_url":"https://cdn.example.com/output.mp4"}`))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	resp, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1/videos",
	}, "task-1", map[string]string{"locale": "zh-CN"})
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if resp.Output.TaskID != "task-1" || resp.Output.TaskStatus != "completed" {
		t.Fatalf("response = %#v", resp)
	}
	if resp.Output.VideoURL != "https://cdn.example.com/output.mp4" {
		t.Fatalf("video URL = %q", resp.Output.VideoURL)
	}
}

func TestCustomAdaptorTaskStatusEscapesTaskID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/v1/videos/task%2F1" {
			t.Fatalf("escaped path = %s, want /v1/videos/task%%2F1", r.URL.EscapedPath())
		}
		_, _ = w.Write([]byte(`{"id":"task/1","status":"processing"}`))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	_, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1/videos",
	}, "task/1")
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
}

func TestCustomAdaptorTaskStatusUsesFullEndpointTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tasks/task-9/result" {
			t.Fatalf("path = %s, want /tasks/task-9/result", r.URL.Path)
		}
		if r.URL.Query().Has("endpoint") {
			t.Fatal("endpoint control value leaked into query string")
		}
		_, _ = w.Write([]byte(`{"id":"task-9","status":"failed","error_code":"400017","error":{"message":"invalid image"}}`))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	resp, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/create",
	}, "task-9", map[string]string{
		"endpoint": server.URL + "/tasks/{task_id}/result",
	})
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if resp.Output.Code != "400017" || resp.Output.Message != "invalid image" {
		t.Fatalf("response error = %#v", resp.Output)
	}
}

func TestCustomAdaptorRejectsRelativeEndpoint(t *testing.T) {
	adaptor := &CustomAdaptor{}
	_, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: "/v1/videos",
	}, &dto.MediaRequest{Type: dto.MediaTypeVideo})
	if err == nil || !strings.Contains(err.Error(), "absolute http(s) URL") {
		t.Fatalf("error = %v, want absolute URL validation error", err)
	}
}

func TestCustomAdaptorReturnsReadableAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"400017","message":"invalid image"}}`))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	_, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1/videos",
	}, &dto.MediaRequest{Type: dto.MediaTypeVideo})
	if err == nil || !strings.Contains(err.Error(), "400017") || !strings.Contains(err.Error(), "invalid image") {
		t.Fatalf("error = %v, want API code and message", err)
	}
}

func TestRegistryBuildsCustomAdaptor(t *testing.T) {
	adaptor, spec, err := NewRegistry("custom").BuildAdaptor("custom")
	if err != nil {
		t.Fatalf("BuildAdaptor error = %v", err)
	}
	if _, ok := adaptor.(*CustomAdaptor); !ok {
		t.Fatalf("adaptor = %T, want *CustomAdaptor", adaptor)
	}
	if spec.Endpoint != "" {
		t.Fatalf("custom endpoint = %q, want empty", spec.Endpoint)
	}
}

func mustJSON(t *testing.T, value interface{}) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return string(raw)
}
