package adapter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestJimengConvertMediaRequest_NoImageExtraDoesNotPanic(t *testing.T) {
	adp := &JimengAdaptor{}
	cfg := &ProviderConfig{Model: "jimeng_ti2v_v30_pro"}
	req := &dto.MediaRequest{
		Prompt: "a cat running",
		Extra:  nil,
	}

	body, err := adp.ConvertMediaRequest(context.Background(), cfg, ModeVideo, req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var payload JimengSubmitTaskRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if len(payload.ImageURLs) != 0 {
		t.Fatalf("expected no image_urls when not provided, got: %+v", payload.ImageURLs)
	}
}

func TestJimengConvertMediaRequest_ImageURLsFromInterfaceSlice(t *testing.T) {
	adp := &JimengAdaptor{}
	cfg := &ProviderConfig{Model: "jimeng_ti2v_v30_pro"}
	req := &dto.MediaRequest{
		Prompt: "a cat running",
		Extra: map[string]interface{}{
			"image_urls": []interface{}{"https://example.com/a.png", "https://example.com/b.png"},
		},
	}

	body, err := adp.ConvertMediaRequest(context.Background(), cfg, ModeVideo, req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var payload JimengSubmitTaskRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if len(payload.ImageURLs) != 2 {
		t.Fatalf("expected 2 image_urls, got: %+v", payload.ImageURLs)
	}
}

func TestJimengConvertMediaRequest_BinaryDataFromInterfaceSlice(t *testing.T) {
	adp := &JimengAdaptor{}
	cfg := &ProviderConfig{Model: "jimeng_ti2v_v30_pro"}
	req := &dto.MediaRequest{
		Prompt: "a cat running",
		Extra: map[string]interface{}{
			"binary_data_base64": []interface{}{"base64a", "base64b"},
		},
	}

	body, err := adp.ConvertMediaRequest(context.Background(), cfg, ModeVideo, req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var payload JimengSubmitTaskRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if len(payload.BinaryDataBase64) != 2 {
		t.Fatalf("expected 2 binary_data_base64 entries, got: %+v", payload.BinaryDataBase64)
	}
}
