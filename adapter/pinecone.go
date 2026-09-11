package adapter

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// PineconeAdapter implements the Pinecone data-plane REST API. Endpoint must
// be the index host for vector operations (for example, https://INDEX-....svc).
type PineconeAdapter struct {
	http    *vectorHTTP
	control *vectorHTTP
}

func NewPineconeAdapter(config *VectorConfig) (*PineconeAdapter, error) {
	if config == nil {
		config = &VectorConfig{Endpoint: "https://api.pinecone.io"}
	}
	h, err := newVectorHTTP(config)
	if err != nil {
		return nil, err
	}
	control := h
	if config != nil && config.ControlPlaneEndpoint != "" {
		controlConfig := *config
		controlConfig.Endpoint = config.ControlPlaneEndpoint
		controlConfig.BaseURL = ""
		control, err = newVectorHTTP(&controlConfig)
		if err != nil {
			return nil, err
		}
	}
	return &PineconeAdapter{http: h, control: control}, nil
}

func NewPineconeAdaptor(config *VectorConfig) (*PineconeAdaptor, error) {
	return NewPineconeAdapter(config)
}

type PineconeAdaptor = PineconeAdapter

func (a *PineconeAdapter) auth() func(*http.Request) {
	return func(request *http.Request) {
		if request.Header.Get("Api-Key") == "" {
			key := a.http.config.APIKey
			if key == "" {
				key = a.http.config.Token
			}
			if key != "" {
				request.Header.Set("Api-Key", key)
			}
		}
	}
}

func (a *PineconeAdapter) CreateCollection(ctx context.Context, collection *VectorCollection) error {
	if err := collection.Validate(); err != nil {
		return err
	}
	metric := distanceName(collection.Distance, string(VectorDistanceCosine))
	if metric == string(VectorDistanceDot) {
		metric = "dotproduct"
	}
	payload := map[string]interface{}{
		"name": collection.Name, "dimension": collection.Dimension, "metric": metric,
		"spec": map[string]interface{}{"serverless": map[string]interface{}{"cloud": "aws", "region": "us-east-1"}},
	}
	if len(collection.Metadata) > 0 {
		if spec, ok := collection.Metadata["spec"]; ok {
			payload["spec"] = spec
		}
		if tags, ok := collection.Metadata["tags"]; ok {
			payload["tags"] = tags
		}
	}
	return a.control.doOperation(ctx, "pinecone", "create_collection", http.MethodPost, "/indexes", payload, nil, a.auth())
}

func (a *PineconeAdapter) Upsert(ctx context.Context, collection string, records []VectorRecord) error {
	if err := validateRecords(collection, records); err != nil {
		return err
	}
	vectors := make([]map[string]interface{}, 0, len(records))
	for _, record := range records {
		metadata := cloneMetadata(record.Metadata)
		if record.Document != "" {
			metadata["document"] = record.Document
		}
		vectors = append(vectors, map[string]interface{}{"id": record.ID, "values": record.Vector, "metadata": metadata})
	}
	payload := map[string]interface{}{"vectors": vectors}
	if a.http.config.Namespace != "" {
		payload["namespace"] = a.http.config.Namespace
	}
	return a.http.doOperation(ctx, "pinecone", "upsert", http.MethodPost, "/vectors/upsert", payload, nil, a.auth())
}

func (a *PineconeAdapter) Search(ctx context.Context, request *VectorSearchRequest) ([]VectorMatch, error) {
	if err := validateSearchRequest(request); err != nil {
		return nil, err
	}
	payload := map[string]interface{}{
		"vector": request.Vector, "topK": defaultTopK(request.TopK),
		"includeValues": request.IncludeVector, "includeMetadata": request.IncludeMetadata,
	}
	namespace := request.Namespace
	if namespace == "" {
		namespace = a.http.config.Namespace
	}
	if namespace != "" {
		payload["namespace"] = namespace
	}
	if request.Filter != nil {
		payload["filter"] = request.Filter
	}
	var response struct {
		Matches []struct {
			ID       string                 `json:"id"`
			Score    float32                `json:"score"`
			Values   []float32              `json:"values"`
			Metadata map[string]interface{} `json:"metadata"`
		} `json:"matches"`
	}
	if err := a.http.doOperation(ctx, "pinecone", "search", http.MethodPost, "/query", payload, &response, a.auth()); err != nil {
		return nil, err
	}
	results := make([]VectorMatch, 0, len(response.Matches))
	for _, item := range response.Matches {
		metadata, document := splitDocument(item.Metadata)
		results = append(results, VectorMatch{ID: item.ID, Score: item.Score, Vector: item.Values, Metadata: metadata, Document: document})
	}
	return results, nil
}

func (a *PineconeAdapter) Delete(ctx context.Context, collection string, ids []string) error {
	if collection == "" || len(ids) == 0 {
		return fmt.Errorf("%w: collection and at least one id are required", ErrVectorRequest)
	}
	payload := map[string]interface{}{"ids": ids}
	if a.http.config.Namespace != "" {
		payload["namespace"] = a.http.config.Namespace
	}
	return a.http.doOperation(ctx, "pinecone", "delete", http.MethodPost, "/vectors/delete", payload, nil, a.auth())
}

func PineconeIndexPath(name string) string { return "/indexes/" + url.PathEscape(name) }

var _ VectorAdapter = (*PineconeAdapter)(nil)
