package adapter

import (
	"context"
	"testing"
)

func TestGoogleClientUsesConfiguredBaseURL(t *testing.T) {
	const baseURL = "https://gateway.example.com/gemini/"

	client, err := (&GoogleAdaptor{}).getClient(context.Background(), &ProviderConfig{
		APIKey:  "test-api-key",
		BaseURL: "  " + baseURL + "  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := client.ClientConfig().HTTPOptions.BaseURL; got != baseURL {
		t.Fatalf("base URL = %q, want %q", got, baseURL)
	}
}
