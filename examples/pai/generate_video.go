package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/YspCoder/omnigo"
	"github.com/YspCoder/omnigo/dto"
)

func main() {
	apiKey := os.Getenv("PAI_API_KEY")
	if apiKey == "" {
		log.Fatal("PAI_API_KEY is not set")
	}

	llm, err := omnigo.NewLLM(
		omnigo.SetProvider("pai"),
		omnigo.SetModel("v6"),
		omnigo.SetAPIKey(apiKey),
	)
	if err != nil {
		log.Fatalf("create LLM failed: %v", err)
	}

	req := &dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Model:    "v6",
		Size:     "16:9",
		Duration: 5,
		Messages: []dto.Message{{Role: "user", Content: "一只机械狐狸在雪夜森林里奔跑，镜头跟拍"}},
		Extra: map[string]interface{}{
			"quality":    "540p",
			"water_mark": false,
		},
	}

	resp, err := llm.Media(context.Background(), req)
	if err != nil {
		log.Fatalf("video generation failed: %v", err)
	}
	log.Printf("Task submitted. ID=%s Status=%s", resp.TaskID, resp.Status)

	for {
		status, err := llm.TaskStatus(context.Background(), resp.TaskID)
		if err != nil {
			log.Fatalf("query task failed: %v", err)
		}
		log.Println("status:", status.Output.TaskStatus)
		if status.Output.TaskStatus == "success" {
			log.Println("video_url:", status.Output.VideoURL)
			break
		}
		if status.Output.TaskStatus == "failed" || status.Output.TaskStatus == "rejected" {
			log.Fatalf("video failed: %s", status.Output.Message)
		}
		time.Sleep(5 * time.Second)
	}
}
