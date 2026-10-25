package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func nativeToolRequest() *dto.MediaRequest {
	return &dto.MediaRequest{Model: "test-model", Tools: []dto.Tool{{Type: "function", Function: dto.ToolFunction{Name: "weather", Parameters: map[string]interface{}{"type": "object"}}}}, ToolChoice: map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "weather"}}, Messages: []dto.Message{
		{Role: "assistant", ToolCalls: []dto.ToolCall{{ID: "old-call", Type: "function", Function: dto.ToolCallFunction{Name: "weather", Arguments: json.RawMessage(`{"city":"Paris"}`)}, Metadata: map[string]interface{}{"thought_signature": "c2ln"}}}},
		{Role: "tool", ToolCallID: "old-call", Content: map[string]interface{}{"temperature": 20}},
		{Role: "user", Content: "And Rome?"},
	}}
}

func TestNativeFunctionHistoryAndResponses(t *testing.T) {
	for _, tc := range []struct {
		name     string
		adaptor  Adaptor
		response string
	}{
		{"google", &GoogleAdaptor{}, `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"new-call","name":"weather","args":{"city":"Rome"}},"thoughtSignature":"c2ln"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5}}`},
		{"ark", &ArkAdaptor{}, `{"id":"r1","status":"completed","output":[{"type":"function_call","call_id":"new-call","name":"weather","arguments":"{\"city\":\"Rome\"}"}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]interface{}
			cfg := &ProviderConfig{APIKey: "test-key", BaseURL: "https://example.invalid", HTTPClient: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(tc.response)), Request: r}, nil
			})}}
			response, err := tc.adaptor.Chat(context.Background(), cfg, nativeToolRequest())
			if err != nil {
				t.Fatal(err)
			}
			if len(response.Choices) != 1 || len(response.Choices[0].Message.ToolCalls) != 1 || response.Usage.TotalTokens != 5 {
				t.Fatalf("response=%+v", response)
			}
			call := response.Choices[0].Message.ToolCalls[0]
			if call.ID != "new-call" || call.Function.Name != "weather" || string(call.Function.Arguments) != `{"city":"Rome"}` {
				t.Fatalf("call=%+v", call)
			}
			if tc.name == "google" {
				contents := body["contents"].([]interface{})
				modelPart := contents[0].(map[string]interface{})["parts"].([]interface{})[0].(map[string]interface{})
				result := contents[1].(map[string]interface{})["parts"].([]interface{})[0].(map[string]interface{})["functionResponse"].(map[string]interface{})
				choice := body["toolConfig"].(map[string]interface{})["functionCallingConfig"].(map[string]interface{})
				if modelPart["thoughtSignature"] != "c2ln" || result["id"] != "old-call" || result["name"] != "weather" || choice["mode"] != "ANY" || call.Metadata["thought_signature"] != "c2ln" {
					t.Fatalf("request=%v call=%+v", body, call)
				}
			} else {
				input := body["input"].([]interface{})
				result := input[1].(map[string]interface{})
				choice := body["tool_choice"].(map[string]interface{})
				if result["type"] != "function_call_output" || result["call_id"] != "old-call" || result["output"] != `{"temperature":20}` || choice["name"] != "weather" {
					t.Fatalf("request=%v", body)
				}
			}
		})
	}
}

func TestNativeFunctionStreamArguments(t *testing.T) {
	for _, tc := range []struct {
		name    string
		adaptor Adaptor
		events  []string
	}{
		{"google", &GoogleAdaptor{}, []string{
			`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call-1","name":"weather","args":{"city":"Rome"}},"thoughtSignature":"c2ln"}]}}]}`,
			`{"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5}}`,
		}},
		{"ark", &ArkAdaptor{}, []string{
			`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"call-1","name":"weather","arguments":""}}`,
			`{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"city\":"}`,
			`{"type":"response.function_call_arguments.delta","output_index":0,"delta":"\"Rome\"}"}`,
			`{"type":"response.function_call_arguments.done","output_index":0,"arguments":"{\"city\":\"Rome\"}"}`,
			`{"type":"response.completed","response":{"status":"completed","output":[{"type":"function_call","call_id":"call-1","name":"weather","arguments":"{\"city\":\"Rome\"}"}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events strings.Builder
			for _, event := range tc.events {
				events.WriteString("data: " + event + "\n\n")
			}
			calls := 0
			stream, err := tc.adaptor.Stream(context.Background(), streamTestConfig(events.String(), "text/event-stream", &calls), nativeToolRequest())
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			var args, name, id string
			usage := 0
			for {
				token, err := stream.Next(context.Background())
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				for _, call := range token.ToolCalls {
					args += call.Function.Arguments
					name += call.Function.Name
					id += call.ID
				}
				if token.Usage != nil {
					usage = token.Usage.TotalTokens
				}
			}
			if args != `{"city":"Rome"}` || name != "weather" || id != "call-1" || usage != 5 {
				t.Fatalf("id=%q name=%q args=%q usage=%d", id, name, args, usage)
			}
		})
	}
}
