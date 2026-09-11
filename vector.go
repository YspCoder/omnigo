package omnigo

import "github.com/YspCoder/omnigo/adapter"

type (
	VectorProvider        = adapter.VectorProvider
	VectorConfig          = adapter.VectorConfig
	VectorCollection      = adapter.VectorCollection
	CollectionConfig      = adapter.CollectionConfig
	VectorRecord          = adapter.VectorRecord
	VectorSearchRequest   = adapter.VectorSearchRequest
	VectorMatch           = adapter.VectorMatch
	VectorDistance        = adapter.VectorDistance
	VectorAdapter         = adapter.VectorAdapter
	VectorAdaptor         = adapter.VectorAdaptor
	VectorStore           = adapter.VectorStore
	VectorDatabaseAdapter = adapter.VectorDatabaseAdapter
	VectorAPIError        = adapter.VectorAPIError
)

const (
	VectorProviderQdrant        = adapter.VectorProviderQdrant
	VectorProviderMilvus        = adapter.VectorProviderMilvus
	VectorProviderZilliz        = adapter.VectorProviderZilliz
	VectorProviderPinecone      = adapter.VectorProviderPinecone
	VectorProviderWeaviate      = adapter.VectorProviderWeaviate
	VectorProviderChroma        = adapter.VectorProviderChroma
	VectorProviderElasticsearch = adapter.VectorProviderElasticsearch
	VectorProviderOpenSearch    = adapter.VectorProviderOpenSearch
)

const (
	VectorDistanceCosine    = adapter.VectorDistanceCosine
	VectorDistanceEuclidean = adapter.VectorDistanceEuclidean
	VectorDistanceDot       = adapter.VectorDistanceDot
	VectorDistanceManhattan = adapter.VectorDistanceManhattan
)

var (
	ErrVectorConfig               = adapter.ErrVectorConfig
	ErrVectorRequest              = adapter.ErrVectorRequest
	ErrVectorUnsupportedOperation = adapter.ErrVectorUnsupportedOperation
)

// VectorClient is the provider-neutral vector database client.
type VectorClient interface {
	VectorAdapter
}

// NewVectorClient builds a REST adapter for a supported vector database.
func NewVectorClient(provider VectorProvider, config *VectorConfig) (VectorClient, error) {
	return adapter.NewVectorAdapter(provider, config)
}

// NewVectorAdapter is an alias for NewVectorClient for code that names the
// provider-specific implementation an adapter.
func NewVectorAdapter(provider VectorProvider, config *VectorConfig) (VectorAdapter, error) {
	return NewVectorClient(provider, config)
}

func NewVectorAdaptor(provider VectorProvider, config *VectorConfig) (VectorAdaptor, error) {
	return NewVectorClient(provider, config)
}

// SupportedVectorProviders lists the built-in adapters in stable order.
func SupportedVectorProviders() []VectorProvider {
	return adapter.SupportedVectorProviders()
}

// Convenience constructors are useful when a caller does not need the factory.
func NewQdrantAdapter(config *VectorConfig) (*adapter.QdrantAdapter, error) {
	return adapter.NewQdrantAdapter(config)
}

func NewQdrantAdaptor(config *VectorConfig) (*adapter.QdrantAdaptor, error) {
	return adapter.NewQdrantAdaptor(config)
}

func NewMilvusAdapter(config *VectorConfig) (*adapter.MilvusAdapter, error) {
	return adapter.NewMilvusAdapter(config)
}

func NewMilvusAdaptor(config *VectorConfig) (*adapter.MilvusAdaptor, error) {
	return adapter.NewMilvusAdaptor(config)
}

func NewZillizAdapter(config *VectorConfig) (*adapter.ZillizAdapter, error) {
	return adapter.NewZillizAdapter(config)
}

func NewZillizAdaptor(config *VectorConfig) (*adapter.ZillizAdaptor, error) {
	return adapter.NewZillizAdaptor(config)
}

func NewPineconeAdapter(config *VectorConfig) (*adapter.PineconeAdapter, error) {
	return adapter.NewPineconeAdapter(config)
}

func NewPineconeAdaptor(config *VectorConfig) (*adapter.PineconeAdaptor, error) {
	return adapter.NewPineconeAdaptor(config)
}

func NewWeaviateAdapter(config *VectorConfig) (*adapter.WeaviateAdapter, error) {
	return adapter.NewWeaviateAdapter(config)
}

func NewWeaviateAdaptor(config *VectorConfig) (*adapter.WeaviateAdaptor, error) {
	return adapter.NewWeaviateAdaptor(config)
}

func NewChromaAdapter(config *VectorConfig) (*adapter.ChromaAdapter, error) {
	return adapter.NewChromaAdapter(config)
}

func NewChromaAdaptor(config *VectorConfig) (*adapter.ChromaAdaptor, error) {
	return adapter.NewChromaAdaptor(config)
}

func NewElasticsearchAdapter(config *VectorConfig) (*adapter.ElasticsearchAdapter, error) {
	return adapter.NewElasticsearchAdapter(config)
}

func NewElasticsearchAdaptor(config *VectorConfig) (*adapter.ElasticsearchAdaptor, error) {
	return adapter.NewElasticsearchAdaptor(config)
}

func NewOpenSearchAdapter(config *VectorConfig) (*adapter.OpenSearchAdapter, error) {
	return adapter.NewOpenSearchAdapter(config)
}

func NewOpenSearchAdaptor(config *VectorConfig) (*adapter.OpenSearchAdaptor, error) {
	return adapter.NewOpenSearchAdaptor(config)
}
