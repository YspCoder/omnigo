package llm

import (
	"testing"

	"github.com/YspCoder/omnigo/config"
)

func TestValidate_AllowsAKSKWithoutAPIKey(t *testing.T) {
	cfg := &config.Config{
		Provider:  "jimeng",
		Model:     "jimeng_ti2v_v30_pro",
		APIKeys:   map[string]string{},
		AccessKey: "test-ak",
		SecretKey: "test-sk",
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected validation to pass with ak/sk, got error: %v", err)
	}
}
