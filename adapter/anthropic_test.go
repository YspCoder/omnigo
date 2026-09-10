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
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": "ok"}},
			"usage":   map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer server.Close()

	_, err := (&AnthropicAdaptor{}).Chat(context.Background(), &ProviderConfig{
		APIKey:  "test-api-key",
		BaseURL: "  " + server.URL + "/proxy/v1/  ",
	}, &dto.MediaRequest{
		Model:     "claude-3-haiku",
		Messages:  []dto.Message{{Role: "user", Content: "hello"}},
		MaxTokens: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := <-requests; got != "/proxy/v1/messages" {
		t.Fatalf("request path = %q, want %q", got, "/proxy/v1/messages")
	}
}
