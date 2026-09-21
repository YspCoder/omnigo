package adapter

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGoogleFetchImageRejectsOversizeContentLength(t *testing.T) {
	client := &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 50<<20 + 1,
			Body:          io.NopCloser(strings.NewReader("small body")),
			Request:       r,
		}, nil
	})}
	if _, _, err := googleFetchImageBytes(context.Background(), client, "https://example.test/image.png"); err == nil {
		t.Fatal("expected oversized image to be rejected")
	}
}

func TestPaiReadImageInputRejectsOversizeContentLength(t *testing.T) {
	client := &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 50<<20 + 1,
			Body:          io.NopCloser(strings.NewReader("small body")),
			Request:       r,
		}, nil
	})}
	if _, _, _, err := paiReadImageInput(context.Background(), client, "https://example.test/image.png"); err == nil {
		t.Fatal("expected oversized image to be rejected")
	}
}
