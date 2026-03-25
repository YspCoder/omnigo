package adapter

import (
	"encoding/base64"
	"testing"

	"github.com/YspCoder/omnigo/dto"
	"google.golang.org/genai"
)

func TestGoogleToVidCfg_VideoPayloadShape(t *testing.T) {
	adp := &GoogleAdaptor{}
	req := &dto.MediaRequest{
		Messages: []dto.Message{{Role: "user", Content: "a cat running"}},
		N:        1,
		Size:     "16:9",
		Duration: 8,
	}

	cfg := adp.toVidCfg(req)
	if cfg == nil {
		t.Fatal("expected video config")
	}

	if cfg.AspectRatio != "16:9" {
		t.Fatalf("expected aspect ratio 16:9, got %q", cfg.AspectRatio)
	}

	if cfg.DurationSeconds == nil || *cfg.DurationSeconds != 8 {
		t.Fatalf("expected durationSeconds 8, got %#v", cfg.DurationSeconds)
	}
}

func TestGoogleImageModelRouting(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{model: "gemini-3.1-flash-image-preview", want: true},
		{model: "gemini-2.5-flash", want: true},
		{model: "imagen-3.0-generate-001", want: false},
	}

	for _, tt := range tests {
		if got := isGoogleGenerateContentImageModel(tt.model); got != tt.want {
			t.Fatalf("isGoogleGenerateContentImageModel(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestGoogleToImgContentCfg(t *testing.T) {
	adp := &GoogleAdaptor{}
	req := &dto.MediaRequest{
		Size:       "16:9",
		Resolution: "2K",
		Extra: map[string]interface{}{
			"person_generation": "ALLOW_ADULT",
		},
	}

	cfg := adp.toImgContentCfg(req)
	if cfg == nil {
		t.Fatal("expected image content config")
	}
	if len(cfg.ResponseModalities) != 2 || cfg.ResponseModalities[0] != "TEXT" || cfg.ResponseModalities[1] != "IMAGE" {
		t.Fatalf("unexpected response modalities: %#v", cfg.ResponseModalities)
	}
	if cfg.ImageConfig == nil {
		t.Fatal("expected image config")
	}
	if cfg.ImageConfig.AspectRatio != "16:9" {
		t.Fatalf("expected aspect ratio 16:9, got %q", cfg.ImageConfig.AspectRatio)
	}
	if cfg.ImageConfig.ImageSize != "2K" {
		t.Fatalf("expected image size 2K, got %q", cfg.ImageConfig.ImageSize)
	}
	if cfg.ImageConfig.PersonGeneration != "ALLOW_ADULT" {
		t.Fatalf("expected person generation override, got %q", cfg.ImageConfig.PersonGeneration)
	}
}

func TestGoogleToImgContentCfg_NormalizesSwappedShapeFields(t *testing.T) {
	adp := &GoogleAdaptor{}
	req := &dto.MediaRequest{
		Size:       "2K",
		Resolution: "16:9",
	}

	cfg := adp.toImgContentCfg(req)
	if cfg == nil || cfg.ImageConfig == nil {
		t.Fatal("expected image config")
	}
	if cfg.ImageConfig.AspectRatio != "16:9" {
		t.Fatalf("expected aspect ratio 16:9, got %q", cfg.ImageConfig.AspectRatio)
	}
	if cfg.ImageConfig.ImageSize != "2K" {
		t.Fatalf("expected image size 2K, got %q", cfg.ImageConfig.ImageSize)
	}
}

func TestGoogleToImgCfg_NormalizesSwappedShapeFields(t *testing.T) {
	adp := &GoogleAdaptor{}
	req := &dto.MediaRequest{
		Size:       "4k",
		Resolution: "1:1",
	}

	cfg := adp.toImgCfg(req)
	if cfg == nil {
		t.Fatal("expected image config")
	}
	if cfg.AspectRatio != "1:1" {
		t.Fatalf("expected aspect ratio 1:1, got %q", cfg.AspectRatio)
	}
	if cfg.ImageSize != "4K" {
		t.Fatalf("expected image size 4K, got %q", cfg.ImageSize)
	}
}

func TestGoogleToImgContentResp(t *testing.T) {
	adp := &GoogleAdaptor{}
	raw := []byte("image-bytes")
	resp := adp.toImgContentResp(&genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				FinishReason: "STOP",
				Content: &genai.Content{
					Role: "model",
					Parts: []*genai.Part{
						{Text: "caption"},
						{InlineData: &genai.Blob{Data: raw, MIMEType: "image/png"}},
					},
				},
			},
		},
	})

	if resp.Text != "caption" {
		t.Fatalf("expected text caption, got %q", resp.Text)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected one text choice, got %d", len(resp.Choices))
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected one image, got %d", len(resp.Data))
	}
	if resp.Data[0].B64JSON != base64.StdEncoding.EncodeToString(raw) {
		t.Fatalf("unexpected image payload: %q", resp.Data[0].B64JSON)
	}
}
