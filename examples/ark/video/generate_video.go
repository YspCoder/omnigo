package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/YspCoder/omnigo"
	"github.com/YspCoder/omnigo/dto"
)

func main() {
	apiKey := os.Getenv("ARK_API_KEY")
	if apiKey == "" {
		log.Fatal("ARK_API_KEY is not set")
	}
	model := os.Getenv("ARK_VIDEO_MODEL")
	if model == "" {
		log.Fatal("ARK_VIDEO_MODEL is not set")
	}

	// Use an enabled Ark video model or endpoint ID from your console.
	llm, err := omnigo.NewLLM(
		omnigo.SetProvider("ark"),
		omnigo.SetModel(model),
		omnigo.SetAPIKey(apiKey),
	)
	if err != nil {
		log.Fatalf("failed to create llm: %v", err)
	}

	req := &dto.MediaRequest{
		Type:   dto.MediaTypeVideo,
		Prompt: "A white rabbit in a suit working in a futuristic lab",
	}

	resp, err := llm.Media(context.Background(), req)
	if err != nil {
		log.Fatalf("video generation failed: %v", err)
	}

	fmt.Printf("Task ID: %s\nStatus: %s\n", resp.TaskID, resp.Status)
}
