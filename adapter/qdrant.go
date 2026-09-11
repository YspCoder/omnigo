package adapter

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// QdrantAdapter implements Qdrant's REST API.
type QdrantAdapter struct{ http *vectorHTTP }

// NewQdrantAdapter creates an adapter for a Qdrant REST endpoint.
func NewQdrantAdapter(config *VectorConfig) (*QdrantAdapter, error) {
	if config == nil {
		config = &VectorConfig{Endpoint: "http://localhost:6333"}
	}
	h, err := newVectorHTTP(config)
	if err != nil {
		return nil, err
	}
	return &QdrantAdapter{http: h}, nil
}

func NewQdrantAdaptor(config *VectorConfig) (*QdrantAdaptor, error) {
	return NewQdrantAdapter(config)
}

// QdrantAdaptor is an alias using the spelling used by existing adapters.
type QdrantAdaptor = QdrantAdapter

func (a *QdrantAdapter) auth() func(*http.Request) {
	return func(request *http.Request) {
		if request.Header.Get("api-key") == "" && a.http.config.APIKey != "" {
			request.Header.Set("api-key", a.http.config.APIKey)
		}
	}
}

func (a *QdrantAdapter) CreateCollection(ctx context.Context, collection *VectorCollection) error {
	if err := collection.Validate(); err != nil {
		return err
	}
	distance := distanceName(collection.Distance, string(VectorDistanceCosine))
	if distance == string(VectorDistanceEuclidean) {
		distance = "Euclid"
	} else if distance == string(VectorDistanceDot) {
		distance = "Dot"
	} else if distance == string(VectorDistanceManhattan) {
		distance = "Manhattan"
	} else if strings.EqualFold(distance, "cosine") {
		distance = "Cosine"
	}
	payload := map[string]interface{}{"vectors": map[string]interface{}{"size": collection.Dimension, "distance": distance}}
	return a.http.doOperation(ctx, "qdrant", "create_collection", http.MethodPut, "/collections/"+url.PathEscape(collection.Name), payload, nil, a.auth())
}

func (a *QdrantAdapter) Upsert(ctx context.Context, collection string, records []VectorRecord) error {
	if err := validateRecords(collection, records); err != nil {
		return err
	}
	points := make([]map[string]interface{}, 0, len(records))
	for _, record := range records {
		payload := cloneMetadata(record.Metadata)
		if record.Document != "" {
			payload["document"] = record.Document
		}
		points = append(points, map[string]interface{}{"id": record.ID, "vector": record.Vector, "payload": payload})
	}
	path := "/collections/" + url.PathEscape(collection) + "/points?wait=true"
	return a.http.doOperation(ctx, "qdrant", "upsert", http.MethodPut, path, map[string]interface{}{"points": points}, nil, a.auth())
}

func (a *QdrantAdapter) Search(ctx context.Context, request *VectorSearchRequest) ([]VectorMatch, error) {
	if err := validateSearchRequest(request); err != nil {
		return nil, err
	}
	payload := map[string]interface{}{
		"vector": request.Vector, "limit": defaultTopK(request.TopK),
		"with_payload": request.IncludeMetadata,
		"with_vector":  request.IncludeVector,
	}
	if request.Filter != nil {
		payload["filter"] = request.Filter
	}
	var response struct {
		Result []struct {
			ID      interface{}            `json:"id"`
			Score   float32                `json:"score"`
			Payload map[string]interface{} `json:"payload"`
			Vector  []float32              `json:"vector"`
		} `json:"result"`
	}
	path := "/collections/" + url.PathEscape(request.Collection) + "/points/search"
	if err := a.http.doOperation(ctx, "qdrant", "search", http.MethodPost, path, payload, &response, a.auth()); err != nil {
		return nil, err
	}
	results := make([]VectorMatch, 0, len(response.Result))
	for _, item := range response.Result {
		metadata, document := splitDocument(item.Payload)
		results = append(results, VectorMatch{ID: fmt.Sprint(item.ID), Score: item.Score, Vector: item.Vector, Metadata: metadata, Document: document})
	}
	return results, nil
}

func (a *QdrantAdapter) Delete(ctx context.Context, collection string, ids []string) error {
	if strings.TrimSpace(collection) == "" || len(ids) == 0 {
		return fmt.Errorf("%w: collection and at least one id are required", ErrVectorRequest)
	}
	path := "/collections/" + url.PathEscape(collection) + "/points?wait=true"
	return a.http.doOperation(ctx, "qdrant", "delete", http.MethodPost, path, map[string]interface{}{"points": ids}, nil, a.auth())
}

// QdrantCollectionPath is useful when callers need to compose provider URLs.
func QdrantCollectionPath(name string) string { return "/collections/" + url.PathEscape(name) }

func cloneMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return map[string]interface{}{}
	}
	copyMap := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		copyMap[key] = value
	}
	return copyMap
}

func splitDocument(metadata map[string]interface{}) (map[string]interface{}, string) {
	copyMap := cloneMetadata(metadata)
	document := ""
	if value, ok := copyMap["document"].(string); ok {
		document = value
		delete(copyMap, "document")
	}
	return copyMap, document
}

var _ VectorAdapter = (*QdrantAdapter)(nil)
