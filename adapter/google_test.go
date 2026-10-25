package adapter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/YspCoder/omnigo/dto"
)

func TestGoogleClientUsesConfiguredBaseURL(t *testing.T) {
	const baseURL = "https://gateway.example.com/gemini/"

	client, err := (&GoogleAdaptor{}).getClient(context.Background(), &ProviderConfig{
		APIKey:  "test-api-key",
		BaseURL: "  " + baseURL + "  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := client.ClientConfig().HTTPOptions.BaseURL; got != baseURL {
		t.Fatalf("base URL = %q, want %q", got, baseURL)
	}
}

func TestSDKStreamCancellation(t *testing.T) {
	for _, provider := range []string{"google", "ark-text", "ark-image"} {
		for _, mode := range []string{"deadline", "close"} {
			t.Run(provider+"/"+mode, func(t *testing.T) {
				started, released := make(chan struct{}), make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					w.(http.Flusher).Flush()
					close(started)
					<-r.Context().Done()
					close(released)
				}))
				defer server.Close()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				cfg := &ProviderConfig{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
				request := &dto.MediaRequest{Model: "test-model", Messages: []dto.Message{{Role: "user", Content: "hello"}}}
				var stream dto.TokenStream
				var err error
				switch provider {
				case "google":
					stream, err = (&GoogleAdaptor{}).Stream(ctx, cfg, request)
				case "ark-text":
					stream, err = (&ArkAdaptor{}).Stream(ctx, cfg, request)
				case "ark-image":
					request.Type = dto.MediaTypeImage
					stream, err = (&ArkAdaptor{}).StreamMedia(ctx, cfg, request)
				}
				if err != nil {
					t.Fatal(err)
				}
				defer stream.Close()
				nextCtx := context.Background()
				if mode == "deadline" {
					var stop context.CancelFunc
					nextCtx, stop = context.WithTimeout(nextCtx, 150*time.Millisecond)
					defer stop()
				}
				result := make(chan error, 1)
				go func() {
					_, err := stream.Next(nextCtx)
					result <- err
				}()
				select {
				case <-started:
				case <-time.After(2 * time.Second):
					t.Fatal("stream request did not start")
				}
				if mode == "close" {
					if err := stream.Close(); err != nil {
						t.Fatal(err)
					}
				}
				select {
				case err := <-result:
					if err == nil || (mode == "deadline" && !errors.Is(err, context.DeadlineExceeded)) {
						t.Fatalf("Next error = %v", err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("Next did not unblock")
				}
				select {
				case <-released:
				case <-time.After(2 * time.Second):
					t.Fatal("upstream request was not cancelled")
				}
				if token, err := stream.Next(context.Background()); token != nil || !errors.Is(err, io.EOF) {
					t.Fatalf("closed stream returned token=%+v error=%v", token, err)
				}
			})
		}
	}
}
