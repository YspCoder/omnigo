package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestAnthropicClientUsesConfiguredBaseURL(t *testing.T) {
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		if r.Header.Get("X-Api-Key") != "override-key" || r.Header.Get("X-Tenant-ID") != "tenant" {
			t.Errorf("headers = %v", r.Header)
		}
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request["system"] != "first\n\nsecond" {
			t.Errorf("system = %v", request["system"])
		}
		content := []map[string]string{{"type": "text", "text": "hello "}, {"type": "text", "text": "world"}}
		if request["model"] == "empty" {
			content = nil
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "msg_test", "model": request["model"], "stop_reason": "end_turn",
			"content": content,
			"usage":   map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer server.Close()

	config := &ProviderConfig{
		APIKey:  "test-api-key",
		BaseURL: "  " + server.URL + "/proxy/v1/  ",
		Headers: map[string]string{"X-Api-Key": "override-key", "X-Tenant-ID": "tenant"},
	}
	for model, want := range map[string]string{"claude-3-haiku": "hello world", "empty": ""} {
		response, err := (&AnthropicAdaptor{}).Chat(context.Background(), config, &dto.MediaRequest{
			Model: model,
			Messages: []dto.Message{
				{Role: "system", Content: "first"},
				{Role: "system", Content: "second"},
				{Role: "user", Content: "hello"},
			},
			MaxTokens: 16,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := <-requests; got != "/proxy/v1/messages" {
			t.Fatalf("request path = %q, want %q", got, "/proxy/v1/messages")
		}
		if response.Text != want || response.ID != "msg_test" || response.Model != model || response.Choices[0].FinishReason != "end_turn" {
			t.Fatalf("response = %+v", response)
		}
	}
}
