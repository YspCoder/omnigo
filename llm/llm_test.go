package llm

import (
	"testing"

	"github.com/YspCoder/omnigo/adapter"
	"github.com/YspCoder/omnigo/config"
	"github.com/YspCoder/omnigo/utils"
)

func TestNewLLM_AllowsAKSKWithoutAPIKey(t *testing.T) {
	cfg := &config.Config{
		Provider:  "jimeng",
		Model:     "jimeng_ti2v_v30_pro",
		APIKeys:   map[string]string{},
		AccessKey: "test-ak",
		SecretKey: "test-sk",
	}

	logger := utils.NewLogger(utils.LogLevelOff)
	registry := adapter.NewRegistry("jimeng")

	_, err := NewLLM(cfg, logger, registry)
	if err != nil {
		t.Fatalf("expected NewLLM success with ak/sk, got error: %v", err)
	}
}
