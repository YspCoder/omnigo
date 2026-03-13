package adapter

import (
	"testing"

	"github.com/YspCoder/omnigo/dto"
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
