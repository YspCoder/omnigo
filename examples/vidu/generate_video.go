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
	apiKey := os.Getenv("VIDU_API_KEY")
	if apiKey == "" {
		log.Fatal("VIDU_API_KEY is not set")
	}

	llm, err := omnigo.NewLLM(
		omnigo.SetProvider("vidu"),
		omnigo.SetModel("viduq2"),
		omnigo.SetAPIKey(apiKey),
	)
	if err != nil {
		log.Fatalf("create LLM failed: %v", err)
	}

	req := &dto.MediaRequest{
		Type:       dto.MediaTypeVideo,
		Model:      "viduq2",
		Messages:   []dto.Message{{Role: "user", Content: "一个机器人站在雨夜霓虹街头，镜头缓慢推进"}},
		Size:       "16:9",
		Duration:   5,
		Resolution: "720p",
		Extra: map[string]interface{}{
			"mode":               "text-to-video",
			"movement_amplitude": "medium",
			"bgm":                true,
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
		if status.Output.TaskStatus == "failed" {
			log.Fatalf("video failed: %s", status.Output.Message)
		}
		time.Sleep(5 * time.Second)
	}
}
