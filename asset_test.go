package omnigo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewAssetClientCreatesNewAPIClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/assets/groups" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"success":true,"message":"","data":{"id":12,"cloud_group_id":"group-1","group_type":"AIGC","group_name":"products"}}`))
	}))
	defer server.Close()

	client, err := NewAssetClient(AssetProviderNewAPI, &AssetConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("NewAssetClient() error = %v", err)
	}

	group, err := client.CreateAssetGroup(context.Background(), &CreateAssetGroupRequest{
		GroupType: "AIGC",
		GroupName: "products",
	})
	if err != nil {
		t.Fatalf("CreateAssetGroup() error = %v", err)
	}
	if group.ID != 12 || group.CloudGroupID != "group-1" {
		t.Fatalf("group = %#v", group)
	}
}

func TestNewAssetClientRejectsUnknownProvider(t *testing.T) {
	_, err := NewAssetClient(AssetProvider("unknown"), &AssetConfig{})
	if err == nil || !strings.Contains(err.Error(), "unknown asset provider") {
		t.Fatalf("NewAssetClient() error = %v", err)
	}
}

func TestNewAssetClientRequiresConfig(t *testing.T) {
	_, err := NewAssetClient(AssetProviderNewAPI, nil)
	if err == nil || !strings.Contains(err.Error(), "config is required") {
		t.Fatalf("NewAssetClient() error = %v", err)
	}
}
