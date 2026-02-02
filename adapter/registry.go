// Package adapter provides provider registry and adaptor factories.
package adapter

import (
	"fmt"
	"sync"
)

// ProviderType represents the general API format.
type ProviderType string

const (
	TypeOpenAI ProviderType = "openai"
	TypeAliAI  ProviderType = "ali"
	TypeCustom ProviderType = "custom"
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
		"ali": {
			Name:              "ali",
			Type:              TypeAliAI,
			Endpoint:          "https://dashscope.aliyuncs.com",
			AuthHeader:        "Authorization",
			AuthPrefix:        "Bearer ",
			RequiredHeaders:   map[string]string{"Content-Type": "application/json"},
			SupportsSchema:    false,
			SupportsStreaming: false,
			AdaptorFactory: func() Adaptor {
				return &AliAdaptor{}
			},
		},
		"jimeng": {
			Name:              "jimeng",
			Type:              TypeCustom,
			Endpoint:          "https://visual.volcengineapi.com",
			AuthHeader:        "Authorization",
			AuthPrefix:        "Bearer ",
			RequiredHeaders:   map[string]string{"Content-Type": "application/json"},
			SupportsSchema:    false,
			SupportsStreaming: false,
			AdaptorFactory: func() Adaptor {
				return &JimengAdaptor{}
			},
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
		adaptor := spec.AdaptorFactory()
		return adaptor, spec, nil
	}
	switch spec.Type {
	case TypeOpenAI:
		return &OpenAIAdaptor{}, spec, nil
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
