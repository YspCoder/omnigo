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

	// Create LLM client for Google Gemini
	llm, err := omnigo.NewLLM(
		omnigo.SetProvider("google"),
		omnigo.SetModel("gemini-2.0-flash-exp"), // Supporting the latest models
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
