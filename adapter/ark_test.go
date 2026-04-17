package adapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/YspCoder/omnigo/dto"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
	"github.com/volcengine/volcengine-go-sdk/volcengine"
)

func TestArkToVidReqOmitsTextRoleAndUsesFirstFrame(t *testing.T) {
	adaptor := &ArkAdaptor{}
	req := adaptor.toVidReq(&dto.MediaRequest{
		Model:      "seedance-1-0-pro-250528",
		Resolution: "2k",
		Messages: []dto.Message{
			{Role: "system", Content: "You are a film director."},
			{Role: "user", Content: "Create a cinematic snow reunion scene."},
		},
		Extra: map[string]interface{}{
			"image": "https://example.com/frame.jpg",
		},
	})

	if len(req.Content) != 3 {
		t.Fatalf("content len = %d, want %d", len(req.Content), 3)
	}
	if req.Content[0].Type != "text" || req.Content[0].Text == nil {
		t.Fatalf("content[0] = %#v, want text item", req.Content[0])
	}
	if req.Content[0].Role != nil {
		t.Fatalf("content[0].role = %v, want nil", *req.Content[0].Role)
	}
	if req.Content[1].Role != nil {
		t.Fatalf("content[1].role = %v, want nil", *req.Content[1].Role)
	}
	if req.Content[2].Type != "image_url" || req.Content[2].ImageURL == nil {
		t.Fatalf("content[2] = %#v, want image item", req.Content[2])
	}
	if req.Content[2].Role == nil || *req.Content[2].Role != "first_frame" {
		t.Fatalf("content[2].role = %v, want first_frame", req.Content[2].Role)
	}
	if req.Resolution != nil {
		t.Fatalf("resolution = %v, want nil for image-to-video", *req.Resolution)
	}
}

func TestArkToChatReqIncludesFileURLPart(t *testing.T) {
	adaptor := &ArkAdaptor{}
	req := adaptor.toChatReq(&dto.MediaRequest{
		Model: "doubao-1.5-pro",
		Messages: []dto.Message{
			{Role: "system", Content: "你是剧本拆解助手"},
			{Role: "user", Content: "请分析这份剧本", FileURL: "https://example.com/script.pdf", FileName: "script.pdf"},
		},
	})

	typed, ok := req.(*arkChatRequest)
	if !ok {
		t.Fatalf("req type = %T, want *arkChatRequest", req)
	}
	if len(typed.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(typed.Messages))
	}
	content := typed.Messages[1].Content
	if content == nil || len(content.ListValue) != 2 {
		t.Fatalf("content = %#v, want 2 parts", content)
	}
	if content.ListValue[0].Type != "text" || content.ListValue[0].Text != "请分析这份剧本" {
		t.Fatalf("content.ListValue[0] = %#v, want text part", content.ListValue[0])
	}
	if content.ListValue[1].Type != "file_url" || content.ListValue[1].FileURL == nil {
		t.Fatalf("content.ListValue[1] = %#v, want file_url part", content.ListValue[1])
	}
	if content.ListValue[1].FileURL.URL != "https://example.com/script.pdf" {
		t.Fatalf("file url = %q, want %q", content.ListValue[1].FileURL.URL, "https://example.com/script.pdf")
	}
	if content.ListValue[1].FileURL.FileName != "script.pdf" {
		t.Fatalf("file name = %q, want %q", content.ListValue[1].FileURL.FileName, "script.pdf")
	}
}

func TestArkToChatReqIncludesFileIDPartWithoutText(t *testing.T) {
	adaptor := &ArkAdaptor{}
	req := adaptor.toChatReq(&dto.MediaRequest{
		Model: "doubao-1.5-pro",
		Messages: []dto.Message{
			{Role: "user", FileID: "file_123", FileName: "script.pdf", Name: "episode-script"},
		},
	})

	typed, ok := req.(*arkChatRequest)
	if !ok {
		t.Fatalf("req type = %T, want *arkChatRequest", req)
	}
	if len(typed.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(typed.Messages))
	}
	content := typed.Messages[0].Content
	if content == nil || len(content.ListValue) != 1 {
		t.Fatalf("content = %#v, want 1 part", content)
	}
	if content.ListValue[0].Type != "file_url" || content.ListValue[0].FileURL == nil {
		t.Fatalf("content.ListValue[0] = %#v, want file_url part", content.ListValue[0])
	}
	if content.ListValue[0].FileURL.FileID != "file_123" {
		t.Fatalf("file id = %q, want %q", content.ListValue[0].FileURL.FileID, "file_123")
	}
	if content.ListValue[0].FileURL.Name != "episode-script" {
		t.Fatalf("name = %q, want %q", content.ListValue[0].FileURL.Name, "episode-script")
	}
}

func TestArkToChatReqSupportsTextPartWithFileURL(t *testing.T) {
	adaptor := &ArkAdaptor{}
	req := adaptor.toChatReq(&dto.MediaRequest{
		Model: "doubao-1.5-pro",
		Messages: []dto.Message{
			{
				Role: "user",
				Content: []map[string]interface{}{
					{
						"type":     "text",
						"text":     "请总结文档要点",
						"file_url": "https://example.com/doc.pdf",
					},
				},
			},
		},
	})

	typed, ok := req.(*arkChatRequest)
	if !ok {
		t.Fatalf("req type = %T, want *arkChatRequest", req)
	}
	if len(typed.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(typed.Messages))
	}
	content := typed.Messages[0].Content
	if content == nil || len(content.ListValue) != 2 {
		t.Fatalf("content = %#v, want 2 parts", content)
	}
	if content.ListValue[0].Type != "text" || content.ListValue[0].Text != "请总结文档要点" {
		t.Fatalf("content.ListValue[0] = %#v, want text part", content.ListValue[0])
	}
	if content.ListValue[1].Type != "file_url" || content.ListValue[1].FileURL == nil {
		t.Fatalf("content.ListValue[1] = %#v, want file_url part", content.ListValue[1])
	}
	if content.ListValue[1].FileURL.URL != "https://example.com/doc.pdf" {
		t.Fatalf("file url = %q, want %q", content.ListValue[1].FileURL.URL, "https://example.com/doc.pdf")
	}
}

func TestArkChatUploadsFileURLAndUsesFileID(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "ark-doc-*.txt")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	_, _ = tmp.WriteString("document body")
	_ = tmp.Close()

	fileURL := (&url.URL{Scheme: "file", Path: tmp.Name()}).String()
	uploadCalled := 0
	retrieveCalled := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/files":
			uploadCalled++
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse multipart: %v", err)
			}
			if got := r.FormValue("purpose"); got != "user_data" {
				t.Fatalf("purpose = %q, want user_data", got)
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("form file: %v", err)
			}
			defer file.Close()
			body, _ := io.ReadAll(file)
			if string(body) != "document body" {
				t.Fatalf("upload file body = %q, want %q", string(body), "document body")
			}
			_, _ = w.Write([]byte(`{"object":"file","id":"file_uploaded_123","purpose":"user_data","filename":"doc.txt","created_at":1,"expire_at":2,"status":"processing"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/files/file_uploaded_123":
			retrieveCalled++
			_, _ = w.Write([]byte(`{"object":"file","id":"file_uploaded_123","purpose":"user_data","filename":"doc.txt","created_at":1,"expire_at":2,"status":"active"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/chat/completions":
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode chat request: %v", err)
			}
			messages, ok := payload["messages"].([]interface{})
			if !ok || len(messages) != 1 {
				t.Fatalf("messages = %#v, want one message", payload["messages"])
			}
			msg, ok := messages[0].(map[string]interface{})
			if !ok {
				t.Fatalf("message = %#v, want map", messages[0])
			}
			content, ok := msg["content"].([]interface{})
			if !ok || len(content) != 2 {
				t.Fatalf("content = %#v, want two parts", msg["content"])
			}
			foundFilePart := false
			for _, item := range content {
				part, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				if part["type"] != "file_url" {
					continue
				}
				filePart, ok := part["file_url"].(map[string]interface{})
				if !ok {
					t.Fatalf("file_url = %#v, want map", part["file_url"])
				}
				if got := filePart["file_id"]; got != "file_uploaded_123" {
					t.Fatalf("file_id = %#v, want file_uploaded_123", got)
				}
				if got := filePart["url"]; got != nil {
					t.Fatalf("file url should be empty after upload, got %#v", got)
				}
				foundFilePart = true
			}
			if !foundFilePart {
				t.Fatalf("chat request has no file_url part: %#v", content)
			}
			_, _ = w.Write([]byte(`{"id":"chat-1","object":"chat.completion","created":1,"model":"doubao-1.5-pro","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	resp, err := (&ArkAdaptor{}).Chat(context.Background(), &ProviderConfig{
		APIKey:     "test",
		Region:     "cn-beijing",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}, &dto.MediaRequest{
		Model: "doubao-1.5-pro",
		Messages: []dto.Message{
			{
				Role: "user",
				Content: []map[string]interface{}{
					{"type": "text", "text": "请总结文档", "file_url": fileURL},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Chat error = %v", err)
	}
	if uploadCalled != 1 {
		t.Fatalf("upload called = %d, want 1", uploadCalled)
	}
	if retrieveCalled < 1 {
		t.Fatalf("retrieve called = %d, want >= 1", retrieveCalled)
	}
	if resp.Text != "done" {
		t.Fatalf("resp.Text = %q, want done", resp.Text)
	}
}

func TestArkToChatReqSupportsContentArrayWithImage(t *testing.T) {
	adaptor := &ArkAdaptor{}
	req := adaptor.toChatReq(&dto.MediaRequest{
		Model: "doubao-1.5-vision-pro",
		Messages: []dto.Message{
			{
				Role: "user",
				Content: []map[string]interface{}{
					{"type": "text", "text": "描述这张图"},
					{"type": "image_url", "image_url": map[string]interface{}{
						"url":    "https://example.com/cat.png",
						"detail": "high",
					}},
				},
			},
		},
	})

	typed, ok := req.(*arkChatRequest)
	if !ok {
		t.Fatalf("req type = %T, want *arkChatRequest", req)
	}
	if len(typed.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(typed.Messages))
	}
	content := typed.Messages[0].Content
	if content == nil || len(content.ListValue) != 2 {
		t.Fatalf("content = %#v, want 2 parts", content)
	}
	if content.ListValue[0].Type != "text" || content.ListValue[0].Text != "描述这张图" {
		t.Fatalf("content.ListValue[0] = %#v, want text part", content.ListValue[0])
	}
	if content.ListValue[1].Type != "image_url" || content.ListValue[1].ImageURL == nil {
		t.Fatalf("content.ListValue[1] = %#v, want image_url part", content.ListValue[1])
	}
	if content.ListValue[1].ImageURL.URL != "https://example.com/cat.png" {
		t.Fatalf("image url = %q, want cat image", content.ListValue[1].ImageURL.URL)
	}
	if content.ListValue[1].ImageURL.Detail != "high" {
		t.Fatalf("image detail = %q, want high", content.ListValue[1].ImageURL.Detail)
	}
}

func TestArkChatExtractsTextFromListResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"chat-1","object":"chat.completion","created":1,"model":"doubao-1.5-vision-pro","choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"text","text":"图里是一只橘猫"},{"type":"image_url","image_url":{"url":"https://example.com/result.png","detail":"auto"}}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`))
	}))
	defer server.Close()

	resp, err := (&ArkAdaptor{}).Chat(context.Background(), &ProviderConfig{
		APIKey:     "test",
		Region:     "cn-beijing",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}, &dto.MediaRequest{
		Model: "doubao-1.5-vision-pro",
		Messages: []dto.Message{
			{Role: "user", Content: "描述这张图"},
		},
	})
	if err != nil {
		t.Fatalf("Chat error = %v", err)
	}
	if resp.Text != "图里是一只橘猫" {
		t.Fatalf("resp.Text = %q, want extracted text", resp.Text)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices len = %d, want 1", len(resp.Choices))
	}
	msg := resp.Choices[0].Message
	if msg.Content != "图里是一只橘猫" {
		t.Fatalf("message content = %#v, want extracted text", msg.Content)
	}
	if msg.ImageURL != "https://example.com/result.png" {
		t.Fatalf("message image_url = %q, want result image", msg.ImageURL)
	}
	if msg.ImageDetail != "auto" {
		t.Fatalf("message image_detail = %q, want auto", msg.ImageDetail)
	}
}

func TestArkToVidReqSupportsSeedanceDocumentFields(t *testing.T) {
	adaptor := &ArkAdaptor{}
	req := adaptor.toVidReq(&dto.MediaRequest{
		Model:      "doubao-seedance-1-5-pro-251215",
		Duration:   5,
		Size:       "16:9",
		Resolution: "720p",
		Seed:       20,
		Messages: []dto.Message{
			{Role: "user", Content: "生成一段短片"},
		},
		Extra: map[string]interface{}{
			"callback_url":            "https://example.com/callback",
			"return_last_frame":       true,
			"service_tier":            "default",
			"execution_expires_after": 172800,
			"generate_audio":          true,
			"draft":                   true,
			"camera_fixed":            false,
			"watermark":               false,
			"frames":                  29,
			"draft_task_id":           "cgt-draft-123",
		},
	})

	if req.CallbackUrl == nil || *req.CallbackUrl != "https://example.com/callback" {
		t.Fatalf("callback_url = %v, want callback url", req.CallbackUrl)
	}
	if req.ReturnLastFrame == nil || !*req.ReturnLastFrame {
		t.Fatalf("return_last_frame = %v, want true", req.ReturnLastFrame)
	}
	if req.ServiceTier == nil || *req.ServiceTier != "default" {
		t.Fatalf("service_tier = %v, want default", req.ServiceTier)
	}
	if req.ExecutionExpiresAfter == nil || *req.ExecutionExpiresAfter != 172800 {
		t.Fatalf("execution_expires_after = %v, want 172800", req.ExecutionExpiresAfter)
	}
	if req.GenerateAudio == nil || !*req.GenerateAudio {
		t.Fatalf("generate_audio = %v, want true", req.GenerateAudio)
	}
	if req.Draft == nil || !*req.Draft {
		t.Fatalf("draft = %v, want true", req.Draft)
	}
	if req.CameraFixed == nil || *req.CameraFixed {
		t.Fatalf("camera_fixed = %v, want false", req.CameraFixed)
	}
	if req.Watermark == nil || *req.Watermark {
		t.Fatalf("watermark = %v, want false", req.Watermark)
	}
	if req.Frames == nil || *req.Frames != 29 {
		t.Fatalf("frames = %v, want 29", req.Frames)
	}
	if len(req.Content) != 2 {
		t.Fatalf("content len = %d, want 2", len(req.Content))
	}
	if req.Content[1].Type != "draft_task" || req.Content[1].DraftTask == nil || req.Content[1].DraftTask.ID != "cgt-draft-123" {
		t.Fatalf("content[1] = %#v, want draft_task content", req.Content[1])
	}
}

func TestArkToVidReqSupportsReferenceImages(t *testing.T) {
	adaptor := &ArkAdaptor{}
	req := adaptor.toVidReq(&dto.MediaRequest{
		Model:    "doubao-seedance-1-0-lite-i2v-250428",
		Duration: 5,
		Size:     "16:9",
		Messages: []dto.Message{
			{Role: "user", Content: "[图1]男生和[图2]柯基坐在[图3]草坪上"},
		},
		Extra: map[string]interface{}{
			"reference_images": []string{
				"https://example.com/ref-1.png",
				"https://example.com/ref-2.png",
				"https://example.com/ref-3.png",
				"https://example.com/ref-4.png",
				"https://example.com/ref-5.png",
			},
		},
	})

	if len(req.Content) != 6 {
		t.Fatalf("content len = %d, want 6", len(req.Content))
	}
	for i := 1; i < len(req.Content); i++ {
		item := req.Content[i]
		if item.Type != "image_url" || item.ImageURL == nil {
			t.Fatalf("content[%d] = %#v, want image_url", i, item)
		}
		if item.Role == nil || *item.Role != "reference_image" {
			t.Fatalf("content[%d].role = %v, want reference_image", i, item.Role)
		}
	}
	if got := req.Content[5].ImageURL.URL; got != "https://example.com/ref-5.png" {
		t.Fatalf("last reference image = %q, want ref-5", got)
	}
}

func TestArkToVidReqSupportsReferenceFiles(t *testing.T) {
	adaptor := &ArkAdaptor{}
	req := adaptor.toVidReq(&dto.MediaRequest{
		Model: "doubao-seedance-1-0-lite-i2v-250428",
		Messages: []dto.Message{
			{Role: "user", Content: "根据素材生成视频"},
		},
		Extra: map[string]interface{}{
			"files": []interface{}{
				map[string]interface{}{
					"url":   "https://example.com/ref-image.png",
					"type":  "image",
					"index": 1,
				},
				map[string]interface{}{
					"url":   "https://example.com/ref-video.mp4",
					"type":  "video",
					"index": 1,
				},
				map[string]interface{}{
					"url":   "https://example.com/ref-audio.mp3",
					"type":  "audio",
					"index": 1,
				},
			},
		},
	})

	if len(req.Content) != 2 {
		t.Fatalf("content len = %d, want 2", len(req.Content))
	}
	if req.Content[1].Type != "image_url" || req.Content[1].ImageURL == nil {
		t.Fatalf("content[1] = %#v, want image_url", req.Content[1])
	}
	if req.Content[1].Role == nil || *req.Content[1].Role != "reference_image" {
		t.Fatalf("content[1].role = %v, want reference_image", req.Content[1].Role)
	}

	raw, ok := req.ExtraBody["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("extra content = %#v, want []map[string]interface{}", req.ExtraBody["content"])
	}
	if len(raw) != 4 {
		t.Fatalf("extra content len = %d, want 4", len(raw))
	}
	if raw[1]["type"] != "image_url" || raw[1]["role"] != "reference_image" {
		t.Fatalf("raw[1] = %#v, want reference_image", raw[1])
	}
	videoURL, ok := raw[2]["video_url"].(map[string]interface{})
	if !ok || raw[2]["type"] != "video_url" || raw[2]["role"] != "reference_video" || videoURL["url"] != "https://example.com/ref-video.mp4" {
		t.Fatalf("raw[2] = %#v, want reference_video", raw[2])
	}
	audioURL, ok := raw[3]["audio_url"].(map[string]interface{})
	if !ok || raw[3]["type"] != "audio_url" || raw[3]["role"] != "reference_audio" || audioURL["url"] != "https://example.com/ref-audio.mp3" {
		t.Fatalf("raw[3] = %#v, want reference_audio", raw[3])
	}
}

func TestFormatUnixMillis(t *testing.T) {
	ts := time.Date(2026, 3, 17, 12, 34, 56, 0, time.UTC).UnixMilli()
	got := formatUnixMillis(ts)
	if got != "2026-03-17T12:34:56Z" {
		t.Fatalf("formatUnixMillis = %q, want RFC3339 UTC", got)
	}
}

func TestInt64ToInt(t *testing.T) {
	got := int64ToInt(volcengine.Int64(12))
	if got != 12 {
		t.Fatalf("int64ToInt = %d, want 12", got)
	}
}

func TestArkResponseText(t *testing.T) {
	content := &model.ChatCompletionMessageContent{
		ListValue: []*model.ChatCompletionMessageContentPart{
			{Type: model.ChatCompletionMessageContentPartTypeText, Text: "第一段"},
			{Type: model.ChatCompletionMessageContentPartTypeImageURL},
			{Type: model.ChatCompletionMessageContentPartTypeText, Text: "第二段"},
		},
	}

	if got := arkResponseText(content); got != "第一段\n第二段" {
		t.Fatalf("arkResponseText = %q, want joined text", got)
	}
}
