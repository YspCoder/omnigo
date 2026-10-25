package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

const customNestedUsageResponse = `{"code":"success","message":"","data":{"id":8234,"task_id":"task_outer","status":"SUCCESS","result_url":"https://example.com/video.mp4","data":{"id":"task_inner","model":"doubao-seedance","usage":{"total_tokens":100858,"completion_tokens":100858},"status":"succeeded"}}}`

func TestCustomAdaptorMapsNestedUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(customNestedUsageResponse))
	}))
	defer server.Close()

	adaptor := &CustomAdaptor{}
	media, err := adaptor.Media(context.Background(), &ProviderConfig{BaseURL: server.URL + "/create"}, &dto.MediaRequest{
		Type: dto.MediaTypeVideo,
	})
	if err != nil {
		t.Fatalf("Media error = %v", err)
	}
	if media.Usage.TotalTokens != 100858 || media.Usage.CompletionTokens != 100858 {
		t.Fatalf("Media usage = %#v, want total/completion 100858", media.Usage)
	}

	status, err := adaptor.TaskStatus(context.Background(), &ProviderConfig{BaseURL: server.URL + "/tasks"}, "task_outer")
	if err != nil {
		t.Fatalf("TaskStatus error = %v", err)
	}
	if status.Usage == nil || status.Usage.TotalTokens != 100858 || status.Usage.CompletionTokens != 100858 {
		t.Fatalf("TaskStatus usage = %#v, want total/completion 100858", status.Usage)
	}
}

func TestCustomUsageFallbackAndNestedResult(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"empty outer", `{"usage":{},"data":{"data":{"id":"inner","status":"succeeded","url":"https://example.com/video.mp4","usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15}}}}`, 15},
		{"explicit outer zero", `{"usage":{"total_tokens":0},"data":{"id":"inner","status":"succeeded","url":"https://example.com/video.mp4","usage":{"total_tokens":15}}}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			response, err := (&CustomAdaptor{}).TaskStatus(context.Background(), streamTestConfig(tc.body, "application/json", &calls), "inner")
			if err != nil {
				t.Fatal(err)
			}
			if response.Output.TaskID != "inner" || response.Output.URL != "https://example.com/video.mp4" || response.Usage == nil || response.Usage.TotalTokens != tc.want {
				t.Fatalf("response lost nested result or usage: %+v", response)
			}
		})
	}
}
