package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/YspCoder/omnigo"
	"github.com/YspCoder/omnigo/dto"
)

const createVideoEndpoint = "https://ai.xxxx.cn/v1/videos"

func main() {
	apiKey := os.Getenv("CANGYUAN_API_KEY")
	if apiKey == "" {
		log.Fatal("CANGYUAN_API_KEY is not set")
	}

	client, err := omnigo.NewLLM(
		omnigo.SetProvider("custom"),
		omnigo.SetModel("seedance-2.0-fast-480p"),
		omnigo.SetEndpoint(createVideoEndpoint),
		omnigo.SetAPIKey(apiKey),
	)
	if err != nil {
		log.Fatalf("create LLM failed: %v", err)
	}

	ctx := context.Background()
	resp, err := client.Media(ctx, &dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Prompt:   "雨夜霓虹街道，镜头缓慢推进，电影感光影",
		Duration: 8,
		Extra: map[string]interface{}{
			"aspect_ratio": "16:9",
		},
	})
	if err != nil {
		log.Fatalf("submit video failed: %v", err)
	}
	log.Printf("task submitted: id=%s status=%s", resp.TaskID, resp.Status)

	for {
		status, err := client.TaskStatus(ctx, resp.TaskID)
		if err != nil {
			log.Fatalf("query task failed: %v", err)
		}
		switch status.Output.TaskStatus {
		case "completed":
			log.Println("video_url:", status.Output.VideoURL)
			return
		case "failed":
			log.Fatalf("video failed: code=%s message=%s", status.Output.Code, status.Output.Message)
		default:
			log.Println("status:", status.Output.TaskStatus)
			time.Sleep(5 * time.Second)
		}
	}
}
