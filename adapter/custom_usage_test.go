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
