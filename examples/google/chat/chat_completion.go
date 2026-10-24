package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/YspCoder/omnigo"
)

func main() {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		log.Fatal("GOOGLE_API_KEY is not set")
	}
	model := os.Getenv("GOOGLE_MODEL")
	if model == "" {
		log.Fatal("GOOGLE_MODEL must name a model available to your account")
	}

	// Create LLM client for Google Gemini
	llm, err := omnigo.NewLLM(
		omnigo.SetProvider("google"),
		omnigo.SetModel(model),
		omnigo.SetAPIKey(apiKey),
	)
	if err != nil {
		log.Fatalf("failed to create llm: %v", err)
	}

	ctx := context.Background()
	prompt := omnigo.NewPrompt("Hello, who are you?")

	resp, err := llm.Generate(ctx, prompt)
	if err != nil {
		log.Fatalf("generate failed: %v", err)
	}

	fmt.Println("Response:", resp)
}
