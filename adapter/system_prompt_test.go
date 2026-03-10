package adapter

import (
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestMediaPromptWithSystem(t *testing.T) {
	request := &dto.MediaRequest{
		Messages: []dto.Message{
			{Role: "system", Content: "be concise"},
			{Role: "user", Content: "draw a cat"},
		},
	}

	got := mediaPromptWithSystem(request)
	want := "be concise\n\ndraw a cat"
	if got != want {
		t.Fatalf("expected combined prompt %q, got %q", want, got)
	}
}

func TestGoogleToGenCfgUsesSystemInstruction(t *testing.T) {
	adaptor := &GoogleAdaptor{}
	cfg := adaptor.toGenCfg(&dto.MediaRequest{
		Messages: []dto.Message{
			{Role: "system", Content: "speak formally"},
			{Role: "user", Content: "hello"},
		},
	})

	if cfg.SystemInstruction == nil {
		t.Fatal("expected system instruction to be set")
	}
	if cfg.SystemInstruction.Role != "system" {
		t.Fatalf("expected system role, got %q", cfg.SystemInstruction.Role)
	}
	if len(cfg.SystemInstruction.Parts) == 0 || cfg.SystemInstruction.Parts[0].Text != "speak formally" {
		t.Fatalf("expected system instruction text, got %#v", cfg.SystemInstruction.Parts)
	}
}
