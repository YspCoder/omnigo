package adapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/YspCoder/omnigo/dto"
)

func TestOpenAIChatUsesResponsesAPIForFileMessages(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_123",
			"model":"gpt-4.1",
			"output_text":"结构化分集结果",
			"usage":{"input_tokens":11,"output_tokens":22,"total_tokens":33}
		}`))
	}))
	defer srv.Close()

	adaptor := &OpenAIAdaptor{}
	resp, err := adaptor.Chat(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, &dto.MediaRequest{
		Model: "gpt-4.1",
		Messages: []dto.Message{
			{Role: "system", Content: "你是拆解助手"},
			{Role: "user", Content: "请分析这份剧本", FileURL: "https://example.com/script.pdf", Name: "script.pdf"},
		},
		MaxTokens:   2048,
		Temperature: 0.3,
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if gotPath != "/responses" {
		t.Fatalf("path = %q, want /responses", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if resp.Text != "结构化分集结果" {
		t.Fatalf("resp.Text = %q", resp.Text)
	}
	input, ok := gotBody["input"].([]interface{})
	if !ok || len(input) != 2 {
		t.Fatalf("input = %#v", gotBody["input"])
	}
	second, _ := input[1].(map[string]interface{})
	content, _ := second["content"].([]interface{})
	if len(content) != 2 {
		t.Fatalf("content = %#v", second["content"])
	}
	foundFile := false
	for _, item := range content {
		part, _ := item.(map[string]interface{})
		if part["type"] == "input_file" && part["file_url"] == "https://example.com/script.pdf" {
			foundFile = true
		}
	}
	if !foundFile {
		t.Fatalf("expected input_file part, got %#v", content)
	}
}

func TestOpenAIChatUsesResponsesOutputContentFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"id":"resp_456",
			"model":"gpt-4.1",
			"output":[
				{
					"type":"message",
					"role":"assistant",
					"content":[
						{"type":"output_text","text":"第一段"},
						{"type":"output_text","text":"第二段"}
					]
				}
			]
		}`))
	}))
	defer srv.Close()

	adaptor := &OpenAIAdaptor{}
	resp, err := adaptor.Chat(context.Background(), &ProviderConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, &dto.MediaRequest{
		Model: "gpt-4.1",
		Messages: []dto.Message{
			{Role: "user", Content: "请读取文件", FileURL: "https://example.com/script.pdf", Name: "script.pdf"},
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if got := strings.TrimSpace(resp.Text); got != "第一段\n第二段" {
		t.Fatalf("resp.Text = %q", got)
	}
}
