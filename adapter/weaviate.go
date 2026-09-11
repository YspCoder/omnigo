package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// WeaviateAdapter implements Weaviate's v1 REST and GraphQL APIs. A schema
// class is used as the collection name.
type WeaviateAdapter struct{ http *vectorHTTP }

func NewWeaviateAdapter(config *VectorConfig) (*WeaviateAdapter, error) {
	if config == nil {
		config = &VectorConfig{Endpoint: "http://localhost:8080"}
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
	if baseProvided && !strings.HasSuffix(strings.TrimRight(base, "/"), "/v1") {
		base = strings.TrimRight(base, "/") + "/v1"
	}
	copyConfig.Endpoint = base
	h, err := newVectorHTTP(&copyConfig)
	if err != nil {
		return nil, err
	}
	return &WeaviateAdapter{http: h}, nil
}

func NewWeaviateAdaptor(config *VectorConfig) (*WeaviateAdaptor, error) {
	return NewWeaviateAdapter(config)
}

type WeaviateAdaptor = WeaviateAdapter

func (a *WeaviateAdapter) CreateCollection(ctx context.Context, collection *VectorCollection) error {
	if err := validateCollectionName(collection); err != nil {
		return err
	}
	if !validateIdentifier(collection.Name) {
		return fmt.Errorf("%w: weaviate class name must contain only letters, digits, and underscores", ErrVectorRequest)
	}
	class := map[string]interface{}{
		"class": collection.Name, "vectorizer": "none",
		"properties": []map[string]interface{}{{"name": "document", "dataType": []string{"text"}}},
	}
	if len(collection.Metadata) > 0 {
		if properties, ok := collection.Metadata["properties"]; ok {
			class["properties"] = properties
		}
	}
	if collection.Distance != "" {
		distance := distanceName(collection.Distance, string(VectorDistanceCosine))
		if distance == string(VectorDistanceEuclidean) {
			distance = "l2-squared"
		}
		class["vectorIndexConfig"] = map[string]interface{}{"distance": distance}
	}
	return a.http.doOperation(ctx, "weaviate", "create_collection", http.MethodPost, "/schema", class, nil, bearerAuth(a.http.config))
}

func (a *WeaviateAdapter) Upsert(ctx context.Context, collection string, records []VectorRecord) error {
	if err := validateRecords(collection, records); err != nil {
		return err
	}
	for _, record := range records {
		properties := cloneMetadata(record.Metadata)
		if record.Document != "" {
			properties["document"] = record.Document
		}
		payload := map[string]interface{}{"class": collection, "id": record.ID, "properties": properties, "vector": record.Vector}
		path := "/objects/" + url.PathEscape(record.ID)
		if err := a.http.doOperation(ctx, "weaviate", "upsert", http.MethodPut, path, payload, nil, bearerAuth(a.http.config)); err != nil {
			return err
		}
	}
	return nil
}

func (a *WeaviateAdapter) Search(ctx context.Context, request *VectorSearchRequest) ([]VectorMatch, error) {
	if err := validateSearchRequest(request); err != nil {
		return nil, err
	}
	if !validateIdentifier(request.Collection) {
		return nil, fmt.Errorf("%w: weaviate class name must contain only letters, digits, and underscores", ErrVectorRequest)
	}
	vector, err := json.Marshal(request.Vector)
	if err != nil {
		return nil, fmt.Errorf("encode weaviate vector: %w", err)
	}
	additionalFields := "id distance"
	if request.IncludeVector {
		additionalFields += " vector"
	}
	additional := "_additional { " + additionalFields + " }"
	filter := ""
	if request.Filter != nil {
		where, marshalErr := json.Marshal(request.Filter)
		if marshalErr != nil {
			return nil, fmt.Errorf("encode weaviate filter: %w", marshalErr)
		}
		filter = " where: " + string(where)
	}
	query := fmt.Sprintf("{ Get { %s(nearVector: {vector: %s}%s limit: %d) { %s } } }", request.Collection, vector, filter, defaultTopK(request.TopK), additional)
	var response struct {
		Data struct {
			Get map[string][]struct {
				Additional struct {
					ID       string    `json:"id"`
					Distance float32   `json:"distance"`
					Vector   []float32 `json:"vector"`
				} `json:"_additional"`
			} `json:"Get"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := a.http.doOperation(ctx, "weaviate", "search", http.MethodPost, "/graphql", map[string]string{"query": query}, &response, bearerAuth(a.http.config)); err != nil {
		return nil, err
	}
	if len(response.Errors) > 0 {
		return nil, fmt.Errorf("weaviate graphql error: %s", response.Errors[0].Message)
	}
	rows := response.Data.Get[request.Collection]
	results := make([]VectorMatch, 0, len(rows))
	for _, row := range rows {
		results = append(results, VectorMatch{ID: row.Additional.ID, Score: row.Additional.Distance, Vector: row.Additional.Vector})
	}
	return results, nil
}

func (a *WeaviateAdapter) Delete(ctx context.Context, collection string, ids []string) error {
	if strings.TrimSpace(collection) == "" || len(ids) == 0 {
		return fmt.Errorf("%w: collection and at least one id are required", ErrVectorRequest)
	}
	for _, id := range ids {
		if err := a.http.doOperation(ctx, "weaviate", "delete", http.MethodDelete, "/objects/"+url.PathEscape(id), nil, nil, bearerAuth(a.http.config)); err != nil {
			return err
		}
	}
	return nil
}

var _ VectorAdapter = (*WeaviateAdapter)(nil)
