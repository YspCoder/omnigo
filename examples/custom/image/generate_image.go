package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/YspCoder/omnigo"
	"github.com/YspCoder/omnigo/dto"
)

const createImageEndpoint = "https://ai.xxxx.cn/v1/images/generations"

func main() {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY is not set")
	}

	client, err := omnigo.NewLLM(
		omnigo.SetProvider("custom"),
		omnigo.SetModel("nano-banana-pro-1k"),
		omnigo.SetEndpoint(createImageEndpoint),
		omnigo.SetAPIKey(apiKey),
	)
	if err != nil {
		log.Fatalf("create LLM failed: %v", err)
	}

	ctx := context.Background()
	created, err := client.Media(ctx, &dto.MediaRequest{
		Type:       dto.MediaTypeImage,
		Prompt:     "电影感城市夜景",
		Resolution: "1K",
		Extra: map[string]interface{}{
			"aspect_ratio": "16:9",
			"images":       []string{"https://example.com/reference.png"},
		},
	})
	if err != nil {
		log.Fatalf("submit image failed: %v", err)
	}
	log.Printf("task submitted: id=%s status=%s", created.TaskID, created.Status)

	for {
		status, err := client.TaskStatus(ctx, created.TaskID)
		if err != nil {
			log.Fatalf("query task failed: %v", err)
		}
		normalized, err := dto.NormalizeTaskStatus(status.Output.TaskStatus)
		if err != nil {
			log.Fatalf("unsupported task status %q: %v", status.Output.TaskStatus, err)
		}
		switch normalized {
		case dto.TaskStatusSucceeded:
			log.Println("image_url:", status.Output.URL)
			return
		case dto.TaskStatusFailed:
			log.Fatalf("image failed: code=%s message=%s", status.Output.Code, status.Output.Message)
		case dto.TaskStatusQueued, dto.TaskStatusInProgress:
			log.Println("status:", status.Output.TaskStatus)
			time.Sleep(5 * time.Second)
		}
	}
}
