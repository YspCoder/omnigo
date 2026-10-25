package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/YspCoder/omnigo/dto"
)

func TestOpenAIResponsesStreamPreservesEvents(t *testing.T) {
	events := strings.Join([]string{
		`data: {"type":"response.output_text.delta","output_index":1,"content_index":0,"delta":"Hello "}`,
		`data: {"type":"response.output_text.delta","output_index":1,"content_index":0,"delta":"world\n"}`,
		`data: {"type":"response.output_text.done","output_index":1,"content_index":0,"text":"Hello world\n"}`,
		`data: {"type":"response.output_item.added","output_index":2,"item":{"type":"function_call","call_id":"call_1","name":"lookup","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":2,"delta":"{\"q\":"}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":2,"delta":"\"Paris\"}"}`,
		`data: {"type":"response.function_call_arguments.done","output_index":2,"arguments":"{\"q\":\"Paris\"}"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[{"type":"reasoning"},{"type":"message","content":[{"type":"output_text","text":"Hello world\n"}]},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"q\":\"Paris\"}"}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7,"input_tokens_details":{"cached_tokens":1}}}}`,
	}, "\n\n") // Exercise a terminal event with no trailing blank line.
	config := &ProviderConfig{APIKey: "test-key", BaseURL: "https://example.invalid", ChatProtocol: "responses", HTTPClient: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if r.URL.Path != "/responses" || body["stream"] != true || body["temperature"] != float64(0) || len(body["tools"].([]interface{})) != 1 {
			t.Fatalf("stream request = %s %+v", r.URL, body)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(events)), Request: r}, nil
	})}}
	stream, err := (&OpenAIAdaptor{}).Stream(context.Background(), config, &dto.MediaRequest{
		Model: "test-model", Messages: []dto.Message{{Role: "user", Content: "hello"}},
		Tools: []dto.Tool{{Type: "function", Function: dto.ToolFunction{Name: "lookup"}}}, Options: map[string]interface{}{"temperature": 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var text, name, id, arguments string
	var final *dto.StreamToken
	for {
		token, err := stream.Next(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if token.Index != 0 {
			t.Fatalf("response candidate index = %d", token.Index)
		}
		if token.Type == "text" {
			if token.Metadata["output_index"] != 1 {
				t.Fatalf("text output index = %v", token.Metadata)
			}
			text += token.Text
		}
		for _, call := range token.ToolCalls {
			if call.Index != 2 {
				t.Fatalf("tool index = %d", call.Index)
			}
			id += call.ID
			name += call.Function.Name
			arguments += call.Function.Arguments
		}
		if token.FinishReason != "" {
			final = token
		}
	}
	if text != "Hello world\n" || id != "call_1" || name != "lookup" || arguments != `{"q":"Paris"}` {
		t.Fatalf("stream text=%q tool=%s/%s arguments=%s", text, id, name, arguments)
	}
	if final == nil || final.FinishReason != "tool_calls" || final.Usage == nil || final.Usage.TotalTokens != 7 || string(final.Usage.PromptTokensDetails["cached_tokens"]) != "1" {
		t.Fatalf("terminal event = %+v", final)
	}
	t.Run("terminal output_text", func(t *testing.T) {
		calls := 0
		config := streamTestConfig(`data: {"type":"response.completed","response":{"status":"completed","output_text":"only text"}}`+"\n\n", "text/event-stream", &calls)
		config.ChatProtocol = "responses"
		stream, err := (&OpenAIAdaptor{}).Stream(context.Background(), config, &dto.MediaRequest{Model: "test-model"})
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		text, err := stream.Next(context.Background())
		if err != nil || text == nil || text.Text != "only text" {
			t.Fatalf("terminal text = %+v, error = %v", text, err)
		}
		finish, err := stream.Next(context.Background())
		if err != nil || finish == nil || finish.FinishReason != "completed" {
			t.Fatalf("terminal result = %+v, error = %v", finish, err)
		}
	})
}

func TestOpenAIStreamFailureAndIncomplete(t *testing.T) {
	for _, tc := range []struct {
		name, event, reason string
		image, wantError    bool
	}{
		{"failed response", `{"type":"response.failed","response":{"status":"failed","error":{"code":"server_error","message":"failed"},"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`, "failed", false, true},
		{"incomplete response", `{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`, "length", false, false},
		{"response error", `{"type":"error","message":"failed","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`, "", false, true},
		{"gateway error", `{"error":{"message":"failed"},"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`, "", false, true},
		{"image error", `{"type":"error","error":{"code":"failed","message":"failed"},"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`, "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			config := streamTestConfig("data: "+tc.event+"\n\n", "text/event-stream", &calls)
			config.ChatProtocol = "responses"
			request := &dto.MediaRequest{Model: "test-model", Type: dto.MediaTypeText}
			if tc.image {
				request.Type = dto.MediaTypeImage
			}
			stream, err := (&OpenAIAdaptor{}).StreamMedia(context.Background(), config, request)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			token, err := stream.Next(context.Background())
			if err != nil || token == nil || token.Usage == nil || token.Usage.TotalTokens != 3 || token.FinishReason != tc.reason {
				t.Fatalf("terminal event = %+v, error = %v", token, err)
			}
			_, err = stream.Next(context.Background())
			if tc.wantError && (err == nil || errors.Is(err, io.EOF)) || !tc.wantError && !errors.Is(err, io.EOF) {
				t.Fatalf("terminal error = %v", err)
			}
			if _, err = stream.Next(context.Background()); !errors.Is(err, io.EOF) {
				t.Fatalf("after terminal error = %v", err)
			}
		})
	}
	t.Run("interrupted response", func(t *testing.T) {
		calls := 0
		config := streamTestConfig(`data: {"type":"response.output_text.delta","delta":"partial"}`+"\n\n", "text/event-stream", &calls)
		config.ChatProtocol = "responses"
		stream, err := (&OpenAIAdaptor{}).Stream(context.Background(), config, &dto.MediaRequest{Model: "test-model"})
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		if _, err := stream.Next(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := stream.Next(context.Background()); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("interrupted stream error = %v", err)
		}
	})
}

func TestOpenAIImageStreamsPreserveParametersAndResults(t *testing.T) {
	for _, edit := range []bool{false, true} {
		name := "generation"
		if edit {
			name = "edit"
		}
		t.Run(name, func(t *testing.T) {
			prefix := "image_generation"
			if edit {
				prefix = "image_edit"
			}
			events := "data: {\"type\":\"" + prefix + ".partial_image\",\"partial_image_index\":0,\"b64_json\":\"partial\"}\n\n" +
				"data: {\"type\":\"" + prefix + ".completed\",\"b64_json\":\"final\",\"output_format\":\"webp\",\"usage\":{\"input_tokens\":10,\"output_tokens\":20,\"total_tokens\":30}}\n\n"
			config := &ProviderConfig{APIKey: "test-key", BaseURL: "https://example.invalid", HTTPClient: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				if edit {
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Fatal(err)
					}
					defer r.MultipartForm.RemoveAll()
					if r.URL.Path != "/images/edits" || r.FormValue("stream") != "true" || r.FormValue("partial_images") != "2" || r.FormValue("output_format") != "webp" || len(r.MultipartForm.File["image"]) != 1 || len(r.MultipartForm.File["mask"]) != 1 {
						t.Fatalf("image edit request = %s %+v", r.URL, r.MultipartForm)
					}
				} else {
					var body map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if r.URL.Path != "/images/generations" || body["stream"] != true || body["partial_images"] != float64(2) || body["output_format"] != "webp" {
						t.Fatalf("image generate request = %s %+v", r.URL, body)
					}
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(events)), Request: r}, nil
			})}}
			request := &dto.MediaRequest{Type: dto.MediaTypeImage, Model: "gpt-image-1", Prompt: "test", Extra: map[string]interface{}{"partial_images": 1}, Options: map[string]interface{}{"partial_images": 2, "output_format": "webp"}}
			if edit {
				request.Extra["image"], request.Extra["mask"] = "data:image/png;base64,aGVsbG8=", "data:image/png;base64,aGVsbG8="
			}
			stream, err := (&OpenAIAdaptor{}).StreamMedia(context.Background(), config, request)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			partial, err := stream.Next(context.Background())
			if err != nil || partial == nil || partial.Text != "partial" || partial.Usage != nil {
				t.Fatalf("partial image = %+v, error = %v", partial, err)
			}
			final, err := stream.Next(context.Background())
			if err != nil || final == nil || final.Text != "final" || final.FinishReason != "completed" || final.Usage == nil || final.Usage.TotalTokens != 30 || final.Metadata["output_format"] != "webp" {
				t.Fatalf("final image = %+v, error = %v", final, err)
			}
			if _, err := stream.Next(context.Background()); !errors.Is(err, io.EOF) {
				t.Fatalf("after final image = %v", err)
			}
		})
	}
}

func TestOpenAINativeStreamsCancelBlockedReads(t *testing.T) {
	for _, mode := range []string{"responses", "image", "chat"} {
		for _, closeStream := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/deadline", true: "/close"}[closeStream], func(t *testing.T) {
				released := make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					w.(http.Flusher).Flush()
					<-r.Context().Done()
					close(released)
				}))
				defer server.Close()
				config := &ProviderConfig{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
				request := &dto.MediaRequest{Type: dto.MediaTypeText, Model: "test-model"}
				if mode == "responses" {
					config.ChatProtocol = "responses"
				} else if mode == "image" {
					request.Type = dto.MediaTypeImage
				}
				stream, err := (&OpenAIAdaptor{}).StreamMedia(context.Background(), config, request)
				if err != nil {
					t.Fatal(err)
				}
				defer stream.Close()
				ctx := context.Background()
				if !closeStream {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
					defer cancel()
				}
				result := make(chan error, 1)
				go func() { _, err := stream.Next(ctx); result <- err }()
				if closeStream {
					if err := stream.Close(); err != nil {
						t.Fatal(err)
					}
				}
				select {
				case err := <-result:
					if err == nil || !closeStream && !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("read error = %v", err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("stream read did not unblock")
				}
				select {
				case <-released:
				case <-time.After(2 * time.Second):
					t.Fatal("upstream request was not cancelled")
				}
			})
		}
	}
}

func TestOpenAIRawStreamsCancelCustomTransport(t *testing.T) {
	for _, image := range []bool{false, true} {
		t.Run(map[bool]string{false: "responses", true: "image"}[image], func(t *testing.T) {
			reader, writer := io.Pipe()
			defer writer.Close()
			config := &ProviderConfig{BaseURL: "https://example.invalid", ChatProtocol: "responses", HTTPClient: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader, Request: r}, nil
			})}}
			request := &dto.MediaRequest{Type: dto.MediaTypeText, Model: "test-model"}
			if image {
				request.Type = dto.MediaTypeImage
			}
			stream, err := (&OpenAIAdaptor{}).StreamMedia(context.Background(), config, request)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if _, err := stream.Next(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("custom transport cancellation = %v", err)
			}
		})
	}
}
