package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewAPIAssetAdaptor(t *testing.T) {
	groupJSON := `{"id":12,"user_id":1,"cloud_group_id":"group-1","group_type":"AIGC","group_name":"group","description":"description","created_at":10,"updated_at":11}`
	assetJSON := `{"id":5,"cloud_asset_id":"asset-1","cloud_group_id":"group-1","asset_type":"Video","asset_status":"ACTIVE","asset_name":"video","asset_url":"https://example.com/video.mp4","created_at":20,"updated_at":21}`
	responses := map[string]string{
		http.MethodPost + " /v1/assets/groups":      `{"success":true,"message":"","data":` + groupJSON + `}`,
		http.MethodPost + " /v1/assets/groups/list": `{"success":true,"message":"","data":{"items":[` + groupJSON + `],"total":1,"page_no":1,"page_size":10}}`,
		http.MethodGet + " /v1/assets/groups/12":    `{"success":true,"message":"","data":` + groupJSON + `}`,
		http.MethodPut + " /v1/assets/groups/12":    `{"success":true,"message":"","data":` + groupJSON + `}`,
		http.MethodDelete + " /v1/assets/groups/12": `{"success":true,"message":"","data":{"deleted":true}}`,
		http.MethodPost + " /v1/assets":             `{"success":true,"message":"","data":` + assetJSON + `}`,
		http.MethodPost + " /v1/assets/list":        `{"success":true,"message":"","data":{"items":[` + assetJSON + `],"total":1,"page_no":1,"page_size":10}}`,
		http.MethodGet + " /v1/assets/5":            `{"success":true,"message":"","data":` + assetJSON + `}`,
		http.MethodPut + " /v1/assets/5":            `{"success":true,"message":"","data":` + assetJSON + `}`,
		http.MethodDelete + " /v1/assets/5":         `{"success":true,"message":"","data":{"deleted":true}}`,
	}
	seen := make(map[string]map[string]interface{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("X-Tenant-ID"); got != "tenant-1" {
			t.Fatalf("X-Tenant-ID = %q", got)
		}
		key := r.Method + " " + r.URL.Path
		response, ok := responses[key]
		if !ok {
			t.Fatalf("unexpected asset management request: %s", key)
		}
		if r.Body != nil && r.ContentLength != 0 {
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode %s request: %v", key, err)
			}
			seen[key] = payload
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	var adaptor AssetAdaptor = &NewAPIAssetAdaptor{}
	cfg := &ProviderConfig{
		APIKey: "test-key", BaseURL: server.URL + "/ignored/path?ignored=true",
		Headers: map[string]string{"X-Tenant-ID": "tenant-1"},
	}
	groupName := "group"
	description := ""
	group, err := adaptor.CreateAssetGroup(context.Background(), cfg, &CreateAssetGroupRequest{
		GroupType: "AIGC", GroupName: groupName, Description: "description",
	})
	if err != nil || group.ID != 12 || group.UserID != 1 || group.CloudGroupID != "group-1" {
		t.Fatalf("CreateAssetGroup() = %#v, %v", group, err)
	}
	groups, err := adaptor.ListAssetGroups(context.Background(), cfg, &ListAssetGroupsRequest{PageNo: 1, PageSize: 10, GroupType: "AIGC"})
	if err != nil || groups.Total != 1 || len(groups.Items) != 1 {
		t.Fatalf("ListAssetGroups() = %#v, %v", groups, err)
	}
	if _, err := adaptor.GetAssetGroup(context.Background(), cfg, 12); err != nil {
		t.Fatalf("GetAssetGroup() error = %v", err)
	}
	if _, err := adaptor.UpdateAssetGroup(context.Background(), cfg, 12, &UpdateAssetGroupRequest{GroupName: &groupName, Description: &description}); err != nil {
		t.Fatalf("UpdateAssetGroup() error = %v", err)
	}
	deletedGroup, err := adaptor.DeleteAssetGroup(context.Background(), cfg, 12)
	if err != nil || !deletedGroup.Deleted {
		t.Fatalf("DeleteAssetGroup() = %#v, %v", deletedGroup, err)
	}

	asset, err := adaptor.CreateAsset(context.Background(), cfg, &CreateAssetRequest{
		CloudGroupID: "group-1", AssetName: "video", AssetURL: "https://example.com/video.mp4", AssetType: "Video",
	})
	if err != nil || asset.ID != 5 || asset.CloudAssetID != "asset-1" || asset.AssetStatus != "ACTIVE" {
		t.Fatalf("CreateAsset() = %#v, %v", asset, err)
	}
	assets, err := adaptor.ListAssets(context.Background(), cfg, &ListAssetsRequest{CloudGroupID: "group-1", PageNo: 1, PageSize: 10})
	if err != nil || assets.Total != 1 || len(assets.Items) != 1 {
		t.Fatalf("ListAssets() = %#v, %v", assets, err)
	}
	if _, err := adaptor.GetAsset(context.Background(), cfg, 5); err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}
	if _, err := adaptor.UpdateAsset(context.Background(), cfg, 5, &UpdateAssetRequest{AssetName: "video"}); err != nil {
		t.Fatalf("UpdateAsset() error = %v", err)
	}
	deletedAsset, err := adaptor.DeleteAsset(context.Background(), cfg, 5)
	if err != nil || !deletedAsset.Deleted {
		t.Fatalf("DeleteAsset() = %#v, %v", deletedAsset, err)
	}

	if got := seen[http.MethodPost+" /v1/assets/groups"]["group_type"]; got != "AIGC" {
		t.Fatalf("create group_type = %#v", got)
	}
	if got, ok := seen[http.MethodPut+" /v1/assets/groups/12"]["description"]; !ok || got != "" {
		t.Fatalf("updated description = %#v, present = %v", got, ok)
	}
	if got := seen[http.MethodPost+" /v1/assets"]["cloud_group_id"]; got != "group-1" {
		t.Fatalf("create cloud_group_id = %#v", got)
	}
	if len(seen) != 6 {
		t.Fatalf("requests with bodies = %d, want 6", len(seen))
	}
}

func TestNewAPIAssetAdaptorReturnsBusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"message":"资源组不存在"}`))
	}))
	defer server.Close()

	_, err := (&NewAPIAssetAdaptor{}).GetAssetGroup(context.Background(), &ProviderConfig{BaseURL: server.URL}, 12)
	if err == nil || !strings.Contains(err.Error(), "资源组不存在") {
		t.Fatalf("GetAssetGroup() error = %v, want business message", err)
	}
}

func TestNewAPIAssetAdaptorReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"message":"素材不存在"}`))
	}))
	defer server.Close()

	_, err := (&NewAPIAssetAdaptor{}).GetAsset(context.Background(), &ProviderConfig{BaseURL: server.URL}, 5)
	if err == nil || !strings.Contains(err.Error(), "status=404") || !strings.Contains(err.Error(), "素材不存在") {
		t.Fatalf("GetAsset() error = %v, want status and API message", err)
	}
}
