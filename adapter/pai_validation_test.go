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

func TestUint64FromAnyRejectsInvalidFloatIDs(t *testing.T) {
	for _, value := range []float64{-1, 1.5} {
		if _, ok := uint64FromAny(value); ok {
			t.Errorf("uint64FromAny(%v) accepted invalid ID", value)
		}
	}
	if got, ok := uint64FromAny(float64(42)); !ok || got != 42 {
		t.Fatalf("uint64FromAny(42) = %d, %v", got, ok)
	}
}

func TestPaiUsageAcrossStepsAndStreams(t *testing.T) {
	for _, tc := range []struct {
		name, mode           string
		auto, failed, stream bool
	}{
		{"disabled mask", paiModeSwap, false, false, false},
		{"auxiliary usage", paiModeSwap, true, false, false},
		{"failed primary", paiModeSwap, true, true, false},
		{"synchronous mask stream", paiModeMaskSelection, false, false, true},
		{"failed primary stream", paiModeSwap, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			maskCalls, primaryCalls := 0, 0
			cfg := &ProviderConfig{BaseURL: "https://example.invalid", HTTPClient: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				status, body := http.StatusOK, `{"ErrCode":0,"Resp":{"video_id":99,"credits":5}}`
				if strings.HasSuffix(r.URL.Path, "/mask/selection") {
					maskCalls++
					body = `{"ErrCode":0,"Resp":{"keyframe_id":1,"keyframe_url":"https://example.invalid/frame.png","credits":2,"mask_info":[{"mask_id":"m"}]}}`
				} else {
					primaryCalls++
					var payload map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Fatal(err)
					}
					if _, exists := payload["auto_mask_selection"]; exists {
						t.Fatal("internal mask option was forwarded to the provider")
					}
					if tc.failed {
						status, body = http.StatusInternalServerError, `{"ErrCode":1,"ErrMsg":"generation unavailable"}`
					}
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}}
			request := &dto.MediaRequest{Type: dto.MediaTypeVideo, Extra: map[string]interface{}{
				"mode": tc.mode, "video_media_id": 11, "img_id": 22, "auto_mask_selection": tc.auto,
			}}
			var usage dto.Usage
			if tc.stream {
				stream, err := (&PaiAdaptor{}).StreamMedia(context.Background(), cfg, request)
				if err != nil {
					t.Fatal(err)
				}
				defer stream.Close()
				token, err := stream.Next(context.Background())
				if err != nil || token == nil || token.Usage == nil {
					t.Fatalf("first event lost usage: %+v, %v", token, err)
				}
				usage = *token.Usage
				if tc.failed {
					if token.Type != "usage" {
						t.Fatalf("failed creation event = %q", token.Type)
					}
					if _, err := stream.Next(context.Background()); err == nil || errors.Is(err, io.EOF) {
						t.Fatalf("missing creation failure: %v", err)
					}
				} else if token.Type != "url" || token.URL != "https://example.invalid/frame.png" || token.Metadata["response"] == nil {
					t.Fatalf("synchronous result was lost: %+v", token)
				}
				if _, err := stream.Next(context.Background()); !errors.Is(err, io.EOF) {
					t.Fatalf("terminal stream error = %v", err)
				}
			} else {
				response, err := (&PaiAdaptor{}).Media(context.Background(), cfg, request)
				if (err != nil) != tc.failed || response == nil {
					t.Fatalf("response=%+v error=%v", response, err)
				}
				usage = response.Usage
			}
			if tc.auto {
				var steps []paiUsageStep
				if err := json.Unmarshal(usage.Billing["steps"], &steps); err != nil || len(steps) != 1 || steps[0].Operation != paiModeMaskSelection || string(steps[0].Usage) != `{"credits":2}` {
					t.Fatalf("auxiliary usage = %s", usage.Billing["steps"])
				}
				if !strings.Contains(string(usage.RawUsage), `"operation":"mask-selection"`) {
					t.Fatalf("raw usage lost its source: %s", usage.RawUsage)
				}
			}
			if !tc.failed {
				want := "5"
				if tc.mode == paiModeMaskSelection {
					want = "2"
				}
				if string(usage.Billing["credits"]) != want {
					t.Fatalf("primary credits = %s, want %s", usage.Billing["credits"], want)
				}
			}
			wantMask, wantPrimary := 0, 1
			if tc.auto || tc.mode == paiModeMaskSelection {
				wantMask = 1
			}
			if tc.mode == paiModeMaskSelection {
				wantPrimary = 0
			}
			if maskCalls != wantMask || primaryCalls != wantPrimary {
				t.Fatalf("requests mask=%d primary=%d, want %d/%d", maskCalls, primaryCalls, wantMask, wantPrimary)
			}
		})
	}
}
