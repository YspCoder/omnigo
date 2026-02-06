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
	apiKey := os.Getenv("JIMENG_API_KEY")
	if apiKey == "" {
		log.Fatal("JIMENG_API_KEY is not set")
	}

	// Create LLM client for Jimeng
	// The Model field now correctly maps to Jimeng's req_key
	llm, err := omnigo.NewLLM(
		omnigo.SetProvider("jimeng"),
		omnigo.SetModel("jimeng_ti2v_v30_pro"), // Use specific model ID
		omnigo.SetAPIKey(apiKey),
	)
	if err != nil {
		log.Fatalf("failed to create llm: %v", err)
	}

	req := &dto.MediaRequest{
		Type:   dto.MediaTypeVideo,
		Prompt: "A white rabbit in a suit working in a futuristic lab",
		Extra: map[string]interface{}{
			"frames": 25, // Optional parameter for Jimeng
		},
	}

	resp, err := llm.Media(context.Background(), req)
	if err != nil {
		log.Fatalf("video generation failed: %v", err)
	}

	fmt.Printf("Task ID: %s\nStatus: %s\n", resp.TaskID, resp.Status)
}
