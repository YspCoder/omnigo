package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// MilvusAdapter implements the Milvus v2 REST API (also usable with Zilliz
// Cloud when Endpoint and authentication are configured accordingly).
type MilvusAdapter struct{ http *vectorHTTP }

func NewMilvusAdapter(config *VectorConfig) (*MilvusAdapter, error) {
	if config == nil {
		config = &VectorConfig{Endpoint: "http://localhost:19530"}
	}
	h, err := newVectorHTTP(config)
	if err != nil {
		return nil, err
	}
	return &MilvusAdapter{http: h}, nil
}

func NewMilvusAdaptor(config *VectorConfig) (*MilvusAdaptor, error) {
	return NewMilvusAdapter(config)
}

func NewZillizAdapter(config *VectorConfig) (*ZillizAdapter, error) {
	return NewMilvusAdapter(config)
}

func NewZillizAdaptor(config *VectorConfig) (*ZillizAdaptor, error) {
	return NewMilvusAdapter(config)
}

type MilvusAdaptor = MilvusAdapter
type ZillizAdapter = MilvusAdapter
type ZillizAdaptor = MilvusAdapter

func (a *MilvusAdapter) auth() func(*http.Request) { return bearerAuth(a.http.config) }

func (a *MilvusAdapter) CreateCollection(ctx context.Context, collection *VectorCollection) error {
	if err := collection.Validate(); err != nil {
		return err
	}
	metric := strings.ToUpper(distanceName(collection.Distance, string(VectorDistanceCosine)))
	if metric == "EUCLIDEAN" {
		metric = "L2"
	} else if metric == "DOT" {
		metric = "IP"
	}
	payload := map[string]interface{}{
		"collectionName": collection.Name,
		"schema": map[string]interface{}{
			"enableDynamicField": true,
			"fields": []map[string]interface{}{
				{"fieldName": "id", "dataType": "VarChar", "isPrimary": true, "elementTypeParams": map[string]interface{}{"max_length": 512}},
				{"fieldName": "vector", "dataType": "FloatVector", "elementTypeParams": map[string]interface{}{"dim": collection.Dimension}},
			},
		},
		"indexParams": []map[string]interface{}{{"fieldName": "vector", "indexName": "vector_index", "indexType": "AUTOINDEX", "metricType": metric}},
	}
	return a.http.doOperation(ctx, "milvus", "create_collection", http.MethodPost, "/v2/vectordb/collections/create", payload, nil, a.auth())
}

func (a *MilvusAdapter) Upsert(ctx context.Context, collection string, records []VectorRecord) error {
	if err := validateRecords(collection, records); err != nil {
		return err
	}
	data := make([]map[string]interface{}, 0, len(records))
	for _, record := range records {
		row := cloneMetadata(record.Metadata)
		row["id"] = record.ID
		row["vector"] = record.Vector
		if record.Document != "" {
			row["document"] = record.Document
		}
		data = append(data, row)
	}
	payload := map[string]interface{}{"collectionName": collection, "data": data}
	return a.http.doOperation(ctx, "milvus", "upsert", http.MethodPost, "/v2/vectordb/entities/upsert", payload, nil, a.auth())
}

func (a *MilvusAdapter) Search(ctx context.Context, request *VectorSearchRequest) ([]VectorMatch, error) {
	if err := validateSearchRequest(request); err != nil {
		return nil, err
	}
	payload := map[string]interface{}{
		"collectionName": request.Collection, "data": [][]float32{request.Vector},
		"annsField": "vector", "limit": defaultTopK(request.TopK), "outputFields": []string{"*"},
	}
	if request.Filter != nil {
		if expression, ok := request.Filter["expression"].(string); ok {
			payload["filter"] = expression
		} else {
			encoded, err := json.Marshal(request.Filter)
			if err != nil {
				return nil, fmt.Errorf("encode milvus filter: %w", err)
			}
			payload["filter"] = string(encoded)
		}
	}
	var response struct {
		Data   []map[string]interface{} `json:"data"`
		Result []map[string]interface{} `json:"result"`
	}
	if err := a.http.doOperation(ctx, "milvus", "search", http.MethodPost, "/v2/vectordb/entities/search", payload, &response, a.auth()); err != nil {
		return nil, err
	}
	rows := response.Data
	if len(rows) == 0 {
		rows = response.Result
	}
	return vectorMatchesFromMaps(rows), nil
}

func (a *MilvusAdapter) Delete(ctx context.Context, collection string, ids []string) error {
	if strings.TrimSpace(collection) == "" || len(ids) == 0 {
		return fmt.Errorf("%w: collection and at least one id are required", ErrVectorRequest)
	}
	quoted := make([]string, 0, len(ids))
	for _, id := range ids {
		quoted = append(quoted, strconv.Quote(id))
	}
	payload := map[string]interface{}{"collectionName": collection, "filter": "id in [" + strings.Join(quoted, ",") + "]"}
	return a.http.doOperation(ctx, "milvus", "delete", http.MethodPost, "/v2/vectordb/entities/delete", payload, nil, a.auth())
}

func vectorMatchesFromMaps(rows []map[string]interface{}) []VectorMatch {
	results := make([]VectorMatch, 0, len(rows))
	for _, row := range rows {
		entity, _ := row["entity"].(map[string]interface{})
		id := fmt.Sprint(row["id"])
		if id == "<nil>" {
			id = fmt.Sprint(row["pk"])
		}
		if id == "<nil>" {
			id = fmt.Sprint(entity["id"])
		}
		score := float32FromAny(row["score"])
		if score == 0 {
			score = float32FromAny(row["distance"])
		}
		metadata := cloneMetadata(row)
		delete(metadata, "id")
		delete(metadata, "pk")
		delete(metadata, "score")
		delete(metadata, "distance")
		if entity != nil {
			for key, value := range entity {
				metadata[key] = value
			}
		}
		vector := float32Slice(row["vector"])
		if vector == nil {
			vector = float32Slice(row["values"])
		}
		metadata, document := splitDocument(metadata)
		results = append(results, VectorMatch{ID: id, Score: score, Vector: vector, Metadata: metadata, Document: document})
	}
	return results
}

func float32FromAny(value interface{}) float32 {
	switch typed := value.(type) {
	case float64:
		return float32(typed)
	case float32:
		return typed
	case json.Number:
		parsed, _ := typed.Float64()
		return float32(parsed)
	default:
		return 0
	}
}

func float32Slice(value interface{}) []float32 {
	items, ok := value.([]interface{})
	if !ok {
		if typed, ok := value.([]float32); ok {
			return typed
		}
		return nil
	}
	result := make([]float32, 0, len(items))
	for _, item := range items {
		result = append(result, float32FromAny(item))
	}
	return result
}

func MilvusCollectionPath(name string) string {
	return "/v2/vectordb/collections/" + url.PathEscape(name)
}

var _ VectorAdapter = (*MilvusAdapter)(nil)
