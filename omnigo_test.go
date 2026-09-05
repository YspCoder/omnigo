package omnigo_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/YspCoder/omnigo"
	"github.com/YspCoder/omnigo/adapter"
	"github.com/YspCoder/omnigo/dto"
)

const chatResponse = `{"id":"test","object":"chat.completion","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mockResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

func newClient(t *testing.T, opts ...omnigo.ConfigOption) omnigo.LLM {
	t.Helper()
	base := []omnigo.ConfigOption{
		func(c *omnigo.Config) { *c = *omnigo.NewConfig() },
		omnigo.SetAPIKey("test-api-key"), omnigo.SetModel("test-model"),
		omnigo.SetLogLevel(omnigo.LogLevelOff), omnigo.SetMaxRetries(0),
	}
	client, err := omnigo.NewLLM(append(base, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func decodeRequest(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestGenerateSendsPromptAndOptions(t *testing.T) {
	var captured map[string]any
	client := newClient(t, omnigo.SetTemperature(0), omnigo.SetTopP(0.2), omnigo.SetSeed(42),
		omnigo.SetExtraHeaders(map[string]string{"X-Tenant-ID": "tenant", "Authorization": "Bearer override"}),
		omnigo.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			captured = decodeRequest(t, r)
			if r.Header.Get("X-Tenant-ID") != "tenant" || r.Header.Get("Authorization") != "Bearer override" {
				t.Errorf("missing configured headers: %v", r.Header)
			}
			return mockResponse(r, 200, chatResponse), nil
		})}))
	client.SetOption("frequency_penalty", 0.8)
	prompt := omnigo.NewPrompt("INPUT_MARKER", omnigo.WithContext("CONTEXT_MARKER"), omnigo.WithDirectives("DIRECTIVE_MARKER"),
		omnigo.WithOutput("OUTPUT_MARKER"), omnigo.WithExamples("EXAMPLE_MARKER"), omnigo.WithMaxLength(17))
	text, err := client.Generate(context.Background(), prompt)
	if err != nil {
		t.Fatal(err)
	}
	if text != "ok" {
		t.Fatalf("text = %q", text)
	}
	messages, _ := json.Marshal(captured["messages"])
	for _, marker := range []string{"INPUT_MARKER", "CONTEXT_MARKER", "DIRECTIVE_MARKER", "OUTPUT_MARKER", "EXAMPLE_MARKER", "17 words"} {
		if strings.Count(string(messages), marker) != 1 {
			t.Errorf("expected %q once in %s", marker, messages)
		}
	}
	for key, want := range map[string]float64{"temperature": 0, "top_p": 0.2, "seed": 42, "frequency_penalty": 0.8} {
		if got, ok := captured[key]; !ok || got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	if _, exists := captured["tool_choice"]; exists {
		t.Errorf("unexpected tool_choice: %v", captured["tool_choice"])
	}
	if prompt.Messages[0].Content != "INPUT_MARKER" {
		t.Fatal("caller prompt was mutated")
	}
	client.SetSystemPrompt("SYSTEM_MARKER", omnigo.CacheTypeEphemeral)
	if _, err := client.Generate(context.Background(), client.NewPrompt("USER_MARKER")); err != nil {
		t.Fatal(err)
	}
	messages, _ = json.Marshal(captured["messages"])
	if !strings.Contains(string(messages), "SYSTEM_MARKER") || !strings.Contains(string(messages), "USER_MARKER") {
		t.Fatalf("missing message: %s", messages)
	}
}

func TestProviderRoutingAndRegistration(t *testing.T) {
	adapter.RegisterProvider("test-registered", adapter.ProviderSpec{Endpoint: "https://registered.invalid/v1", AdaptorFactory: func() adapter.Adaptor { return &adapter.OpenAIAdaptor{} }})
	for _, tc := range []struct{ provider, endpoint, host string }{
		{"groq", "", "api.groq.com"}, {"moonshot", "", "api.moonshot.cn"},
		{"test-registered", "", "registered.invalid"}, {"groq", "https://override.invalid/v1", "override.invalid"},
	} {
		t.Run(tc.provider+tc.host, func(t *testing.T) {
			client := newClient(t, omnigo.SetProvider(tc.provider), omnigo.SetAPIKey("test-provider-key"), omnigo.SetEndpoint(tc.endpoint),
				omnigo.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if r.URL.Host != tc.host {
						t.Errorf("host = %s, want %s", r.URL.Host, tc.host)
					}
					return mockResponse(r, 200, chatResponse), nil
				})}))
			if _, err := client.Generate(context.Background(), omnigo.NewPrompt("hello")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOpenAITimeoutAndRetries(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(time.Second):
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, chatResponse)
		}))
		defer server.Close()
		client := newClient(t, omnigo.SetEndpoint(server.URL), omnigo.SetTimeout(20*time.Millisecond))
		_, err := client.Generate(context.Background(), omnigo.NewPrompt("hello"))
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected timeout, got %v", err)
		}
	})
	for _, retries := range []int{0, 2} {
		t.Run(strconv.Itoa(retries)+" retries", func(t *testing.T) {
			var attempts int
			client := newClient(t, omnigo.SetMaxRetries(retries), omnigo.SetRetryDelay(time.Millisecond),
				omnigo.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					attempts++
					body := decodeRequest(t, r)
					if body["model"] != "test-model" {
						t.Errorf("body was not replayed: %v", body)
					}
					return mockResponse(r, 500, `{"error":{"message":"temporary"}}`), nil
				})}))
			_, err := client.Generate(context.Background(), omnigo.NewPrompt("hello"))
			if err == nil || attempts != retries+1 {
				t.Fatalf("attempts=%d err=%v", attempts, err)
			}
		})
	}
}

func TestConcurrentGenerate(t *testing.T) {
	var calls atomic.Int32
	client := newClient(t, omnigo.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return mockResponse(r, 200, chatResponse), nil
	})}))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			client.SetSystemPrompt("system", omnigo.CacheTypeEphemeral)
			client.UpdateLogLevel(omnigo.LogLevelOff)
			client.GetLogLevel()
			client.SetOption("frequency_penalty", 0.5)
			if _, err := client.Generate(context.Background(), omnigo.NewPrompt("hello")); err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if calls.Load() != 16 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestLogLevelSettersShareState(t *testing.T) {
	client := newClient(t)
	client.SetLogLevel(omnigo.LogLevelInfo)
	if got := client.GetLogLevel(); got != omnigo.LogLevelInfo {
		t.Fatalf("GetLogLevel after SetLogLevel = %v, want INFO", got)
	}
	client.UpdateLogLevel(omnigo.LogLevelWarn)
	if got := client.GetLogLevel(); got != omnigo.LogLevelWarn {
		t.Fatalf("GetLogLevel after UpdateLogLevel = %v, want WARN", got)
	}
}

func TestStreamPreservesToolCallsAndUsage(t *testing.T) {
	var captured map[string]any
	client := newClient(t, omnigo.SetTemperature(0), omnigo.SetTopP(0),
		omnigo.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			captured = decodeRequest(t, r)
			events := []string{
				`{"id":"test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"weather","arguments":"{\"city\":"}},{"index":1,"id":"call_2","type":"function","function":{"name":"clock","arguments":"{"}}]}},{"index":1,"delta":{"content":"alternative"}}]}`,
				`{"id":"test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}},{"index":1,"function":{"arguments":"}"}}]},"finish_reason":"tool_calls"}]}`,
				`{"id":"test","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
				`[DONE]`,
			}
			response := mockResponse(r, 200, "data: "+strings.Join(events, "\n\ndata: ")+"\n\n")
			response.Header.Set("Content-Type", "text/event-stream")
			return response, nil
		})}))
	client.SetOption("frequency_penalty", 0.5)
	stream, err := client.Stream(context.Background(), omnigo.NewPrompt("hello"))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var tokens []*dto.StreamToken
	for {
		token, err := stream.Next(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}
	if len(tokens) != 4 {
		t.Fatalf("tokens = %+v", tokens)
	}
	if tokens[0].Type != "function_call" || len(tokens[0].ToolCalls) != 2 {
		t.Fatalf("tool event = %+v", tokens[0])
	}
	if tokens[0].ToolCalls[0].ID != "call_1" || tokens[0].ToolCalls[0].Function.Name != "weather" || tokens[0].ToolCalls[1].Index != 1 {
		t.Fatalf("tool deltas = %+v", tokens[0].ToolCalls)
	}
	if tokens[1].Index != 1 || tokens[1].Text != "alternative" {
		t.Fatalf("second choice = %+v", tokens[1])
	}
	arguments := tokens[0].ToolCalls[0].Function.Arguments + tokens[2].ToolCalls[0].Function.Arguments
	if arguments != `{"city":"Paris"}` || tokens[2].FinishReason != "tool_calls" {
		t.Fatalf("arguments=%s finish=%s", arguments, tokens[2].FinishReason)
	}
	if tokens[3].Type != "usage" || tokens[3].Usage == nil || tokens[3].Usage.TotalTokens != 12 {
		t.Fatalf("usage = %+v", tokens[3])
	}
	if captured["temperature"] != float64(0) || captured["top_p"] != float64(0) || captured["frequency_penalty"] != 0.5 {
		t.Errorf("stream options = %v", captured)
	}
	if options, ok := captured["stream_options"].(map[string]any); !ok || options["include_usage"] != true {
		t.Errorf("stream_options = %v", captured["stream_options"])
	}
}

func TestStreamNextCancellation(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	client := newClient(t, omnigo.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader, Request: r}, nil
	})}))
	stream, err := client.Stream(context.Background(), omnigo.NewPrompt("hello"))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := stream.Next(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Next error = %v", err)
	}
}

func TestResponsesAPISendsOptionsAndRetries(t *testing.T) {
	var attempts int
	client := newClient(t, omnigo.SetChatProtocol("responses"), omnigo.SetTemperature(0), omnigo.SetTopP(0.4),
		omnigo.SetMaxRetries(1), omnigo.SetRetryDelay(time.Millisecond),
		omnigo.SetExtraHeaders(map[string]string{"X-Tenant-ID": "tenant"}),
		omnigo.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			attempts++
			if r.URL.Path != "/v1/responses" || r.Header.Get("X-Tenant-ID") != "tenant" {
				t.Errorf("request = %s %v", r.URL, r.Header)
			}
			body := decodeRequest(t, r)
			if body["temperature"] != float64(0) || body["top_p"] != 0.4 {
				t.Errorf("body = %v", body)
			}
			if attempts == 1 {
				return mockResponse(r, 429, `{"error":{"message":"rate limit"}}`), nil
			}
			return mockResponse(r, 200, `{"id":"test","output_text":"ok"}`), nil
		})}))
	if text, err := client.Generate(context.Background(), omnigo.NewPrompt("hello")); err != nil || text != "ok" {
		t.Fatalf("text=%s err=%v", text, err)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d", attempts)
	}
}

func TestGenerateJSONSchemaPointer(t *testing.T) {
	type document struct {
		Name string `json:"name" validate:"required"`
	}
	valueSchema, err := omnigo.GenerateJSONSchema(document{})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{&document{}, (*document)(nil)} {
		got, err := omnigo.GenerateJSONSchema(value)
		if err != nil || !reflect.DeepEqual(got, valueSchema) {
			t.Fatalf("schema=%s error=%v", got, err)
		}
	}
	for _, value := range []any{nil, 1, "hello", []string{}} {
		if _, err := omnigo.GenerateJSONSchema(value); err == nil {
			t.Errorf("expected error for %T", value)
		}
	}
}
