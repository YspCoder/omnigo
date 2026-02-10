package adapter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestGoogleConvertMediaRequest_VideoPayloadShape(t *testing.T) {
	adp := &GoogleAdaptor{}
	cfg := &ProviderConfig{Model: "veo"}
	req := &dto.MediaRequest{
		Prompt:   "a cat running",
		N:        1,
		Size:     "16:9",
		Duration: 8,
	}

	body, err := adp.ConvertMediaRequest(context.Background(), cfg, ModeVideo, req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	parameters, ok := payload["parameters"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parameters map, got: %#v", payload["parameters"])
	}
	if _, ok := parameters["aspectRatio"]; !ok {
		t.Fatalf("expected aspectRatio in parameters, got: %#v", parameters)
	}
	if _, ok := parameters["durationSeconds"]; !ok {
		t.Fatalf("expected durationSeconds in parameters, got: %#v", parameters)
	}
}
