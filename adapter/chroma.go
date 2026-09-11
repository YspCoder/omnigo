package adapter

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// ChromaAdapter implements Chroma's v1 REST API. Collection names are
// accepted by all methods; the adapter remembers IDs returned by creation.
type ChromaAdapter struct {
	http         *vectorHTTP
	mu           sync.RWMutex
	collectionID map[string]string
}

func NewChromaAdapter(config *VectorConfig) (*ChromaAdapter, error) {
	if config == nil {
		config = &VectorConfig{Endpoint: "http://localhost:8000"}
	}
	copyConfig := *config
	base := copyConfig.Endpoint
	if strings.TrimSpace(base) == "" {
		base = copyConfig.BaseURL
	}
	baseProvided := strings.TrimSpace(base) != ""
	if !baseProvided {
		base = firstConfiguredOperationURL(copyConfig)
	}
	base = strings.TrimRight(base, "/")
	if baseProvided && !strings.HasSuffix(base, "/api/v1") {
		copyConfig.Endpoint = base + "/api/v1"
	} else {
		copyConfig.Endpoint = base
	}
	h, err := newVectorHTTP(&copyConfig)
	if err != nil {
		return nil, err
	}
	return &ChromaAdapter{http: h, collectionID: make(map[string]string)}, nil
}

func NewChromaAdaptor(config *VectorConfig) (*ChromaAdaptor, error) {
	return NewChromaAdapter(config)
}

type ChromaAdaptor = ChromaAdapter

func (a *ChromaAdapter) auth() func(*http.Request) {
	return func(request *http.Request) {
		if request.Header.Get("X-Chroma-Token") == "" {
			token := a.http.config.Token
			if token == "" {
				token = a.http.config.APIKey
			}
			if token != "" {
				request.Header.Set("X-Chroma-Token", token)
			}
		}
	}
}

func (a *ChromaAdapter) CreateCollection(ctx context.Context, collection *VectorCollection) error {
	if err := validateCollectionName(collection); err != nil {
		return err
	}
	payload := map[string]interface{}{"name": collection.Name, "get_or_create": true}
	if len(collection.Metadata) > 0 {
		payload["metadata"] = collection.Metadata
	}
	var response struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := a.http.doOperation(ctx, "chroma", "create_collection", http.MethodPost, "/collections", payload, &response, a.auth()); err != nil {
		return err
	}
	id := response.ID
	if id == "" {
		id = collection.Name
	}
	a.mu.Lock()
	a.collectionID[collection.Name] = id
	a.mu.Unlock()
	return nil
}

func (a *ChromaAdapter) collectionPath(name string) string {
	a.mu.RLock()
	id := a.collectionID[name]
	a.mu.RUnlock()
	if id == "" {
		id = name
	}
	return "/collections/" + url.PathEscape(id)
}

func (a *ChromaAdapter) Upsert(ctx context.Context, collection string, records []VectorRecord) error {
	if err := validateRecords(collection, records); err != nil {
		return err
	}
	ids := make([]string, 0, len(records))
	embeddings := make([][]float32, 0, len(records))
	metadatas := make([]map[string]interface{}, 0, len(records))
	documents := make([]string, 0, len(records))
	hasMetadata := false
	hasDocument := false
	for _, record := range records {
		ids = append(ids, record.ID)
		embeddings = append(embeddings, record.Vector)
		metadata := cloneMetadata(record.Metadata)
		if len(metadata) == 0 {
			metadata = nil
		}
		metadatas = append(metadatas, metadata)
		if len(metadata) > 0 {
			hasMetadata = true
		}
		documents = append(documents, record.Document)
		if record.Document != "" {
			hasDocument = true
		}
	}
	payload := map[string]interface{}{"ids": ids, "embeddings": embeddings}
	if hasMetadata {
		payload["metadatas"] = metadatas
	}
	if hasDocument {
		payload["documents"] = documents
	}
	return a.http.doOperation(ctx, "chroma", "upsert", http.MethodPost, a.collectionPath(collection)+"/upsert", payload, nil, a.auth())
}

func (a *ChromaAdapter) Search(ctx context.Context, request *VectorSearchRequest) ([]VectorMatch, error) {
	if err := validateSearchRequest(request); err != nil {
		return nil, err
	}
	payload := map[string]interface{}{
		"query_embeddings": [][]float32{request.Vector}, "n_results": defaultTopK(request.TopK),
		"include": []string{"distances", "metadatas", "documents"},
	}
	if request.Filter != nil {
		payload["where"] = request.Filter
	}
	var response struct {
		IDs       [][]string                 `json:"ids"`
		Distances [][]float32                `json:"distances"`
		Metadatas [][]map[string]interface{} `json:"metadatas"`
		Documents [][]string                 `json:"documents"`
	}
	if err := a.http.doOperation(ctx, "chroma", "search", http.MethodPost, a.collectionPath(request.Collection)+"/query", payload, &response, a.auth()); err != nil {
		return nil, err
	}
	results := make([]VectorMatch, 0)
	if len(response.IDs) == 0 {
		return results, nil
	}
	for i, id := range response.IDs[0] {
		var score float32
		if len(response.Distances) > 0 && i < len(response.Distances[0]) {
			score = response.Distances[0][i]
		}
		var metadata map[string]interface{}
		if len(response.Metadatas) > 0 && i < len(response.Metadatas[0]) {
			metadata = response.Metadatas[0][i]
		}
		document := ""
		if len(response.Documents) > 0 && i < len(response.Documents[0]) {
			document = response.Documents[0][i]
		}
		results = append(results, VectorMatch{ID: id, Score: score, Metadata: metadata, Document: document})
	}
	return results, nil
}

func (a *ChromaAdapter) Delete(ctx context.Context, collection string, ids []string) error {
	if strings.TrimSpace(collection) == "" || len(ids) == 0 {
		return fmt.Errorf("%w: collection and at least one id are required", ErrVectorRequest)
	}
	return a.http.doOperation(ctx, "chroma", "delete", http.MethodPost, a.collectionPath(collection)+"/delete", map[string]interface{}{"ids": ids}, nil, a.auth())
}

var _ VectorAdapter = (*ChromaAdapter)(nil)
