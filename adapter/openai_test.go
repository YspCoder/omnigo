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

func TestOpenAIChatUsesResponsesAPIForFileMessages(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_123",
			"model":"gpt-4.1",
			"output_text":"结构化分集结果",
			"usage":{"input_tokens":11,"output_tokens":22,"total_tokens":33}
		}`))
	}))
	defer srv.Close()

	adaptor := &OpenAIAdaptor{}
	resp, err := adaptor.Chat(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, &dto.MediaRequest{
		Model: "gpt-4.1",
		Messages: []dto.Message{
			{Role: "system", Content: "你是拆解助手"},
			{Role: "user", Content: "请分析这份剧本", FileURL: "https://example.com/script.pdf", Name: "script.pdf"},
		},
		MaxTokens:   2048,
		Temperature: 0.3,
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if gotPath != "/responses" {
		t.Fatalf("path = %q, want /responses", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if resp.Text != "结构化分集结果" {
		t.Fatalf("resp.Text = %q", resp.Text)
	}
	input, ok := gotBody["input"].([]interface{})
	if !ok || len(input) != 2 {
		t.Fatalf("input = %#v", gotBody["input"])
	}
	second, _ := input[1].(map[string]interface{})
	content, _ := second["content"].([]interface{})
	if len(content) != 2 {
		t.Fatalf("content = %#v", second["content"])
	}
	foundFile := false
	for _, item := range content {
		part, _ := item.(map[string]interface{})
		if part["type"] == "input_file" && part["file_url"] == "https://example.com/script.pdf" {
			foundFile = true
		}
	}
	if !foundFile {
		t.Fatalf("expected input_file part, got %#v", content)
	}
}

func TestOpenAIChatUsesResponsesOutputContentFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"id":"resp_456",
			"model":"gpt-4.1",
			"output":[
				{
					"type":"message",
					"role":"assistant",
					"content":[
						{"type":"output_text","text":"第一段"},
						{"type":"output_text","text":"第二段"}
					]
				}
			]
		}`))
	}))
	defer srv.Close()

	adaptor := &OpenAIAdaptor{}
	resp, err := adaptor.Chat(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, &dto.MediaRequest{
		Model: "gpt-4.1",
		Messages: []dto.Message{
			{Role: "user", Content: "请读取文件", FileURL: "https://example.com/script.pdf", Name: "script.pdf"},
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if got := strings.TrimSpace(resp.Text); got != "第一段\n第二段" {
		t.Fatalf("resp.Text = %q", got)
	}
}

func TestOpenAIChatUsesResponsesAPIForMultiImageContent(t *testing.T) {
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_multi_img",
			"model":"gpt-4.1",
			"output_text":"ok"
		}`))
	}))
	defer srv.Close()

	adaptor := &OpenAIAdaptor{}
	_, err := adaptor.Chat(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, &dto.MediaRequest{
		Model: "gpt-4.1",
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: []interface{}{"https://example.com/a.png", "https://example.com/b.png"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	input, ok := gotBody["input"].([]interface{})
	if !ok || len(input) != 1 {
		t.Fatalf("input = %#v", gotBody["input"])
	}
	first, _ := input[0].(map[string]interface{})
	content, _ := first["content"].([]interface{})
	if len(content) != 2 {
		t.Fatalf("content = %#v", first["content"])
	}
	for i, want := range []string{"https://example.com/a.png", "https://example.com/b.png"} {
		part, _ := content[i].(map[string]interface{})
		if part["type"] != "input_image" {
			t.Fatalf("content[%d].type = %#v, want input_image", i, part["type"])
		}
		if part["image_url"] != want {
			t.Fatalf("content[%d].image_url = %#v, want %q", i, part["image_url"], want)
		}
	}
}

func TestOpenAIMediaImageUsesEditWhenExtraImageProvided(t *testing.T) {
	var gotPath string
	var gotPrompt string
	var gotImage string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if gotPath != "/images/edits" {
			t.Fatalf("path = %q, want /images/edits", gotPath)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		gotPrompt = r.FormValue("prompt")
		file, _, err := r.FormFile("image")
		if err != nil {
			t.Fatalf("form file image: %v", err)
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read form file: %v", err)
		}
		gotImage = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"edited-image"}]}`))
	}))
	defer srv.Close()

	adaptor := &OpenAIAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeImage,
		Model: "gpt-image-1",
		Messages: []dto.Message{
			{Role: "user", Content: "make it cinematic"},
		},
		Extra: map[string]interface{}{
			"image": "data:image/png;base64,cmVmZXJlbmNlLWltYWdl",
		},
	})
	if err != nil {
		t.Fatalf("Media() error = %v", err)
	}
	if gotPrompt != "make it cinematic" {
		t.Fatalf("prompt = %q", gotPrompt)
	}
	if gotImage != "reference-image" {
		t.Fatalf("image body = %q", gotImage)
	}
	if resp == nil || len(resp.Data) != 1 || resp.Data[0].B64JSON != "edited-image" {
		t.Fatalf("response = %#v", resp)
	}
}

func TestOpenAIMediaImageUsesEditWhenExtraImageArrayProvided(t *testing.T) {
	var gotPrompt string
	var gotImages []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/edits" {
			t.Fatalf("path = %q, want /images/edits", r.URL.Path)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		gotPrompt = r.FormValue("prompt")
		for _, headers := range r.MultipartForm.File {
			for _, header := range headers {
				if header == nil {
					continue
				}
				file, err := header.Open()
				if err != nil {
					t.Fatalf("open form file: %v", err)
				}
				body, err := io.ReadAll(file)
				file.Close()
				if err != nil {
					t.Fatalf("read form file: %v", err)
				}
				gotImages = append(gotImages, string(body))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"edited-image"}]}`))
	}))
	defer srv.Close()

	adaptor := &OpenAIAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeImage,
		Model: "gpt-image-1",
		Messages: []dto.Message{
			{Role: "user", Content: "make it cinematic"},
		},
		Extra: map[string]interface{}{
			"image": []string{
				"data:image/png;base64,Zmlyc3QtaW1hZ2U=",
				"data:image/png;base64,c2Vjb25kLWltYWdl",
			},
		},
	})
	if err != nil {
		t.Fatalf("Media() error = %v", err)
	}
	if gotPrompt != "make it cinematic" {
		t.Fatalf("prompt = %q", gotPrompt)
	}
	if len(gotImages) != 2 {
		t.Fatalf("got %d images, want 2", len(gotImages))
	}
	if gotImages[0] != "first-image" || gotImages[1] != "second-image" {
		t.Fatalf("image bodies = %#v", gotImages)
	}
	if resp == nil || len(resp.Data) != 1 || resp.Data[0].B64JSON != "edited-image" {
		t.Fatalf("response = %#v", resp)
	}
}

func TestOpenAIMediaImageUsesGenerateWithoutExtraImage(t *testing.T) {
	var gotPath string
	var gotPrompt string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if gotPath != "/images/generations" {
			t.Fatalf("path = %q, want /images/generations", gotPath)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		gotPrompt, _ = body["prompt"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://example.com/generated.png"}]}`))
	}))
	defer srv.Close()

	adaptor := &OpenAIAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeImage,
		Model: "gpt-image-1",
		Messages: []dto.Message{
			{Role: "user", Content: "make a poster"},
		},
	})
	if err != nil {
		t.Fatalf("Media() error = %v", err)
	}
	if gotPrompt != "make a poster" {
		t.Fatalf("prompt = %q", gotPrompt)
	}
	if resp == nil || resp.URL != "https://example.com/generated.png" {
		t.Fatalf("response = %#v", resp)
	}
}

func TestOpenAIMediaImagePropagatesAsyncExtra(t *testing.T) {
	var gotAsync bool
	var hasAsync bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/generations" {
			t.Fatalf("path = %q, want /images/generations", r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		async, ok := body["async"].(bool)
		hasAsync = ok
		gotAsync = async
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://example.com/generated.png"}]}`))
	}))
	defer srv.Close()

	adaptor := &OpenAIAdaptor{}
	_, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeImage,
		Model: "gpt-image-1",
		Messages: []dto.Message{
			{Role: "user", Content: "make a poster"},
		},
		Extra: map[string]interface{}{
			"async": true,
		},
	})
	if err != nil {
		t.Fatalf("Media() error = %v", err)
	}
	if !hasAsync {
		t.Fatal("request body missing async field")
	}
	if !gotAsync {
		t.Fatalf("async = %v, want true", gotAsync)
	}
}

func TestOpenAIMediaImageAsyncReturnsTaskID(t *testing.T) {
	var gotAsync bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/generations" {
			t.Fatalf("path = %q, want /images/generations", r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		gotAsync, _ = body["async"].(bool)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req-img-1","task_id":"task-img-1","status":"queued"}`))
	}))
	defer srv.Close()

	adaptor := &OpenAIAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeImage,
		Model: "gpt-image-1",
		Messages: []dto.Message{
			{Role: "user", Content: "make a poster"},
		},
		Extra: map[string]interface{}{
			"async": true,
		},
	})
	if err != nil {
		t.Fatalf("Media() error = %v", err)
	}
	if !gotAsync {
		t.Fatal("request body missing async=true")
	}
	if resp.TaskID != "task-img-1" {
		t.Fatalf("task id = %q, want task-img-1", resp.TaskID)
	}
	if resp.Status != "queued" {
		t.Fatalf("status = %q, want queued", resp.Status)
	}
	if resp.RequestID != "req-img-1" {
		t.Fatalf("request id = %q, want req-img-1", resp.RequestID)
	}
}

func TestOpenAIMediaImageEditAsyncReturnsTaskID(t *testing.T) {
	const png1x1 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aF9sAAAAASUVORK5CYII="

	var gotAsync bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/edits" {
			t.Fatalf("path = %q, want /images/edits", r.URL.Path)
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		gotAsync = r.FormValue("async") == "true"
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req-edit-1","task_id":"task-edit-1","task_status":"processing"}`))
	}))
	defer srv.Close()

	adaptor := &OpenAIAdaptor{}
	resp, err := adaptor.Media(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, &dto.MediaRequest{
		Type:  dto.MediaTypeImage,
		Model: "gpt-image-1",
		Messages: []dto.Message{
			{Role: "user", Content: "edit this image"},
		},
		Extra: map[string]interface{}{
			"async": true,
			"image": png1x1,
		},
	})
	if err != nil {
		t.Fatalf("Media() error = %v", err)
	}
	if !gotAsync {
		t.Fatal("multipart form missing async=true")
	}
	if resp.TaskID != "task-edit-1" {
		t.Fatalf("task id = %q, want task-edit-1", resp.TaskID)
	}
	if resp.Status != "processing" {
		t.Fatalf("status = %q, want processing", resp.Status)
	}
	if resp.RequestID != "req-edit-1" {
		t.Fatalf("request id = %q, want req-edit-1", resp.RequestID)
	}
}

func TestOpenAITaskStatusMapsImageResult(t *testing.T) {
	var gotPath string
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"img_task_123",
			"status":"succeeded",
			"created_at":1710000000,
			"completed_at":1710000060,
			"data":[{"url":"https://example.com/result.png"}]
		}`))
	}))
	defer srv.Close()

	adaptor := &OpenAIAdaptor{}
	resp, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, "img_task_123")
	if err != nil {
		t.Fatalf("TaskStatus() error = %v", err)
	}
	if gotPath != "/v1/gpt/images/img_task_123" {
		t.Fatalf("path = %q, want /v1/gpt/images/img_task_123", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if resp.Output.TaskID != "img_task_123" {
		t.Fatalf("task id = %q", resp.Output.TaskID)
	}
	if resp.Output.TaskStatus != "succeeded" {
		t.Fatalf("task status = %q", resp.Output.TaskStatus)
	}
	if resp.Output.URL != "https://example.com/result.png" {
		t.Fatalf("url = %q", resp.Output.URL)
	}
	if resp.Output.SubmitTime == "" {
		t.Fatalf("submit time should not be empty")
	}
	if resp.Output.EndTime == "" {
		t.Fatalf("end time should not be empty")
	}
}

func TestOpenAITaskStatusReturnsAPIErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"error":{"code":"task_failed","message":"generation failed"}
		}`))
	}))
	defer srv.Close()

	adaptor := &OpenAIAdaptor{}
	_, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, "img_task_456")
	if err == nil {
		t.Fatal("TaskStatus() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "generation failed") {
		t.Fatalf("error = %v", err)
	}
}
