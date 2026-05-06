package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestViduBuildPayloadUsesModeAndCommonFields(t *testing.T) {
	adaptor := &ViduAdaptor{}
	req := &dto.MediaRequest{
		Model:      "viduq2",
		Size:       "16:9",
		Duration:   5,
		Resolution: "720p",
		Seed:       7,
		Messages:   []dto.Message{{Role: "user", Content: "cinematic mountain sunrise"}},
		Extra: map[string]interface{}{
			"mode":               "image-to-video",
			"image":              "https://example.com/input.jpg",
			"movement_amplitude": "large",
			"bgm":                true,
		},
	}

	mode, _, err := adaptor.resolveMode(req)
	if err != nil {
		t.Fatalf("resolveMode error = %v", err)
	}
	if mode != viduModeImage {
		t.Fatalf("mode = %q, want %q", mode, viduModeImage)
	}

	payload, err := adaptor.buildPayload(mode, req)
	if err != nil {
		t.Fatalf("buildPayload error = %v", err)
	}

	if payload["model"] != "viduq2" {
		t.Fatalf("model = %#v, want viduq2", payload["model"])
	}
	if payload["aspect_ratio"] != "16:9" {
		t.Fatalf("aspect_ratio = %#v, want 16:9", payload["aspect_ratio"])
	}
	if payload["resolution"] != "720p" {
		t.Fatalf("resolution = %#v, want 720p", payload["resolution"])
	}
	if payload["movement_amplitude"] != "large" {
		t.Fatalf("movement_amplitude = %#v, want large", payload["movement_amplitude"])
	}
	images, ok := payload["images"].([]string)
	if !ok || len(images) != 1 || images[0] != "https://example.com/input.jpg" {
		t.Fatalf("images = %#v, want single input image", payload["images"])
	}
}

func TestViduBuildPayloadReferenceWrapsImagesAsSubjects(t *testing.T) {
	adaptor := &ViduAdaptor{}
	payload, err := adaptor.buildPayload(viduModeReference, &dto.MediaRequest{
		Model:    "viduq2",
		Messages: []dto.Message{{Role: "user", Content: "keep the character consistent"}},
		Extra: map[string]interface{}{
			"images": []string{"https://example.com/a.png", "https://example.com/b.png"},
		},
	})
	if err != nil {
		t.Fatalf("buildPayload error = %v", err)
	}

	subjects, ok := payload["subjects"].([]map[string]interface{})
	if !ok || len(subjects) != 1 {
		t.Fatalf("subjects = %#v, want one wrapped subject", payload["subjects"])
	}
	images, ok := subjects[0]["images"].([]string)
	if !ok || len(images) != 2 {
		t.Fatalf("subject images = %#v, want two images", subjects[0]["images"])
	}
}

func TestViduTaskStatusMapsCreationURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ent/v2/tasks/task-123/creations" {
			t.Fatalf("path = %q, want task creations endpoint", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("authorization = %q, want Bearer secret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"task-123","state":"success","creations":[{"id":"creation-1","url":"https://cdn.example.com/video.mp4","cover_url":"https://cdn.example.com/video.jpg"}]}`))
	}))
	defer server.Close()

	adaptor := &ViduAdaptor{}
	status, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, "task-123")
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if status.Output.TaskID != "task-123" {
		t.Fatalf("task id = %q, want task-123", status.Output.TaskID)
	}
	if status.Output.TaskStatus != "success" {
		t.Fatalf("task status = %q, want success", status.Output.TaskStatus)
	}
	if status.Output.VideoURL != "https://cdn.example.com/video.mp4" {
		t.Fatalf("video url = %q, want mapped creation url", status.Output.VideoURL)
	}
}

func TestViduListTasksBuildsQueryString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ent/v2/tasks" {
			t.Fatalf("path = %q, want /ent/v2/tasks", r.URL.Path)
		}
		if r.URL.Query().Get("page_num") != "2" || r.URL.Query().Get("page_size") != "20" {
			t.Fatalf("query = %v, want page_num/page_size", r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"task-1","state":"success"}],"total":1,"page_num":2,"page_size":20}`))
	}))
	defer server.Close()

	adaptor := &ViduAdaptor{}
	resp, err := adaptor.ListTasks(context.Background(), &ProviderConfig{
		APIKey:  "secret",
		BaseURL: server.URL,
	}, map[string]string{
		"page_num":  "2",
		"page_size": "20",
	})
	if err != nil {
		t.Fatalf("ListTasks error = %v", err)
	}
	if resp.Total != 1 || resp.PageNum != 2 || resp.PageSize != 20 {
		t.Fatalf("resp = %#v, want parsed pagination", resp)
	}
	if len(resp.Items) != 1 || resp.Items[0].ID != "task-1" {
		t.Fatalf("items = %#v, want one task item", resp.Items)
	}
}
