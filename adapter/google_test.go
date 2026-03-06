package adapter

import (
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestGoogleToVidCfg_VideoPayloadShape(t *testing.T) {
	adp := &GoogleAdaptor{}
	req := &dto.MediaRequest{
		Prompt:   "a cat running",
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
