package llm

import (
	"strings"
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

func TestValidateAgainstSchemaEnforcesGeneratedConstraints(t *testing.T) {
	type payload struct {
		Name  string   `json:"name" validate:"required,min=2,max=4"`
		Count int      `json:"count" validate:"min=1,max=3"`
		Tags  []string `json:"tags" validate:"len=2,unique=true"`
	}

	schema, err := GenerateJSONSchema(payload{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAgainstSchema(`{"name":"go","count":2,"tags":["a","b"]}`, schema); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	for _, response := range []string{
		`{"count":2,"tags":["a","b"]}`,                 // required name
		`{"name":"x","count":2,"tags":["a","b"]}`,      // min length
		`{"name":"golang","count":2,"tags":["a","b"]}`, // max length
		`{"name":"go","count":4,"tags":["a","b"]}`,     // maximum
		`{"name":"go","count":2,"tags":["a","a"]}`,     // unique items
	} {
		if err := ValidateAgainstSchema(response, schema); err == nil {
			t.Errorf("expected invalid payload to be rejected: %s", response)
		}
	}
}

func TestValidateAgainstSchemaAcceptsDirectMapAndRejectsFractionalInteger(t *testing.T) {
	schema := map[string]interface{}{
		"type":     "object",
		"required": []string{"status"},
		"properties": map[string]interface{}{
			"status": map[string]interface{}{"type": "string", "enum": []string{"ok", "done"}},
			"count":  map[string]interface{}{"type": "integer"},
		},
	}
	if err := ValidateAgainstSchema(`{"status":"ok","count":2}`, schema); err != nil {
		t.Fatalf("valid direct schema rejected: %v", err)
	}
	for _, response := range []string{`{"count":2}`, `{"status":"unknown"}`, `{"status":"ok","count":1.5}`} {
		if err := ValidateAgainstSchema(response, schema); err == nil {
			t.Errorf("expected invalid response to be rejected: %s", response)
		}
	}
}

func TestValidateAgainstSchemaEnforcesPattern(t *testing.T) {
	schema := `{"type":"string","pattern":"^[a-z]+$"}`
	if err := ValidateAgainstSchema(`"hello"`, schema); err != nil {
		t.Fatalf("valid pattern rejected: %v", err)
	}
	if err := ValidateAgainstSchema(`"Hello"`, schema); err == nil || !strings.Contains(err.Error(), "pattern") {
		t.Fatalf("invalid pattern response error = %v", err)
	}
}
