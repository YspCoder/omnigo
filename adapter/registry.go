// Package adapter provides provider registry and adaptor factories.
package adapter

import (
	"fmt"
	"sync"
)

// ProviderType represents the general API format.
type ProviderType string

const (
	TypeOpenAI    ProviderType = "openai"
	TypeAnthropic ProviderType = "anthropic"
	TypeCohere    ProviderType = "cohere"
	TypeOllama    ProviderType = "ollama"
	TypeCustom    ProviderType = "custom"
)

// ProviderSpec describes a provider's defaults and adaptor mapping.
type ProviderSpec struct {
	Name              string
	Type              ProviderType
	Endpoint          string
	AuthHeader        string
	AuthPrefix        string
	RequiredHeaders   map[string]string
	SupportsSchema    bool
	SupportsStreaming bool
	AdaptorFactory    func() Adaptor
}

// Registry manages adaptor registration.
type Registry struct {
	mu    sync.RWMutex
	specs map[string]ProviderSpec
}

// NewRegistry creates a new registry with known providers.
func NewRegistry(providerNames ...string) *Registry {
	registry := &Registry{
		specs: make(map[string]ProviderSpec),
	}

	known := map[string]ProviderSpec{
		"openai": {
			Name:              "openai",
			Type:              TypeOpenAI,
			Endpoint:          "https://api.openai.com/v1/chat/completions",
			AuthHeader:        "Authorization",
			AuthPrefix:        "Bearer ",
			RequiredHeaders:   map[string]string{"Content-Type": "application/json"},
			SupportsSchema:    true,
			SupportsStreaming: true,
		},
		"azure-openai": {
			Name:              "azure-openai",
			Type:              TypeOpenAI,
			Endpoint:          "",
			AuthHeader:        "api-key",
			AuthPrefix:        "",
			RequiredHeaders:   map[string]string{"Content-Type": "application/json"},
			SupportsSchema:    true,
			SupportsStreaming: true,
		},
		"anthropic": {
			Name:              "anthropic",
			Type:              TypeAnthropic,
			Endpoint:          "https://api.anthropic.com/v1/messages",
			AuthHeader:        "x-api-key",
			AuthPrefix:        "",
			RequiredHeaders:   map[string]string{"Content-Type": "application/json", "anthropic-version": "2023-06-01"},
			SupportsSchema:    true,
			SupportsStreaming: true,
		},
		"groq": {
			Name:              "groq",
			Type:              TypeOpenAI,
			Endpoint:          "https://api.groq.com/openai/v1/chat/completions",
			AuthHeader:        "Authorization",
			AuthPrefix:        "Bearer ",
			RequiredHeaders:   map[string]string{"Content-Type": "application/json"},
			SupportsSchema:    true,
			SupportsStreaming: true,
		},
		"ollama": {
			Name:              "ollama",
			Type:              TypeOllama,
			Endpoint:          "http://localhost:11434/api/generate",
			AuthHeader:        "",
			AuthPrefix:        "",
			RequiredHeaders:   map[string]string{"Content-Type": "application/json"},
			SupportsSchema:    false,
			SupportsStreaming: true,
		},
		"deepseek": {
			Name:              "deepseek",
			Type:              TypeOpenAI,
			Endpoint:          "https://api.deepseek.com/chat/completions",
			AuthHeader:        "Authorization",
			AuthPrefix:        "Bearer ",
			RequiredHeaders:   map[string]string{"Content-Type": "application/json"},
			SupportsSchema:    true,
			SupportsStreaming: true,
		},
		"google-openai": {
			Name:              "google-openai",
			Type:              TypeOpenAI,
			Endpoint:          "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
			AuthHeader:        "Authorization",
			AuthPrefix:        "Bearer ",
			RequiredHeaders:   map[string]string{"Content-Type": "application/json"},
			SupportsSchema:    true,
			SupportsStreaming: true,
		},
		"mistral": {
			Name:              "mistral",
			Type:              TypeOpenAI,
			Endpoint:          "https://api.mistral.ai/v1/chat/completions",
			AuthHeader:        "Authorization",
			AuthPrefix:        "Bearer ",
			RequiredHeaders:   map[string]string{"Content-Type": "application/json"},
			SupportsSchema:    true,
			SupportsStreaming: true,
		},
		"cohere": {
			Name:              "cohere",
			Type:              TypeCohere,
			Endpoint:          "https://api.cohere.ai/v2/chat",
			AuthHeader:        "Authorization",
			AuthPrefix:        "Bearer ",
			RequiredHeaders:   map[string]string{"Content-Type": "application/json"},
			SupportsSchema:    true,
			SupportsStreaming: true,
		},
		"openrouter": {
			Name:              "openrouter",
			Type:              TypeOpenAI,
			Endpoint:          "https://openrouter.ai/api/v1/chat/completions",
			AuthHeader:        "Authorization",
			AuthPrefix:        "Bearer ",
			RequiredHeaders:   map[string]string{"Content-Type": "application/json", "HTTP-Referer": "https://github.com/YspCoder/omnigo", "X-Title": "Omnigo Integration"},
			SupportsSchema:    true,
			SupportsStreaming: true,
		},
	}

	if len(providerNames) == 0 {
		for name, spec := range known {
			registry.specs[name] = spec
		}
	} else {
		for _, name := range providerNames {
			if spec, ok := known[name]; ok {
				registry.specs[name] = spec
			}
		}
	}

	return registry
}

// GetProviderSpec returns the provider spec by name.
func (r *Registry) GetProviderSpec(name string) (ProviderSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, ok := r.specs[name]
	return spec, ok
}

// RegisterProviderSpec registers or overrides a provider spec.
func (r *Registry) RegisterProviderSpec(name string, spec ProviderSpec) {
	r.mu.Lock()
	defer r.mu.Unlock()
	spec.Name = name
	r.specs[name] = spec
}

// BuildAdaptor returns an adaptor for the provider.
func (r *Registry) BuildAdaptor(name string) (Adaptor, ProviderSpec, error) {
	spec, ok := r.GetProviderSpec(name)
	if !ok {
		return nil, ProviderSpec{}, fmt.Errorf("unknown provider: %s", name)
	}
	if spec.AdaptorFactory != nil {
		return spec.AdaptorFactory(), spec, nil
	}
	switch spec.Type {
	case TypeOpenAI:
		return &OpenAIAdaptor{}, spec, nil
	case TypeAnthropic:
		return &AnthropicAdaptor{}, spec, nil
	case TypeCohere:
		return &CohereAdaptor{}, spec, nil
	case TypeOllama:
		return &OllamaAdaptor{}, spec, nil
	default:
		return nil, ProviderSpec{}, fmt.Errorf("provider %s requires a custom adaptor", name)
	}
}

var defaultRegistry *Registry
var defaultRegistryOnce sync.Once

// GetDefaultRegistry returns the default registry.
func GetDefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewRegistry()
	})
	return defaultRegistry
}

// RegisterProvider registers a provider spec and optional adaptor factory in the default registry.
func RegisterProvider(name string, spec ProviderSpec) {
	r := GetDefaultRegistry()
	r.RegisterProviderSpec(name, spec)
}

// RegisterAdaptor registers a provider with a custom adaptor factory in the default registry.
func RegisterAdaptor(name string, spec ProviderSpec, factory func() Adaptor) {
	spec.AdaptorFactory = factory
	RegisterProvider(name, spec)
}
