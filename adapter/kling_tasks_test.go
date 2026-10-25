package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestKlingTasksProtocol(t *testing.T) {
	const task = `{"id":"task/1","status":"succeeded","create_time":1781080778802,"update_time":1781080794151,"outputs":[{"type":"video","url":"https://example.com/one.mp4"},{"type":"image","url":"https://example.com/two.png"}],"billing":[{"charge_type":"cash","amount":"0.123456789","currency":"CNY"}],"usage":{"input_duration":"8.03"}}`
	var create, list map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/text-to-video/kling-3.0" || r.URL.Path == "/image-to-video/kling-2.6":
			if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
				t.Error(err)
			}
			fmt.Fprint(w, `{"code":0,"request_id":"req_create","data":{"id":"task/1","status":"submitted"}}`)
		case r.URL.Path == "/tasks" && r.Method == http.MethodGet:
			if got := r.URL.Query().Get("task_ids"); got != "task/1" {
				t.Errorf("task_ids = %q", got)
			}
			fmt.Fprint(w, `{"code":0,"request_id":"req_query","data":[`+task+`]}`)
		case r.URL.Path == "/tasks" && r.Method == http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
				t.Error(err)
			}
			fmt.Fprint(w, `{"code":0,"data":{"result":[`+task+`],"count":1,"has_more":true,"next_cursor":"next_page"}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	config := &ProviderConfig{BaseURL: server.URL, HTTPClient: server.Client()}
	adaptor := &KlingAdaptor{}
	settings := map[string]any{"audio": "native"}
	request := &dto.MediaRequest{Type: dto.MediaTypeVideo, Model: "kling-3.0", Prompt: "hello",
		Duration: "5", Resolution: "1080p", Extra: map[string]any{"protocol": "tasks", "settings": settings,
			"callback_url": "https://example.com/callback"}}
	created, err := adaptor.Media(context.Background(), config, request)
	if err != nil || created.TaskID != "task/1" {
		t.Fatalf("create = %+v, error = %v", created, err)
	}
	if create["prompt"] != "hello" || create["settings"].(map[string]any)["duration"] != float64(5) || create["options"].(map[string]any)["callback_url"] != "https://example.com/callback" || len(settings) != 1 {
		t.Fatalf("payload = %v; caller settings = %v", create, settings)
	}
	if _, exists := create["protocol"]; exists {
		t.Fatal("local protocol was sent upstream")
	}
	request.Model = "ignored-for-explicit-endpoint"
	request.Extra["endpoint"] = "/image-to-video/kling-2.6"
	request.Messages = []dto.Message{{Role: "user", ImageURL: "https://example.com/start.png"}, {Role: "user", ImageURL: "https://example.com/end.png"}}
	if _, err := adaptor.Media(context.Background(), config, request); err != nil {
		t.Fatal(err)
	}
	contents := create["contents"].([]any)
	if len(contents) != 3 || contents[0].(map[string]any)["type"] != "prompt" || contents[1].(map[string]any)["type"] != "first_frame" || contents[2].(map[string]any)["type"] != "last_frame" {
		t.Fatalf("image contents = %v", contents)
	}
	status, err := adaptor.TaskStatus(context.Background(), config, "task/1", map[string]string{"protocol": "tasks"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Output.TaskStatus != "succeeded" || len(status.Output.Outputs) != 2 || status.Output.VideoURL != "https://example.com/one.mp4" || status.Usage.TotalTokens != 0 {
		t.Fatalf("status = %+v", status)
	}
	if string(status.Usage.Billing["billing"]) != `[{"charge_type":"cash","amount":"0.123456789","currency":"CNY"}]` || string(status.Usage.Billing["usage"]) != `{"input_duration":"8.03"}` {
		t.Fatalf("billing = %+v", status.Usage)
	}
	page, err := adaptor.ListTasks(context.Background(), config, map[string]string{"protocol": "tasks", "cursor": "prior", "limit": "25", "filters": `[{"key":"status","values":["succeeded"]}]`})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor != "next_page" || !page.HasMore || page.Total != 1 || page.Items[0].Usage == nil || list["limit"] != float64(25) || list["cursor"] != "prior" {
		t.Fatalf("page = %+v, request = %v", page, list)
	}
	stream, err := adaptor.StreamMedia(context.Background(), config, request)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	token, err := stream.Next(context.Background())
	if err != nil || token.Type != "url" || token.URL != status.Output.VideoURL || token.Usage == nil {
		t.Fatalf("terminal token = %+v, error = %v", token, err)
	}
	if _, err := stream.Next(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("after terminal = %v", err)
	}
}

func TestKlingTasksFailureRetainsUsage(t *testing.T) {
	calls := 0
	config := streamTestConfig(`{"code":0,"data":[{"id":"task-1","status":"failed","message":"rejected","billing":[{"charge_type":"unit","amount":"1.25","package_type":"video"}]}]}`, "application/json", &calls)
	stream := &klingPollingStream{adaptor: &KlingAdaptor{}, cfg: config, taskID: "task-1", query: map[string]string{"protocol": "tasks"}}
	token, err := stream.Next(context.Background())
	if err != nil || token.Usage == nil || token.Usage.TotalTokens != 0 || token.Type != "usage" {
		t.Fatalf("failure usage = %+v, error = %v", token, err)
	}
	if _, err := stream.Next(context.Background()); err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("failure error = %v", err)
	}
	if _, err := stream.Next(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("after failure = %v", err)
	}
}
