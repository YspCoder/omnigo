package adapter

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ElasticsearchAdapter implements Elasticsearch's dense_vector kNN API.
type ElasticsearchAdapter struct{ http *vectorHTTP }

func NewElasticsearchAdapter(config *VectorConfig) (*ElasticsearchAdapter, error) {
	if config == nil {
		config = &VectorConfig{Endpoint: "http://localhost:9200"}
	}
	h, err := newVectorHTTP(config)
	if err != nil {
		return nil, err
	}
	return &ElasticsearchAdapter{http: h}, nil
}

func NewElasticsearchAdaptor(config *VectorConfig) (*ElasticsearchAdaptor, error) {
	return NewElasticsearchAdapter(config)
}

type ElasticsearchAdaptor = ElasticsearchAdapter

func (a *ElasticsearchAdapter) auth() func(*http.Request) {
	return func(request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			return
		}
		if a.http.config.Username != "" || a.http.config.Password != "" {
			request.SetBasicAuth(a.http.config.Username, a.http.config.Password)
			return
		}
		if a.http.config.Token != "" {
			request.Header.Set("Authorization", "Bearer "+a.http.config.Token)
		} else if a.http.config.APIKey != "" {
			request.Header.Set("Authorization", "ApiKey "+a.http.config.APIKey)
		}
	}
}

func (a *ElasticsearchAdapter) CreateCollection(ctx context.Context, collection *VectorCollection) error {
	if err := collection.Validate(); err != nil {
		return err
	}
	similarity := distanceName(collection.Distance, string(VectorDistanceCosine))
	if similarity == string(VectorDistanceEuclidean) {
		similarity = "l2_norm"
	} else if similarity == string(VectorDistanceDot) {
		similarity = "dot_product"
	} else if similarity == string(VectorDistanceManhattan) {
		similarity = "l1_norm"
	}
	payload := map[string]interface{}{
		"mappings": map[string]interface{}{"properties": map[string]interface{}{
			"vector":   map[string]interface{}{"type": "dense_vector", "dims": collection.Dimension, "index": true, "similarity": similarity},
			"document": map[string]interface{}{"type": "text"},
			"metadata": map[string]interface{}{"type": "object", "enabled": true},
		}},
	}
	return a.http.doOperation(ctx, "elasticsearch", "create_collection", http.MethodPut, "/"+url.PathEscape(collection.Name), payload, nil, a.auth())
}

func (a *ElasticsearchAdapter) Upsert(ctx context.Context, collection string, records []VectorRecord) error {
	if err := validateRecords(collection, records); err != nil {
		return err
	}
	for _, record := range records {
		source := map[string]interface{}{"vector": record.Vector, "metadata": cloneMetadata(record.Metadata)}
		if record.Document != "" {
			source["document"] = record.Document
		}
		path := "/" + url.PathEscape(collection) + "/_doc/" + url.PathEscape(record.ID)
		if err := a.http.doOperation(ctx, "elasticsearch", "upsert", http.MethodPut, path, source, nil, a.auth()); err != nil {
			return err
		}
	}
	return nil
}

func (a *ElasticsearchAdapter) Search(ctx context.Context, request *VectorSearchRequest) ([]VectorMatch, error) {
	if err := validateSearchRequest(request); err != nil {
		return nil, err
	}
	query := map[string]interface{}{
		"knn": map[string]interface{}{
			"field": "vector", "query_vector": request.Vector,
			"k": defaultTopK(request.TopK), "num_candidates": defaultTopK(request.TopK) * 10,
		},
	}
	if request.Filter != nil {
		query["knn"].(map[string]interface{})["filter"] = request.Filter
	}
	var response struct {
		Hits struct {
			Hits []struct {
				ID     string                 `json:"_id"`
				Score  float32                `json:"_score"`
				Source map[string]interface{} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := a.http.doOperation(ctx, "elasticsearch", "search", http.MethodPost, "/"+url.PathEscape(request.Collection)+"/_search", query, &response, a.auth()); err != nil {
		return nil, err
	}
	results := make([]VectorMatch, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		vector := float32Slice(hit.Source["vector"])
		metadata := cloneMetadata(nil)
		if raw, ok := hit.Source["metadata"].(map[string]interface{}); ok {
			metadata = raw
		}
		document, _ := hit.Source["document"].(string)
		results = append(results, VectorMatch{ID: hit.ID, Score: hit.Score, Vector: vector, Metadata: metadata, Document: document})
	}
	return results, nil
}

func (a *ElasticsearchAdapter) Delete(ctx context.Context, collection string, ids []string) error {
	if strings.TrimSpace(collection) == "" || len(ids) == 0 {
		return fmt.Errorf("%w: collection and at least one id are required", ErrVectorRequest)
	}
	for _, id := range ids {
		if err := a.http.doOperation(ctx, "elasticsearch", "delete", http.MethodDelete, "/"+url.PathEscape(collection)+"/_doc/"+url.PathEscape(id), nil, nil, a.auth()); err != nil {
			return err
		}
	}
	return nil
}

var _ VectorAdapter = (*ElasticsearchAdapter)(nil)

// OpenSearchAdapter implements OpenSearch's k-NN plugin REST API. OpenSearch
// uses a knn_vector field and wraps the nearest-neighbor clause in query.knn,
// so it cannot be a type alias of ElasticsearchAdapter.
type OpenSearchAdapter struct{ http *vectorHTTP }

func NewOpenSearchAdapter(config *VectorConfig) (*OpenSearchAdapter, error) {
	if config == nil {
		config = &VectorConfig{Endpoint: "http://localhost:9200"}
	}
	h, err := newVectorHTTP(config)
	if err != nil {
		return nil, err
	}
	return &OpenSearchAdapter{http: h}, nil
}

type OpenSearchAdaptor = OpenSearchAdapter

func NewOpenSearchAdaptor(config *VectorConfig) (*OpenSearchAdaptor, error) {
	return NewOpenSearchAdapter(config)
}

func (a *OpenSearchAdapter) auth() func(*http.Request) {
	return func(request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			return
		}
		if a.http.config.Username != "" || a.http.config.Password != "" {
			request.SetBasicAuth(a.http.config.Username, a.http.config.Password)
		} else if a.http.config.Token != "" {
			request.Header.Set("Authorization", "Bearer "+a.http.config.Token)
		} else if a.http.config.APIKey != "" {
			request.Header.Set("Authorization", "ApiKey "+a.http.config.APIKey)
		}
	}
}

func (a *OpenSearchAdapter) CreateCollection(ctx context.Context, collection *VectorCollection) error {
	if err := collection.Validate(); err != nil {
		return err
	}
	space := "cosinesimil"
	if collection.Distance == VectorDistanceEuclidean {
		space = "l2"
	} else if collection.Distance == VectorDistanceDot {
		space = "innerproduct"
	}
	payload := map[string]interface{}{
		"settings": map[string]interface{}{"index": map[string]interface{}{"knn": true}},
		"mappings": map[string]interface{}{"properties": map[string]interface{}{
			"vector":   map[string]interface{}{"type": "knn_vector", "dimension": collection.Dimension, "method": map[string]interface{}{"name": "hnsw", "space_type": space}},
			"document": map[string]interface{}{"type": "text"},
			"metadata": map[string]interface{}{"type": "object", "enabled": true},
		}},
	}
	return a.http.doOperation(ctx, "opensearch", "create_collection", http.MethodPut, "/"+url.PathEscape(collection.Name), payload, nil, a.auth())
}

func (a *OpenSearchAdapter) Upsert(ctx context.Context, collection string, records []VectorRecord) error {
	if err := validateRecords(collection, records); err != nil {
		return err
	}
	for _, record := range records {
		source := map[string]interface{}{"vector": record.Vector, "metadata": cloneMetadata(record.Metadata)}
		if record.Document != "" {
			source["document"] = record.Document
		}
		path := "/" + url.PathEscape(collection) + "/_doc/" + url.PathEscape(record.ID)
		if err := a.http.doOperation(ctx, "opensearch", "upsert", http.MethodPut, path, source, nil, a.auth()); err != nil {
			return err
		}
	}
	return nil
}

func (a *OpenSearchAdapter) Search(ctx context.Context, request *VectorSearchRequest) ([]VectorMatch, error) {
	if err := validateSearchRequest(request); err != nil {
		return nil, err
	}
	knn := map[string]interface{}{"vector": map[string]interface{}{"vector": request.Vector, "k": defaultTopK(request.TopK)}}
	if request.Filter != nil {
		knn["vector"].(map[string]interface{})["filter"] = request.Filter
	}
	payload := map[string]interface{}{"query": map[string]interface{}{"knn": knn}}
	var response struct {
		Hits struct {
			Hits []struct {
				ID     string                 `json:"_id"`
				Score  float32                `json:"_score"`
				Source map[string]interface{} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := a.http.doOperation(ctx, "opensearch", "search", http.MethodPost, "/"+url.PathEscape(request.Collection)+"/_search", payload, &response, a.auth()); err != nil {
		return nil, err
	}
	results := make([]VectorMatch, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		metadata := cloneMetadata(nil)
		if raw, ok := hit.Source["metadata"].(map[string]interface{}); ok {
			metadata = raw
		}
		document, _ := hit.Source["document"].(string)
		results = append(results, VectorMatch{ID: hit.ID, Score: hit.Score, Vector: float32Slice(hit.Source["vector"]), Metadata: metadata, Document: document})
	}
	return results, nil
}

func (a *OpenSearchAdapter) Delete(ctx context.Context, collection string, ids []string) error {
	if strings.TrimSpace(collection) == "" || len(ids) == 0 {
		return fmt.Errorf("%w: collection and at least one id are required", ErrVectorRequest)
	}
	for _, id := range ids {
		if err := a.http.doOperation(ctx, "opensearch", "delete", http.MethodDelete, "/"+url.PathEscape(collection)+"/_doc/"+url.PathEscape(id), nil, nil, a.auth()); err != nil {
			return err
		}
	}
	return nil
}

var _ VectorAdapter = (*OpenSearchAdapter)(nil)
