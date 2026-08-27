package adapter

import (
	"context"
	"encoding/json"
	"io"
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

func TestCustomPayloadConvertsDramaFilesToReferences(t *testing.T) {
	payload, err := customPayload(&ProviderConfig{
		BaseURL: "https://drama.dafeiyangapi.top/v1/videos",
	}, &dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Model:    "seedance2.5",
		Prompt:   "animate the reference image",
		Duration: 5,
		Extra: map[string]interface{}{
			"image_url": "https://example.com/reference.png",
			"files": []map[string]interface{}{
				{
					"name": "reference image",
					"type": "image",
					"url":  "https://example.com/reference.png",
				},
				{
					"type":   "image",
					"role":   "first_frame",
					"source": "https://example.com/first-frame.png",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("customPayload error = %v", err)
	}
	if _, exists := payload["files"]; exists {
		t.Fatalf("files leaked into payload: %#v", payload)
	}
	if _, exists := payload["image_url"]; exists {
		t.Fatalf("duplicate image_url leaked into payload: %#v", payload)
	}
	wantReferences := []map[string]string{
		{
			"type":   "image",
			"role":   "reference",
			"source": "https://example.com/reference.png",
		},
		{
			"type":   "image",
			"role":   "first_frame",
			"source": "https://example.com/first-frame.png",
		},
	}
	if got := mustJSON(t, payload["references"]); got != mustJSON(t, wantReferences) {
		t.Fatalf("references = %s, want %s", got, mustJSON(t, wantReferences))
	}
}

func TestCustomPayloadPrefersExplicitReferencesOverFiles(t *testing.T) {
	explicitReferences := []map[string]interface{}{
		{
			"type":   "image",
			"role":   "first_frame",
			"source": "https://example.com/first-frame.png",
		},
	}
	payload, err := customPayload(&ProviderConfig{
		BaseURL: "https://drama.dafeiyangapi.top/v1/videos",
	}, &dto.MediaRequest{
		Type: dto.MediaTypeVideo,
		Extra: map[string]interface{}{
			"files": []map[string]interface{}{
				{"type": "image", "url": "https://example.com/reference.png"},
			},
			"references": explicitReferences,
		},
	})
	if err != nil {
		t.Fatalf("customPayload error = %v", err)
	}
	if _, exists := payload["files"]; exists {
		t.Fatalf("files leaked into payload: %#v", payload)
	}
	if got := mustJSON(t, payload["references"]); got != mustJSON(t, explicitReferences) {
		t.Fatalf("references = %s, want explicit %s", got, mustJSON(t, explicitReferences))
	}
}

func TestCustomPayloadConvertsFilesForOtherEndpoints(t *testing.T) {
	files := []map[string]interface{}{
		{"type": "image", "role": "last_frame", "url": "https://example.com/reference.png"},
	}
	payload, err := customPayload(&ProviderConfig{
		BaseURL: "https://api.example.com/v1/videos",
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeVideo,
		Extra: map[string]interface{}{"files": files},
	})
	if err != nil {
		t.Fatalf("customPayload error = %v", err)
	}
	if _, exists := payload["files"]; exists {
		t.Fatalf("files leaked into payload: %#v", payload)
	}
	wantReferences := []map[string]string{
		{"type": "image", "role": "last_frame", "source": "https://example.com/reference.png"},
	}
	if got := mustJSON(t, payload["references"]); got != mustJSON(t, wantReferences) {
		t.Fatalf("references = %s, want %s", got, mustJSON(t, wantReferences))
	}
}

func TestCustomAdaptorMediaSubmitsAsyncImageGeneration(t *testing.T) {
	var gotPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("path = %s, want /v1/images/generations", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"task-img-1","model":"nano-banana-pro-1k","object":"image.generation","status":"queued"}`))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1/images/generations",
	}, &dto.MediaRequest{
		Type:           dto.MediaTypeImage,
		Model:          "nano-banana-pro-1k",
		Prompt:         "cinematic city at night",
		Resolution:     "1K",
		N:              1,
		Seed:           42,
		ResponseFormat: "b64_json",
		Extra: map[string]interface{}{
			"aspect_ratio": "16:9",
			"async":        false,
			"images":       []string{"https://example.com/reference.png"},
		},
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if gotPayload["async"] != true {
		t.Fatalf("async = %#v, want true", gotPayload["async"])
	}
	if gotPayload["output_resolution"] != "1K" {
		t.Fatalf("output_resolution = %#v, want 1K", gotPayload["output_resolution"])
	}
	if _, exists := gotPayload["resolution"]; exists {
		t.Fatalf("image payload must not contain video resolution: %#v", gotPayload)
	}
	if _, exists := gotPayload["seed"]; exists {
		t.Fatalf("async image payload must not contain seed: %#v", gotPayload)
	}
	if _, exists := gotPayload["response_format"]; exists {
		t.Fatalf("async image payload must not contain response_format: %#v", gotPayload)
	}
	if resp.TaskID != "task-img-1" || resp.Status != "queued" || resp.Model != "nano-banana-pro-1k" || resp.Object != "image.generation" {
		t.Fatalf("response = %#v", resp)
	}
}

func TestCustomPayloadRejectsAsyncImageCountFromExtra(t *testing.T) {
	_, err := customPayload(nil, &dto.MediaRequest{
		Type:  dto.MediaTypeImage,
		Extra: map[string]interface{}{"n": "2"},
	})
	if err == nil || !strings.Contains(err.Error(), "n=1") {
		t.Fatalf("customPayload error = %v, want n=1 validation", err)
	}
}

func TestCustomAdaptorMediaSubmitsAsyncMultipartImageEdit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("path = %s, want /v1/images/edits", r.URL.Path)
		}
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		if r.FormValue("async") != "true" {
			t.Fatalf("async = %q, want true", r.FormValue("async"))
		}
		if r.FormValue("model") != "gpt-image-2" || r.FormValue("prompt") != "replace the sky" {
			t.Fatalf("multipart fields = %#v", r.MultipartForm.Value)
		}
		if r.FormValue("output_resolution") != "1K" {
			t.Fatalf("output_resolution = %q, want 1K", r.FormValue("output_resolution"))
		}
		if got := multipartFileContents(t, r, "image"); len(got) != 2 || got[0] != "first-image" || got[1] != "second-image" {
			t.Fatalf("image files = %#v", got)
		}
		if got := r.MultipartForm.File["image"][0].Header.Get("Content-Type"); got != "image/png" {
			t.Fatalf("image Content-Type = %q, want image/png", got)
		}
		if got := multipartFileContents(t, r, "mask"); len(got) != 1 || got[0] != "mask-image" {
			t.Fatalf("mask files = %#v", got)
		}
		_, _ = w.Write([]byte(`{"id":"task-edit-1","status":"queued"}`))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1/images/edits",
	}, &dto.MediaRequest{
		Type:       dto.MediaTypeImage,
		Model:      "gpt-image-2",
		Prompt:     "replace the sky",
		Resolution: "1K",
		Extra: map[string]interface{}{
			"image": []string{
				"data:image/png;base64,Zmlyc3QtaW1hZ2U=",
				"data:image/png;base64,c2Vjb25kLWltYWdl",
			},
			"mask": "data:image/png;base64,bWFzay1pbWFnZQ==",
		},
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if resp.TaskID != "task-edit-1" || resp.Status != "queued" {
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

func TestCustomAdaptorTaskStatusUsesTopLevelVideoURLPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("video_url_path") {
			t.Fatal("video_url_path control value leaked into query string")
		}
		_, _ = w.Write([]byte(`{"id":"task-1","status":"completed","video_url":"https://cdn.example.com/top-level.mp4"}`))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	resp, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1/videos",
	}, "task-1", map[string]string{"video_url_path": "video_url"})
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if resp.Output.URL != "https://cdn.example.com/top-level.mp4" || resp.Output.VideoURL != "https://cdn.example.com/top-level.mp4" {
		t.Fatalf("response URLs = %#v", resp.Output)
	}
}

func TestCustomAdaptorTaskStatusUsesNestedVideoURLPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("video_url_path") {
			t.Fatal("video_url_path control value leaked into query string")
		}
		if got := r.URL.Query().Get("locale"); got != "zh-CN" {
			t.Fatalf("locale = %q, want zh-CN", got)
		}
		_, _ = w.Write([]byte(`{"id":"task-1","status":"completed","metadata":{"url":"https://cdn.example.com/nested.mp4"}}`))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	resp, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1/videos",
	}, "task-1", map[string]string{
		"locale":         "zh-CN",
		"video_url_path": "metadata.url",
	})
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if resp.Output.URL != "https://cdn.example.com/nested.mp4" || resp.Output.VideoURL != "https://cdn.example.com/nested.mp4" {
		t.Fatalf("response URLs = %#v", resp.Output)
	}
}

func TestCustomAdaptorTaskStatusPreservesInProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"task-1","status":"in_progress"}`))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	resp, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1/videos",
	}, "task-1")
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if resp.Output.TaskStatus != "in_progress" {
		t.Fatalf("task status = %q, want raw in_progress", resp.Output.TaskStatus)
	}
	if !dto.IsPending(resp.Output.TaskStatus) {
		t.Fatalf("IsPending(%q) = false, want true", resp.Output.TaskStatus)
	}
}

func TestCustomAdaptorTaskStatusMapsAsyncImageResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"task-img-1","model":"nano-banana-pro-1k","object":"image.generation","status":"completed","data":[{"url":"https://example.com/image.png"}]}`))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	resp, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1/images/generations",
	}, "task-img-1")
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if resp.Output.TaskStatus != "completed" || resp.Output.URL != "https://example.com/image.png" {
		t.Fatalf("response = %#v", resp)
	}
	if resp.Output.VideoURL != "" {
		t.Fatalf("image result leaked into video URL: %#v", resp.Output)
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

func TestCustomAdaptorTaskStatusParsesWrappedFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"code":"success",
			"message":"",
			"data":{
				"id":160,
				"task_id":"task_LNjy5l7HkyCmKjDVKyYlrMQdnljGH4lb",
				"status":"FAILURE",
				"fail_reason":"output video may be related to copyright restrictions",
				"result_url":"output video may be related to copyright restrictions",
				"data":{
					"id":"cgt-20260806114637-b8n8m",
					"status":"failed",
					"error":{
						"code":"OutputVideoSensitiveContentDetected.PolicyViolation",
						"message":"output video may be related to copyright restrictions"
					}
				}
			}
		}`))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	resp, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1/videos",
	}, "fallback-task-id")
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if resp.Output.TaskID != "task_LNjy5l7HkyCmKjDVKyYlrMQdnljGH4lb" || resp.Output.TaskStatus != "FAILURE" {
		t.Fatalf("response task = %#v", resp.Output)
	}
	if resp.Output.Code != "OutputVideoSensitiveContentDetected.PolicyViolation" {
		t.Fatalf("response code = %q", resp.Output.Code)
	}
	if resp.Output.Message != "output video may be related to copyright restrictions" {
		t.Fatalf("response message = %q", resp.Output.Message)
	}
	if resp.Output.URL != "output video may be related to copyright restrictions" {
		t.Fatalf("response URL = %q", resp.Output.URL)
	}
}

func TestCustomAdaptorTaskStatusKeepsFlatTaskWithNestedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"task_id":"task-1",
			"status":"FAILURE",
			"fail_reason":"generation failed",
			"data":{"error":{"code":"policy_violation","message":"blocked"}}
		}`))
	}))
	defer server.Close()

	resp, err := (&CustomAdaptor{}).TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1/videos",
	}, "fallback-task-id")
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if resp.Output.TaskID != "task-1" || resp.Output.TaskStatus != "FAILURE" {
		t.Fatalf("response task = %#v", resp.Output)
	}
	if resp.Output.Code != "policy_violation" || resp.Output.Message != "generation failed" {
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

func multipartFileContents(t *testing.T, r *http.Request, field string) []string {
	t.Helper()
	var contents []string
	for _, header := range r.MultipartForm.File[field] {
		file, err := header.Open()
		if err != nil {
			t.Fatalf("open multipart %s: %v", field, err)
		}
		body, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			t.Fatalf("read multipart %s: %v", field, err)
		}
		contents = append(contents, string(body))
	}
	return contents
}
