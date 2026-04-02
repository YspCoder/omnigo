package adapter

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
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
		{model: "gemini-2.5-flash", want: false},
		{model: "imagen-3.0-generate-001", want: false},
	}

	for _, tt := range tests {
		if got := isGoogleGenerateContentImageModel(tt.model); got != tt.want {
			t.Fatalf("isGoogleGenerateContentImageModel(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestGoogleToContentsIncludesExtraImages(t *testing.T) {
	adp := &GoogleAdaptor{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png-a"))
		case "/b.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpg-b"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	req := &dto.MediaRequest{
		Messages: []dto.Message{{Role: "user", Content: "edit this image"}},
		Extra: map[string]interface{}{
			"image":  server.URL + "/a.png",
			"images": []string{server.URL + "/b.jpg"},
		},
	}

	contents, err := adp.toContents(context.Background(), nil, req)
	if err != nil {
		t.Fatalf("toContents() error = %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("expected one content, got %d", len(contents))
	}
	if len(contents[0].Parts) != 3 {
		t.Fatalf("expected one text part and two image parts, got %d", len(contents[0].Parts))
	}
	if contents[0].Parts[1].InlineData == nil || string(contents[0].Parts[1].InlineData.Data) != "png-a" {
		t.Fatalf("unexpected first image part: %#v", contents[0].Parts[1])
	}
	if contents[0].Parts[2].InlineData == nil || string(contents[0].Parts[2].InlineData.Data) != "jpg-b" {
		t.Fatalf("unexpected second image part: %#v", contents[0].Parts[2])
	}
	if contents[0].Parts[2].InlineData.MIMEType != "image/jpeg" {
		t.Fatalf("unexpected mime type: %q", contents[0].Parts[2].InlineData.MIMEType)
	}
}

func TestGoogleToContentsOnlyAppendsRequestLevelImagesToLastUserMessage(t *testing.T) {
	adp := &GoogleAdaptor{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	defer server.Close()

	req := &dto.MediaRequest{
		Messages: []dto.Message{
			{Role: "user", Content: "first", ImageURL: server.URL + "/first.png"},
			{Role: "assistant", Content: "noted"},
			{Role: "user", Content: "second", ImageURL: server.URL + "/second.jpg"},
		},
		Extra: map[string]interface{}{
			"image":  server.URL + "/extra.png",
			"images": []string{server.URL + "/final.webp"},
		},
	}

	contents, err := adp.toContents(context.Background(), nil, req)
	if err != nil {
		t.Fatalf("toContents() error = %v", err)
	}
	if len(contents) != 3 {
		t.Fatalf("expected three contents, got %d", len(contents))
	}
	if len(contents[0].Parts) != 2 {
		t.Fatalf("expected first user message to keep only its own image, got %d parts", len(contents[0].Parts))
	}
	if got := string(contents[0].Parts[1].InlineData.Data); got != "/first.png" {
		t.Fatalf("unexpected first user image payload: %q", got)
	}
	if len(contents[2].Parts) != 4 {
		t.Fatalf("expected last user message to include text, own image, and request-level images, got %d parts", len(contents[2].Parts))
	}
	if got := string(contents[2].Parts[1].InlineData.Data); got != "/second.jpg" {
		t.Fatalf("unexpected second user image payload: %q", got)
	}
	if got := string(contents[2].Parts[2].InlineData.Data); got != "/extra.png" {
		t.Fatalf("unexpected extra image payload: %q", got)
	}
	if got := string(contents[2].Parts[3].InlineData.Data); got != "/final.webp" {
		t.Fatalf("unexpected final image payload: %q", got)
	}
}

func TestGoogleToMediaPromptContentsIncludesPromptAndImages(t *testing.T) {
	adp := &GoogleAdaptor{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/source.jpeg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("source"))
		case "/reference.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("reference"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	req := &dto.MediaRequest{
		Messages: []dto.Message{
			{Role: "system", Content: "preserve composition"},
			{Role: "user", Content: "turn this into cyberpunk", ImageURL: server.URL + "/source.jpeg"},
		},
		Extra: map[string]interface{}{
			"images": []string{server.URL + "/reference.png"},
		},
	}

	contents, err := adp.toMediaPromptContents(context.Background(), nil, req)
	if err != nil {
		t.Fatalf("toMediaPromptContents() error = %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("expected one content, got %d", len(contents))
	}
	if len(contents[0].Parts) != 3 {
		t.Fatalf("expected prompt text plus two image parts, got %d", len(contents[0].Parts))
	}
	if got := contents[0].Parts[0].Text; got != "preserve composition\n\nturn this into cyberpunk" {
		t.Fatalf("unexpected prompt text: %q", got)
	}
	if got := string(contents[0].Parts[1].InlineData.Data); got != "source" {
		t.Fatalf("unexpected source image payload: %q", got)
	}
	if got := string(contents[0].Parts[2].InlineData.Data); got != "reference" {
		t.Fatalf("unexpected reference image payload: %q", got)
	}
	if got := contents[0].Parts[1].InlineData.MIMEType; got != "image/jpeg" {
		t.Fatalf("unexpected source image mime: %q", got)
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

func TestGoogleVideoPayloadUsesInlineBytes(t *testing.T) {
	adp := &GoogleAdaptor{}
	raw := []byte("video-bytes")
	url, b64JSON, mimeType := adp.googleVideoPayload(nil, nil, &genai.GenerateVideosResponse{
		GeneratedVideos: []*genai.GeneratedVideo{
			{
				Video: &genai.Video{
					URI:        "https://example.com/video.mp4",
					VideoBytes: raw,
					MIMEType:   "video/mp4",
				},
			},
		},
	})

	if url != "https://example.com/video.mp4" {
		t.Fatalf("unexpected url: %q", url)
	}
	if b64JSON != base64.StdEncoding.EncodeToString(raw) {
		t.Fatalf("unexpected base64 payload: %q", b64JSON)
	}
	if mimeType != "video/mp4" {
		t.Fatalf("unexpected mime type: %q", mimeType)
	}
}

func TestGoogleToVidRespUsesVideoBase64(t *testing.T) {
	adp := &GoogleAdaptor{}
	raw := []byte("video-bytes")
	resp := adp.toVidResp(&genai.GenerateVideosOperation{
		Name: "op-1",
		Done: true,
		Response: &genai.GenerateVideosResponse{
			GeneratedVideos: []*genai.GeneratedVideo{
				{
					Video: &genai.Video{
						URI:        "https://example.com/video.mp4",
						VideoBytes: raw,
						MIMEType:   "video/mp4",
					},
				},
			},
		},
	})

	if resp.Status != "SUCCEEDED" {
		t.Fatalf("unexpected status: %q", resp.Status)
	}
	if resp.Video.URL != "https://example.com/video.mp4" {
		t.Fatalf("unexpected video url: %q", resp.Video.URL)
	}
	if resp.Video.B64JSON != base64.StdEncoding.EncodeToString(raw) {
		t.Fatalf("unexpected video base64 payload: %q", resp.Video.B64JSON)
	}
	if resp.Video.MIMEType != "video/mp4" {
		t.Fatalf("unexpected video mime type: %q", resp.Video.MIMEType)
	}
}
