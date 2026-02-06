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
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		log.Fatal("GOOGLE_API_KEY is not set")
	}

	// Create LLM client for Google Imagen 3
	llm, err := omnigo.NewLLM(
		omnigo.SetProvider("google"),
		omnigo.SetModel("imagen-3.0-generate-001"), // Example Imagen 3 model
		omnigo.SetAPIKey(apiKey),
	)
	if err != nil {
		log.Fatalf("failed to create llm: %v", err)
	}

	req := &dto.MediaRequest{
		Type:   dto.MediaTypeImage,
		Prompt: "A futuristic white rabbit wearing a navy blue business suit, cinematic lighting",
		N:      1,
		Size:   "1:1", // Google Imagen uses aspect ratios like 1:1, 4:3, etc.
	}

	resp, err := llm.Media(context.Background(), req)
	if err != nil {
		log.Fatalf("image generation failed: %v", err)
	}

	if resp.URL != "" {
		fmt.Printf("Image Generated! URL/Data: %s...\n", resp.URL[:50])
	} else if resp.TaskID != "" {
		fmt.Printf("Async Task ID: %s (Status: %s)\n", resp.TaskID, resp.Status)
	}
}
