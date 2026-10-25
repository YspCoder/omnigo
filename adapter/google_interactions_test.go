package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/YspCoder/omnigo/dto"
)

func TestGoogleInteractionsCustomTransportCancellation(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	cfg := &ProviderConfig{ChatProtocol: "interactions", APIKey: "test", BaseURL: "https://example.invalid", HTTPClient: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader, Request: r}, nil
	})}}
	stream, err := (&GoogleAdaptor{}).Stream(context.Background(), cfg, nativeToolRequest())
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := stream.Next(ctx); result <- err }()
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Next error=%v", err)
		}
	case <-time.After(time.Second):
		reader.Close()
		t.Fatal("Next did not interrupt a custom transport body")
	}
	if token, err := stream.Next(context.Background()); token != nil || !errors.Is(err, io.EOF) {
		t.Fatalf("closed stream token=%+v error=%v", token, err)
	}
}

func TestGoogleInteractionsRequestsAndPolling(t *testing.T) {
	const complete = `{"id":"int-1","status":"completed","steps":[{"type":"thought","signature":"opaque"},{"type":"model_output","content":[{"type":"text","text":"done"},{"type":"image","data":"aW1hZ2U=","mime_type":"image/png"}]}],"usage":{"total_input_tokens":7,"total_output_tokens":20,"total_thought_tokens":22,"total_tokens":49}}`
	var bodies []map[string]interface{}
	var paths []string
	cfg := &ProviderConfig{APIKey: "test-key", BaseURL: "https://example.invalid", Headers: map[string]string{"X-Tenant": "test"}, HTTPClient: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Header.Get("X-Goog-Api-Key") != "test-key" || r.Header.Get("X-Tenant") != "test" {
			t.Errorf("headers=%v", r.Header)
		}
		body := complete
		if r.Method == http.MethodPost {
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			bodies = append(bodies, payload)
			if payload["background"] == true {
				body = `{"id":"int-1","status":"in_progress"}`
			}
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}}
	a := &GoogleAdaptor{}
	r := nativeToolRequest()
	r.Type, r.Size, r.Resolution = dto.MediaTypeImage, "16:9", "1K"
	r.Options = map[string]interface{}{"api": "interactions"}
	response, err := a.Media(context.Background(), cfg, r)
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "done" || response.Usage.PromptTokens != 7 || response.Usage.CompletionTokens != 42 || response.Usage.TotalTokens != 49 || len(response.Data) != 1 || response.Data[0].MIMEType != "image/png" {
		t.Fatalf("response=%+v", response)
	}
	input := bodies[0]["input"].([]interface{})
	result := input[1].(map[string]interface{})
	choice := bodies[0]["generation_config"].(map[string]interface{})["tool_choice"].(map[string]interface{})["allowed_tools"].(map[string]interface{})
	if result["call_id"] != "old-call" || result["name"] != "weather" || choice["mode"] != "any" || bodies[0]["response_format"] == nil {
		t.Fatalf("payload=%v", bodies[0])
	}
	r.Extra = map[string]interface{}{"input": response.Metadata["steps"], "background": true, "api_version": "v1beta2"}
	background, err := a.Media(context.Background(), cfg, r)
	if err != nil || background.TaskID != "interactions/v1beta2/int-1" {
		t.Fatalf("background=%+v error=%v", background, err)
	}
	replayed := bodies[1]["input"].([]interface{})
	if replayed[0].(map[string]interface{})["signature"] != "opaque" {
		t.Fatalf("replayed=%v", replayed)
	}
	status, err := a.TaskStatus(context.Background(), cfg, background.TaskID)
	if err != nil || status.Output.TaskID != background.TaskID || status.Output.Result.Text != "done" || status.Usage.TotalTokens != 49 || paths[2] != "GET /v1beta2/interactions/int-1" {
		t.Fatalf("status=%+v paths=%v error=%v", status, paths, err)
	}
	toolUsage := googleInteractionUsage(json.RawMessage(`{"total_input_tokens":100,"total_output_tokens":25,"total_thought_tokens":0,"total_tool_use_tokens":50,"total_tokens":125}`))
	if toolUsage.PromptTokens != 100 || toolUsage.TotalTokens != 125 || string(toolUsage.PromptTokensDetails["tool_tokens"]) != "50" {
		t.Fatalf("usage=%+v", toolUsage)
	}
	r.Extra = map[string]interface{}{"agent": "deep-research", "agent_config": map[string]interface{}{"type": "deep-research"}}
	r.Temperature, r.MaxTokens = 0.5, 100
	if _, err := a.Chat(context.Background(), cfg, r); err != nil {
		t.Fatal(err)
	}
	if bodies[2]["model"] != nil || bodies[2]["generation_config"] != nil {
		t.Fatalf("agent payload=%v", bodies[2])
	}
	r.Extra["generation_config"] = map[string]interface{}{"max_output_tokens": 100}
	if _, err := a.Chat(context.Background(), cfg, r); err == nil {
		t.Fatal("expected agent generation_config conflict")
	}
}

func TestGoogleInteractionsStreamTerminalAndNativeEvents(t *testing.T) {
	for _, tc := range []struct {
		name, terminal string
		wantErr        bool
	}{
		{"complete", `{"event_type":"interaction.completed","interaction":{"id":"int-1","status":"completed"}}`, false},
		{"failed", `{"event_type":"interaction.failed","interaction":{"id":"int-1","status":"failed","error":{"message":"failed"}}}`, true},
		{"truncated", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := []string{
				`{"event_type":"interaction.status_update","interaction_id":"int-1","status":"in_progress"}`,
				`{"event_type":"step.start","index":2,"step":{"type":"function_call","id":"call-1","name":"weather","arguments":{}}}`,
				`{"event_type":"step.delta","index":2,"delta":{"type":"arguments_delta","arguments":"{\"city\":"}}`,
				`{"event_type":"step.delta","index":2,"delta":{"type":"arguments_delta","arguments":"\"Rome\"}"}}`,
				`{"event_type":"step.delta","index":3,"delta":{"type":"code_execution_call","arguments":{"code":"print(1)"}}}`,
				`{"event_type":"step.delta","index":4,"delta":{"type":"text","text":"Hello"}}`,
				`{"event_type":"step.stop","index":2,"usage":{"total_input_tokens":7,"total_output_tokens":20,"total_thought_tokens":22,"total_tokens":49}}`,
			}
			if tc.terminal != "" {
				events = append(events, tc.terminal)
			}
			var wire strings.Builder
			for _, event := range events {
				wire.WriteString("data: " + event + "\n\n")
			}
			closed := 0
			cfg := &ProviderConfig{ChatProtocol: "interactions", APIKey: "test", BaseURL: "https://example.invalid", HTTPClient: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: &googleVideoTestBody{Reader: strings.NewReader(wire.String()), closed: &closed}, Request: r}, nil
			})}}
			stream, err := (&GoogleAdaptor{}).Stream(context.Background(), cfg, nativeToolRequest())
			if err != nil {
				t.Fatal(err)
			}
			var arguments, name, id string
			usage, native, progress, text := 0, false, false, false
			for {
				token, err := stream.Next(context.Background())
				if err != nil {
					if tc.wantErr == errors.Is(err, io.EOF) || tc.name == "truncated" && !errors.Is(err, io.ErrUnexpectedEOF) {
						t.Fatalf("terminal error=%v", err)
					}
					break
				}
				for _, call := range token.ToolCalls {
					arguments += call.Function.Arguments
					name += call.Function.Name
					id += call.ID
				}
				if token.Type == "progress" {
					progress = token.Text == "in_progress" && token.Metadata["interaction_id"] == "int-1"
				}
				if token.Type == "event" && strings.Contains(string(token.Metadata["event"].(json.RawMessage)), "print(1)") {
					native = true
				}
				if token.Type == "text" {
					text = token.Text == "Hello" && token.Index == 0 && token.Metadata["step_index"] == 4
				}
				if token.Usage != nil {
					usage = token.Usage.TotalTokens
				}
			}
			if err := stream.Close(); err != nil {
				t.Fatal(err)
			}
			if arguments != `{"city":"Rome"}` || name != "weather" || id != "call-1" || usage != 49 || !native || !progress || !text || closed != 1 {
				t.Fatalf("args=%q name=%q id=%q usage=%d native=%v progress=%v text=%v closed=%d", arguments, name, id, usage, native, progress, text, closed)
			}
		})
	}
}

func TestGoogleMediaStreams(t *testing.T) {
	for _, model := range []string{"gemini-image", "imagen-4.0", "veo-3.1"} {
		t.Run(model, func(t *testing.T) {
			polls := 0
			cfg := &ProviderConfig{APIKey: "test", BaseURL: "https://example.invalid", HTTPClient: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				body, contentType := "", "application/json"
				switch model {
				case "gemini-image":
					contentType = "text/event-stream"
					body = "data: " + `{"candidates":[{"content":{"parts":[{"text":"drawing"},{"inlineData":{"data":"aW1hZ2U=","mimeType":"image/png"}}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5}}` + "\n\n"
				case "imagen-4.0":
					body = `{"predictions":[{"bytesBase64Encoded":"aW1hZ2U=","mimeType":"image/png"},{"bytesBase64Encoded":"aW1hZ2Uy","mimeType":"image/png"}]}`
				case "veo-3.1":
					switch {
					case strings.Contains(r.URL.Path, ":download"):
						body, contentType = "video", "video/mp4"
					case r.Method == http.MethodPost:
						body = `{"name":"operations/video-1","done":false}`
					default:
						polls++
						body = `{"name":"operations/video-1","done":false}`
						if polls > 1 {
							body = `{"name":"operations/video-1","done":true,"response":{"generateVideoResponse":{"generatedSamples":[{"video":{"uri":"files/video-1"}}]}}}`
						}
					}
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}}
			request := &dto.MediaRequest{Type: dto.MediaTypeImage, Model: model, Prompt: "draw"}
			if model == "veo-3.1" {
				request.Type = dto.MediaTypeVideo
			}
			stream, err := (&GoogleAdaptor{}).StreamMedia(context.Background(), cfg, request)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			images, usage, videos, progress := 0, 0, 0, 0
			for {
				token, err := stream.Next(context.Background())
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				switch token.Type {
				case "image":
					images++
					if token.Text == "" || token.Metadata["mime_type"] != "image/png" {
						t.Fatalf("image=%+v", token)
					}
				case "video":
					videos++
					if token.Text != "dmlkZW8=" || token.Metadata["mime_type"] != "video/mp4" {
						t.Fatalf("video=%+v", token)
					}
				case "progress":
					progress++
				}
				if token.Usage != nil {
					usage = token.Usage.TotalTokens
				}
			}
			if model == "gemini-image" && (images != 1 || usage != 5) || model == "imagen-4.0" && images != 2 || model == "veo-3.1" && (videos != 1 || progress != 1) {
				t.Fatalf("images=%d usage=%d videos=%d progress=%d", images, usage, videos, progress)
			}
		})
	}
}
