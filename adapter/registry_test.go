package adapter

import "testing"

func TestZeroValueRegistryCanRegisterProvider(t *testing.T) {
	var registry Registry
	registry.RegisterProviderSpec("test", ProviderSpec{
		AdaptorFactory: func() Adaptor { return &CustomAdaptor{} },
	})

	adaptor, spec, err := registry.BuildAdaptor("test")
	if err != nil {
		t.Fatal(err)
	}
	if adaptor == nil || spec.Name != "test" {
		t.Fatalf("adaptor=%T spec=%+v", adaptor, spec)
	}
}
