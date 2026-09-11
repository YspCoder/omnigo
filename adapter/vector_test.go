package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVectorFactoryAdaptersUseProviderDefaults(t *testing.T) {
	builders := map[string]func(*VectorConfig) (VectorAdapter, error){
		"qdrant":        func(c *VectorConfig) (VectorAdapter, error) { return NewQdrantAdapter(c) },
		"milvus":        func(c *VectorConfig) (VectorAdapter, error) { return NewMilvusAdapter(c) },
		"pinecone":      func(c *VectorConfig) (VectorAdapter, error) { return NewPineconeAdapter(c) },
		"weaviate":      func(c *VectorConfig) (VectorAdapter, error) { return NewWeaviateAdapter(c) },
		"chroma":        func(c *VectorConfig) (VectorAdapter, error) { return NewChromaAdapter(c) },
		"elasticsearch": func(c *VectorConfig) (VectorAdapter, error) { return NewElasticsearchAdapter(c) },
		"opensearch":    func(c *VectorConfig) (VectorAdapter, error) { return NewOpenSearchAdapter(c) },
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			adapter, err := build(&VectorConfig{Endpoint: "http://example.invalid"})
			if err != nil || adapter == nil {
				t.Fatalf("adapter=%T err=%v", adapter, err)
			}
		})
	}
}

func TestQdrantAdapterMapsRequestsAndResults(t *testing.T) {
	var requests []struct {
		Method string
		Path   string
		Body   map[string]interface{}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "secret" {
			t.Errorf("api-key=%q", r.Header.Get("api-key"))
		}
		body := map[string]interface{}{}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		requests = append(requests, struct {
			Method string
			Path   string
			Body   map[string]interface{}
		}{r.Method, r.URL.Path, body})
		if r.URL.Path == "/collections/articles/points/search" {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"result":[{"id":"a-1","score":0.95,"payload":{"lang":"zh","document":"hello"},"vector":[1,2]}]}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"ok"}`)
	}))
	defer server.Close()
	client, err := NewQdrantAdapter(&VectorConfig{Endpoint: server.URL, APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := client.CreateCollection(ctx, &VectorCollection{Name: "articles", Dimension: 2, Distance: VectorDistanceCosine}); err != nil {
		t.Fatal(err)
	}
	if err := client.Upsert(ctx, "articles", []VectorRecord{{ID: "a-1", Vector: []float32{1, 2}, Document: "hello"}}); err != nil {
		t.Fatal(err)
	}
	results, err := client.Search(ctx, &VectorSearchRequest{Collection: "articles", Vector: []float32{1, 2}, TopK: 3, IncludeMetadata: true})
	if err != nil || len(results) != 1 || results[0].ID != "a-1" || results[0].Document != "hello" {
		t.Fatalf("results=%+v err=%v", results, err)
	}
	if err := client.Delete(ctx, "articles", []string{"a-1"}); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 4 || requests[0].Path != "/collections/articles" || requests[1].Path != "/collections/articles/points" || requests[2].Path != "/collections/articles/points/search" {
		t.Fatalf("requests=%+v", requests)
	}
	for _, request := range requests {
		if request.Body == nil {
			t.Fatal("request body missing")
		}
	}
}

func TestChromaRemembersCollectionID(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/collections":
			io.WriteString(w, `{"id":"collection-id","name":"docs"}`)
		case "/api/v1/collections/collection-id/query":
			io.WriteString(w, `{"ids":[["doc-1"]],"distances":[[0.2]],"metadatas":[[{"source":"test"}]],"documents":[["text"]]}`)
		default:
			io.WriteString(w, `{}`)
		}
	}))
	defer server.Close()
	client, err := NewChromaAdapter(&VectorConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CreateCollection(context.Background(), &VectorCollection{Name: "docs"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Search(context.Background(), &VectorSearchRequest{Collection: "docs", Vector: []float32{0.1}}); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[1] != "/api/v1/collections/collection-id/query" {
		t.Fatalf("paths=%v", paths)
	}
}

func TestVectorValidationAndAPIError(t *testing.T) {
	client, err := NewQdrantAdapter(&VectorConfig{Endpoint: "http://example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Upsert(context.Background(), "", nil); !errors.Is(err, ErrVectorRequest) {
		t.Fatalf("validation error=%v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"unauthorized"}`)
	}))
	defer server.Close()
	client, err = NewQdrantAdapter(&VectorConfig{Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	err = client.Delete(context.Background(), "docs", []string{"id"})
	var apiErr *VectorAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized || !strings.Contains(apiErr.Body, "unauthorized") {
		t.Fatalf("api error=%T %v", err, err)
	}
}

func TestVectorAdapterSupportsFixedPushURLWithoutBaseEndpoint(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{}`)
	}))
	defer server.Close()
	client, err := NewQdrantAdapter(&VectorConfig{PushURL: server.URL + "/ingest/fixed"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Upsert(context.Background(), "ignored-by-fixed-url", []VectorRecord{{ID: "1", Vector: []float32{1}}}); err != nil {
		t.Fatal(err)
	}
	if path != "/ingest/fixed" {
		t.Fatalf("path=%q", path)
	}
}

func TestFixedPushURLWorksForAllAdapters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	providers := []VectorProvider{VectorProviderQdrant, VectorProviderMilvus, VectorProviderPinecone, VectorProviderWeaviate, VectorProviderChroma, VectorProviderElasticsearch, VectorProviderOpenSearch}
	for _, provider := range providers {
		if _, err := NewVectorAdapter(provider, &VectorConfig{PushURL: server.URL + "/fixed-push"}); err != nil {
			t.Errorf("provider %s: %v", provider, err)
		}
	}
}
