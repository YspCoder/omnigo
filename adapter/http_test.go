package adapter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"testing"
	"time"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProviderHTTPClientPreservesConfiguration(t *testing.T) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 20
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	original := &http.Client{Transport: transport, Timeout: time.Minute, Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	client, err := providerHTTPClient(&ProviderConfig{HTTPClient: original, Timeout: 3 * time.Second, Proxy: "localhost:8080"})
	if err != nil {
		t.Fatal(err)
	}
	if client == original || original.Timeout != time.Minute || client.Timeout != 3*time.Second || client.Jar != jar || client.CheckRedirect == nil {
		t.Fatalf("client was not cloned correctly: %+v", client)
	}
	cloned := client.Transport.(*http.Transport)
	if cloned == transport || cloned.MaxIdleConnsPerHost != 20 || cloned.ForceAttemptHTTP2 != transport.ForceAttemptHTTP2 {
		t.Fatal("transport settings lost")
	}
	proxy, err := cloned.Proxy(&http.Request{})
	if err != nil || proxy.String() != "http://localhost:8080" {
		t.Fatalf("proxy=%v err=%v", proxy, err)
	}
	client, err = providerHTTPClient(&ProviderConfig{HTTPClient: original})
	if err != nil || client.Timeout != time.Minute || client.Transport != transport {
		t.Fatal("unset options changed the supplied client")
	}
	for _, proxy := range []string{"http://", "ftp://localhost", "://bad"} {
		if _, err := providerHTTPClient(&ProviderConfig{Proxy: proxy}); err == nil {
			t.Errorf("accepted invalid proxy %q", proxy)
		}
	}
	custom := &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("unused") })}
	if _, err := providerHTTPClient(&ProviderConfig{HTTPClient: custom, Proxy: "http://localhost"}); err == nil {
		t.Fatal("silently replaced a custom transport")
	}
}

func TestRetryPolicy(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		retryHeader string
		replayable  bool
		attempts    int
	}{
		{"bad request", 400, "", true, 1}, {"rate limit", 429, "", true, 3},
		{"server error", 503, "", true, 3}, {"provider forbids", 503, "false", true, 1},
		{"provider requests", 400, "true", true, 3}, {"non replayable", 503, "", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempts := 0
			client := &retryHTTPClient{maxRetries: 2, retryDelay: time.Millisecond, client: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				attempts++
				body, err := io.ReadAll(r.Body)
				r.Body.Close()
				if err != nil || string(body) != "request" {
					t.Errorf("body=%s err=%v", body, err)
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"X-Should-Retry": {tc.retryHeader}}, Body: io.NopCloser(strings.NewReader("response")), Request: r}, nil
			})}}
			request, err := http.NewRequest(http.MethodPost, "http://example.invalid", strings.NewReader("request"))
			if err != nil {
				t.Fatal(err)
			}
			if !tc.replayable {
				request.GetBody = nil
			}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || string(body) != "response" || attempts != tc.attempts {
				t.Fatalf("body=%s attempts=%d err=%v", body, attempts, err)
			}
		})
	}
}

func TestRetryBackoffCancellation(t *testing.T) {
	attempts := 0
	client := &retryHTTPClient{maxRetries: 3, retryDelay: time.Hour, client: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("retry")), Request: r}, nil
	})}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(request); !errors.Is(err, context.DeadlineExceeded) || attempts != 1 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
}

func TestRetryDelay(t *testing.T) {
	client := &retryHTTPClient{retryDelay: 2 * time.Second}
	if client.delay(2, nil) != 8*time.Second || client.delay(100, nil) != 30*time.Second {
		t.Fatal("unexpected exponential backoff")
	}
	for _, tc := range []struct {
		header, value string
		want          time.Duration
	}{
		{"Retry-After", "5", 5 * time.Second}, {"Retry-After-Ms", "125", 125 * time.Millisecond},
		{"Retry-After", "invalid", 2 * time.Second}, {"Retry-After", "0", 0},
	} {
		response := &http.Response{Header: make(http.Header)}
		response.Header.Set(tc.header, tc.value)
		if got := client.delay(0, response); got != tc.want {
			t.Errorf("%s=%s delay=%s", tc.header, tc.value, got)
		}
	}
}

func TestProviderClientInitializationConcurrent(t *testing.T) {
	cfg := &ProviderConfig{APIKey: "test-api-key"}
	openAI, google, anthropic, ark := &OpenAIAdaptor{}, &GoogleAdaptor{}, &AnthropicAdaptor{}, &ArkAdaptor{}
	for name, build := range map[string]func() (any, error){
		"openai":    func() (any, error) { return openAI.getClient(cfg) },
		"google":    func() (any, error) { return google.getClient(context.Background(), cfg) },
		"anthropic": func() (any, error) { return anthropic.getClient(cfg), nil },
		"ark":       func() (any, error) { return ark.getClient(cfg), nil },
	} {
		t.Run(name, func(t *testing.T) {
			var wg sync.WaitGroup
			start := make(chan struct{})
			clients := make(chan any, 16)
			for range 16 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					client, err := build()
					if err != nil {
						t.Error(err)
						return
					}
					clients <- client
				}()
			}
			close(start)
			wg.Wait()
			close(clients)
			var first any
			for client := range clients {
				if first == nil {
					first = client
				}
				if client != first {
					t.Error("initialized more than one SDK client")
				}
			}
		})
	}
}
