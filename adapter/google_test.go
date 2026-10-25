package adapter

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/YspCoder/omnigo/dto"
	"google.golang.org/genai"
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

func TestGoogleVideoDownloadCancellation(t *testing.T) {
	for _, operation := range []string{"media", "task status"} {
		for _, deadline := range []bool{false, true} {
			mode := "cancel"
			if deadline {
				mode = "deadline"
			}
			t.Run(operation+"/"+mode, func(t *testing.T) {
				started, released := make(chan struct{}, 1), make(chan struct{}, 1)
				shutdown := make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if !strings.Contains(r.URL.Path, ":download") {
						w.Header().Set("Content-Type", "application/json")
						io.WriteString(w, `{"name":"operations/video-1","done":true,"response":{"generateVideoResponse":{"generatedSamples":[{"video":{"uri":"files/video-1"}}]}}}`)
						return
					}
					w.Header().Set("Content-Type", "video/mp4")
					w.(http.Flusher).Flush()
					started <- struct{}{}
					select {
					case <-r.Context().Done():
						released <- struct{}{}
					case <-shutdown:
					}
				}))
				defer server.Close()
				defer close(shutdown)
				ctx, cancel := context.WithCancel(context.Background())
				wantErr := context.Canceled
				if deadline {
					cancel()
					ctx, cancel = context.WithTimeout(context.Background(), 150*time.Millisecond)
					wantErr = context.DeadlineExceeded
				}
				defer cancel()
				cfg := &ProviderConfig{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
				result := make(chan error, 1)
				go func() {
					adaptor := &GoogleAdaptor{}
					var err error
					if operation == "media" {
						_, err = adaptor.Media(ctx, cfg, &dto.MediaRequest{Type: dto.MediaTypeVideo, Model: "test-model", Prompt: "hello"})
					} else {
						_, err = adaptor.TaskStatus(ctx, cfg, "operations/video-1")
					}
					result <- err
				}()
				select {
				case <-started:
				case <-time.After(2 * time.Second):
					t.Fatal("video download did not start")
				}
				if !deadline {
					cancel()
				}
				select {
				case err := <-result:
					if !errors.Is(err, wantErr) {
						t.Fatalf("download error = %v, want %v", err, wantErr)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("video download did not unblock")
				}
				select {
				case <-released:
				case <-time.After(2 * time.Second):
					t.Fatal("upstream download was not cancelled")
				}
			})
		}
	}
}

func TestGoogleVideoPayloadPreservesFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name       string
		inline     bool
		status     int
		redirect   bool
		wantBase64 string
		wantCalls  int
	}{
		{"forbidden", false, http.StatusForbidden, false, "", 1},
		{"download", false, http.StatusOK, false, base64.StdEncoding.EncodeToString([]byte("video")), 1},
		{"redirect", false, http.StatusOK, true, base64.StdEncoding.EncodeToString([]byte("video")), 2},
		{"inline", true, http.StatusOK, false, base64.StdEncoding.EncodeToString([]byte("video")), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, closed := 0, 0
			adaptor := &GoogleAdaptor{}
			client, err := adaptor.getClient(context.Background(), &ProviderConfig{
				APIKey:  "test-key",
				Headers: map[string]string{"X-Tenant-ID": "tenant"},
				HTTPClient: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.Header.Get("X-Goog-Api-Key") != "test-key" || r.Header.Get("X-Tenant-ID") != "tenant" {
						t.Errorf("download headers = %v", r.Header)
					}
					status, header, body := tc.status, make(http.Header), "video"
					if tc.status == http.StatusForbidden {
						body = "forbidden"
					}
					if tc.redirect && calls == 1 {
						status = http.StatusFound
						header.Set("Location", "/video-result")
					}
					return &http.Response{StatusCode: status, Header: header, Body: &googleVideoTestBody{Reader: strings.NewReader(body), closed: &closed}, Request: r}, nil
				})},
			})
			if err != nil {
				t.Fatal(err)
			}
			video := &genai.Video{URI: "files/video-1"}
			if tc.inline {
				video.VideoBytes = []byte("video")
			}
			url, encoded, mimeType, err := adaptor.googleVideoPayload(context.Background(), client, &genai.GenerateVideosResponse{
				GeneratedVideos: []*genai.GeneratedVideo{{Video: video}},
			})
			if err != nil || url != video.URI || encoded != tc.wantBase64 || mimeType != "video/mp4" || calls != tc.wantCalls || closed != tc.wantCalls {
				t.Fatalf("url=%q base64=%q mime=%q calls=%d closed=%d error=%v", url, encoded, mimeType, calls, closed, err)
			}
			if !tc.inline && len(video.VideoBytes) != 0 {
				t.Fatal("download mutated the source video")
			}
		})
	}
}

type googleVideoTestBody struct {
	io.Reader
	closed *int
}

func (b *googleVideoTestBody) Close() error {
	*b.closed++
	return nil
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
