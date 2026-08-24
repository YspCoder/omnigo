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
	accessKey := os.Getenv("JIMENG_ACCESS_KEY")
	secretKey := os.Getenv("JIMENG_SECRET_KEY")
	if accessKey == "" || secretKey == "" {
		log.Fatal("JIMENG_ACCESS_KEY or JIMENG_SECRET_KEY is not set")
	}

	// Create LLM client for Jimeng
	// The Model field now correctly maps to Jimeng's req_key
	llm, err := omnigo.NewLLM(
		omnigo.SetProvider("jimeng"),
		omnigo.SetModel("jimeng_ti2v_v30_pro"), // Use specific model ID
		omnigo.SetAccessKey(accessKey),
		omnigo.SetSecretKey(secretKey),
	)
	if err != nil {
		log.Fatalf("failed to create llm: %v", err)
	}

	req := &dto.MediaRequest{
		Type:     dto.MediaTypeVideo,
		Duration: 5,
		Messages: []dto.Message{{
			Role:    "user",
			Content: "A white rabbit in a suit working in a futuristic lab",
		}},
	}

	resp, err := llm.Media(context.Background(), req)
	if err != nil {
		log.Fatalf("video generation failed: %v", err)
	}

	fmt.Printf("Task ID: %s\nStatus: %s\n", resp.TaskID, resp.Status)
}
