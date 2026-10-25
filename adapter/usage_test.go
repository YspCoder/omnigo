package adapter

import (
	"encoding/json"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestUsagePreservesPresenceAliasesAndBilling(t *testing.T) {
	for _, tc := range []struct {
		name, raw            string
		present              bool
		input, output, total int
	}{
		{"absent", "null", false, 0, 0, 0},
		{"empty wrapper", "{}", false, 0, 0, 0},
		{"explicit zero", `{"input_tokens":0,"output_tokens":0,"total_tokens":0}`, true, 0, 0, 0},
		{"aliases", `{"input_tokens":29,"output_tokens":477,"total_tokens":506}`, true, 29, 477, 506},
		{"canonical zero wins", `{"prompt_tokens":0,"input_tokens":29,"completion_tokens":3}`, true, 0, 3, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := usageFromJSON(json.RawMessage(tc.raw))
			if usage.HasUsage() != tc.present || usage.PromptTokens != tc.input || usage.CompletionTokens != tc.output || usage.TotalTokens != tc.total {
				t.Fatalf("usage = %+v", usage)
			}
			raw, err := json.Marshal(dto.MediaResponse{Usage: usage})
			if err != nil {
				t.Fatal(err)
			}
			var result map[string]json.RawMessage
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if _, ok := result["usage"]; ok != tc.present {
				t.Fatalf("presence lost: %s", raw)
			}
			var decoded dto.MediaResponse
			if err := json.Unmarshal([]byte(`{"usage":`+tc.raw+`}`), &decoded); err != nil || decoded.Usage.HasUsage() != tc.present {
				t.Fatalf("JSON usage presence = %v, error = %v", decoded.Usage.HasUsage(), err)
			}
		})
	}
	usage := taskUsage(usageFromJSON(json.RawMessage(`{"duration":10.5,"video_duration":0,"credits":0,"final_unit_deduction":"0.25"}`)))
	if usage == nil || usage.VideoDuration != 10 || string(usage.Billing["duration"]) != "10.5" || string(streamUsage(usage).Billing["credits"]) != "0" {
		t.Fatalf("billing lost: %+v", usage)
	}
	if !usageFromValue(struct{}{}).HasUsage() {
		t.Fatal("SDK usage object lost")
	}
}
