package adapter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestGoogleVideoForwardsFramesAndReferences(t *testing.T) {
	png := []byte("\x89PNG fixture")
	inline := base64.StdEncoding.EncodeToString(png)
	for _, tc := range []struct {
		name  string
		extra map[string]interface{}
		check func(t *testing.T, instance map[string]any, parameters map[string]any)
	}{
		{"first_and_last_frame", map[string]interface{}{"first_frame": "/first.png", "last_frame": "data:image/jpeg;base64," + inline},
			func(t *testing.T, instance, _ map[string]any) {
				image, _ := instance["image"].(map[string]any)
				last, _ := instance["lastFrame"].(map[string]any)
				if image["bytesBase64Encoded"] != inline || image["mimeType"] != "image/png" {
					t.Fatalf("first frame not forwarded: %#v", instance)
				}
				if last["bytesBase64Encoded"] != inline || last["mimeType"] != "image/jpeg" {
					t.Fatalf("last frame not forwarded: %#v", instance)
				}
			}},
		{"reference_images", map[string]interface{}{"reference_images": []interface{}{"/a.png", "/b.png"}},
			func(t *testing.T, instance, _ map[string]any) {
				references, _ := instance["referenceImages"].([]any)
				if len(references) != 2 || instance["image"] != nil {
					t.Fatalf("reference images not forwarded: %#v", instance)
				}
				second, _ := references[1].(map[string]any)
				referenceType, _ := second["referenceType"].(string)
				if image, _ := second["image"].(map[string]any); !strings.EqualFold(referenceType, "asset") || image["bytesBase64Encoded"] != inline {
					t.Fatalf("unexpected reference image: %#v", second)
				}
			}},
		{"aspect_ratio", map[string]interface{}{},
			func(t *testing.T, instance, parameters map[string]any) {
				if parameters["aspectRatio"] != "9:16" || instance["image"] != nil {
					t.Fatalf("unexpected text-to-video request: %#v %#v", instance, parameters)
				}
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, ".png") {
					w.Header().Set("Content-Type", "image/png")
					w.Write(png)
					return
				}
				if !strings.HasSuffix(r.URL.Path, ":predictLongRunning") {
					t.Errorf("unexpected request %s", r.URL.Path)
				}
				raw, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(raw, &body); err != nil {
					t.Errorf("invalid request body: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"name":"operations/video-1","done":false}`)
			}))
			defer server.Close()
			extra := map[string]interface{}{}
			for key, value := range tc.extra {
				if s, ok := value.(string); ok && strings.HasPrefix(s, "/") {
					value = server.URL + s
				}
				if items, ok := value.([]interface{}); ok {
					for i, item := range items {
						if s, ok := item.(string); ok && strings.HasPrefix(s, "/") {
							items[i] = server.URL + s
						}
					}
				}
				extra[key] = value
			}
			cfg := &ProviderConfig{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
			_, err := (&GoogleAdaptor{}).Media(context.Background(), cfg, &dto.MediaRequest{
				Type: dto.MediaTypeVideo, Model: "veo-3.1-generate-001", Prompt: "hello", Size: "9:16", Extra: extra,
			})
			if err != nil {
				t.Fatal(err)
			}
			instances, _ := body["instances"].([]any)
			if len(instances) != 1 {
				t.Fatalf("unexpected request body: %#v", body)
			}
			parameters, _ := body["parameters"].(map[string]any)
			tc.check(t, instances[0].(map[string]any), parameters)
		})
	}
}

func TestGoogleVideoRejectsInvalidFrameCombinations(t *testing.T) {
	for name, extra := range map[string]map[string]interface{}{
		"last_without_first":    {"last_frame": "gs://b/last.png"},
		"references_and_frames": {"first_frame": "gs://b/first.png", "reference_images": []string{"gs://b/a.png"}},
		"too_many_references":   {"reference_images": []string{"gs://b/1.png", "gs://b/2.png", "gs://b/3.png", "gs://b/4.png"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := googleVideoFramesFromRequest(&dto.MediaRequest{Extra: extra}); err == nil {
				t.Fatal("invalid frame combination must be rejected before the upstream request")
			}
		})
	}
}
