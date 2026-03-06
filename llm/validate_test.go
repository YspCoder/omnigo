package llm

import (
	"testing"

	"github.com/YspCoder/omnigo/config"
)

func TestValidate_AllowsAKSKWithoutAPIKey(t *testing.T) {
	cfg := &config.Config{
		Provider:  "ark",
		Model:     "doubao-seed-1-6-250615",
		APIKeys:   map[string]string{},
		AccessKey: "test-ak",
		SecretKey: "test-sk",
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected validation to pass with ak/sk, got error: %v", err)
	}
}
