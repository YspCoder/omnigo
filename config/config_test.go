package config

import "testing"

func TestLoadAPIKeysNormalizesProviderKey(t *testing.T) {
	t.Setenv("UNITTEST_API_KEY", "secret")

	cfg := &Config{
		Provider: "UNITTEST",
		APIKeys:  make(map[string]string),
	}

	loadAPIKeys(cfg)

	if cfg.APIKeys["unittest"] != "secret" {
		t.Fatalf("expected lower-case provider key to be set, got %q", cfg.APIKeys["unittest"])
	}

	if cfg.APIKeys["UNITTEST"] != "secret" {
		t.Fatalf("expected provider key to be set with original casing, got %q", cfg.APIKeys["UNITTEST"])
	}
}

func TestSetProviderNormalizesValue(t *testing.T) {
	cfg := &Config{}

	SetProvider("OpenAI")(cfg)

	if cfg.Provider != "openai" {
		t.Fatalf("expected provider to be normalized to lower case, got %q", cfg.Provider)
	}
}

func TestLoadConfigNormalizesProvider(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "OpenAI")
	t.Setenv("UNITTEST_API_KEY", "secret")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected LoadConfig to succeed, got error: %v", err)
	}

	if cfg.Provider != "openai" {
		t.Fatalf("expected provider to be normalized to lower case, got %q", cfg.Provider)
	}
}
