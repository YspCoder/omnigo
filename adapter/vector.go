package adapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// VectorDistance is the distance function used by a vector collection.
type VectorDistance string

const (
	VectorDistanceCosine    VectorDistance = "cosine"
	VectorDistanceEuclidean VectorDistance = "euclidean"
	VectorDistanceDot       VectorDistance = "dot"
	VectorDistanceManhattan VectorDistance = "manhattan"
)

// VectorProvider identifies one of the built-in vector database adapters.
type VectorProvider string

const (
	VectorProviderQdrant        VectorProvider = "qdrant"
	VectorProviderMilvus        VectorProvider = "milvus"
	VectorProviderZilliz        VectorProvider = "zilliz"
	VectorProviderPinecone      VectorProvider = "pinecone"
	VectorProviderWeaviate      VectorProvider = "weaviate"
	VectorProviderChroma        VectorProvider = "chroma"
	VectorProviderElasticsearch VectorProvider = "elasticsearch"
	VectorProviderOpenSearch    VectorProvider = "opensearch"
)

// VectorConfig contains the transport and authentication settings shared by
// vector database adapters. Endpoint is the data-plane base URL. BaseURL is
// accepted as an alias to make the config convenient to share with LLM code.
type VectorConfig struct {
	Endpoint             string
	BaseURL              string
	ControlPlaneEndpoint string
	// OperationURLs overrides provider paths with absolute URLs. Supported
	// keys are create_collection, upsert (or push/insert), search, and delete.
	OperationURLs map[string]string
	// UpsertURL and PushURL are convenient aliases for the upsert operation.
	// They are useful when a service exposes only one fixed ingestion URL.
	UpsertURL           string
	PushURL             string
	InsertURL           string
	SearchURL           string
	DeleteURL           string
	CreateCollectionURL string
	APIKey              string
	Token               string
	Username            string
	Password            string
	Namespace           string
	Headers             map[string]string
	HTTPClient          *http.Client
	Proxy               string
	Timeout             time.Duration
	MaxRetries          int
	RetryDelay          time.Duration
}

// VectorCollection describes a collection/index to create.
type VectorCollection struct {
	Name      string
	Dimension int
	Distance  VectorDistance
	Metadata  map[string]interface{}
}

// CollectionConfig is a shorter alias commonly used by callers.
type CollectionConfig = VectorCollection

// VectorRecord is a vector and its application payload.
type VectorRecord struct {
	ID       string                 `json:"id"`
	Vector   []float32              `json:"vector"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Document string                 `json:"document,omitempty"`
}

// VectorSearchRequest describes a nearest-neighbor search.
type VectorSearchRequest struct {
	Collection      string                 `json:"collection"`
	Vector          []float32              `json:"vector"`
	TopK            int                    `json:"top_k,omitempty"`
	Filter          map[string]interface{} `json:"filter,omitempty"`
	Namespace       string                 `json:"namespace,omitempty"`
	IncludeVector   bool                   `json:"include_vector,omitempty"`
	IncludeMetadata bool                   `json:"include_metadata,omitempty"`
}

// VectorMatch is a provider-neutral nearest-neighbor result.
type VectorMatch struct {
	ID       string                 `json:"id"`
	Score    float32                `json:"score"`
	Vector   []float32              `json:"vector,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Document string                 `json:"document,omitempty"`
}

// VectorAdapter is the common contract implemented by supported vector
// databases. Collection creation is intentionally explicit because most
// providers require a dimension and distance metric before writes.
type VectorAdapter interface {
	CreateCollection(context.Context, *VectorCollection) error
	Upsert(context.Context, string, []VectorRecord) error
	Search(context.Context, *VectorSearchRequest) ([]VectorMatch, error)
	Delete(context.Context, string, []string) error
}

// VectorAdaptor keeps the British spelling used by the existing adapter API.
type VectorAdaptor = VectorAdapter
type VectorStore = VectorAdapter
type VectorDatabaseAdapter = VectorAdapter

// NewVectorAdapter builds one of the built-in vector database adapters.
func NewVectorAdapter(provider VectorProvider, config *VectorConfig) (VectorAdapter, error) {
	switch strings.ToLower(strings.TrimSpace(string(provider))) {
	case string(VectorProviderQdrant):
		return NewQdrantAdapter(config)
	case string(VectorProviderMilvus), string(VectorProviderZilliz):
		return NewMilvusAdapter(config)
	case string(VectorProviderPinecone):
		return NewPineconeAdapter(config)
	case string(VectorProviderWeaviate):
		return NewWeaviateAdapter(config)
	case string(VectorProviderChroma):
		return NewChromaAdapter(config)
	case string(VectorProviderElasticsearch):
		return NewElasticsearchAdapter(config)
	case string(VectorProviderOpenSearch):
		return NewOpenSearchAdapter(config)
	default:
		return nil, fmt.Errorf("unknown vector provider: %s", provider)
	}
}

func NewVectorAdaptor(provider VectorProvider, config *VectorConfig) (VectorAdaptor, error) {
	return NewVectorAdapter(provider, config)
}

// SupportedVectorProviders lists built-in providers in stable order.
func SupportedVectorProviders() []VectorProvider {
	return []VectorProvider{VectorProviderQdrant, VectorProviderMilvus, VectorProviderPinecone, VectorProviderWeaviate, VectorProviderChroma, VectorProviderElasticsearch, VectorProviderOpenSearch}
}

var (
	// ErrVectorConfig is returned when a vector adapter is missing required input.
	ErrVectorConfig = errors.New("invalid vector database configuration")
	// ErrVectorRequest is returned when a vector operation is malformed.
	ErrVectorRequest = errors.New("invalid vector database request")
	// ErrVectorUnsupportedOperation is returned for provider capabilities that
	// cannot be represented by the common contract.
	ErrVectorUnsupportedOperation = errors.New("vector database operation is not supported")
)

// Validate checks the common collection and search constraints.
func (c *VectorCollection) Validate() error {
	if c == nil || strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("%w: collection name is required", ErrVectorRequest)
	}
	if c.Dimension <= 0 {
		return fmt.Errorf("%w: collection dimension must be positive", ErrVectorRequest)
	}
	return nil
}

func validateCollectionName(collection *VectorCollection) error {
	if collection == nil || strings.TrimSpace(collection.Name) == "" {
		return fmt.Errorf("%w: collection name is required", ErrVectorRequest)
	}
	return nil
}

func validateIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_' || (i > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func validateSearchRequest(request *VectorSearchRequest) error {
	if request == nil || strings.TrimSpace(request.Collection) == "" {
		return fmt.Errorf("%w: collection and vector are required", ErrVectorRequest)
	}
	if len(request.Vector) == 0 {
		return fmt.Errorf("%w: collection and vector are required", ErrVectorRequest)
	}
	if request.TopK < 0 {
		return fmt.Errorf("%w: top_k cannot be negative", ErrVectorRequest)
	}
	return nil
}

func validateRecords(collection string, records []VectorRecord) error {
	if strings.TrimSpace(collection) == "" {
		return fmt.Errorf("%w: collection is required", ErrVectorRequest)
	}
	if len(records) == 0 {
		return fmt.Errorf("%w: at least one vector record is required", ErrVectorRequest)
	}
	for i, record := range records {
		if strings.TrimSpace(record.ID) == "" {
			return fmt.Errorf("%w: record %d has no id", ErrVectorRequest, i)
		}
		if len(record.Vector) == 0 {
			return fmt.Errorf("%w: record %q has no vector", ErrVectorRequest, record.ID)
		}
	}
	return nil
}
