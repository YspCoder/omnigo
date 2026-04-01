package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/YspCoder/omnigo"
	"github.com/YspCoder/omnigo/dto"
)

func main() {
	apiKey := os.Getenv("KLING_API_KEY")
	if apiKey == "" {
		log.Fatal("KLING_API_KEY is not set")
	}

	llm, err := omnigo.NewLLM(
		omnigo.SetProvider("kling"),
		omnigo.SetModel("kling-v2-new"), // 也可尝试 kling-image-o1
		omnigo.SetAPIKey(apiKey),
	)
	if err != nil {
		log.Fatalf("failed to create llm: %v", err)
	}

	req := &dto.MediaRequest{
		Type:  dto.MediaTypeImage,
		Model: "kling-v2-new",
		Messages: []dto.Message{{
			Role:    "user",
			Content: "一只穿着风衣的橘猫，站在上海外滩夜景前，电影感灯光，超清细节",
		}},
		N:    1,
		Size: "1:1",
		Extra: map[string]interface{}{
			"mode": "image-generation",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	resp, err := llm.Media(ctx, req)
	if err != nil {
		log.Fatalf("image generation failed: %v", err)
	}

	if resp.URL != "" {
		fmt.Printf("Image URL: %s\n", resp.URL)
		return
	}

	if resp.TaskID == "" {
		log.Fatalf("unexpected empty result: status=%s request_id=%s", resp.Status, resp.RequestID)
	}

	fmt.Printf("Task submitted: task_id=%s status=%s\n", resp.TaskID, resp.Status)
	statusQuery := map[string]string{
		"media_type":      "image",
		"generation_type": "text",
		"model":           req.Model,
	}
	for {
		status, err := llm.TaskStatus(ctx, resp.TaskID, statusQuery)
		if err != nil {
			log.Fatalf("query task status failed: %v", err)
		}

		fmt.Printf("task_status=%s\n", status.Output.TaskStatus)
		switch status.Output.TaskStatus {
		case "succeed":
			fmt.Printf("Image URL: %s\n", status.Output.URL)
			return
		case "failed":
			log.Fatalf("task failed: %s", status.Output.Message)
		}

		time.Sleep(2 * time.Second)
	}
}
