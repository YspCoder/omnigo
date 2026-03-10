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
