package adapter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestOpenAIImagePreservesUsage(t *testing.T) {
	const body = `{"created":1,"data":[{"b64_json":"aGVsbG8="}],"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30,"input_tokens_details":{"text_tokens":6,"image_tokens":4},"output_tokens_details":{"text_tokens":1,"image_tokens":19}}}`
	calls := 0
	response, err := (&OpenAIAdaptor{}).Media(context.Background(), streamTestConfig(body, "application/json", &calls), &dto.MediaRequest{
		Type: dto.MediaTypeImage, Model: "gpt-image-1", Prompt: "test image",
	})
	if err != nil {
		t.Fatal(err)
	}
	usage := response.Usage
	if usage.PromptTokens != 10 || usage.CompletionTokens != 20 || usage.TotalTokens != 30 {
		t.Fatalf("image usage = %+v", usage)
	}
	if string(usage.PromptTokensDetails["image_tokens"]) != "4" || string(usage.CompletionTokensDetails["image_tokens"]) != "19" {
		t.Fatalf("image token details = %+v", usage.UsageDetails)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(usage.RawUsage, &raw); err != nil || len(raw["input_tokens_details"]) == 0 || len(raw["output_tokens_details"]) == 0 {
		t.Fatalf("native image usage = %s, error = %v", usage.RawUsage, err)
	}
}

func TestOpenAIResponsesReportsTokenLimit(t *testing.T) {
	const body = `{"id":"resp_test","model":"test-model","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial"}]}],"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`
	calls := 0
	config := streamTestConfig(body, "application/json", &calls)
	config.ChatProtocol = "responses"
	response, err := (&OpenAIAdaptor{}).Chat(context.Background(), config, &dto.MediaRequest{
		Model: "test-model", MaxTokens: 20, Messages: []dto.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != "incomplete" || len(response.Choices) != 1 || response.Choices[0].FinishReason != "length" || response.Text != "partial" {
		t.Fatalf("incomplete response = %+v", response)
	}
}
