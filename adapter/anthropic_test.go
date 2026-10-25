package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
		APIKey:     "test-api-key",
		BaseURL:    "  " + server.URL + "/proxy/v1/  ",
		Headers:    map[string]string{"X-Api-Key": "override-key", "X-Tenant-ID": "tenant"},
		HTTPClient: server.Client(),
	}
	if err := PrepareProviderConfig(config); err != nil {
		t.Fatal(err)
	}
	sharedClient := config.HTTPClient
	sharedTransport := sharedClient.Transport
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
		if config.HTTPClient != sharedClient || sharedClient.Transport != sharedTransport {
			t.Fatal("Anthropic header wrapping modified the shared HTTP client")
		}
		if got := <-requests; got != "/proxy/v1/messages" {
			t.Fatalf("request path = %q, want %q", got, "/proxy/v1/messages")
		}
		if response.Text != want || response.ID != "msg_test" || response.Model != model || response.Choices[0].FinishReason != "end_turn" {
			t.Fatalf("response = %+v", response)
		}
	}
}

func TestAnthropicToolsAndMultimodalRoundTrip(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"msg_tools","role":"assistant","model":"test","content":[{"type":"thinking","thinking":"reason","signature":"signed"},{"type":"text","text":"Checking"},{"type":"tool_use","id":"call_1","name":"weather","input":{"city":"Paris"}}],"stop_reason":"tool_use","usage":{"input_tokens":2,"output_tokens":3}}`)
	}))
	defer server.Close()
	config := &ProviderConfig{BaseURL: server.URL, HTTPClient: server.Client()}
	request := &dto.MediaRequest{Model: "test", MaxTokens: 64,
		Options: map[string]any{"temperature": 0.7, "top_p": 0.9},
		Messages: []dto.Message{{Role: "user", Content: []map[string]any{
			{"type": "text", "text": "Describe"},
			{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,aGVsbG8="}},
		}, FileURL: "https://example.com/test.pdf"}},
		Tools:      []dto.Tool{{Type: "function", Function: dto.ToolFunction{Name: "weather", Parameters: map[string]any{"type": "object"}}}},
		ToolChoice: map[string]any{"type": "function", "function": map[string]any{"name": "weather"}},
	}
	adaptor := &AnthropicAdaptor{}
	response, err := adaptor.Chat(context.Background(), config, request)
	if err != nil {
		t.Fatal(err)
	}
	message := response.Choices[0].Message
	if response.Text != "Checking" || len(message.ToolCalls) != 1 || string(message.ToolCalls[0].Function.Arguments) != `{"city":"Paris"}` {
		t.Fatalf("tool response = %+v", response)
	}
	request.Messages = append(request.Messages, message, dto.Message{Role: "tool", ToolCallID: "call_1", Content: "sunny"})
	if _, err := adaptor.Chat(context.Background(), config, request); err != nil {
		t.Fatal(err)
	}
	body := bodies[1]
	if _, exists := body["top_p"]; exists {
		t.Fatal("both temperature and top_p were sent to Claude")
	}
	if choice := body["tool_choice"].(map[string]any); choice["type"] != "tool" || choice["name"] != "weather" {
		t.Fatalf("tool choice = %v", choice)
	}
	messages := body["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	image := content[1].(map[string]any)["source"].(map[string]any)
	document := content[2].(map[string]any)["source"].(map[string]any)
	if image["type"] != "base64" || image["media_type"] != "image/png" || document["url"] != "https://example.com/test.pdf" {
		t.Fatalf("multimodal content = %v", content)
	}
	assistant := messages[1].(map[string]any)["content"].([]any)
	if len(assistant) != 3 || assistant[0].(map[string]any)["signature"] != "signed" || assistant[2].(map[string]any)["id"] != "call_1" {
		t.Fatalf("assistant round trip = %v", assistant)
	}
	result := messages[2].(map[string]any)
	if result["role"] != "user" || result["content"].([]any)[0].(map[string]any)["tool_use_id"] != "call_1" {
		t.Fatalf("tool result = %v", result)
	}
}

func TestAnthropicStreamLifecycle(t *testing.T) {
	const start = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"usage\":{\"input_tokens\":2,\"cache_read_input_tokens\":5,\"cache_creation_input_tokens\":3,\"output_tokens\":0}}}\n\n"
	const text = "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n"
	const tool = "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"call_1\",\"name\":\"lookup\",\"input\":{}}}\n\n"
	const arguments = "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"x\\\":1}\"}}\n\n"
	const toolStop = "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n"
	const finish = "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":4,\"server_tool_use\":{\"web_search_requests\":1}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	for _, tt := range []struct {
		name, body, arguments string
		wantError             bool
	}{
		{"tools", start + text + tool + arguments + toolStop + finish, `{"x":1}`, false},
		{"empty arguments", start + text + tool + toolStop + finish, `{}`, false},
		{"truncated", start + text, "", true},
		{"provider error", start + "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"busy\"}}\n\n", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			stream, err := (&AnthropicAdaptor{}).Stream(context.Background(), streamTestConfig(tt.body, "text/event-stream", &calls), &dto.MediaRequest{Model: "test", MaxTokens: 16, Prompt: "hello"})
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			var args, output, reason string
			var usage *dto.Usage
			for {
				token, nextErr := stream.Next(context.Background())
				if nextErr != nil {
					err = nextErr
					break
				}
				output += token.Text
				for _, call := range token.ToolCalls {
					if call.Index != 1 {
						t.Fatalf("tool index = %d", call.Index)
					}
					args += call.Function.Arguments
				}
				if token.Usage != nil {
					usage = token.Usage
				}
				if token.FinishReason != "" {
					reason = token.FinishReason
				}
			}
			if tt.wantError == errors.Is(err, io.EOF) {
				t.Fatalf("terminal error = %v", err)
			}
			if usage == nil || usage.PromptTokens != 10 || args != tt.arguments {
				t.Fatalf("usage = %+v; arguments = %s", usage, args)
			}
			if !tt.wantError && (usage.TotalTokens != 14 || reason != "tool_use" || output != "hello" || !strings.Contains(string(usage.RawUsage), "web_search_requests")) {
				t.Fatalf("usage = %+v, reason = %s, output = %s", usage, reason, output)
			}
			if _, err := stream.Next(context.Background()); !errors.Is(err, io.EOF) {
				t.Fatalf("after terminal = %v", err)
			}
		})
	}
	t.Run("close interrupts read", func(t *testing.T) {
		cancelled := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, start)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			close(cancelled)
		}))
		defer server.Close()
		stream, err := (&AnthropicAdaptor{}).Stream(context.Background(), &ProviderConfig{BaseURL: server.URL, HTTPClient: server.Client()}, &dto.MediaRequest{Model: "test", MaxTokens: 8, Prompt: "hello"})
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		if _, err := stream.Next(context.Background()); err != nil {
			t.Fatal(err)
		}
		read := make(chan error, 1)
		go func() { _, err := stream.Next(context.Background()); read <- err }()
		if err := stream.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-cancelled:
		case <-time.After(2 * time.Second):
			t.Fatal("Close did not cancel the HTTP request")
		}
		select {
		case <-read:
		case <-time.After(2 * time.Second):
			t.Fatal("Close did not release Next")
		}
	})
}

func TestAnthropicUsageIncludesCacheTokens(t *testing.T) {
	const body = `{"id":"msg_test","type":"message","role":"assistant","model":"test-model","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":50,"cache_read_input_tokens":100000,"cache_creation_input_tokens":200,"cache_creation":{"ephemeral_5m_input_tokens":200,"ephemeral_1h_input_tokens":0},"output_tokens":5}}`
	calls := 0
	response, err := (&AnthropicAdaptor{}).Chat(context.Background(), streamTestConfig(body, "application/json", &calls), &dto.MediaRequest{
		Model: "test-model", MaxTokens: 8, Messages: []dto.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	usage := response.Usage
	if usage.PromptTokens != 100250 || usage.CompletionTokens != 5 || usage.TotalTokens != 100255 {
		t.Fatalf("cached usage = %+v", usage)
	}
	if string(usage.PromptTokensDetails["cached_tokens"]) != "100000" || string(usage.PromptTokensDetails["cache_creation_tokens"]) != "200" {
		t.Fatalf("cache token details = %+v", usage.UsageDetails)
	}
}
