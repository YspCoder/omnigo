package adapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/YspCoder/omnigo/dto"
)

func streamTestConfig(body, contentType string, calls *int) *ProviderConfig {
	return &ProviderConfig{
		APIKey: "test-key", BaseURL: "https://example.invalid",
		HTTPClient: &http.Client{Transport: testTransport(func(request *http.Request) (*http.Response, error) {
			*calls++
			return &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}},
				Body: io.NopCloser(strings.NewReader(body)), Request: request,
			}, nil
		})},
	}
}

func TestMediaStreamsDeliverResultBeforeEOF(t *testing.T) {
	const resultURL = "https://example.com/result.mp4"
	ark := func(cfg *ProviderConfig) dto.TokenStream {
		return &arkVidStream{c: (&ArkAdaptor{}).getClient(cfg), id: "task-1"}
	}
	for _, tc := range []struct {
		name   string
		body   string
		stream func(*ProviderConfig) dto.TokenStream
		failed bool
		calls  int
		tokens int
	}{
		{"ali", `{"output":{"task_id":"task-1","task_status":"SUCCEEDED","video_url":"https://example.com/result.mp4"},"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`, func(cfg *ProviderConfig) dto.TokenStream {
			return &aliPollingStream{adaptor: &AliAdaptor{}, cfg: cfg, taskID: "task-1"}
		}, false, 1, 5},
		{"ark", `{"id":"task-1","status":"succeeded","content":{"video_url":"https://example.com/result.mp4"},"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`, ark, false, 1, 5},
		{"vidu", `{"id":"task-1","state":"success","creations":[{"url":"https://example.com/result.mp4"}]}`, func(cfg *ProviderConfig) dto.TokenStream {
			return &viduPollingStream{adaptor: &ViduAdaptor{}, cfg: cfg, taskID: "task-1"}
		}, false, 1, 0},
		{"kling", `{"code":0,"data":{"task_id":"task-1","task_status":"succeed","task_result":{"videos":[{"url":"https://example.com/result.mp4"}]}}}`, func(cfg *ProviderConfig) dto.TokenStream {
			return &klingPollingStream{adaptor: &KlingAdaptor{}, cfg: cfg, taskID: "task-1"}
		}, false, 1, 0},
		{"pai", `{"ErrCode":0,"Resp":{"id":1,"status":1,"url":"https://example.com/result.mp4"}}`, func(cfg *ProviderConfig) dto.TokenStream {
			return &paiPollingStream{adaptor: &PaiAdaptor{}, cfg: cfg, taskID: "task-1"}
		}, false, 1, 0},
		{"single result", "", func(*ProviderConfig) dto.TokenStream {
			return &klingSingleTokenStream{token: &dto.StreamToken{Type: "url", URL: resultURL}}
		}, false, 0, 0},
		{"ark cancelled", `{"id":"task-1","status":"cancelled"}`, ark, true, 1, 0},
		{"ali failed with usage", `{"output":{"task_id":"task-1","task_status":"FAILED","message":"generation failed"},"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`, func(cfg *ProviderConfig) dto.TokenStream {
			return &aliPollingStream{adaptor: &AliAdaptor{}, cfg: cfg, taskID: "task-1"}
		}, true, 1, 5},
		{"ark failed with usage", `{"id":"task-1","status":"failed","error":{"message":"generation failed"},"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`, ark, true, 1, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			stream := tc.stream(streamTestConfig(tc.body, "application/json", &calls))
			defer stream.Close()
			token, err := stream.Next(context.Background())
			if tc.tokens > 0 && (err != nil || token == nil || token.Usage == nil || token.Usage.PromptTokens != 2 || token.Usage.CompletionTokens != 3 || token.Usage.TotalTokens != tc.tokens) {
				t.Fatalf("final usage = %+v, error = %v", token, err)
			}
			if tc.failed {
				if tc.tokens > 0 {
					if token.Type != "usage" {
						t.Fatalf("failed task should deliver usage first, got %+v", token)
					}
					token, err = stream.Next(context.Background())
				}
				if err == nil || errors.Is(err, io.EOF) {
					t.Fatalf("cancelled task should fail, got token=%+v err=%v", token, err)
				}
			} else if err != nil || token == nil || token.Type != "url" || token.URL != resultURL {
				t.Fatalf("final result = %+v, error = %v", token, err)
			}
			for range 2 {
				if token, err := stream.Next(context.Background()); token != nil || !errors.Is(err, io.EOF) {
					t.Fatalf("finished stream returned token=%+v err=%v", token, err)
				}
			}
			if calls != tc.calls {
				t.Fatalf("HTTP calls = %d, want %d", calls, tc.calls)
			}
		})
	}
}

func TestArkStreamPreservesWhitespace(t *testing.T) {
	var events strings.Builder
	for _, delta := range []string{"Hello", " ", "world", "\n"} {
		fmt.Fprintf(&events, "data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\n", delta)
	}
	events.WriteString("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5}}}\n\ndata: [DONE]\n\n")
	calls := 0
	stream, err := (&ArkAdaptor{}).Stream(context.Background(), streamTestConfig(events.String(), "text/event-stream", &calls), &dto.MediaRequest{Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var text strings.Builder
	usageEvents := 0
	for {
		token, err := stream.Next(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if token.Usage != nil {
			usageEvents++
			if token.Type != "usage" || token.Usage.PromptTokens != 2 || token.Usage.CompletionTokens != 3 || token.Usage.TotalTokens != 5 {
				t.Fatalf("completion usage = %+v", token)
			}
		}
		text.WriteString(token.Text)
	}
	if got := text.String(); got != "Hello world\n" {
		t.Fatalf("text = %q, want spaces and newlines preserved", got)
	}
	if usageEvents != 1 {
		t.Fatalf("usage events = %d, want 1", usageEvents)
	}
}

func TestArkImageStreamPreservesPartialFailures(t *testing.T) {
	events := strings.Join([]string{
		`data: {"type":"image_generation.partial_failed","image_index":0,"error":{"code":"ContentBlocked","message":"first image blocked"}}`,
		`data: {"type":"image_generation.partial_succeeded","image_index":1,"url":"https://example.com/one.png"}`,
		`data: {"type":"image_generation.completed","usage":{"output_tokens":17,"total_tokens":17,"generated_images":1}}`,
	}, "\n\n") + "\n\n"
	calls := 0
	stream, err := (&ArkAdaptor{}).StreamMedia(context.Background(), streamTestConfig(events, "text/event-stream", &calls), &dto.MediaRequest{Type: dto.MediaTypeImage, Model: "test-model", Prompt: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	token, err := stream.Next(context.Background())
	if err != nil || token == nil || token.Type != "error" || token.Metadata["code"] != "ContentBlocked" {
		t.Fatalf("partial failure = %+v, error = %v", token, err)
	}
	token, err = stream.Next(context.Background())
	if err != nil || token == nil || token.URL != "https://example.com/one.png" || token.Index != 1 {
		t.Fatalf("remaining image = %+v, error = %v", token, err)
	}
	token, err = stream.Next(context.Background())
	if err != nil || token == nil || token.Usage == nil || token.Usage.TotalTokens != 17 {
		t.Fatalf("completion usage = %+v, error = %v", token, err)
	}
	if token, err = stream.Next(context.Background()); token != nil || !errors.Is(err, io.EOF) {
		t.Fatalf("finished stream = %+v, error = %v", token, err)
	}
}

func TestMediaStreamsPreserveProgressUsage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		bodies  []string
		adaptor Adaptor
		want    []int
	}{
		{"ali", []string{
			`{"output":{"task_id":"task-1","task_status":"RUNNING"},"usage":{"input_tokens":1,"total_tokens":1}}`,
			`{"output":{"task_id":"task-1","task_status":"RUNNING"},"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`,
			`{"output":{"task_id":"task-1","task_status":"RUNNING"},"usage":{"input_tokens":2,"output_tokens":2,"total_tokens":4}}`,
			`{"output":{"task_id":"task-1","task_status":"SUCCEEDED","video_url":"https://example.com/video.mp4"}}`,
		}, &AliAdaptor{}, []int{1, 3, 4, 4}},
		{"ark", []string{
			`{"id":"task-1"}`,
			`{"id":"task-1","status":"running","usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
			`{"id":"task-1","status":"running","usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}}`,
			`{"id":"task-1","status":"succeeded","duration":5,"content":{"video_url":"https://example.com/video.mp4"}}`,
		}, &ArkAdaptor{}, []int{3, 4, 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			cfg := &ProviderConfig{APIKey: "test-key", BaseURL: "https://example.invalid", HTTPClient: &http.Client{Transport: testTransport(func(request *http.Request) (*http.Response, error) {
				if calls >= len(tc.bodies) {
					return nil, fmt.Errorf("unexpected request after final result")
				}
				body := tc.bodies[calls]
				calls++
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
			})}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			stream, err := tc.adaptor.StreamMedia(ctx, cfg, &dto.MediaRequest{Type: dto.MediaTypeVideo, Model: "wan2.6-t2v", Prompt: "test"})
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			for i, total := range tc.want {
				token, err := stream.Next(ctx)
				if err != nil || token == nil || token.Usage == nil || token.Usage.TotalTokens != total {
					t.Fatalf("event %d = %+v, error = %v, want total %d", i, token, err, total)
				}
				if i == len(tc.want)-1 && (token.Type != "url" || token.URL != "https://example.com/video.mp4") {
					t.Fatalf("final result = %+v", token)
				}
			}
			if token, err := stream.Next(ctx); token != nil || !errors.Is(err, io.EOF) || calls != len(tc.bodies) {
				t.Fatalf("finished stream = %+v, error = %v, HTTP calls = %d", token, err, calls)
			}
		})
	}
}
