package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestArkTaskStatusPreservesUsageAndHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tenant-ID") != "tenant-1" || r.Header.Get("Authorization") != "Bearer override-key" {
			t.Errorf("headers = %v", r.Header)
		}
		status := "succeeded"
		if strings.HasSuffix(r.URL.Path, "/failed") {
			status = "failed"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": status, "status": status, "duration": 5,
			"usage": map[string]int{"prompt_tokens": 12, "completion_tokens": 100858, "total_tokens": 100870},
		})
	}))
	defer server.Close()
	cfg := &ProviderConfig{
		APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client(),
		Headers: map[string]string{"X-Tenant-ID": "tenant-1", "Authorization": "Bearer override-key"},
	}
	if err := PrepareProviderConfig(cfg); err != nil {
		t.Fatal(err)
	}
	sharedClient, sharedTransport := cfg.HTTPClient, cfg.HTTPClient.Transport
	adaptor := &ArkAdaptor{}
	for _, status := range []string{"succeeded", "failed"} {
		response, err := adaptor.TaskStatus(context.Background(), cfg, status)
		if err != nil {
			t.Fatal(err)
		}
		if response.Usage == nil || response.Usage.PromptTokens != 12 || response.Usage.CompletionTokens != 100858 || response.Usage.TotalTokens != 100870 || len(response.Usage.RawUsage) == 0 {
			t.Fatalf("task %s lost reported usage: %+v", status, response.Usage)
		}
		if status == "succeeded" && (response.Usage.VideoDuration != 5 || response.Usage.VideoCount != 1) {
			t.Fatalf("successful task lost media usage: %+v", response.Usage)
		}
		if status == "failed" && response.Usage.VideoCount != 0 {
			t.Fatalf("failed task invented a generated video: %+v", response.Usage)
		}
		if cfg.HTTPClient != sharedClient || cfg.HTTPClient.Transport != sharedTransport {
			t.Fatal("Ark header wrapping modified the shared HTTP client")
		}
		cfg.Headers["X-Tenant-ID"] = "changed-tenant"
	}
}

func TestNativeChatTemperature(t *testing.T) {
	for _, tc := range []struct {
		name        string
		temperature float64
		options     map[string]interface{}
		want        interface{}
	}{
		{"omitted", 0, nil, nil},
		{"configured zero", 0, map[string]interface{}{"temperature": float64(0)}, float64(0)},
		{"explicit zero", 0.7, map[string]interface{}{"temperature": 0}, float64(0)},
		{"nonzero", 0.4, nil, float64(0.4)},
		{"json number", 0.7, map[string]interface{}{"temperature": json.Number("0.3")}, float64(0.3)},
		{"invalid option", 0.4, map[string]interface{}{"temperature": nil}, float64(0.4)},
	} {
		for _, provider := range []string{"ark", "google"} {
			t.Run(provider+"/"+tc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var request map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					parameters := request
					if provider == "google" {
						parameters, _ = request["generationConfig"].(map[string]interface{})
					}
					if got := parameters["temperature"]; got != tc.want {
						t.Errorf("temperature = %#v, want %#v", got, tc.want)
					}
					w.Header().Set("Content-Type", "application/json")
					if provider == "google" {
						_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}`))
					} else {
						_, _ = w.Write([]byte(`{"id":"resp-1","model":"test-model","status":"completed","output":[]}`))
					}
				}))
				defer server.Close()
				var adaptor Adaptor = &ArkAdaptor{}
				if provider == "google" {
					adaptor = &GoogleAdaptor{}
				}
				_, err := adaptor.Chat(context.Background(), &ProviderConfig{
					APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client(),
				}, &dto.MediaRequest{Model: "test-model", Temperature: tc.temperature, Options: tc.options})
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
